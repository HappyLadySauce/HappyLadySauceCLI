package logger

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
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
	if err := applyKlogFileOnly(); err != nil {
		t.Fatalf("applyKlogFileOnly() error = %v", err)
	}
	klog.LogToStderr(false)
	klog.SetOutputBySeverity("INFO", &buf)
	klog.SetOutputBySeverity("ERROR", &buf)
	fn()
	klog.Flush()
	return buf.String()
}

func TestConfigureFileWritesInfoAndErrorToSameFile(t *testing.T) {
	logDir := t.TempDir()

	closer, paths, err := configureFile(logDir)
	if err != nil {
		t.Fatalf("configureFile() error = %v", err)
	}
	defer closer.Close()

	Info(context.Background(), 0, "info only", "phase", "test")
	Error(context.Background(), errors.New("boom"), "error only", "phase", "test")
	klog.Flush()

	logBytes, err := os.ReadFile(paths.Path)
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}

	logText := string(logBytes)
	if !strings.Contains(logText, "info only") {
		t.Fatalf("log missing info message: %q", logText)
	}
	if !strings.Contains(logText, "error only") {
		t.Fatalf("log missing error message: %q", logText)
	}
	if strings.Count(logText, "error only") != 1 {
		t.Fatalf("log duplicated error message: %q", logText)
	}
}

func TestConfigureFileDoesNotWriteErrorsToStderr(t *testing.T) {
	logDir := t.TempDir()

	closer, _, err := configureFile(logDir)
	if err != nil {
		t.Fatalf("configureFile() error = %v", err)
	}
	defer closer.Close()

	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = stderrW
	t.Cleanup(func() {
		os.Stderr = oldStderr
		_ = stderrW.Close()
	})

	Error(context.Background(), errors.New("boom"), "stderr should stay clean", "phase", "test")
	klog.Flush()
	_ = stderrW.Close()

	stderrOut, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("ReadAll(stderr) error = %v", err)
	}
	if len(stderrOut) != 0 {
		t.Fatalf("expected empty stderr, got %q", string(stderrOut))
	}
}
