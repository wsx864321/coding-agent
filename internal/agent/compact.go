package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	openai "github.com/sashabaranov/go-openai"
)

const (
	minPruneBytes        = 1024
	minFoldTokens        = 400
	minRecentMessages    = 4
	summaryTimeout       = 90 * time.Second
	maxPinnedUserChars   = 1500
	summaryTagOpen       = "<compaction-summary>"
	summaryTagClose      = "</compaction-summary>"
	prunedMarkerPrefix   = "[历史工具结果已折叠 - "
	prunedMarkerTemplate = "[历史工具结果已折叠 - 工具=%s，已释放 %d 字节；如需细节请重新调用工具]"
)

const summarySystemPrompt = `你正在为 coding-agent 压缩历史对话以节省上下文。
只基于给定 transcript 总结，不要编造信息。标识符、路径、版本、参数、约束必须原样保留。
请使用精炼要点，在有内容时输出以下标题：

## 既有事实与约束
## 当前目标
## 关键决策与理由
## 文件与代码变更
## 命令与结果
## 错误与修复
## 未完成事项与下一步`

func (a *Agent) maybeCompact(ctx context.Context, promptTokens int) {
	if a.contextWindow <= 0 || promptTokens == 0 {
		return
	}
	high := int(float64(a.contextWindow) * a.compactRatio)
	soft := int(float64(a.contextWindow) * a.softCompactRatio)
	forceLine := int(float64(a.contextWindow) * a.compactForceRatio)

	// Soft threshold: notice once without touching messages — pruning here
	// would needlessly crater the prompt cache for no real gain.
	if promptTokens >= soft && promptTokens < high && !a.softCompactNoticed {
		a.softCompactNoticed = true
		return
	}
	// Below compact threshold: healthy breathing room — clear stuck state and
	// leave the cache-stable prefix intact (NO prune, NO snip).
	if promptTokens < high {
		a.softCompactNoticed = false
		a.compactStuck = false
		a.consecutiveCompacts = 0
		return
	}
	if a.compactStuck {
		return
	}

	// Only now — above the compact threshold — do we touch the message history.
	// Save pre-compact snapshot for memory extraction.
	if a.memSet != nil {
		snap := make([]openai.ChatCompletionMessage, len(a.messages))
		copy(snap, a.messages)
		a.preCompactSnapshot = snap
	}

	force := promptTokens >= forceLine

	// Compute tokPerChar ratio BEFORE prune/snip mutates messages, so the
	// re-check below uses pre-prune calibration.
	ratio := a.tokPerChar()

	// Prune before folding: when eliding stale tool results alone clears the
	// trigger, this turn's (paid) summarize call can be skipped entirely.
	_ = a.pruneStaleToolResults()
	a.snipCompact()

	// Re-check after prune/snip: if non-force and we're now below the trigger,
	// skip the expensive LLM summarization.
	if !force {
		afterTokens := int(float64(charsOfMessages(a.messages)) * ratio)
		if afterTokens < high {
			return
		}
	}

	compacted, err := a.compactHistory(ctx, "auto", "", force)
	if err != nil || !compacted {
		return
	}
	a.consecutiveCompacts++
	// 连续两轮压缩仍在高位，说明窗口过小或最近尾部过重，暂停自动压缩避免循环。
	if a.consecutiveCompacts >= 2 {
		a.compactStuck = true
	}
}

func (a *Agent) pruneStaleToolResults() error {
	if a.contextWindow <= 0 {
		return nil
	}
	if len(a.messages) == 0 {
		return nil
	}
	protected := len(a.messages) - max(a.recentKeep*2, minRecentMessages)
	if protected < 0 {
		protected = 0
	}

	var archived []openai.ChatCompletionMessage
	changed := false
	for i := 0; i < protected; i++ {
		msg := a.messages[i]
		if msg.Role != openai.ChatMessageRoleTool {
			continue
		}
		if len(msg.Content) < minPruneBytes {
			continue
		}
		if strings.HasPrefix(msg.Content, prunedMarkerPrefix) {
			continue
		}
		archived = append(archived, msg)
		msg.Content = fmt.Sprintf(prunedMarkerTemplate, msg.Name, len(msg.Content))
		a.messages[i] = msg
		changed = true
	}
	if !changed {
		return nil
	}
	if len(archived) > 0 {
		_, _ = archiveMessages(a.archiveDir, archived)
	}
	return nil
}

func (a *Agent) snipCompact() {
	if a.maxMessagesSnip <= 0 || len(a.messages) <= a.maxMessagesSnip {
		return
	}
	headEnd := a.pinnedPrefixLen(a.messages)
	if headEnd >= len(a.messages) {
		return
	}

	tailKeep := a.maxMessagesSnip - headEnd - 1 // 1 for placeholder
	if tailKeep < minRecentMessages {
		tailKeep = minRecentMessages
	}
	tailStart := len(a.messages) - tailKeep
	if tailStart <= headEnd {
		return
	}

	// 不切断 assistant(tool_calls) -> tool(result) 对。
	if headEnd > 0 && hasToolCalls(a.messages[headEnd-1]) {
		for headEnd < len(a.messages) && a.messages[headEnd].Role == openai.ChatMessageRoleTool {
			headEnd++
		}
	}
	// 关键：tail 不允许从 tool 消息开始，否则前一个非 tool 可能是占位 user，
	// 会触发 provider 校验错误（tool 必须挂在前面的 assistant tool_calls 上）。
	for tailStart > headEnd && tailStart < len(a.messages) && a.messages[tailStart].Role == openai.ChatMessageRoleTool {
		tailStart--
	}
	if tailStart <= headEnd {
		return
	}

	snipped := tailStart - headEnd
	next := make([]openai.ChatCompletionMessage, 0, headEnd+1+len(a.messages)-tailStart)
	next = append(next, a.messages[:headEnd]...)
	next = append(next, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: fmt.Sprintf("[为节省上下文，已裁剪中间 %d 条历史消息]", snipped),
	})
	next = append(next, a.messages[tailStart:]...)
	a.messages = next
}

func (a *Agent) compactHistory(ctx context.Context, trigger, focus string, force bool) (bool, error) {
	if len(a.messages) == 0 {
		return false, nil
	}
	head := a.pinnedPrefixLen(a.messages)
	start := len(a.messages) - max(a.recentKeep*2, minRecentMessages)
	if start < head {
		start = head
	}
	for start > head && start < len(a.messages) && a.messages[start].Role == openai.ChatMessageRoleTool {
		start--
	}
	if start-head < 1 {
		return false, nil
	}

	region := a.messages[head:start]
	kept, fold := a.partitionFold(region)
	if len(fold) == 0 {
		return false, nil
	}
	if !force && estimateMessagesTokens(fold) < minFoldTokens {
		return false, nil
	}

	archivePath := ""
	if a.archiveDir != "" {
		p, err := archiveMessages(a.archiveDir, fold)
		if err != nil {
			return false, err
		}
		archivePath = p
	}

	summary, err := a.summarize(ctx, fold, focus)
	if err != nil {
		summary = mechanicalFoldDigest(len(fold), archivePath, err)
	}

	tagged := summaryTagOpen + "\n" +
		fmt.Sprintf("历史对话摘要（触发方式=%s）：\n", trigger) +
		summary + "\n" + summaryTagClose

	compacted := make([]openai.ChatCompletionMessage, 0, head+len(kept)+1+len(a.messages)-start)
	compacted = append(compacted, a.messages[:head]...)
	compacted = append(compacted, kept...)
	compacted = append(compacted, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: tagged,
	})
	compacted = append(compacted, a.messages[start:]...)
	a.messages = compacted
	return true, nil
}

func (a *Agent) summarize(ctx context.Context, region []openai.ChatCompletionMessage, focus string) (string, error) {
	sctx, cancel := context.WithTimeout(ctx, summaryTimeout)
	defer cancel()

	sys := summarySystemPrompt
	if strings.TrimSpace(focus) != "" {
		sys += "\n\n额外关注点（优先保留）：\n" + strings.TrimSpace(focus)
	}
	req := openai.ChatCompletionRequest{
		Model: a.cfg.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: sys},
			{Role: openai.ChatMessageRoleUser, Content: renderTranscript(region)},
		},
		MaxTokens:   1800,
		Temperature: a.cfg.Temperature,
	}
	resp, err := a.client.CreateChatCompletion(sctx, req)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("摘要接口返回空 choices")
	}
	s := strings.TrimSpace(resp.Choices[0].Message.Content)
	if s == "" {
		return "", errors.New("摘要内容为空")
	}
	return s, nil
}

func renderTranscript(msgs []openai.ChatCompletionMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case openai.ChatMessageRoleUser:
			fmt.Fprintf(&b, "[用户]\n%s\n\n", m.Content)
		case openai.ChatMessageRoleAssistant:
			if strings.TrimSpace(m.Content) != "" {
				fmt.Fprintf(&b, "[助手]\n%s\n", m.Content)
			}
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "[助手调用工具 %s] %s\n", tc.Function.Name, tc.Function.Arguments)
			}
			b.WriteString("\n")
		case openai.ChatMessageRoleTool:
			fmt.Fprintf(&b, "[工具 %s 输出]\n%s\n\n", m.Name, m.Content)
		case openai.ChatMessageRoleSystem:
			fmt.Fprintf(&b, "[系统]\n%s\n\n", m.Content)
		}
	}
	return b.String()
}

func archiveMessages(dir string, msgs []openai.ChatCompletionMessage) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", nil
	}
	projectDir := filepath.Join(dir, archiveProjectBucket(""))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return "", err
	}
	_ = cleanupArchiveProjectDir(projectDir, time.Duration(DefaultArchiveRetention)*time.Second, DefaultArchiveProjectCap)
	path := filepath.Join(projectDir, time.Now().Format("20060102-150405.000")+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			return "", err
		}
	}
	return path, nil
}

func mechanicalFoldDigest(n int, archive string, cause error) string {
	where := "。"
	if archive != "" {
		where = "。归档文件：" + archive
	}
	return fmt.Sprintf("为释放上下文，已机械折叠 %d 条历史消息；但自动摘要失败：%v%s 如需使用更早信息，请先向用户确认关键细节。", n, cause, where)
}

func hasToolCalls(m openai.ChatCompletionMessage) bool {
	return m.Role == openai.ChatMessageRoleAssistant && len(m.ToolCalls) > 0
}

func isCompactionSummary(m openai.ChatCompletionMessage) bool {
	return m.Role == openai.ChatMessageRoleUser &&
		strings.HasPrefix(strings.TrimLeft(m.Content, "\n "), summaryTagOpen)
}

func (a *Agent) pinnedPrefixLen(msgs []openai.ChatCompletionMessage) int {
	i := 0
	if i < len(msgs) && msgs[i].Role == openai.ChatMessageRoleSystem {
		i++
	}
	if i < len(msgs) &&
		msgs[i].Role == openai.ChatMessageRoleUser &&
		!isCompactionSummary(msgs[i]) &&
		estimateTextTokens(msgs[i].Content) <= maxPinnedUserChars {
		i++
	}
	for i < len(msgs) && isCompactionSummary(msgs[i]) {
		i++
	}
	return i
}

func (a *Agent) partitionFold(region []openai.ChatCompletionMessage) (kept, fold []openai.ChatCompletionMessage) {
	for _, m := range region {
		if isCompactionSummary(m) || (m.Role == openai.ChatMessageRoleUser && estimateTextTokens(m.Content) <= maxPinnedUserChars) {
			kept = append(kept, m)
		} else {
			fold = append(fold, m)
		}
	}
	return kept, fold
}

func estimateMessagesTokens(msgs []openai.ChatCompletionMessage) int {
	total := 0
	for _, m := range msgs {
		total += 4
		total += estimateTextTokens(m.Content)
		total += estimateTextTokens(m.Name)
		total += estimateTextTokens(m.ToolCallID)
		for _, tc := range m.ToolCalls {
			total += 8
			total += estimateTextTokens(tc.ID)
			total += estimateTextTokens(tc.Function.Name)
			total += estimateTextTokens(tc.Function.Arguments)
		}
	}
	return total
}

func estimateTextTokens(s string) int {
	if s == "" {
		return 0
	}
	byBytes := (len(s) + 3) / 4
	runes := utf8.RuneCountInString(s)
	if runes > byBytes {
		return runes
	}
	return byBytes
}

// tokPerChar 从最近一次 LLM 响应的真实 PromptTokens 校准 tokens-per-char 比例，
// 避免纯启发式估算在不同语言/模型下的偏差。首轮 fallback 到 0.25（~4 chars/token）。
// 异常比例（<0.05 或 >2）fallback 到启发式。
func (a *Agent) tokPerChar() float64 {
	if a.lastPromptTokens > 0 {
		if c := charsOfMessages(a.messages); c > 0 {
			if r := float64(a.lastPromptTokens) / float64(c); r > 0.05 && r < 2 {
				return r
			}
		}
	}
	return 0.25
}

// msgChars 统计一条消息实际发送给 provider 的字符数（content + tool call name/arguments）
func msgChars(m openai.ChatCompletionMessage) int {
	n := len(m.Content)
	for _, tc := range m.ToolCalls {
		n += len(tc.Function.Name) + len(tc.Function.Arguments)
	}
	return n
}

func charsOfMessages(msgs []openai.ChatCompletionMessage) int {
	n := 0
	for _, m := range msgs {
		n += msgChars(m)
	}
	return n
}

func cleanupArchiveProjectDir(dir string, retention time.Duration, capBytes int64) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type fileMeta struct {
		path string
		size int64
		mod  time.Time
	}
	files := make([]fileMeta, 0, len(entries))
	now := time.Now()
	var total int64
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".jsonl") {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(dir, ent.Name())
		if retention > 0 && now.Sub(info.ModTime()) > retention {
			_ = os.Remove(p)
			continue
		}
		files = append(files, fileMeta{
			path: p,
			size: info.Size(),
			mod:  info.ModTime(),
		})
		total += info.Size()
	}
	if capBytes <= 0 || total <= capBytes {
		return nil
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].mod.Equal(files[j].mod) {
			return files[i].path < files[j].path
		}
		return files[i].mod.Before(files[j].mod)
	})
	for _, f := range files {
		if total <= capBytes {
			break
		}
		if err := os.Remove(f.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		total -= f.size
	}
	return nil
}
