package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/wsx864321/coding-agent/internal/event"
)

// gitStatus 保存最近一次 git 状态快照。
type gitStatus struct {
	branch string
	ahead  int
	behind int
	dirty  bool
}

// todoItem 表示 todo_write 工具中的单个任务项。
type todoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm"`
}

// gitStatusMsg 携带异步 git 查询结果。
type gitStatusMsg struct {
	status gitStatus
}

// balanceMsg 携带异步余额查询结果。
type balanceMsg struct {
	text string
}

// statuslineMsg 携带自定义状态行命令的输出。
type statuslineMsg struct {
	out string
}

const interruptedStatusMsg = "已中断"

const maxEventDrain = 512

const maxShellOutputSize = 1024 * 1024 // 1MB

// truncateShellOutput 对超过 1MB 的输出保留最后 1MB，并标注截断。
func truncateShellOutput(output string) string {
	if len(output) <= maxShellOutputSize {
		return output
	}
	truncated := output[len(output)-maxShellOutputSize:]
	return "[output truncated] " + truncated
}

// Model 是 Bubble Tea 聊天界面的状态机。
type Model struct {
	transcript       []TranscriptEntry
	textarea         textarea.Model
	viewport         viewport.Model
	spinner          spinner.Model
	mdRenderer       MarkdownRenderer
	width            int
	height           int
	modelName        string
	runStart         time.Time
	quitting         bool
	busy             bool
	lastError        string
	statusMsg        string
	statusLabel      string
	interrupted      bool
	pending          *strings.Builder
	pendingToolName  string
	pendingToolArgs  string
	approval         *pendingApproval
	runner           Runner
	tuiSink          *TuiSink
	streamCh         <-chan event.Event
	turnCancel       context.CancelFunc
	reasoning        *strings.Builder
	reasoningLineIdx int
	showReasoning    bool
	thinkStart       time.Time
	toolStreamIdx    int
	toolStreamID     string
	toolTail         []string
	toolPartial      string
	toolLineCount    int
	toolStreamStart  time.Time
	shellOutputs     map[string]string
	shellExpanded    map[string]bool
	sel              selection
	diffMaxLines     int // 0 = no limit, >0 = max visible lines before collapsing diff output

	// --- 状态面板字段 ---
	gitStatus     gitStatus
	contextUsed   int
	contextWindow int
	cacheHitRate  int    // 0-100 百分比
	balance       string // 格式化后的余额文本，如 "¥110.00"
	todoArgs      string // 最近一次 todo_write 的原始 JSON 参数
	todoItems     []todoItem
	statuslineCmd string
	statuslineOut string
}

// New 构造初始 TUI model。
func New() Model {
	return Model{
		textarea:   newTextarea(),
		viewport:   newViewport(),
		spinner:    newSpinner(),
		mdRenderer: NewGlamourRenderer(),
		modelName:  "coding-agent",
		pending:    &strings.Builder{},
		reasoning:  &strings.Builder{},
		reasoningLineIdx: -1,
		toolStreamIdx:    -1,
		shellOutputs:     make(map[string]string),
		shellExpanded:    make(map[string]bool),
	}
}

// NewWithRunner 构造带会话执行器的 TUI model。
func NewWithRunner(runner Runner, tuiSink *TuiSink) Model {
	m := New()
	m.runner = runner
	m.tuiSink = tuiSink
	return m
}

// Init 启动时不发送额外命令。
func (m Model) Init() tea.Cmd {
	return nil
}

// Update 处理窗口尺寸、输入编辑、滚动、流式事件与 Ctrl+C 退出。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.layout()
		m = m.rebuildTranscript()
		m = m.syncViewportContent()
		return m, nil

	case event.Event:
		switch msg.Kind {
		case event.Text:
			if !m.busy {
				return m, nil
			}
			// Finalize reasoning summary when text arrives
			if m.reasoningLineIdx >= 0 {
				m = m.finalizeReasoningSummary()
			}
			m.pending.WriteString(msg.Text)
			if renderable, rest := flushableMarkdownPrefix(m.pending.String()); renderable != "" {
				rendered := m.mdRenderer.Render(renderable, m.assistantInnerWidth())
				m = m.appendAssistantRendered(rendered, renderable)
				m.pending.Reset()
				m.pending.WriteString(rest)
			}
			m = m.syncViewportContent()

		case event.ToolDispatch:
			if !m.busy {
				return m, nil
			}
			m.statusLabel = "running " + msg.ToolName + "..."
			m.pendingToolName = msg.ToolName
			m.pendingToolArgs = msg.ToolArgs

		case event.ToolResult:
			if !m.busy {
				return m, nil
			}
			// Collapse active stream block into a summary before adding ToolResult.
			if m.toolStreamIdx >= 0 {
				m = m.collapseToolStream()
			}
			// Store bash output in shellOutputs with 1MB truncation.
			if msg.ToolName == "bash" && msg.ToolCallID != "" {
				m.shellOutputs[msg.ToolCallID] = truncateShellOutput(msg.ToolOutput)
			}
			name := msg.ToolName
			if name == "" {
				name = m.pendingToolName
			}
			args := m.pendingToolArgs
			w := m.contentWidth()
			if msg.ToolIsErr {
				m = m.appendEntry(TranscriptEntry{
					Kind:    EntryToolCard,
					Content: renderToolCardError(name, msg.ToolOutput, w),
					Raw:     encodeToolCardRaw(name, msg.ToolOutput, true),
				})
			} else {
				m = m.appendEntry(TranscriptEntry{
					Kind:    EntryToolCard,
					Content: renderToolCard(name, args, w),
					Raw:     encodeToolCardRaw(name, args, false),
				})
				if msg.ToolOutput != "" {
					raw := msg.ToolOutput
					if msg.ToolCallID != "" {
						raw = encodeToolOutputRaw(msg.ToolCallID, msg.ToolOutput)
					}
					renderOutput := msg.ToolOutput
					if isDiffOutput(msg.ToolOutput) {
						m = m.appendEntry(TranscriptEntry{
							Kind:    EntryToolOutput,
							Content: renderDiffOutput(renderOutput, m.diffMaxLines),
							Raw:     raw,
						})
					} else {
						m = m.appendEntry(TranscriptEntry{
							Kind:    EntryToolOutput,
							Content: renderToolOutput(renderOutput, toolOutputCollapseLines),
							Raw:     raw,
						})
					}
				}
			}
			m.pendingToolName = ""
			m.pendingToolArgs = ""
			m.statusLabel = "thinking"
			m = m.syncViewportContent()

		case event.ApprovalRequest:
			if !m.busy {
				return m, nil
			}
			m.approval = &pendingApproval{
				toolName: msg.ApprovalName,
				args:     msg.ApprovalArgs,
				respond:  msg.ApprovalRespond,
			}
			m = m.syncLayout()

		case event.Notice:
			m.statusMsg = msg.Text

		case event.ReasoningText:
			if !m.busy {
				return m, nil
			}
			m = m.ingestReasoningChunk(msg.ReasoningChunk)
			m = m.syncViewportContent()
			if m.streamCh != nil {
				return m, waitStreamEvent(m.streamCh)
			}
			return m, nil

		case event.ToolProgress:
			if !m.busy {
				return m, nil
			}
			m = m.ingestToolProgress(msg.ToolCallID, msg.ToolChunk)
			m = m.syncViewportContent()
			if m.streamCh != nil {
				return m, waitStreamEvent(m.streamCh)
			}
			return m, nil

		case event.TurnDone:
			if m.interrupted {
				m.interrupted = false
				m.busy = false
				m.streamCh = nil
				m.turnCancel = nil
				m.toolStreamIdx = -1
				m.toolStreamID = ""
				m.toolTail = nil
				m.toolPartial = ""
				m.toolLineCount = 0
				m.toolStreamStart = time.Time{}
				return m, nil
			}
			m = m.flushPending()
			if msg.Err != nil {
				m.lastError = msg.Err.Error()
			}
			m.busy = false
			m.streamCh = nil
			m.turnCancel = nil
			m.statusLabel = ""
			m.pendingToolName = ""
			m.pendingToolArgs = ""
			m.toolStreamIdx = -1
			m.toolStreamID = ""
			m.toolTail = nil
			m.toolPartial = ""
			m.toolLineCount = 0
			m.toolStreamStart = time.Time{}
			m = m.syncViewportContent()
			if msg.Err != nil {
				m = m.syncLayout()
			}
			return m, tea.Batch(fetchGitStatus())
		}
		if m.streamCh != nil {
			return m, waitStreamEvent(m.streamCh)
		}
		return m, nil

	case gitStatusMsg:
		m.gitStatus = msg.status
		return m, nil

	case streamClosedMsg:
		m = m.flushPending()
		m.busy = false
		m.streamCh = nil
		m = m.syncViewportContent()
		return m, nil

	case drainBatchMsg:
		for _, e := range msg.events {
			m = m.ingestDrainEvent(e)
		}
		m = m.syncViewportContent()
		if m.streamCh != nil {
			return m, drainEvents(m.streamCh)
		}
		return m, nil

	case spinner.TickMsg:
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			if m.sel.active {
				// 单击取消选择
				m.sel = selection{}
			} else {
				// 左键按下开始选择
				m.sel = selection{
					startLine: msg.Y,
					startCol:  msg.X,
					endLine:   msg.Y,
					endCol:    msg.X,
					active:    true,
					dragging:  false,
				}
			}
		}
		return m, nil

	case tea.MouseMotionMsg:
		if m.sel.active && msg.Button == tea.MouseLeft {
			m.sel.dragging = true
			m.sel.endLine = msg.Y
			m.sel.endCol = msg.X
		}
		return m, nil

	case tea.MouseReleaseMsg:
		if m.sel.active && msg.Button == tea.MouseLeft {
			m.sel.dragging = false
			m.sel.endLine = msg.Y
			m.sel.endCol = msg.X
		}
		return m, nil

	case tea.MouseWheelMsg:
		if m.sel.active {
			// 选择时滚轮扩展选择范围
			if msg.Button == tea.MouseWheelDown {
				m.sel.endLine++
			} else if msg.Button == tea.MouseWheelUp {
				if m.sel.endLine > 0 {
					m.sel.endLine--
				}
			}
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		if m.approval != nil {
			switch msg.String() {
			case "y", "Y", "n", "N":
				next, cmd := m.handleApprovalKey(msg)
				next = next.syncLayout()
				return next, cmd
			case "esc":
				m = m.interruptTurn()
				m.approval = nil
				return m, nil
			default:
				return m, nil
			}
		}

		switch {
		case msg.String() == "ctrl+o":
			m.showReasoning = !m.showReasoning
			m = m.rerenderReasoningEntry()
			return m, nil

		case msg.String() == "ctrl+b":
			if m.busy {
				return m, nil
			}
			m = m.toggleShellExpand()
			return m, nil

		case msg.String() == "ctrl+c":
			if m.sel.active && !m.sel.empty() {
				// 选中时 Ctrl+C 复制到剪贴板
				lines := strings.Split(m.viewport.View(), "\n")
				text := m.sel.extractSelectedText(lines)
				if text != "" {
					_ = clipboard.WriteAll(text)
				}
				m.sel = selection{}
				return m, nil
			}
			m = m.interruptTurn()
			m.approval = nil
			m.quitting = true
			return m, tea.Quit

		case msg.String() == "esc":
			return m.interruptTurn(), nil

		case isSubmitKey(msg):
			return m.submit()

		case m.sel.active && (msg.String() == "pgup" || msg.String() == "pgdown"):
			// 选择时 PgUp/PgDn 扩展选择范围
			if msg.String() == "pgdown" {
				m.sel.endLine++
			} else {
				if m.sel.endLine > 0 {
					m.sel.endLine--
				}
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd

		case m.shouldRouteScrollToViewport(msg):
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd

		default:
			if m.busy {
				return m, nil
			}
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			m = m.syncLayout()
			return m, cmd
		}
	}

	return m, nil
}

func isSubmitKey(msg tea.KeyPressMsg) bool {
	if msg.Mod.Contains(tea.ModShift) || msg.Mod.Contains(tea.ModAlt) {
		return false
	}
	switch msg.Key().Code {
	case tea.KeyEnter:
		return true
	default:
		return false
	}
}

func (m Model) shouldRouteScrollToViewport(msg tea.KeyPressMsg) bool {
	hasText := strings.TrimSpace(m.textarea.Value()) != ""
	k := msg.String()

	if hasText {
		switch k {
		case "pgup", "pgdown":
			return true
		default:
			return false
		}
	}

	switch k {
	case "up", "down", "j", "k", "pgup", "pgdown", "b", "f", "u", "d":
		return true
	default:
		return key.Matches(msg, m.viewport.KeyMap.Up) ||
			key.Matches(msg, m.viewport.KeyMap.Down) ||
			key.Matches(msg, m.viewport.KeyMap.PageUp) ||
			key.Matches(msg, m.viewport.KeyMap.PageDown)
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	text := strings.TrimSpace(m.textarea.Value())
	if m.busy || text == "" || m.runner == nil {
		return m, nil
	}

	// 检测 /diff-fold 斜杠命令：解析 /diff-fold N 更新 diffMaxLines
	if strings.HasPrefix(text, "/diff-fold") {
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/diff-fold"))
		if arg == "" {
			m.statusMsg = fmt.Sprintf("diff 折叠行数：%d (0=不限制)", m.diffMaxLines)
		} else if n, err := strconv.Atoi(arg); err == nil {
			if n < 0 {
				n = 0
			}
			m.diffMaxLines = n
			if n == 0 {
				m.statusMsg = "diff 折叠行数：不限制"
			} else {
				m.statusMsg = fmt.Sprintf("diff 折叠行数设为 %d", n)
			}
		} else {
			m.statusMsg = fmt.Sprintf("无效参数: /diff-fold %s (需要整数)", arg)
		}
		m.textarea.Reset()
		m = m.syncLayout()
		return m, nil
	}

	m.textarea.Reset()
	m = m.syncLayout()
	m.busy = true
	m.runStart = time.Now()
	m.lastError = ""
	m.statusMsg = ""
	m.statusLabel = ""
	m.interrupted = false
	m.pending.Reset()
	m.reasoning.Reset()
	m.reasoningLineIdx = -1
	m.showReasoning = false
	m.toolStreamIdx = -1
	m.toolStreamID = ""
	m.toolTail = nil
	m.toolPartial = ""
	m.toolLineCount = 0
	m.toolStreamStart = time.Time{}
	m = m.appendUserMessage(text)
	m = m.appendEntry(TranscriptEntry{Kind: EntryAssistantChunk})
	m = m.syncViewportContent()

	ch := make(chan event.Event, 16)
	if m.tuiSink != nil {
		m.tuiSink.SetChan(ch)
	}
	runner := m.runner
	ctx, cancel := context.WithCancel(context.Background())
	m.turnCancel = cancel
	go func() {
		defer close(ch)
		defer func() { _ = recover() }()
		_ = runner.RunTurn(ctx, text)
	}()

	m.streamCh = ch
	return m, tea.Batch(waitStreamEvent(ch), m.spinner.Tick, fetchGitStatus())
}

func waitStreamEvent(ch <-chan event.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return streamClosedMsg{}
		}
		return e
	}
}

func (m Model) appendAssistantChunk(text string) Model {
	if text == "" {
		return m
	}
	if len(m.transcript) == 0 || m.transcript[len(m.transcript)-1].Kind != EntryAssistantChunk {
		e := TranscriptEntry{Kind: EntryAssistantChunk, Raw: text}
		e = m.renderEntry(e)
		m.transcript = append(m.transcript, e)
		return m
	}
	last := len(m.transcript) - 1
	m.transcript[last].Raw += text
	m.transcript[last] = m.renderEntry(m.transcript[last])
	return m
}

func (m Model) ingestReasoningChunk(chunk string) Model {
	if chunk == "" || !m.busy {
		return m
	}
	m.reasoning.WriteString(chunk)
	if m.reasoningLineIdx < 0 {
		// First reasoning chunk: create a new EntryReasoning summary line.
		e := TranscriptEntry{Kind: EntryReasoning, Raw: m.reasoning.String()}
		e = m.renderEntry(e)
		m.transcript = append(m.transcript, e)
		m.reasoningLineIdx = len(m.transcript) - 1
		return m
	}
	// Subsequent chunks: update the existing EntryReasoning entry.
	if m.reasoningLineIdx < len(m.transcript) && m.transcript[m.reasoningLineIdx].Kind == EntryReasoning {
		m.transcript[m.reasoningLineIdx].Raw = m.reasoning.String()
		m.transcript[m.reasoningLineIdx] = m.renderEntry(m.transcript[m.reasoningLineIdx])
	}
	return m
}

func (m Model) ingestToolProgress(toolCallID, chunk string) Model {
	if chunk == "" || !m.busy {
		return m
	}

	// If toolCallID changed, reset stream state.
	if m.toolStreamID != "" && m.toolStreamID != toolCallID {
		m.toolStreamIdx = -1
		m.toolStreamID = ""
		m.toolTail = nil
		m.toolPartial = ""
		m.toolLineCount = 0
		m.toolStreamStart = time.Time{}
	}

	// First ToolProgress: create a new stream block entry.
	if m.toolStreamIdx < 0 {
		m.toolStreamID = toolCallID
		m.toolStreamStart = time.Now()
		m.toolTail = nil
		m.toolPartial = ""
		m.toolLineCount = 0
		e := TranscriptEntry{Kind: EntryToolStream}
		e = m.renderEntry(e)
		m.transcript = append(m.transcript, e)
		m.toolStreamIdx = len(m.transcript) - 1
	}

	// Append chunk and split into lines.
	m.toolPartial += chunk
	for {
		idx := strings.Index(m.toolPartial, "\n")
		if idx < 0 {
			break
		}
		line := m.toolPartial[:idx]
		m.toolPartial = m.toolPartial[idx+1:]
		m.toolTail = append(m.toolTail, line)
		m.toolLineCount++
		// Keep only last 20 lines.
		if len(m.toolTail) > 20 {
			m.toolTail = m.toolTail[len(m.toolTail)-20:]
		}
	}

	// Update the stream block entry in-place.
	if m.toolStreamIdx >= 0 && m.toolStreamIdx < len(m.transcript) &&
		m.transcript[m.toolStreamIdx].Kind == EntryToolStream {
		m.transcript[m.toolStreamIdx] = m.renderEntry(m.transcript[m.toolStreamIdx])
	}

	return m
}

func (m Model) renderToolStreamBlock() string {
	dur := time.Since(m.toolStreamStart)
	durSec := int(dur.Seconds())
	header := fmt.Sprintf("  ⎿  working · %ds", durSec)

	var b strings.Builder
	b.WriteString(header)

	for _, line := range m.toolTail {
		b.WriteString("\n  ⎿  ")
		b.WriteString(line)
	}

	// Show partial line if any.
	if m.toolPartial != "" {
		b.WriteString("\n  ⎿  ")
		b.WriteString(m.toolPartial)
	}

	footer := fmt.Sprintf("\n  ⎿  %d lines", m.toolLineCount)
	b.WriteString(footer)

	return b.String()
}

func (m Model) syncViewportContent() Model {
	wasAtBottom := m.viewport.AtBottom() || len(m.transcript) == 0
	content := m.renderTranscriptContent()
	m.viewport.SetContent(content)
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
	return m
}

func (m Model) layout() Model {
	contentWidth := m.width
	if contentWidth <= 0 {
		contentWidth = 80
	}
	m.textarea.SetWidth(contentWidth)
	m.viewport.SetWidth(contentWidth)

	vpHeight := m.height - m.bottomHeight()
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewport.SetHeight(vpHeight)
	return m
}

func (m Model) syncLayout() Model {
	return m.layout()
}

func (m Model) flushPending() Model {
	if m.pending.Len() > 0 {
		rendered := m.mdRenderer.Render(m.pending.String(), m.assistantInnerWidth())
		m = m.appendAssistantRendered(rendered, m.pending.String())
		m.pending.Reset()
	}
	return m
}

func (m Model) interruptTurn() Model {
	if !m.busy {
		return m
	}
	if m.turnCancel != nil {
		m.turnCancel()
		m.turnCancel = nil
	}
	m = m.flushPending()
	m.statusLabel = ""
	m.pendingToolName = ""
	m.pendingToolArgs = ""
	m.busy = false
	m.streamCh = nil
	m.approval = nil
	m.statusMsg = interruptedStatusMsg
	m.interrupted = true
	m.reasoning.Reset()
	m.reasoningLineIdx = -1
	m.showReasoning = false
	m.toolStreamIdx = -1
	m.toolStreamID = ""
	m.toolTail = nil
	m.toolPartial = ""
	m.toolLineCount = 0
	m.toolStreamStart = time.Time{}
	m = m.syncViewportContent()
	m = m.syncLayout()
	return m
}

// finalizeReasoningSummary updates the reasoning summary line to show
// completed state and resets reasoningLineIdx to -1.
func (m Model) finalizeReasoningSummary() Model {
	if m.reasoningLineIdx < 0 || m.reasoningLineIdx >= len(m.transcript) {
		return m
	}
	if m.transcript[m.reasoningLineIdx].Kind != EntryReasoning {
		return m
	}
	m.transcript[m.reasoningLineIdx].Raw = m.reasoning.String()
	m.transcript[m.reasoningLineIdx] = m.renderEntry(m.transcript[m.reasoningLineIdx])
	m.reasoningLineIdx = -1
	return m
}

// rerenderReasoningEntry re-renders the reasoning entry in-place when
// showReasoning is toggled, so the transcript reflects expanded/collapsed state.
func (m Model) rerenderReasoningEntry() Model {
	if m.reasoningLineIdx < 0 || m.reasoningLineIdx >= len(m.transcript) {
		return m
	}
	if m.transcript[m.reasoningLineIdx].Kind != EntryReasoning {
		return m
	}
	m.transcript[m.reasoningLineIdx] = m.renderEntry(m.transcript[m.reasoningLineIdx])
	m = m.syncViewportContent()
	return m
}

// collapseToolStream converts the active EntryToolStream block into a collapsed
// EntryToolOutput summary and resets all stream state fields.
func (m Model) collapseToolStream() Model {
	if m.toolStreamIdx < 0 || m.toolStreamIdx >= len(m.transcript) {
		return m
	}
	if m.transcript[m.toolStreamIdx].Kind != EntryToolStream {
		return m
	}
	// Build a collapsed summary from the stream tail lines.
	var b strings.Builder
	for _, line := range m.toolTail {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if m.toolPartial != "" {
		b.WriteString(m.toolPartial)
	}
	raw := b.String()
	m.transcript[m.toolStreamIdx].Kind = EntryToolOutput
	m.transcript[m.toolStreamIdx].Raw = raw
	m.transcript[m.toolStreamIdx] = m.renderEntry(m.transcript[m.toolStreamIdx])

	// Reset stream state.
	m.toolStreamIdx = -1
	m.toolStreamID = ""
	m.toolTail = nil
	m.toolPartial = ""
	m.toolLineCount = 0
	m.toolStreamStart = time.Time{}
	return m
}

// toggleShellExpand reverse-scans the transcript from the end to find the
// nearest EntryToolOutput entry whose Raw encodes a toolCallID. It toggles
// the shellExpanded flag for that toolCallID and re-renders the entry in-place.
func (m Model) toggleShellExpand() Model {
	for i := len(m.transcript) - 1; i >= 0; i-- {
		if m.transcript[i].Kind != EntryToolOutput {
			continue
		}
		toolCallID, _ := decodeToolOutputRaw(m.transcript[i].Raw)
		if toolCallID == "" {
			continue
		}
		if _, ok := m.shellOutputs[toolCallID]; !ok {
			continue
		}
		// Toggle the expanded state.
		m.shellExpanded[toolCallID] = !m.shellExpanded[toolCallID]
		// Re-render the entry in-place.
		m.transcript[i] = m.renderEntry(m.transcript[i])
		m = m.syncViewportContent()
		return m
	}
	return m
}

// ingestDrainEvent applies a single event during a drain loop, without
// syncing the viewport (the caller batches syncViewportContent at the end).
func (m Model) ingestDrainEvent(e event.Event) Model {
	switch e.Kind {
	case event.Text:
		if !m.busy {
			return m
		}
		if m.reasoningLineIdx >= 0 {
			m = m.finalizeReasoningSummary()
		}
		m.pending.WriteString(e.Text)
		if renderable, rest := flushableMarkdownPrefix(m.pending.String()); renderable != "" {
			rendered := m.mdRenderer.Render(renderable, m.assistantInnerWidth())
			m = m.appendAssistantRendered(rendered, renderable)
			m.pending.Reset()
			m.pending.WriteString(rest)
		}

	case event.ToolDispatch:
		if !m.busy {
			return m
		}
		m.statusLabel = "running " + e.ToolName + "..."
		m.pendingToolName = e.ToolName
		m.pendingToolArgs = e.ToolArgs

	case event.ToolResult:
		if !m.busy {
			return m
		}
		if m.toolStreamIdx >= 0 {
			m = m.collapseToolStream()
		}
		// Store bash output in shellOutputs with 1MB truncation.
		if e.ToolName == "bash" && e.ToolCallID != "" {
			m.shellOutputs[e.ToolCallID] = truncateShellOutput(e.ToolOutput)
		}
		name := e.ToolName
		if name == "" {
			name = m.pendingToolName
		}
		args := m.pendingToolArgs
		w := m.contentWidth()
		if e.ToolIsErr {
			m = m.appendEntry(TranscriptEntry{
				Kind:    EntryToolCard,
				Content: renderToolCardError(name, e.ToolOutput, w),
				Raw:     encodeToolCardRaw(name, e.ToolOutput, true),
			})
		} else {
			m = m.appendEntry(TranscriptEntry{
				Kind:    EntryToolCard,
				Content: renderToolCard(name, args, w),
				Raw:     encodeToolCardRaw(name, args, false),
			})
			if e.ToolOutput != "" {
				raw := e.ToolOutput
				if e.ToolCallID != "" {
					raw = encodeToolOutputRaw(e.ToolCallID, e.ToolOutput)
				}
				renderOutput := e.ToolOutput
				if isDiffOutput(e.ToolOutput) {
					m = m.appendEntry(TranscriptEntry{
						Kind:    EntryToolOutput,
						Content: renderDiffOutput(renderOutput, m.diffMaxLines),
						Raw:     raw,
					})
				} else {
					m = m.appendEntry(TranscriptEntry{
						Kind:    EntryToolOutput,
						Content: renderToolOutput(renderOutput, toolOutputCollapseLines),
						Raw:     raw,
					})
				}
			}
		}
		m.pendingToolName = ""
		m.pendingToolArgs = ""
		m.statusLabel = "thinking"

	case event.ApprovalRequest:
		if !m.busy {
			return m
		}
		m.approval = &pendingApproval{
			toolName: e.ApprovalName,
			args:     e.ApprovalArgs,
			respond:  e.ApprovalRespond,
		}

	case event.Notice:
		m.statusMsg = e.Text

	case event.ReasoningText:
		if !m.busy {
			return m
		}
		m = m.ingestReasoningChunk(e.ReasoningChunk)

	case event.ToolProgress:
		if !m.busy {
			return m
		}
		m = m.ingestToolProgress(e.ToolCallID, e.ToolChunk)

	case event.TurnDone:
		if m.interrupted {
			m.interrupted = false
			m.busy = false
			m.streamCh = nil
			m.turnCancel = nil
			m.toolStreamIdx = -1
			m.toolStreamID = ""
			m.toolTail = nil
			m.toolPartial = ""
			m.toolLineCount = 0
			m.toolStreamStart = time.Time{}
			return m
		}
		m = m.flushPending()
		if e.Err != nil {
			m.lastError = e.Err.Error()
		}
		m.busy = false
		m.streamCh = nil
		m.turnCancel = nil
		m.statusLabel = ""
		m.pendingToolName = ""
		m.pendingToolArgs = ""
		m.toolStreamIdx = -1
		m.toolStreamID = ""
		m.toolTail = nil
		m.toolPartial = ""
		m.toolLineCount = 0
		m.toolStreamStart = time.Time{}
	}
	return m
}

// drainBatchMsg carries a batch of events drained from the stream channel.
type drainBatchMsg struct {
	events []event.Event
}

// drainEvents reads up to maxEventDrain events from the channel, then returns
// a drainBatchMsg. If the channel is closed, it returns streamClosedMsg.
func drainEvents(ch <-chan event.Event) tea.Cmd {
	return func() tea.Msg {
		events := make([]event.Event, 0, maxEventDrain)
		for i := 0; i < maxEventDrain; i++ {
			e, ok := <-ch
			if !ok {
				if len(events) == 0 {
					return streamClosedMsg{}
				}
				break
			}
			events = append(events, e)
			// If we just got a TurnDone, stop draining so it can be processed.
			if e.Kind == event.TurnDone {
				break
			}
		}
		return drainBatchMsg{events: events}
	}
}

// fetchGitStatus 异步执行 git 命令获取分支和状态。
func fetchGitStatus() tea.Cmd {
	return func() tea.Msg {
		branch := runGitCmd("rev-parse", "--abbrev-ref", "HEAD")
		porcelain := runGitCmd("status", "--porcelain")
		ahead := runGitCmd("rev-list", "--count", "HEAD..@{upstream}")
		behind := runGitCmd("rev-list", "--count", "@{upstream}..HEAD")

		gs := gitStatus{
			branch: strings.TrimSpace(branch),
			dirty:  strings.TrimSpace(porcelain) != "",
		}
		// 解析 ahead/behind 计数（忽略解析错误）
		if n, err := strconv.Atoi(strings.TrimSpace(ahead)); err == nil {
			gs.ahead = n
		}
		if n, err := strconv.Atoi(strings.TrimSpace(behind)); err == nil {
			gs.behind = n
		}
		return gitStatusMsg{status: gs}
	}
}

// runGitCmd 执行 git 命令，忽略错误并返回 stdout 字符串。
func runGitCmd(args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}
