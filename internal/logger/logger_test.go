package logger

import (
	"bytes"
	"context"
	"errors"
	"reflect"
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

func TestValuesWithTracePrependsTraceFields(t *testing.T) {
	ctx := AttachTurn(context.Background(), "s1", "c1", 3)
	NextModelCall(ctx) // model_call=1

	got := valuesWithTrace(ctx, "phase", "model_call_end", "prompt", 10)
	want := []any{
		"session_id", "s1",
		"conversation_id", "c1",
		"user_turn_seq", 3,
		"model_call", 1,
		"phase", "model_call_end",
		"prompt", 10,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("valuesWithTrace() = %#v, want %#v", got, want)
	}
}

func TestValuesWithTraceNoTrace(t *testing.T) {
	t.Parallel()

	got := valuesWithTrace(context.Background(), "phase", "model_call_end")
	want := []any{"phase", "model_call_end"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("valuesWithTrace() = %#v, want %#v", got, want)
	}
}

func TestInfoWritesStructuredKlogWithTrace(t *testing.T) {
	ctx := AttachTurn(context.Background(), "s1", "c1", 3)
	NextModelCall(ctx)

	output := captureKlogOutput(t, func() {
		Info(ctx, 0, "Model call completed",
			"phase", "model_call_end",
			"prompt", 10)
	})
	for _, want := range []string{
		`"Model call completed"`,
		`session_id="s1"`,
		`conversation_id="c1"`,
		`user_turn_seq=3`,
		`model_call=1`,
		`phase="model_call_end"`,
		`prompt=10`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("Info() output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "detail_log") {
		t.Fatalf("Info() output should not contain detail_log:\n%s", output)
	}
}

func TestInfoVerbosityGate(t *testing.T) {
	output := captureKlogOutput(t, func() {
		Info(context.Background(), klog.Level(10000), "Should not appear", "phase", "test")
	})
	if output != "" {
		t.Fatalf("Info() with disabled verbosity wrote output:\n%s", output)
	}
}

func TestErrorWritesStructuredKlogWithTrace(t *testing.T) {
	ctx := AttachTurn(context.Background(), "s1", "c1", 3)
	NextModelCall(ctx)

	output := captureKlogOutput(t, func() {
		Error(ctx, errors.New("boom"), "Could not persist conversation", "phase", "persistence")
	})
	for _, want := range []string{
		`"Could not persist conversation"`,
		`err="boom"`,
		`session_id="s1"`,
		`conversation_id="c1"`,
		`phase="persistence"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("Error() output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "detail_log") {
		t.Fatalf("Error() output should not contain detail_log:\n%s", output)
	}
}

func captureKlogOutput(t *testing.T, fn func()) string {
	t.Helper()

	state := klog.CaptureState()
	defer state.Restore()

	var buf bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&buf)
	fn()
	klog.Flush()
	return buf.String()
}
