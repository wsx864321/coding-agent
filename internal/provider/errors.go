package provider

import (
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
)

// APIError 报告非认证类的 HTTP 错误状态
type APIError struct {
	Provider string
	Status   int
	Body     string
}

func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("%s: HTTP %d", e.Provider, e.Status)
	}
	return fmt.Sprintf("%s: HTTP %d: %s", e.Provider, e.Status, e.Body)
}

// AuthError 报告 API key 认证失败（401/403）
type AuthError struct {
	Provider string
	KeyEnv   string
	Status   int
	HasKey   bool
}

func (e *AuthError) Error() string {
	if !e.HasKey {
		return fmt.Sprintf("%s: 认证失败 (HTTP %d): 未设置 API key（请检查环境变量 %s）",
			e.Provider, e.Status, e.KeyEnv)
	}
	return fmt.Sprintf("%s: 认证失败 (HTTP %d): API key 无效或已过期（来源: %s）",
		e.Provider, e.Status, e.KeyEnv)
}

// StreamInterruptedError 标记在已产生部分输出后的传输中断。
// Provider 不应自行重试这类错误（会导致重复输出），而应交由 agent 层处理。
type StreamInterruptedError struct {
	Err error
}

func (e *StreamInterruptedError) Error() string {
	return fmt.Sprintf("stream interrupted: %v", e.Err)
}

func (e *StreamInterruptedError) Unwrap() error {
	return e.Err
}

// IsStreamInterrupted 检查 err 是否为 StreamInterruptedError
func IsStreamInterrupted(err error) bool {
	var interrupted *StreamInterruptedError
	return errors.As(err, &interrupted)
}

// IsConnReset 判断 err 是否为连接级别的断开（对端重置、EOF、连接关闭等），
// 与协议错误或调用方错误区分。
func IsConnReset(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// IsPromptTooLong 检测上下文溢出错误
func IsPromptTooLong(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, kw := range []string{
		"prompt_too_long",
		"prompt is too long",
		"context_length_exceeded",
		"max_context_length",
		"maximum context length",
		"too many tokens",
		"request too large",
	} {
		if containsFold(s, kw) {
			return true
		}
	}
	return false
}

func containsFold(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
