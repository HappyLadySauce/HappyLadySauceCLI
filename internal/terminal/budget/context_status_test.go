package budget

import (
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
)

func TestFormatTurnStatusLine(t *testing.T) {
	t.Parallel()

	line := FormatTurnStatusLine(budget.TurnStats{
		ElapsedMs:        766,
		PromptTokens:     318,
		CompletionTokens: 37,
		ContextTokens:    318,
		MaxContext:       128000,
	})
	want := "[Stats: elapsed=766ms prompt↑=318 completion↓=37 total↑↓=355 <1% 128K]"
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
	want := "[Stats: elapsed=100ms prompt↑=50 completion↓=5 total↑↓=55]"
	if line != want {
		t.Fatalf("FormatTurnStatusLine() = %q, want %q", line, want)
	}
}

func TestFormatTurnStatusLinePercentRounding(t *testing.T) {
	t.Parallel()

	tiny := FormatTurnStatusLine(budget.TurnStats{ContextTokens: 4, MaxContext: 1000})
	if tiny != "[Stats: elapsed=0ms prompt↑=0 completion↓=0 total↑↓=0 <1% 1K]" {
		t.Fatalf("tiny percent line = %q", tiny)
	}

	rounded := FormatTurnStatusLine(budget.TurnStats{ContextTokens: 415, MaxContext: 1000})
	if rounded != "[Stats: elapsed=0ms prompt↑=0 completion↓=0 total↑↓=0 42% 1K]" {
		t.Fatalf("rounded percent line = %q", rounded)
	}
}
