package memory

import (
	"fmt"
	"strings"
	"sync"
)

// Queue 管理中会话记忆变更通知
//
// 设计目标：保护 system prompt 的前缀缓存稳定性。
// 新增/删除的记忆不直接修改 system prompt，而是通过 Queue 暂存，
// 在下一次用户输入时前置到 user 消息前面。
//
// 格式：
//
//	<memory-update>
//	新增记忆：prefers-tabs — User prefers tabs
//	删除记忆：old-preference
//	</memory-update>
type Queue struct {
	mu    sync.Mutex
	items []string // 待注入的通知内容
}

// NewQueue 创建一个空队列
func NewQueue() *Queue {
	return &Queue{}
}

// EnqueueSave 追加一条"新增记忆"通知
func (q *Queue) EnqueueSave(m Memory) {
	q.mu.Lock()
	defer q.mu.Unlock()
	msg := fmt.Sprintf("新增记忆：%s — %s", m.Name, m.Description)
	q.items = append(q.items, msg)
}

// EnqueueDelete 追加一条"删除记忆"通知
func (q *Queue) EnqueueDelete(name string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	msg := fmt.Sprintf("删除记忆：%s", name)
	q.items = append(q.items, msg)
}

// EnqueueNote 追加一条自定义通知
func (q *Queue) EnqueueNote(note string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, note)
}

// Flush 消费并清空队列中的所有通知
//
// 返回格式化的通知文本（可前置到 user 消息）；若无待处理通知则返回空字符串。
func (q *Queue) Flush() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return ""
	}
	content := "<memory-update>\n" + strings.Join(q.items, "\n") + "\n</memory-update>"
	q.items = q.items[:0]
	return content
}

// Pending 检查是否有待处理通知
func (q *Queue) Pending() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) > 0
}
