// Package logger provides structured diagnostic logging and trace correlation.
// Package logger 提供结构化诊断日志与 trace 关联。
package logger

import (
	"context"
)

type traceKey struct{}

// Trace holds correlation IDs propagated across one user interaction.
// Trace 保存一次用户交互在各层之间传播的关联 ID。
type Trace struct {
	SessionID      string
	ConversationID string
	UserTurnSeq    int
	modelCallSeq   int
}

// AttachTurn stores a new trace on context for one user interaction.
// AttachTurn 为一次用户交互在 context 上附加新的 trace。
func AttachTurn(ctx context.Context, sessionID, conversationID string, userTurnSeq int) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, traceKey{}, &Trace{
		SessionID:      sessionID,
		ConversationID: conversationID,
		UserTurnSeq:    userTurnSeq,
	})
}

// FromContext returns the trace attached to context, or nil.
// FromContext 返回 context 上附加的 trace；不存在时返回 nil。
func FromContext(ctx context.Context) *Trace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(traceKey{}).(*Trace)
	return trace
}

// NextModelCall increments and returns the model-call sequence for the active trace.
// NextModelCall 递增并返回当前 trace 的 model call 序号。
func NextModelCall(ctx context.Context) int {
	trace := FromContext(ctx)
	if trace == nil {
		return 0
	}
	trace.modelCallSeq++
	return trace.modelCallSeq
}

// ModelCall returns the current model-call sequence without incrementing.
// ModelCall 返回当前 model call 序号，不递增。
func ModelCall(ctx context.Context) int {
	trace := FromContext(ctx)
	if trace == nil {
		return 0
	}
	return trace.modelCallSeq
}
