package agent

import "context"

// StreamEmitter 接收 agent 产生的流式事件（文本增量、工具调用、审批请求等）。
type StreamEmitter interface {
	OnChunk(text string)
	OnToolStart(name string, args string)
	OnToolEnd(name string, result string, isError bool)
	OnApprovalRequest(name string, args map[string]any, respond func(bool))
	OnDone()
	OnError(err error)
}

type emitterContextKey struct{}

// WithEmitter 将 StreamEmitter 注入 context，供 loop 与工具执行路径读取。
func WithEmitter(ctx context.Context, e StreamEmitter) context.Context {
	if e == nil {
		return ctx
	}
	return context.WithValue(ctx, emitterContextKey{}, e)
}

// EmitterFromContext 从 context 取出 StreamEmitter；未注入时返回 nil。
func EmitterFromContext(ctx context.Context) StreamEmitter {
	if ctx == nil {
		return nil
	}
	e, _ := ctx.Value(emitterContextKey{}).(StreamEmitter)
	return e
}
