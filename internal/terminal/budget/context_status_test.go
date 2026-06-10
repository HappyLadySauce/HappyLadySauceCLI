package budget

import (
	"strings"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

func TestFormatStatsLine(t *testing.T) {
	t.Parallel()

	line := FormatStatsLine(budget.TurnStats{
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

	if got := FormatStatsLine(budget.TurnStats{}); got != "" {
		t.Fatalf("FormatStatsLine(empty) = %q, want empty", got)
	}
}

func TestFormatContextStatusLinePostTurn(t *testing.T) {
	t.Parallel()

	line := FormatContextStatusLine(&usage.Breakdown{
		MaxContext:     128000,
		EstimatedTotal: 458, // 318+103+37, for PercentUsed() ≈ 0.36% → "<1%"
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
	if got := FormatContextStatusLine(&usage.Breakdown{}); got != "" {
		t.Fatalf("FormatContextStatusLine(invalid) = %q, want empty", got)
	}
}

func TestFormatContextStatusLinePercentRounding(t *testing.T) {
	t.Parallel()

	tiny := FormatContextStatusLine(&usage.Breakdown{
		MaxContext:     1000,
		EstimatedTotal: 4, // 0.4% → "<1%"
		Segs:           usage.SegmentCounts{Conversation: 4},
	})
	if !strings.Contains(tiny, "<1%") {
		t.Fatalf("tiny percent line = %q, want <1%%", tiny)
	}
	rounded := FormatContextStatusLine(&usage.Breakdown{
		MaxContext:     1000,
		EstimatedTotal: 415, // 41.5% → "42%"
		Segs:           usage.SegmentCounts{Conversation: 415},
	})
	if !strings.Contains(rounded, "42%") {
		t.Fatalf("rounded percent line = %q, want 42%%", rounded)
	}
}

func TestFormatContextStatusLineTokenFormatting(t *testing.T) {
	t.Parallel()

	// Use ActualPrompt to control the percent display independently of segment values.
	// ActualPrompt 控制百分比显示，使其与分段值独立。
	line := FormatContextStatusLine(&usage.Breakdown{
		MaxContext:     32768,
		ActualPrompt:   3932, // 3932/32768 ≈ 12%
		EstimatedTotal: 34599,
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
