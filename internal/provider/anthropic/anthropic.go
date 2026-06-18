// Package anthropic 实现 Anthropic Messages API 的 Provider 后端。
//
// 使用原生 net/http + SSE 流式解析。Anthropic 的 wire format 与 OpenAI 差异较大：
// - 认证用 x-api-key 头（不是 Bearer）
// - system 消息独立于 messages 列表之外
// - 消息内容是 content block 数组，不是纯文本
// - tool_use / tool_result 使用专有 block 类型
// - SSE 事件类型与 OpenAI 完全不同
package anthropic

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
	defaultBaseURL    = "https://api.anthropic.com"
	messagesPath      = "/v1/messages"
	anthropicVersion  = "2023-06-01"
	streamIdleTimeout = 120 * time.Second
	defaultMaxTokens  = 4096
)

func init() {
	provider.Register("anthropic", func(cfg provider.Config) (provider.Provider, error) {
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

// New 构建 Anthropic Messages API 后端
func New(cfg provider.Config) (provider.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic: API key 不能为空")
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	name := cfg.Name
	if name == "" {
		name = "anthropic"
	}
	return &client{
		name:    name,
		baseURL: baseURL,
		model:   cfg.Model,
		apiKey:  cfg.APIKey,
		keyEnv:  cfg.KeyEnv,
		httpClient: &http.Client{
			Timeout: 0,
		},
	}, nil
}

func (c *client) Name() string { return c.name }

// Stream 发起流式 /v1/messages 请求
func (c *client) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}

	body := c.buildRequestBody(model, req)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: 序列化请求失败: %w", err)
	}

	resp, err := provider.SendWithRetry(ctx, c.httpClient, provider.SendOptions{
		Provider:  c.name,
		KeyEnv:    c.keyEnv,
		HasKey:    c.apiKey != "",
		RetryAuth: c.authed.Load(),
	}, func(ctx context.Context) (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+messagesPath, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", c.apiKey)
		httpReq.Header.Set("anthropic-version", anthropicVersion)
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

func (c *client) buildRequestBody(model string, req provider.Request) anthRequest {
	var system string
	msgs := make([]anthMessage, 0, len(req.Messages))

	for _, m := range req.Messages {
		if m.Role == provider.RoleSystem {
			system = m.Content
			continue
		}

		if m.Role == provider.RoleTool {
			msgs = append(msgs, anthMessage{
				Role: "user",
				Content: []anthContentBlock{
					{
						Type:      "tool_result",
						ToolUseID: m.ToolCallID,
						Content:   m.Content,
					},
				},
			})
			continue
		}

		if m.Role == provider.RoleAssistant && len(m.ToolCalls) > 0 {
			blocks := make([]anthContentBlock, 0, len(m.ToolCalls)+1)
			if strings.TrimSpace(m.Content) != "" {
				blocks = append(blocks, anthContentBlock{
					Type: "text",
					Text: m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				var input json.RawMessage
				if tc.Arguments != "" {
					input = json.RawMessage(tc.Arguments)
				} else {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				})
			}
			msgs = append(msgs, anthMessage{
				Role:    "assistant",
				Content: blocks,
			})
			continue
		}

		msgs = append(msgs, anthMessage{
			Role: string(m.Role),
			Content: []anthContentBlock{
				{
					Type: "text",
					Text: m.Content,
				},
			},
		})
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	r := anthRequest{
		Model:     model,
		System:    system,
		Messages:  msgs,
		MaxTokens: maxTokens,
		Stream:    true,
	}

	if len(req.Tools) > 0 {
		tools := make([]anthTool, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = anthTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.Parameters,
			}
		}
		r.Tools = tools
	}

	return r
}

// readStream 解析 Anthropic SSE 流
func (c *client) readStream(ctx context.Context, body io.ReadCloser, ch chan<- provider.Chunk) {
	defer close(ch)
	defer body.Close()

	hasOutput := false
	var currentEvent string
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

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
				if err := <-errCh; err != nil {
					if hasOutput {
						ch <- provider.Chunk{Type: provider.ChunkError, Err: &provider.StreamInterruptedError{Err: err}}
					} else {
						ch <- provider.Chunk{Type: provider.ChunkError, Err: err}
					}
				}
				return
			}

			if strings.HasPrefix(line, "event: ") {
				currentEvent = strings.TrimPrefix(line, "event: ")
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")

			chunks := c.processEvent(currentEvent, []byte(data), &hasOutput)
			for _, chunk := range chunks {
				ch <- chunk
			}
			currentEvent = ""
		}
	}
}

func (c *client) processEvent(event string, data []byte, hasOutput *bool) []provider.Chunk {
	switch event {
	case "message_start":
		var evt anthMessageStart
		if json.Unmarshal(data, &evt) == nil && evt.Message.Usage.InputTokens > 0 {
			return []provider.Chunk{{
				Type: provider.ChunkUsage,
				Usage: &provider.Usage{
					PromptTokens: evt.Message.Usage.InputTokens,
				},
			}}
		}

	case "content_block_start":
		var evt anthContentBlockStart
		if json.Unmarshal(data, &evt) != nil {
			return nil
		}
		switch evt.ContentBlock.Type {
		case "text":
			// 文本块开始，等 delta 再发送
		case "tool_use":
			*hasOutput = true
			return []provider.Chunk{{
				Type: provider.ChunkToolCallStart,
				ToolCall: &provider.ToolCall{
					ID:   evt.ContentBlock.ID,
					Name: evt.ContentBlock.Name,
				},
			}}
		}

	case "content_block_delta":
		var evt anthContentBlockDelta
		if json.Unmarshal(data, &evt) != nil {
			return nil
		}
		switch evt.Delta.Type {
		case "text_delta":
			*hasOutput = true
			return []provider.Chunk{{
				Type: provider.ChunkText,
				Text: evt.Delta.Text,
			}}
		case "input_json_delta":
			*hasOutput = true
			return []provider.Chunk{{
				Type: provider.ChunkToolCallDelta,
				ToolCall: &provider.ToolCall{
					Arguments: evt.Delta.PartialJSON,
				},
			}}
		}

	case "message_delta":
		var evt anthMessageDelta
		if json.Unmarshal(data, &evt) != nil {
			return nil
		}
		reason := evt.Delta.StopReason
		switch reason {
		case "end_turn":
			reason = provider.FinishReasonStop
		case "tool_use":
			reason = provider.FinishReasonToolCalls
		case "max_tokens":
			reason = provider.FinishReasonLength
		}
		return []provider.Chunk{{
			Type: provider.ChunkUsage,
			Usage: &provider.Usage{
				CompletionTokens: evt.Usage.OutputTokens,
				FinishReason:     reason,
			},
		}}

	case "message_stop":
		return []provider.Chunk{{Type: provider.ChunkDone}}

	case "error":
		var evt anthError
		if json.Unmarshal(data, &evt) == nil {
			err := fmt.Errorf("anthropic: %s: %s", evt.Error.Type, evt.Error.Message)
			if *hasOutput {
				return []provider.Chunk{{
					Type: provider.ChunkError,
					Err:  &provider.StreamInterruptedError{Err: err},
				}}
			}
			return []provider.Chunk{{
				Type: provider.ChunkError,
				Err:  err,
			}}
		}
	}

	return nil
}

// --- Wire types ---

type anthRequest struct {
	Model     string        `json:"model"`
	System    string        `json:"system,omitempty"`
	Messages  []anthMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens"`
	Stream    bool          `json:"stream"`
	Tools     []anthTool    `json:"tools,omitempty"`
}

type anthMessage struct {
	Role    string             `json:"role"`
	Content []anthContentBlock `json:"content"`
}

type anthContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthMessageStart struct {
	Message struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type anthContentBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"content_block"`
}

type anthContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta"`
}

type anthMessageDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}
