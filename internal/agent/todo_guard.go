package agent

import (
	"context"
	"fmt"

	"github.com/wsx864321/coding-agent/internal/event"
	"github.com/wsx864321/coding-agent/internal/evidence"
)

const maxGuardBlocks = 3

func (a *Agent) checkTodoGuard(ctx context.Context) string {
	ledger, ok := evidence.FromContext(ctx)
	if !ok {
		return ""
	}
	todos := ledger.CurrentTodos()
	if len(todos) == 0 {
		return ""
	}
	var incomplete []string
	for _, t := range todos {
		if t.Status != "completed" {
			incomplete = append(incomplete, fmt.Sprintf("%s [%s]", t.Content, t.Status))
		}
	}
	if len(incomplete) == 0 {
		return ""
	}
	blocks := ledger.IncrementGuardBlock()
	if blocks > maxGuardBlocks {
		a.sink.Emit(event.Event{
			Kind:  event.Notice,
			Level: event.LevelInfo,
			Text: fmt.Sprintf("终答守卫: %d/%d 未完成，已超过最大阻断次数 (%d)，放行",
				len(incomplete), len(todos), maxGuardBlocks),
		})
		return ""
	}
	a.sink.Emit(event.Event{
		Kind:  event.Notice,
		Level: event.LevelInfo,
		Text: fmt.Sprintf("终答守卫: 阻断最终回答 — %d/%d 待办未完成（第 %d/%d 次阻断）",
			len(incomplete), len(todos), blocks, maxGuardBlocks),
	})

	force := fmt.Sprintf(
		"宿主就绪检查失败: %d/%d 待办仍未完成。请先完成剩余任务再给出最终回答:\n",
		len(incomplete), len(todos))
	for _, item := range incomplete {
		force += "  - " + item + "\n"
	}
	force += "请执行必要的工具调用（complete_step + todo_write），待所有待办完成后再回答。"
	return force
}
