package builtin

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/evidence"
)

// MaxGuardBlocks 终答守卫最大阻断次数；超过后放行，防止无限循环
const MaxGuardBlocks = 3

// TodoGuardHook 在 Agent 尝试给出最终回答时检查 todo 是否全部完成。
//
// 行为：
//   - 无 todo 列表 → 放行
//   - 所有 todo 已 completed → 放行
//   - 存在未完成 todo → 阻断，注入续跑消息让 Agent 继续工作
//   - 连续阻断超过 MaxGuardBlocks 次 → 放行（防死循环）
//
// 通过 context 读取 evidence.Ledger，不持有 Ledger 引用。
type TodoGuardHook struct {
	Sink *Sink
}

// NewTodoGuardHook 创建 TodoGuardHook
func NewTodoGuardHook() *TodoGuardHook {
	return &TodoGuardHook{}
}

// Handle 实现 hooks.StopHook
func (h *TodoGuardHook) Handle(ctx context.Context, _ []openai.ChatCompletionMessage) (string, bool) {
	ledger, ok := evidence.FromContext(ctx)
	if !ok {
		return "", false
	}

	todos := ledger.CurrentTodos()
	if len(todos) == 0 {
		return "", false
	}

	// 统计未完成项
	var incomplete []string
	for _, t := range todos {
		if t.Status != "completed" {
			incomplete = append(incomplete, fmt.Sprintf("%s [%s]", t.Content, t.Status))
		}
	}
	if len(incomplete) == 0 {
		return "", false
	}

	// 检查阻断次数上限
	blocks := ledger.IncrementGuardBlock()
	if blocks > MaxGuardBlocks {
		h.sink().Fprintf("[HOOK] 终答守卫: %d/%d 未完成，已超过最大阻断次数 (%d)，放行\n",
			len(incomplete), len(todos), MaxGuardBlocks)
		return "", false
	}

	h.sink().Fprintf("[HOOK] 终答守卫: 阻断最终回答 — %d/%d 待办未完成（第 %d/%d 次阻断）\n",
		len(incomplete), len(todos), blocks, MaxGuardBlocks)

	force := fmt.Sprintf(
		"宿主就绪检查失败: %d/%d 待办仍未完成。"+
			"请先完成剩余任务再给出最终回答:\n",
		len(incomplete), len(todos))
	for _, item := range incomplete {
		force += "  - " + item + "\n"
	}
	force += "请执行必要的工具调用（complete_step + todo_write），待所有待办完成后再回答。"

	return force, true
}

func (h *TodoGuardHook) sink() *Sink {
	if h.Sink != nil {
		return h.Sink
	}
	return &Sink{}
}
