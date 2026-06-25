package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"
)

// BaseClient 是 JSON-RPC stdio 客户端的通用实现。
//
// 管理子进程生命周期、请求 ID 分配、响应分发、reader goroutine。
// 传输协议通过 Transport 接口注入（LineTransport 或 ContentLengthTransport）。
type BaseClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	transport Transport
	logger     *log.Logger

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan *Message

	// OnNotification 收到通知时回调；nil 表示忽略
	OnNotification func(msg *Message)
}

// BaseClientOptions 配置 BaseClient
type BaseClientOptions struct {
	Command   string
	Args      []string
	Dir       string
	Env       []string
	Transport Transport
}

// NewBaseClient 创建并启动一个 JSON-RPC 客户端
func NewBaseClient(opts BaseClientOptions) (*BaseClient, error) {
	c := &BaseClient{
		transport: opts.Transport,
		logger:    log.Default(),
		pending:   make(map[int64]chan *Message),
	}
	if c.transport == nil {
		c.transport = LineTransport{}
	}

	c.cmd = exec.Command(opts.Command, opts.Args...)
	c.cmd.Dir = opts.Dir
	c.cmd.Env = opts.Env

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	c.stdout, err = c.cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := c.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", opts.Command, err)
	}

	// 启动 reader goroutine
	go func() {
		if err := c.transport.ReadLoop(c.stdout, c.handleFrame); err != nil {
			c.logger.Printf("[jsonrpc] read loop: %v", err)
		}
	}()

	return c, nil
}

// handleFrame 处理每一帧（已按传输协议解出完整 JSON body）
func (c *BaseClient) handleFrame(body []byte) error {
	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	// 通知
	if msg.IsNotification() {
		if c.OnNotification != nil {
			c.OnNotification(&msg)
		}
		return nil
	}

	// 响应
	if msg.IsResponse() {
		c.mu.Lock()
		ch, ok := c.pending[msg.ID]
		if ok {
			delete(c.pending, msg.ID)
		}
		c.mu.Unlock()
		if ok {
			ch <- &msg
		}
		return nil
	}

	// 服务端→客户端请求（罕见，忽略）
	return nil
}

// Call 发送请求并等待响应
func (c *BaseClient) Call(ctx context.Context, method string, params any) (*Message, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan *Message, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	msg, err := NewRequest(id, method, params)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	if err := c.write(msg); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("timeout: %w", ctx.Err())
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp, nil
	}
}

// Notify 发送通知（不等待响应）
func (c *BaseClient) Notify(method string, params any) error {
	msg, err := NewNotify(method, params)
	if err != nil {
		return err
	}
	return c.write(msg)
}

// write 序列化并发送消息
func (c *BaseClient) write(msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.transport.WriteFrame(c.stdin, data)
}

// Close 关闭客户端连接
func (c *BaseClient) Close() error {
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		timer := time.AfterFunc(5*time.Second, func() {
			c.cmd.Process.Kill()
		})
		c.cmd.Wait()
		timer.Stop()
	}
	return nil
}

// SetLogger 设置日志记录器；nil 恢复默认
func (c *BaseClient) SetLogger(l *log.Logger) {
	if l == nil {
		l = log.Default()
	}
	c.logger = l
}
