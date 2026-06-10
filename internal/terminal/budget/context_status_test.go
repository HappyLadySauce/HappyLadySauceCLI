package budget

import (
	"strings"
	"testing"

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

func TestFormatStatsLine(t *testing.T) {
	t.Parallel()

	line := FormatStatsLine(contextbudget.TurnStats{
		ElapsedMs:        960,
		PromptTokens:     1340,
		CompletionTokens: 31,
	})
	want := "[ Stats: elapsed=960ms prompt↑=1340 completion↓=31 ]"
	if line != want {
		t.Fatalf("FormatStatsLine() = %q, want %q", line, want)
	}
}

func TestFormatStatsLineEmpty(t *testing.T) {
	t.Parallel()

	if got := FormatStatsLine(contextbudget.TurnStats{}); got != "" {
		t.Fatalf("FormatStatsLine(empty) = %q, want empty", got)
	}
}

func TestFormatContextStatusLinePostTurn(t *testing.T) {
	t.Parallel()

	line := FormatContextStatusLine(&contextbudget.ContextBudget{
		MaxTokens:   128000,
		PercentFull: 0.35,
		Segs: usage.SegmentCounts{
			Conversation: 318,
			Tools:        103,
			System:       37,
		},
	})

	want := "[context <1% 128K | conv 318 | tools 103 | sys 37]"
	if line != want {
		t.Fatalf("FormatContextStatusLine() = %q, want %q", line, want)
	}
}

func TestFormatContextStatusLineNilAndInvalid(t *testing.T) {
	t.Parallel()

	if got := FormatContextStatusLine(nil); got != "" {
		t.Fatalf("FormatContextStatusLine(nil) = %q, want empty", got)
	}
	if got := FormatContextStatusLine(&contextbudget.ContextBudget{}); got != "" {
		t.Fatalf("FormatContextStatusLine(invalid) = %q, want empty", got)
	}
}

func TestFormatContextStatusLinePercentRounding(t *testing.T) {
	t.Parallel()

	tiny := FormatContextStatusLine(&contextbudget.ContextBudget{
		MaxTokens:   1000,
		PercentFull: 0.4,
		Segs:        usage.SegmentCounts{Conversation: 1},
	})
	if !strings.Contains(tiny, "<1%") {
		t.Fatalf("tiny percent line = %q, want <1%%", tiny)
	}
	rounded := FormatContextStatusLine(&contextbudget.ContextBudget{
		MaxTokens:   1000,
		PercentFull: 41.5,
		Segs:        usage.SegmentCounts{Conversation: 415},
	})
	if !strings.Contains(rounded, "42%") {
		t.Fatalf("rounded percent line = %q, want 42%%", rounded)
	}
}

func TestFormatContextStatusLineTokenFormatting(t *testing.T) {
	t.Parallel()

	line := FormatContextStatusLine(&contextbudget.ContextBudget{
		MaxTokens:   32768,
		PercentFull: 12,
		Segs: usage.SegmentCounts{
			Conversation: 32600,
			System:       1000,
			Tools:        999,
		},
	})

	want := "[context 12% 32K | conv 32.6k | tools 999 | sys 1.0k]"
	if line != want {
		t.Fatalf("FormatContextStatusLine() = %q, want %q", line, want)
	}
}
