package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/wsx864321/coding-agent/internal/event"
)

type approvalCapture struct {
	mu     sync.Mutex
	events []event.Event
}

func (c *approvalCapture) Emit(e event.Event) {
	if e.Kind != event.ApprovalRequest {
		return
	}
	c.mu.Lock()
	c.events = append(c.events, e)
	c.mu.Unlock()
}

func TestSinkAsker_Approves(t *testing.T) {
	cap := &approvalCapture{}
	asker := SinkAsker{Sink: cap}
	done := make(chan bool, 1)
	go func() {
		done <- asker.Ask(context.Background(), "write_file", map[string]any{"path": "a.txt"}, "reason")
	}()
	time.Sleep(10 * time.Millisecond)
	cap.mu.Lock()
	if len(cap.events) != 1 {
		cap.mu.Unlock()
		t.Fatal("expected ApprovalRequest event")
	}
	cap.events[0].ApprovalRespond(true)
	cap.mu.Unlock()
	if !<-done {
		t.Fatal("Ask() = false, want true")
	}
}

func TestSinkAsker_NoSinkReturnsFalse(t *testing.T) {
	got := (SinkAsker{}).Ask(context.Background(), "write_file", nil, "reason")
	if got {
		t.Fatal("Ask() = true, want false without sink")
	}
}

func TestRequestApprovalViaSink_ReturnsFalseWhenDenied(t *testing.T) {
	cap := &approvalCapture{}
	done := make(chan bool, 1)
	go func() {
		done <- requestApprovalViaSink(context.Background(), cap, "bash", map[string]any{"command": "rm x"})
	}()
	time.Sleep(10 * time.Millisecond)
	cap.mu.Lock()
	cap.events[0].ApprovalRespond(false)
	cap.mu.Unlock()
	if got := <-done; got {
		t.Fatal("requestApprovalViaSink() = true, want false")
	}
}

func TestRequestApprovalViaSink_ReturnsFalseOnContextCancel(t *testing.T) {
	cap := &approvalCapture{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() {
		done <- requestApprovalViaSink(ctx, cap, "write_file", nil)
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	if got := <-done; got {
		t.Fatal("requestApprovalViaSink() = true, want false on ctx cancel")
	}
}

func TestRequestApprovalViaSink_RespondCalledOnce(t *testing.T) {
	cap := &approvalCapture{}
	done := make(chan bool, 1)
	go func() {
		done <- requestApprovalViaSink(context.Background(), cap, "write_file", nil)
	}()
	time.Sleep(10 * time.Millisecond)
	cap.mu.Lock()
	cap.events[0].ApprovalRespond(true)
	cap.events[0].ApprovalRespond(false)
	cap.mu.Unlock()
	if got := <-done; !got {
		t.Fatal("requestApprovalViaSink() = false, want true from first respond")
	}
}
