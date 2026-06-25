package tui

import "context"

// Runner 执行一轮用户输入并推送流式事件。
type Runner interface {
	RunTurn(ctx context.Context, prompt string) error
}

// BalanceProvider 是提供余额查询的可选接口。
// Runner 可选择实现此接口；TUI 通过类型断言使用。
type BalanceProvider interface {
	Balance(ctx context.Context) (string, error)
}

// ContextSnapshotProvider 提供上下文窗口用量查询。
type ContextSnapshotProvider interface {
	ContextSnapshot() (used int, window int)
}
