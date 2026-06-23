package agent

import (
	"context"
	"sync"
)

// requestApproval 通过 emitter 向 TUI 发起审批请求并阻塞等待用户响应。
func requestApproval(ctx context.Context, emitter StreamEmitter, name string, args map[string]any) bool {
	ch := make(chan bool, 1)
	var once sync.Once
	respond := func(ok bool) { once.Do(func() { ch <- ok; close(ch) }) }
	emitter.OnApprovalRequest(name, args, respond)
	select {
	case approved := <-ch:
		return approved
	case <-ctx.Done():
		respond(false)
		return false
	}
}

// EmitterAsker 通过 context 中的 StreamEmitter 实现 permission.Asker。
type EmitterAsker struct{}

// Ask 向 TUI 请求用户审批；context 无 emitter 时返回 false。
func (EmitterAsker) Ask(ctx context.Context, name string, args map[string]any, _ string) bool {
	emitter := EmitterFromContext(ctx)
	if emitter == nil {
		return false
	}
	return requestApproval(ctx, emitter, name, args)
}
