// Package openai 实现 OpenAI 兼容的 Provider 后端。
//
// 支持所有 OpenAI 兼容 API（OpenAI / DeepSeek / MiniMax / 其它网关）。
// 使用原生 net/http + SSE 流式解析，不依赖第三方 SDK。
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wsx864321/coding-agent/internal/provider"
)

const (
	defaultBaseURL         = "https://api.openai.com/v1"
	chatCompletionsPath    = "/chat/completions"
	maxStreamReconnects    = 3
	streamIdleTimeout      = 120 * time.Second
)

func init() {
	provider.Register("openai", func(cfg provider.Config) (provider.Provider, error) {
		return New(cfg)
	})
}

type client struct {
	name       string
	baseURL    string
	model      string
	apiKey     string
	keyEnv     string
	httpClient *http.Client
	authed     atomic.Bool
}

// New 构建 OpenAI 兼容后端
func New(cfg provider.Config) (provider.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai: API key 不能为空")
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	name := cfg.Name
	if name == "" {
		name = "openai"
	}
	return &client{
		name:    name,
		baseURL: baseURL,
		model:   cfg.Model,
		apiKey:  cfg.APIKey,
		keyEnv:  cfg.KeyEnv,
		httpClient: &http.Client{
			Timeout: 0, // streaming, no timeout
		},
	}, nil
}

func (c *client) Name() string { return c.name }

// Stream 发起流式 /chat/completions 请求
func (c *client) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}

	body := c.buildRequestBody(model, req)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: 序列化请求失败: %w", err)
	}

	resp, err := provider.SendWithRetry(ctx, c.httpClient, provider.SendOptions{
		Provider:  c.name,
		KeyEnv:    c.keyEnv,
		HasKey:    c.apiKey != "",
		RetryAuth: c.authed.Load(),
	}, func(ctx context.Context) (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+chatCompletionsPath, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("Accept", "text/event-stream")
		return httpReq, nil
	})
	if err != nil {
		return nil, err
	}

	c.authed.Store(true)

	ch := make(chan provider.Chunk, 32)
	go c.readStream(ctx, resp.Body, ch)
	return ch, nil
}

// buildRequestBody 构造 OpenAI chat/completions 请求体
func (c *client) buildRequestBody(model string, req provider.Request) chatRequest {
	msgs := make([]chatMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		cm := chatMessage{
			Role:    string(m.Role),
			Content: m.Content,
			Name:    m.Name,
		}
		if m.ToolCallID != "" {
			cm.ToolCallID = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			tcs := make([]chatToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				tcs[i] = chatToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: chatFunction{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				}
			}
			cm.ToolCalls = tcs
		}
		// DeepSeek 等 API 要求 assistant 消息必须有 content 字段
		if m.Role == provider.RoleAssistant && cm.Content == "" && len(cm.ToolCalls) > 0 {
			cm.Content = " "
		}
		// DeepSeek 等 API 要求 tool 消息 content 不能为空
		if m.Role == provider.RoleTool && cm.Content == "" {
			cm.Content = " "
		}
		msgs = append(msgs, cm)
	}

	r := chatRequest{
		Model:    model,
		Messages: msgs,
		Stream:   true,
		StreamOptions: &streamOptions{
			IncludeUsage: true,
		},
	}

	if len(req.Tools) > 0 {
		tools := make([]chatTool, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = chatTool{
				Type: "function",
				Function: chatFunctionDef{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.Parameters,
				},
			}
		}
		r.Tools = tools
	}

	if req.MaxTokens > 0 {
		r.MaxTokens = req.MaxTokens
	}
	if req.Temperature > 0 {
		r.Temperature = &req.Temperature
	}

	return r
}

// readStream 解析 SSE 流并将 Chunk 推入 channel
func (c *client) readStream(ctx context.Context, body io.ReadCloser, ch chan<- provider.Chunk) {
	defer close(ch)
	defer body.Close()

	hasOutput := false
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 最大 1MB 单行

	idleTimer := time.NewTimer(streamIdleTimeout)
	defer idleTimer.Stop()

	lineCh := make(chan string, 64)
	errCh := make(chan error, 1)
	go func() {
		defer close(lineCh)
		for scanner.Scan() {
			select {
			case lineCh <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	for {
		idleTimer.Reset(streamIdleTimeout)
		select {
		case <-ctx.Done():
			return
		case <-idleTimer.C:
			err := fmt.Errorf("stream idle timeout (%v)", streamIdleTimeout)
			if hasOutput {
				ch <- provider.Chunk{Type: provider.ChunkError, Err: &provider.StreamInterruptedError{Err: err}}
			} else {
				ch <- provider.Chunk{Type: provider.ChunkError, Err: err}
			}
			return
		case line, ok := <-lineCh:
			if !ok {
				// scanner 结束
				if err := <-errCh; err != nil {
					if hasOutput {
						ch <- provider.Chunk{Type: provider.ChunkError, Err: &provider.StreamInterruptedError{Err: err}}
					} else {
						ch <- provider.Chunk{Type: provider.ChunkError, Err: err}
					}
				}
				return
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- provider.Chunk{Type: provider.ChunkDone}
				return
			}

			var delta streamDelta
			if err := json.Unmarshal([]byte(data), &delta); err != nil {
				continue
			}

			chunks := c.processDelta(&delta, &hasOutput)
			for _, chunk := range chunks {
				ch <- chunk
			}
		}
	}
}

// processDelta 将 SSE delta 转换为 provider.Chunk 列表
func (c *client) processDelta(delta *streamDelta, hasOutput *bool) []provider.Chunk {
	var chunks []provider.Chunk

	if delta.Usage != nil {
		chunks = append(chunks, provider.Chunk{
			Type: provider.ChunkUsage,
			Usage: &provider.Usage{
				PromptTokens:     delta.Usage.PromptTokens,
				CompletionTokens: delta.Usage.CompletionTokens,
				TotalTokens:      delta.Usage.TotalTokens,
			},
		})
	}

	if len(delta.Choices) == 0 {
		return chunks
	}

	choice := delta.Choices[0]

	if choice.FinishReason != "" {
		reason := choice.FinishReason
		if len(chunks) > 0 && chunks[len(chunks)-1].Type == provider.ChunkUsage {
			chunks[len(chunks)-1].Usage.FinishReason = reason
		} else {
			chunks = append(chunks, provider.Chunk{
				Type: provider.ChunkUsage,
				Usage: &provider.Usage{
					FinishReason: reason,
				},
			})
		}
	}

	d := choice.Delta

	if d.Content != "" {
		*hasOutput = true
		chunks = append(chunks, provider.Chunk{
			Type: provider.ChunkText,
			Text: d.Content,
		})
	}

	for _, tc := range d.ToolCalls {
		*hasOutput = true
		if tc.ID != "" {
			chunks = append(chunks, provider.Chunk{
				Type: provider.ChunkToolCallStart,
				ToolCall: &provider.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		} else {
			chunks = append(chunks, provider.Chunk{
				Type: provider.ChunkToolCallDelta,
				ToolCall: &provider.ToolCall{
					Arguments: tc.Function.Arguments,
				},
			})
		}
	}

	return chunks
}

// --- Wire types ---

type chatRequest struct {
	Model         string         `json:"model"`
	Messages      []chatMessage  `json:"messages"`
	Stream        bool           `json:"stream"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
	Tools         []chatTool     `json:"tools,omitempty"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	Temperature   *float32       `json:"temperature,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	Name       string         `json:"name,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
}

type chatToolCall struct {
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type chatTool struct {
	Type     string          `json:"type"`
	Function chatFunctionDef `json:"function"`
}

type chatFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type streamDelta struct {
	Choices []streamChoice `json:"choices"`
	Usage   *streamUsage   `json:"usage,omitempty"`
}

type streamChoice struct {
	Delta        streamDeltaContent `json:"delta"`
	FinishReason string             `json:"finish_reason,omitempty"`
}

type streamDeltaContent struct {
	Role      string            `json:"role,omitempty"`
	Content   string            `json:"content,omitempty"`
	ToolCalls []streamToolDelta `json:"tool_calls,omitempty"`
}

type streamToolDelta struct {
	Index    int          `json:"index"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function chatFunction `json:"function,omitempty"`
}

type streamUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
