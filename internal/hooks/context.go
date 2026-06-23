package hooks

import "context"

type subagentCtxKey struct{}

func WithSubagentFlag(ctx context.Context) context.Context {
	return context.WithValue(ctx, subagentCtxKey{}, true)
}

func IsSubagent(ctx context.Context) bool {
	v, _ := ctx.Value(subagentCtxKey{}).(bool)
	return v
}
