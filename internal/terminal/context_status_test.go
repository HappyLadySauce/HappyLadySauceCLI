package terminal

import (
	"bytes"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
)

func TestRendererWriteTurnStatusUsesErrOut(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	renderer := NewRenderer(&out, &errOut)
	renderer.WriteTurnStatus(budget.TurnStats{
		ElapsedMs:        960,
		PromptTokens:     1340,
		CompletionTokens: 31,
		ContextTokens:    458,
		MaxContext:       128000,
	})

	if out.Len() != 0 {
		t.Fatalf("stdout buffer = %q, want empty", out.String())
	}
	want := "[ Stats: elapsed=960ms prompt↑=1340 completion↓=31 context <1% 128K ]\n"
	if got := errOut.String(); got != want {
		t.Fatalf("stderr buffer = %q, want %q", got, want)
	}

	renderer.WriteTurnStatus(budget.TurnStats{})
	if got := errOut.String(); got != want {
		t.Fatalf("empty status should not write, got %q", got)
	}
}
