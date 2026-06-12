package logger

import (
	"context"

	"k8s.io/klog/v2"
)

// Info writes a structured diagnostic log entry with trace fields injected from context.
// Info 写入结构化诊断日志，并自动注入 context 中的 trace 字段。
func Info(ctx context.Context, v klog.Level, msg string, kvs ...any) {
	klog.V(v).InfoS(msg, valuesWithTrace(ctx, kvs...)...)
}

// Error writes a structured diagnostic error log entry with trace fields injected from context.
// Error 写入结构化错误诊断日志，并自动注入 context 中的 trace 字段。
func Error(ctx context.Context, err error, msg string, kvs ...any) {
	klog.ErrorS(err, msg, valuesWithTrace(ctx, kvs...)...)
}

func valuesWithTrace(ctx context.Context, kvs ...any) []any {
	trace := FromContext(ctx)
	if trace == nil {
		return append([]any(nil), kvs...)
	}

	values := make([]any, 0, 8+len(kvs))
	if trace.SessionID != "" {
		values = append(values, "session_id", trace.SessionID)
	}
	if trace.ConversationID != "" {
		values = append(values, "conversation_id", trace.ConversationID)
	}
	if trace.UserTurnSeq > 0 {
		values = append(values, "user_turn_seq", trace.UserTurnSeq)
	}
	if trace.modelCallSeq > 0 {
		values = append(values, "model_call", trace.modelCallSeq)
	}
	return append(values, kvs...)
}
