package logger

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/klog/v2"
)

// Severity indicates the log level for a phase entry.
// Severity 表示 phase 日志条目的级别。
type Severity int

const (
	// SeverityInfo is gated by klog verbosity (V-level).
	// SeverityInfo 受 klog verbosity (V-level) 控制。
	SeverityInfo Severity = iota

	// SeverityWarn always logs via klog.Warning, regardless of V-level.
	// SeverityWarn 始终通过 klog.Warning 输出，不受 V-level 控制。
	SeverityWarn

	// SeverityError always logs via klog.Error, regardless of V-level.
	// SeverityError 始终通过 klog.Error 输出，不受 V-level 控制。
	SeverityError
)

// PhaseInfo writes a phase-prefixed diagnostic line at the given verbosity.
// v=0 bypasses the verbosity gate (always logs).
//
// PhaseInfo 按指定 verbosity 写入带 phase 前缀的诊断日志。
// v=0 绕过 verbosity 门控（始终输出）。
func PhaseInfo(ctx context.Context, v klog.Level, phase string, kvs ...any) {
	if v > 0 && !klog.V(v).Enabled() {
		return
	}
	emitPhase(ctx, SeverityInfo, v, phase, kvs...)
}

// PhaseWarn writes a phase-prefixed warning that always appears regardless of V-level.
// PhaseWarn 写入始终可见的 warning 级别 phase 日志。
func PhaseWarn(ctx context.Context, phase string, kvs ...any) {
	emitPhase(ctx, SeverityWarn, 0, phase, kvs...)
}

// PhaseError writes a phase-prefixed error that always appears regardless of V-level.
// PhaseError 写入始终可见的 error 级别 phase 日志。
func PhaseError(ctx context.Context, phase string, kvs ...any) {
	emitPhase(ctx, SeverityError, 0, phase, kvs...)
}

func emitPhase(ctx context.Context, severity Severity, v klog.Level, phase string, kvs ...any) string {
	parts := make([]string, 0, 8+len(kvs)/2)
	parts = append(parts, "phase="+phase)

	trace := FromContext(ctx)
	if trace != nil {
		if trace.SessionID != "" {
			parts = append(parts, "session_id="+trace.SessionID)
		}
		if trace.ConversationID != "" {
			parts = append(parts, "conversation_id="+trace.ConversationID)
		}
		if trace.UserTurnSeq > 0 {
			parts = append(parts, fmt.Sprintf("user_turn_seq=%d", trace.UserTurnSeq))
		}
		if trace.modelCallSeq > 0 {
			parts = append(parts, fmt.Sprintf("model_call=%d", trace.modelCallSeq))
		}
		if trace.SessionID != "" {
			parts = append(parts, "detail_log=session/"+trace.SessionID+".jsonl")
		}
	}

	for i := 0; i < len(kvs)-1; i += 2 {
		key, ok := kvs[i].(string)
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", key, formatFieldValue(kvs[i+1])))
	}

	line := strings.Join(parts, " ")

	switch severity {
	case SeverityWarn:
		klog.Warning(line)
	case SeverityError:
		klog.Error(line)
	default:
		klog.V(v).Info(line)
	}

	return line
}

func formatFieldValue(value any) string {
	switch typed := value.(type) {
	case string:
		if strings.ContainsAny(typed, " \t\n\"") {
			return fmt.Sprintf("%q", typed)
		}
		return typed
	default:
		return fmt.Sprint(value)
	}
}
