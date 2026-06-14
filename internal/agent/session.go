package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// SessionInfo 概括一个已保存的 session，供 --list / --resume 展示。
type SessionInfo struct {
	Path      string    // 文件完整路径
	Preview   string    // 首条 user 消息截取，长度 ≤ 80 个 rune
	Turns     int       // 用户输入轮次
	UpdatedAt time.Time // 最后活跃时间（文件 mtime）
	ID        string    // 文件名（不含 .jsonl 后缀）
}

// SaveSession 将 messages 以 JSONL 格式原子写入 path。
//
// 内部使用临时文件 + rename，避免崩溃残留半截文件。
func SaveSession(path string, messages []openai.ChatCompletionMessage) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("session path 不能为空")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建 session 目录失败: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".session.*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()

	enc := json.NewEncoder(tmp)
	encodeErr := func() error {
		for _, m := range messages {
			if err := enc.Encode(m); err != nil {
				return fmt.Errorf("编码消息失败: %w", err)
			}
		}
		return nil
	}()
	if encodeErr != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return encodeErr
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	return os.Rename(tmpPath, path)
}

// LoadSession 读取 SaveSession 写入的 JSONL 文件，返回完整 messages 切片。
//
// 文件不存在时返回 os.IsNotExist 错误，调用方可据此回退到新 session。
// 使用 json.Decoder 流式解析，避免超大单条消息（如 MiB 级 tool 输出）超出行缓冲限制。
func LoadSession(path string) ([]openai.ChatCompletionMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var messages []openai.ChatCompletionMessage
	dec := json.NewDecoder(f)
	for {
		var m openai.ChatCompletionMessage
		if err := dec.Decode(&m); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// ListSessions 返回 dir 下所有 *.jsonl session，按更新时间倒序排列。
//
// 跳过空 session（无 user 消息的会话），缺失目录不报错，返回空列表。
func ListSessions(dir string) ([]SessionInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []SessionInfo
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		full := filepath.Join(dir, e.Name())
		preview, turns := previewSession(full)
		if turns == 0 {
			continue
		}
		out = append(out, SessionInfo{
			Path:      full,
			Preview:   preview,
			Turns:     turns,
			UpdatedAt: info.ModTime(),
			ID:        strings.TrimSuffix(e.Name(), ".jsonl"),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Path < out[j].Path
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})

	return out, nil
}

// previewSession 返回首条 user 消息的前 80 字符（截断）以及 user 消息总数。
//
// 解析失败时静默返回空预览和 0 turns——调用方可据此过滤掉坏文件。
func previewSession(path string) (preview string, turns int) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	for {
		var m openai.ChatCompletionMessage
		if err := dec.Decode(&m); err != nil {
			break
		}
		if m.Role == openai.ChatMessageRoleUser {
			turns++
			if preview == "" {
				s := strings.TrimSpace(m.Content)
				// 跳过 compaction summary（以 summaryTagOpen 开头）
				if strings.HasPrefix(s, summaryTagOpen) {
					continue
				}
				if r := []rune(s); len(r) > 80 {
					s = string(r[:77]) + "…"
				}
				preview = s
			}
		}
	}
	return preview, turns
}

// NewSessionPath 生成 session 文件路径：<dir>/<时间戳>-<模型名>.jsonl。
//
// 文件名格式：20060102-150405.000-deepseek-v3.jsonl
// 纳秒精度 UTC 时间戳保证可排序，模型名提供可读信息。
func NewSessionPath(dir, model string) string {
	safe := strings.NewReplacer("/", "-", "\\", "-").Replace(model)
	if safe == "" {
		safe = "session"
	}
	return filepath.Join(dir,
		fmt.Sprintf("%s-%s.jsonl", time.Now().UTC().Format("20060102-150405.000"), safe))
}

// sessionBucket 返回 session 目录下的项目分桶子目录。
//
// 与 archive 共用相同的 archiveProjectBucket 分桶策略，
// 确保 session 与 archive 的目录结构对称一致。
func sessionBucket(workdir string) string {
	return archiveProjectBucket(workdir)
}

// SessionBucket 返回 session 根目录下的项目分桶完整路径。
//
// 例如 SessionBucket("~/.coding-agent/sessions", "E:\\goproject\\coding-agent")
//
//	→ "~/.coding-agent/sessions/coding-agent-a1b2c3d4e5f6"
func SessionBucket(baseDir, workdir string) string {
	return filepath.Join(baseDir, archiveProjectBucket(workdir))
}
