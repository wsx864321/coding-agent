package memory

import (
	"strings"
	"testing"
)

func TestQueueEnqueueSave(t *testing.T) {
	q := NewQueue()
	if q.Pending() {
		t.Error("new queue should not be pending")
	}

	q.EnqueueSave(Memory{Name: "test", Description: "a test memory"})
	if !q.Pending() {
		t.Error("queue should be pending after enqueue")
	}
}

func TestQueueEnqueueDelete(t *testing.T) {
	q := NewQueue()
	q.EnqueueDelete("old-memory")
	if !q.Pending() {
		t.Error("queue should be pending after delete enqueue")
	}
}

func TestQueueEnqueueNote(t *testing.T) {
	q := NewQueue()
	q.EnqueueNote("custom note")
	if !q.Pending() {
		t.Error("queue should be pending after note enqueue")
	}
}

func TestQueueFlush(t *testing.T) {
	q := NewQueue()

	// 空 flush
	if s := q.Flush(); s != "" {
		t.Errorf("flush empty queue = %q, want empty", s)
	}

	q.EnqueueSave(Memory{Name: "m1", Description: "desc1"})
	q.EnqueueSave(Memory{Name: "m2", Description: "desc2"})

	result := q.Flush()
	if !strings.Contains(result, "<memory-update>") {
		t.Error("flush missing memory-update tag")
	}
	if !strings.Contains(result, "m1") {
		t.Error("flush missing memory name")
	}
	if !strings.Contains(result, "desc1") {
		t.Error("flush missing memory description")
	}
	if !strings.Contains(result, "m2") {
		t.Error("flush missing second memory name")
	}
	if !strings.Contains(result, "</memory-update>") {
		t.Error("flush missing closing tag")
	}

	// 清空后不应 pending
	if q.Pending() {
		t.Error("queue should not be pending after flush")
	}

	// 第二次 flush 应为空
	if s := q.Flush(); s != "" {
		t.Errorf("second flush = %q, want empty", s)
	}
}

func TestQueueFlushMixed(t *testing.T) {
	q := NewQueue()
	q.EnqueueSave(Memory{Name: "added", Description: "new"})
	q.EnqueueDelete("removed")
	q.EnqueueNote("noted")

	result := q.Flush()
	if !strings.Contains(result, "added") {
		t.Error("missing save notification")
	}
	if !strings.Contains(result, "removed") {
		t.Error("missing delete notification")
	}
	if !strings.Contains(result, "noted") {
		t.Error("missing custom note")
	}
}

func TestQueueConcurrent(t *testing.T) {
	q := NewQueue()
	done := make(chan bool)

	// 并发写入
	for i := 0; i < 10; i++ {
		go func(i int) {
			q.EnqueueSave(Memory{Name: "test", Description: "desc"})
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Flush 应返回非空且包含 memory-update 标签
	result := q.Flush()
	if !strings.Contains(result, "<memory-update>") {
		t.Errorf("concurrent flush: %q", result)
	}
}
