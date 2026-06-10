package budget

import (
	"strings"
	"testing"

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

func TestFormatContextStatusLineFull(t *testing.T) {
	t.Parallel()

	line := FormatContextStatusLine(&contextbudget.ContextBudget{
		MaxTokens:            128000,
		TotalTokens:          52500,
		EstimatedTotalTokens: 52500,
		PercentFull:          41,
		Segs: usage.SegmentCounts{
			Conversation: 32600,
			Tools:        8600,
			System:       500,
		},
	})

	want := "[context 41% 128K | estimated 52.5k | conv 32.6k | tools 8.6k | sys 500]"
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

	tiny := FormatContextStatusLine(&contextbudget.ContextBudget{MaxTokens: 1000, PercentFull: 0.4})
	if !strings.Contains(tiny, "<1%") {
		t.Fatalf("tiny percent line = %q, want <1%%", tiny)
	}
	rounded := FormatContextStatusLine(&contextbudget.ContextBudget{MaxTokens: 1000, PercentFull: 41.5})
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

	want := "[context 12% 32K | conv 32.6k | sys 1.0k | tools 999]"
	if line != want {
		t.Fatalf("FormatContextStatusLine() = %q, want %q", line, want)
	}
}

func TestFormatContextStatusLineStableTopThreeOrdering(t *testing.T) {
	t.Parallel()

	line := FormatContextStatusLine(&contextbudget.ContextBudget{
		MaxTokens:   100000,
		PercentFull: 10,
		Segs: usage.SegmentCounts{
			System:       100,
			Conversation: 100,
			Tools:        100,
		},
	})

	want := "[context 10% 100K | conv 100 | tools 100 | sys 100]"
	if line != want {
		t.Fatalf("FormatContextStatusLine() = %q, want %q", line, want)
	}
}

func TestFormatContextStatusLineActualUsage(t *testing.T) {
	t.Parallel()

	line := FormatContextStatusLine(&contextbudget.ContextBudget{
		MaxTokens:              128000,
		EstimatedTotalTokens:   52600,
		ActualPromptTokens:     54200,
		ActualCompletionTokens: 1100,
		PercentFull:            42.34375,
	})

	want := "[context 42% 128K | actual prompt 54.2k | out 1.1k | est 52.6k]"
	if line != want {
		t.Fatalf("FormatContextStatusLine() = %q, want %q", line, want)
	}
}
