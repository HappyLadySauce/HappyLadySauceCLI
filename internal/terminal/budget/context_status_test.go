package budget

import (
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
)

func TestFormatTurnStatusLine(t *testing.T) {
	t.Parallel()

	line := FormatTurnStatusLine(budget.TurnStats{
		ElapsedMs:        960,
		PromptTokens:     1340,
		CompletionTokens: 31,
		ContextTokens:    458,
		MaxContext:       128000,
	})
	want := "[ Stats: elapsed=960ms prompt↑=1340 completion↓=31 context <1% 128K ]"
	if line != want {
		t.Fatalf("FormatTurnStatusLine() = %q, want %q", line, want)
	}
}

func TestFormatTurnStatusLineEmpty(t *testing.T) {
	t.Parallel()

	if got := FormatTurnStatusLine(budget.TurnStats{}); got != "" {
		t.Fatalf("FormatTurnStatusLine(empty) = %q, want empty", got)
	}
}

func TestFormatTurnStatusLineWithoutContext(t *testing.T) {
	t.Parallel()

	line := FormatTurnStatusLine(budget.TurnStats{
		ElapsedMs:        100,
		PromptTokens:     50,
		CompletionTokens: 5,
	})
	want := "[ Stats: elapsed=100ms prompt↑=50 completion↓=5 ]"
	if line != want {
		t.Fatalf("FormatTurnStatusLine() = %q, want %q", line, want)
	}
}

func TestFormatTurnStatusLinePercentRounding(t *testing.T) {
	t.Parallel()

	tiny := FormatTurnStatusLine(budget.TurnStats{ContextTokens: 4, MaxContext: 1000})
	if tiny != "[ Stats: elapsed=0ms prompt↑=0 completion↓=0 context <1% 1K ]" {
		t.Fatalf("tiny percent line = %q", tiny)
	}

	rounded := FormatTurnStatusLine(budget.TurnStats{ContextTokens: 415, MaxContext: 1000})
	if rounded != "[ Stats: elapsed=0ms prompt↑=0 completion↓=0 context 42% 1K ]" {
		t.Fatalf("rounded percent line = %q", rounded)
	}
}
