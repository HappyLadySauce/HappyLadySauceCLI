package terminal

import (
	"bytes"
	"strings"
	"testing"

	contextcommon "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common"
)

func TestFormatContextStatusLineFull(t *testing.T) {
	t.Parallel()

	line := FormatContextStatusLine(&contextcommon.ContextBudget{
		MaxTokens:   128000,
		TotalTokens: 52500,
		PercentFull: 41,
		Segments: map[contextcommon.Segment]int{
			contextcommon.SegmentConversation: 32600,
			contextcommon.SegmentTools:        8600,
			contextcommon.SegmentSystem:       500,
			contextcommon.SegmentRules:        400,
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
	if got := FormatContextStatusLine(&contextcommon.ContextBudget{}); got != "" {
		t.Fatalf("FormatContextStatusLine(invalid) = %q, want empty", got)
	}
}

func TestFormatContextStatusLinePercentRounding(t *testing.T) {
	t.Parallel()

	tiny := FormatContextStatusLine(&contextcommon.ContextBudget{MaxTokens: 1000, PercentFull: 0.4})
	if !strings.Contains(tiny, "<1%") {
		t.Fatalf("tiny percent line = %q, want <1%%", tiny)
	}
	rounded := FormatContextStatusLine(&contextcommon.ContextBudget{MaxTokens: 1000, PercentFull: 41.5})
	if !strings.Contains(rounded, "42%") {
		t.Fatalf("rounded percent line = %q, want 42%%", rounded)
	}
}

func TestFormatContextStatusLineTokenFormatting(t *testing.T) {
	t.Parallel()

	line := FormatContextStatusLine(&contextcommon.ContextBudget{
		MaxTokens:   32768,
		PercentFull: 12,
		Segments: map[contextcommon.Segment]int{
			contextcommon.SegmentConversation: 32600,
			contextcommon.SegmentSystem:       1000,
			contextcommon.SegmentTools:        999,
		},
	})

	want := "[context 12% 32K | conv 32.6k | sys 1.0k | tools 999]"
	if line != want {
		t.Fatalf("FormatContextStatusLine() = %q, want %q", line, want)
	}
}

func TestFormatContextStatusLineStableTopThreeOrdering(t *testing.T) {
	t.Parallel()

	line := FormatContextStatusLine(&contextcommon.ContextBudget{
		MaxTokens:   100000,
		PercentFull: 10,
		Segments: map[contextcommon.Segment]int{
			contextcommon.SegmentSubagents:    100,
			contextcommon.SegmentMCP:          100,
			contextcommon.SegmentSkills:       100,
			contextcommon.SegmentRules:        100,
			contextcommon.SegmentSystem:       100,
			contextcommon.SegmentTools:        100,
			contextcommon.SegmentConversation: 100,
		},
	})

	want := "[context 10% 100K | conv 100 | tools 100 | sys 100]"
	if line != want {
		t.Fatalf("FormatContextStatusLine() = %q, want %q", line, want)
	}
}

func TestRendererWriteContextStatusUsesErrOut(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	renderer := NewRenderer(&out, &errOut)
	renderer.WriteContextStatus(&contextcommon.ContextBudget{
		MaxTokens:   1000,
		PercentFull: 1,
		Segments:    map[contextcommon.Segment]int{contextcommon.SegmentConversation: 10},
	})

	if out.Len() != 0 {
		t.Fatalf("stdout buffer = %q, want empty", out.String())
	}
	if got := errOut.String(); got != "[context 1% 1K | conv 10]\n" {
		t.Fatalf("stderr buffer = %q", got)
	}
	renderer.WriteContextStatus(nil)
	if got := errOut.String(); got != "[context 1% 1K | conv 10]\n" {
		t.Fatalf("nil budget should not write, got %q", got)
	}
}
