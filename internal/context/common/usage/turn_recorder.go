package usage

import "context"

type turnRecorderContextKey struct{}

// TurnRecorder records per-turn provider usage from individual model hops.
// TurnRecorder 记录单轮内各次模型 hop 的 provider 用量。
type TurnRecorder interface {
	AddUsage(snapshot UsageSnapshot)
}

// WithTurnRecorder attaches a per-turn usage recorder to ctx.
// WithTurnRecorder 将单轮用量记录器附加到 ctx。
func WithTurnRecorder(ctx context.Context, recorder TurnRecorder) context.Context {
	if ctx == nil || recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, turnRecorderContextKey{}, recorder)
}

// turnRecorderFromContext returns the per-turn usage recorder attached to ctx.
// turnRecorderFromContext 返回附加在 ctx 上的单轮用量记录器。
func turnRecorderFromContext(ctx context.Context) TurnRecorder {
	if ctx == nil {
		return nil
	}
	recorder, _ := ctx.Value(turnRecorderContextKey{}).(TurnRecorder)
	return recorder
}
