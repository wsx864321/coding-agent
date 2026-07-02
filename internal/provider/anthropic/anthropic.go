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
			Transport: &http.Transport{
				TLSHandshakeTimeout:   30 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
			},
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
				// Arguments 来自流式增量拼接，可能为空或被截断成非法 JSON；
				// json.RawMessage 对空/非法内容会报 "unexpected end of JSON input"。
				// 用 json.Valid 校验，非法时回退为 "{}"，保证序列化成功。
				input := json.RawMessage("{}")
				if json.Valid([]byte(tc.Arguments)) {
					input = json.RawMessage(tc.Arguments)
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
			// Parameters 可能来自 MCP 等外部源，可能为空或非法 JSON；
			// 空切片会让 json.RawMessage 序列化报 "unexpected end of JSON input"。
			// 非法时回退为最小合法 schema。
			schema := t.Parameters
			if len(schema) == 0 || !json.Valid(schema) {
				schema = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			tools[i] = anthTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: schema,
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

			// 只处理 data 行。不同网关对 "event:" 行冒号后是否带空格处理不一
			// （实测 deepseek 网关发 "event:message_start" 无空格），而每个 data
			// JSON 的顶层 "type" 字段才是权威的事件判别依据，与 Anthropic 官方一致。
			// 因此忽略 event 行，直接从 data JSON 解析 type。
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}

			// message_stop / error 是终止事件：发出对应 chunk 后必须立即 return。
			// 否则消费端（CollectWithText 收到 ChunkError 会立刻放弃读 channel，
			// 收到 ChunkDone 会继续 range 等 channel 关闭）与 readStream 产生死锁
			// 或 120s 空等——继续往无人读取的 channel 写会永久阻塞。
			terminal, chunks := c.processEvent([]byte(data), &hasOutput)
			for _, chunk := range chunks {
				ch <- chunk
			}
			if terminal {
				return
			}
		}
	}
}

// processEvent 解析单个 SSE data 事件。返回 terminal=true 表示该事件是流的
// 终止信号（message_stop / error），调用方应立即结束 readStream。
// 事件类型取自 data JSON 的顶层 "type" 字段（而非 SSE event 行），以兼容各网关。
func (c *client) processEvent(data []byte, hasOutput *bool) (terminal bool, chunks []provider.Chunk) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return false, nil
	}
	switch head.Type {
	case "message_start":
		var evt anthMessageStart
		if json.Unmarshal(data, &evt) == nil && evt.Message.Usage.InputTokens > 0 {
			return false, []provider.Chunk{{
				Type: provider.ChunkUsage,
				Usage: &provider.Usage{
					PromptTokens: evt.Message.Usage.InputTokens,
				},
			}}
		}

	case "content_block_start":
		var evt anthContentBlockStart
		if json.Unmarshal(data, &evt) != nil {
			return false, nil
		}
		switch evt.ContentBlock.Type {
		case "text":
			// 文本块开始，等 delta 再发送
		case "tool_use":
			*hasOutput = true
			return false, []provider.Chunk{{
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
			return false, nil
		}
		switch evt.Delta.Type {
		case "text_delta":
			*hasOutput = true
			return false, []provider.Chunk{{
				Type: provider.ChunkText,
				Text: evt.Delta.Text,
			}}
		case "input_json_delta":
			*hasOutput = true
			return false, []provider.Chunk{{
				Type: provider.ChunkToolCallDelta,
				ToolCall: &provider.ToolCall{
					Arguments: evt.Delta.PartialJSON,
				},
			}}
		}

	case "message_delta":
		var evt anthMessageDelta
		if json.Unmarshal(data, &evt) != nil {
			return false, nil
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
		return false, []provider.Chunk{{
			Type: provider.ChunkUsage,
			Usage: &provider.Usage{
				CompletionTokens: evt.Usage.OutputTokens,
				FinishReason:     reason,
			},
		}}

	case "message_stop":
		return true, []provider.Chunk{{Type: provider.ChunkDone}}

	case "error":
		var evt anthError
		if json.Unmarshal(data, &evt) == nil {
			err := fmt.Errorf("anthropic: %s: %s", evt.Error.Type, evt.Error.Message)
			if *hasOutput {
				return true, []provider.Chunk{{
					Type: provider.ChunkError,
					Err:  &provider.StreamInterruptedError{Err: err},
				}}
			}
			return true, []provider.Chunk{{
				Type: provider.ChunkError,
				Err:  err,
			}}
		}
	}

	return false, nil
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
