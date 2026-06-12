package terminal

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	contextstatus "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/status"
)

func TestRendererWriteConversationStatusUsesErrOut(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	renderer := NewRenderer(&out, &errOut)
	renderer.WriteConversationStatus(testConversation(766*time.Millisecond, 318, 37, 634, 318), 128000)

	if out.Len() != 0 {
		t.Fatalf("stdout buffer = %q, want empty", out.String())
	}
	want := "[Stats: elapsed=0.77s prompt↑=318 completion↓=37 total↑↓=634 content=318 0.25%(128K)]\n"
	if got := errOut.String(); got != want {
		t.Fatalf("stderr buffer = %q, want %q", got, want)
	}

	renderer.WriteConversationStatus(contextstatus.Status{}, 128000)
	if got := errOut.String(); got != want {
		t.Fatalf("empty status should not write, got %q", got)
	}
}

func TestRendererWriteConversationStatusAppliesColorOnTerminal(t *testing.T) {
	t.Parallel()

	renderer := NewRenderer(os.Stdout, os.Stderr)
	renderer.colorEnabled = true

	line := renderer.formatConversationStatusLine(testConversation(766*time.Millisecond, 318, 37, 634, 318), 128000)
	if !strings.Contains(line, "\x1b[") {
		t.Fatalf("colored line = %q, want ANSI escape sequences", line)
	}
	if !strings.Contains(line, "total↑↓=634") || !strings.Contains(line, "content=318") {
		t.Fatalf("colored line = %q, want total and content token counts", line)
	}
}

func testConversation(elapsed time.Duration, prompt, completion, total, contextTokens int) contextstatus.Status {
	return contextstatus.Status{
		Elapsed:       elapsed,
		Prompt:        prompt,
		Completion:    completion,
		Total:         total,
		ContextTokens: contextTokens,
	}
}
