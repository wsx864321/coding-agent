package agent

import (
	"context"
	"testing"
	"time"
)

type approvalEmitter struct {
	name    string
	args    map[string]any
	respond func(bool)
}

func (e *approvalEmitter) OnChunk(string) {}
func (e *approvalEmitter) OnToolStart(string, string) {}
func (e *approvalEmitter) OnToolEnd(string, string, bool) {}
func (e *approvalEmitter) OnDone() {}
func (e *approvalEmitter) OnError(error) {}

func (e *approvalEmitter) OnApprovalRequest(name string, args map[string]any, respond func(bool)) {
	e.name = name
	e.args = args
	e.respond = respond
}

func TestRequestApprovalReturnsTrueWhenApproved(t *testing.T) {
	emit := &approvalEmitter{}
	done := make(chan bool, 1)
	go func() {
		done <- requestApproval(context.Background(), emit, "write_file", map[string]any{"path": "a.txt"})
	}()

	time.Sleep(10 * time.Millisecond)
	emit.respond(true)

	if got := <-done; !got {
		t.Fatal("requestApproval() = false, want true")
	}
}

func TestRequestApprovalReturnsFalseWhenDenied(t *testing.T) {
	emit := &approvalEmitter{}
	done := make(chan bool, 1)
	go func() {
		done <- requestApproval(context.Background(), emit, "bash", map[string]any{"command": "rm x"})
	}()

	time.Sleep(10 * time.Millisecond)
	emit.respond(false)

	if got := <-done; got {
		t.Fatal("requestApproval() = true, want false")
	}
}

func TestRequestApprovalReturnsFalseOnContextCancel(t *testing.T) {
	emit := &approvalEmitter{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() {
		done <- requestApproval(ctx, emit, "write_file", nil)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	if got := <-done; got {
		t.Fatal("requestApproval() = true, want false on ctx cancel")
	}
}

func TestRequestApprovalRespondCalledOnce(t *testing.T) {
	emit := &approvalEmitter{}
	done := make(chan bool, 1)
	go func() {
		done <- requestApproval(context.Background(), emit, "write_file", nil)
	}()

	time.Sleep(10 * time.Millisecond)
	emit.respond(true)
	emit.respond(false)

	if got := <-done; !got {
		t.Fatal("requestApproval() = false, want true from first respond")
	}
}

func TestEmitterAskerUsesContextEmitter(t *testing.T) {
	emit := &approvalEmitter{}
	ctx := WithEmitter(context.Background(), emit)
	done := make(chan bool, 1)
	go func() {
		done <- (EmitterAsker{}).Ask(ctx, "write_file", map[string]any{"path": "x"}, "reason")
	}()

	time.Sleep(10 * time.Millisecond)
	emit.respond(true)

	if got := <-done; !got {
		t.Fatal("EmitterAsker.Ask() = false, want true")
	}
	if emit.name != "write_file" {
		t.Fatalf("OnApprovalRequest name = %q, want write_file", emit.name)
	}
}

func TestEmitterAskerReturnsFalseWithoutEmitter(t *testing.T) {
	got := (EmitterAsker{}).Ask(context.Background(), "write_file", nil, "reason")
	if got {
		t.Fatal("EmitterAsker.Ask() = true, want false without emitter in context")
	}
}
