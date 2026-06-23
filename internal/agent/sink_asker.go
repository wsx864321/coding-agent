package agent

import (
	"context"
	"sync"

	"github.com/wsx864321/coding-agent/internal/event"
)

func requestApprovalViaSink(ctx context.Context, sink event.Sink, name string, args map[string]any) bool {
	if sink == nil {
		return false
	}
	ch := make(chan bool, 1)
	var once sync.Once
	respond := func(ok bool) { once.Do(func() { ch <- ok; close(ch) }) }
	sink.Emit(event.Event{
		Kind:            event.ApprovalRequest,
		ApprovalName:    name,
		ApprovalArgs:    args,
		ApprovalRespond: respond,
	})
	select {
	case approved := <-ch:
		return approved
	case <-ctx.Done():
		respond(false)
		return false
	}
}

type SinkAsker struct {
	Sink event.Sink
}

func (a SinkAsker) Ask(ctx context.Context, name string, args map[string]any, _ string) bool {
	return requestApprovalViaSink(ctx, a.Sink, name, args)
}
