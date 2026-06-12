package logger

import (
	"context"
	"strings"
	"testing"

	"k8s.io/klog/v2"
)

func TestAttachTurnAndFromContext(t *testing.T) {
	t.Parallel()

	ctx := AttachTurn(context.Background(), "s1", "c1", 3)
	trace := FromContext(ctx)
	if trace == nil {
		t.Fatal("FromContext() returned nil")
	}
	if trace.SessionID != "s1" || trace.ConversationID != "c1" || trace.UserTurnSeq != 3 {
		t.Fatalf("trace = %#v", trace)
	}
}

func TestNextModelCall(t *testing.T) {
	t.Parallel()

	ctx := AttachTurn(context.Background(), "s1", "c1", 1)
	if got, want := NextModelCall(ctx), 1; got != want {
		t.Fatalf("NextModelCall() first = %d, want %d", got, want)
	}
	if got, want := NextModelCall(ctx), 2; got != want {
		t.Fatalf("NextModelCall() second = %d, want %d", got, want)
	}
	if got, want := ModelCall(ctx), 2; got != want {
		t.Fatalf("ModelCall() = %d, want %d", got, want)
	}
}

func TestEmitPhaseTraceInjection(t *testing.T) {
	t.Parallel()

	ctx := AttachTurn(context.Background(), "s1", "c1", 3)
	NextModelCall(ctx) // model_call=1

	line := emitPhase(ctx, SeverityInfo, 0, "test_phase", "key1", "val1")
	for _, want := range []string{
		"phase=test_phase",
		"session_id=s1",
		"conversation_id=c1",
		"user_turn_seq=3",
		"model_call=1",
		"detail_log=session/s1.jsonl",
		"key1=val1",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("emitPhase() line missing %q:\n  %s", want, line)
		}
	}
}

func TestEmitPhaseNoTrace(t *testing.T) {
	t.Parallel()

	line := emitPhase(context.Background(), SeverityInfo, 0, "test_phase", "a", 1)
	if !strings.Contains(line, "phase=test_phase") {
		t.Fatalf("emitPhase() = %q, want phase=test_phase", line)
	}
	if !strings.Contains(line, "a=1") {
		t.Fatalf("emitPhase() = %q, want a=1", line)
	}
	if strings.Contains(line, "session_id") {
		t.Fatalf("emitPhase() with no trace should not contain session_id: %s", line)
	}
}

func TestEmitPhaseFieldsOrder(t *testing.T) {
	t.Parallel()

	ctx := AttachTurn(context.Background(), "s1", "c1", 1)
	line := emitPhase(ctx, SeverityInfo, 0, "test_phase", "z", "last", "a", "first")

	// User fields appear in caller order (z before a).
	zIdx := strings.Index(line, "z=last")
	aIdx := strings.Index(line, "a=first")
	if zIdx < 0 || aIdx < 0 || zIdx >= aIdx {
		t.Fatalf("emitPhase() fields out of order: %s", line)
	}
}

func TestFormatFieldValue(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{"hello", "hello"},
		{"hello world", `"hello world"`},
		{"a\tb", `"a\tb"`},
		{"a\nb", `"a\nb"`},
	}
	for _, tc := range tests {
		if got := formatFieldValue(tc.in); got != tc.want {
			t.Fatalf("formatFieldValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPhaseInfoVerbosityGate(t *testing.T) {
	// klog.V(10000) is never enabled → should produce no output.
	PhaseInfo(context.Background(), klog.Level(10000), "should_not_appear", "x", "y")
}

func TestPhaseWarnAndErrorSmoke(t *testing.T) {
	// Smoke test: these should not panic.
	PhaseWarn(context.Background(), "test_warn", "reason", "smoke_test")
	PhaseError(context.Background(), "test_error", "reason", "smoke_test")
}
