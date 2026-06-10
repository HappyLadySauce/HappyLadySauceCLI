package terminal

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
)

func TestRendererWriteTurnStatusUsesErrOut(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	renderer := NewRenderer(&out, &errOut)
	renderer.WriteTurnStatus(budget.TurnStats{
		ElapsedMs:        766,
		PromptTokens:     318,
		CompletionTokens: 37,
		ContextTokens:    318,
		MaxContext:       128000,
	})

	if out.Len() != 0 {
		t.Fatalf("stdout buffer = %q, want empty", out.String())
	}
	want := "[Stats: elapsed=0.77s prompt↑=318 completion↓=37 total↑↓=355 <1% 128K]\n"
	if got := errOut.String(); got != want {
		t.Fatalf("stderr buffer = %q, want %q", got, want)
	}

	renderer.WriteTurnStatus(budget.TurnStats{})
	if got := errOut.String(); got != want {
		t.Fatalf("empty status should not write, got %q", got)
	}
}

func TestRendererWriteTurnStatusAppliesColorOnTerminal(t *testing.T) {
	t.Parallel()

	renderer := NewRenderer(os.Stdout, os.Stderr)
	renderer.colorEnabled = true

	line := renderer.formatTurnStatusLine(budget.TurnStats{
		ElapsedMs:        766,
		PromptTokens:     318,
		CompletionTokens: 37,
		ContextTokens:    318,
		MaxContext:       128000,
	})
	if !strings.Contains(line, "\x1b[") {
		t.Fatalf("colored line = %q, want ANSI escape sequences", line)
	}
	if !strings.Contains(line, "total↑↓=355") {
		t.Fatalf("colored line = %q, want total token count", line)
	}
}
