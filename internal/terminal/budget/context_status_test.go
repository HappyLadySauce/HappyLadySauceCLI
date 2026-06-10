package budget

import (
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
)

func TestFormatElapsed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		elapsedMs int64
		want      string
	}{
		{0, "0.00s"},
		{766, "0.77s"},
		{2910, "2.91s"},
		{2559, "2.56s"},
		{59999, "60.00s"},
		{60000, "1m0s"},
		{65000, "1m5s"},
		{125000, "2m5s"},
	}
	for _, tc := range tests {
		if got := FormatElapsed(tc.elapsedMs); got != tc.want {
			t.Fatalf("FormatElapsed(%d) = %q, want %q", tc.elapsedMs, got, tc.want)
		}
	}
}

func TestFormatTurnStatusLine(t *testing.T) {
	t.Parallel()

	line := FormatTurnStatusLine(budget.TurnStats{
		ElapsedMs:        766,
		PromptTokens:     318,
		CompletionTokens: 37,
		ContextTokens:    318,
		MaxContext:       128000,
	})
	want := "[Stats: elapsed=0.77s prompt↑=318 completion↓=37 total↑↓=355 <1% 128K]"
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
	want := "[Stats: elapsed=0.10s prompt↑=50 completion↓=5 total↑↓=55]"
	if line != want {
		t.Fatalf("FormatTurnStatusLine() = %q, want %q", line, want)
	}
}

func TestFormatTurnStatusLineOverOneMinute(t *testing.T) {
	t.Parallel()

	line := FormatTurnStatusLine(budget.TurnStats{
		ElapsedMs:        65000,
		PromptTokens:     1000,
		CompletionTokens: 200,
		ContextTokens:    5000,
		MaxContext:       128000,
	})
	want := "[Stats: elapsed=1m5s prompt↑=1000 completion↓=200 total↑↓=1200 4% 128K]"
	if line != want {
		t.Fatalf("FormatTurnStatusLine() = %q, want %q", line, want)
	}
}

func TestFormatTurnStatusLinePercentRounding(t *testing.T) {
	t.Parallel()

	tiny := FormatTurnStatusLine(budget.TurnStats{ContextTokens: 4, MaxContext: 1000})
	if tiny != "[Stats: elapsed=0.00s prompt↑=0 completion↓=0 total↑↓=0 <1% 1K]" {
		t.Fatalf("tiny percent line = %q", tiny)
	}

	rounded := FormatTurnStatusLine(budget.TurnStats{ContextTokens: 415, MaxContext: 1000})
	if rounded != "[Stats: elapsed=0.00s prompt↑=0 completion↓=0 total↑↓=0 42% 1K]" {
		t.Fatalf("rounded percent line = %q", rounded)
	}
}
