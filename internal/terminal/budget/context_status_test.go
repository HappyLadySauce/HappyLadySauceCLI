package budget

import (
	"strings"
	"testing"

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
)

func TestFormatContextStatusLineFull(t *testing.T) {
	t.Parallel()

	line := FormatContextStatusLine(&contextbudget.ContextBudget{
		MaxTokens:   128000,
		TotalTokens: 52500,
		PercentFull: 41,
		Segments: map[contextbudget.Segment]int{
			contextbudget.SegmentConversation: 32600,
			contextbudget.SegmentTools:        8600,
			contextbudget.SegmentSystem:       500,
			contextbudget.SegmentRules:        400,
		},
	})

	want := "[context 41% 128K | conv 32.6k | tools 8.6k | sys 500]"
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
		Segments: map[contextbudget.Segment]int{
			contextbudget.SegmentConversation: 32600,
			contextbudget.SegmentSystem:       1000,
			contextbudget.SegmentTools:        999,
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
		Segments: map[contextbudget.Segment]int{
			contextbudget.SegmentSubagents:    100,
			contextbudget.SegmentMCP:          100,
			contextbudget.SegmentSkills:       100,
			contextbudget.SegmentRules:        100,
			contextbudget.SegmentSystem:       100,
			contextbudget.SegmentTools:        100,
			contextbudget.SegmentConversation: 100,
		},
	})

	want := "[context 10% 100K | conv 100 | tools 100 | sys 100]"
	if line != want {
		t.Fatalf("FormatContextStatusLine() = %q, want %q", line, want)
	}
}
