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

	"github.com/wsx864321/coding-agent/internal/provider"
)

// SessionInfo 概括一个已保存的 session，供 --list / --resume 展示。
type SessionInfo struct {
	Path      string
	Preview   string
	Turns     int
	UpdatedAt time.Time
	ID        string
}

// SaveSession 将 messages 以 JSONL 格式原子写入 path。
func SaveSession(path string, messages []provider.Message) error {
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
func LoadSession(path string) ([]provider.Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var messages []provider.Message
	dec := json.NewDecoder(f)
	for {
		var m provider.Message
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

func previewSession(path string) (preview string, turns int) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	for {
		var m provider.Message
		if err := dec.Decode(&m); err != nil {
			break
		}
		if m.Role == provider.RoleUser {
			turns++
			if preview == "" {
				s := strings.TrimSpace(m.Content)
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

// NewSessionPath 生成 session 文件路径
func NewSessionPath(dir, model string) string {
	safe := strings.NewReplacer("/", "-", "\\", "-").Replace(model)
	if safe == "" {
		safe = "session"
	}
	return filepath.Join(dir,
		fmt.Sprintf("%s-%s.jsonl", time.Now().UTC().Format("20060102-150405.000"), safe))
}

func sessionBucket(workdir string) string {
	return archiveProjectBucket(workdir)
}

// SessionBucket 返回 session 根目录下的项目分桶完整路径。
func SessionBucket(baseDir, workdir string) string {
	return filepath.Join(baseDir, archiveProjectBucket(workdir))
}
