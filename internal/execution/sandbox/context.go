package sandbox

import (
	"context"
	"errors"
)

type runnerContextKey struct{}

// WithRunner attaches the authorized command sandbox runner to ctx.
// WithRunner 将已授权的 command sandbox runner 注入 ctx。
func WithRunner(ctx context.Context, runner Runner) context.Context {
	if runner == nil {
		return ctx
	}
	return context.WithValue(ctx, runnerContextKey{}, runner)
}

// RunnerFromContext returns the command sandbox runner attached by security middleware.
// RunnerFromContext 返回 security middleware 注入的 command sandbox runner。
func RunnerFromContext(ctx context.Context) (Runner, bool) {
	runner, ok := ctx.Value(runnerContextKey{}).(Runner)
	return runner, ok && runner != nil
}

// RunFromContext executes a command through the authorized sandbox runner on ctx.
// RunFromContext 通过 ctx 中已授权的 sandbox runner 执行命令。
func RunFromContext(ctx context.Context, request Request) (Result, error) {
	runner, ok := RunnerFromContext(ctx)
	if !ok {
		return Result{}, errors.New("authorized command sandbox runner is required")
	}
	return runner.Run(ctx, request)
}
