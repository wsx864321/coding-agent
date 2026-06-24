package tui

import "context"

// Runner 执行一轮用户输入并推送流式事件。
type Runner interface {
	RunTurn(ctx context.Context, prompt string) error
}
