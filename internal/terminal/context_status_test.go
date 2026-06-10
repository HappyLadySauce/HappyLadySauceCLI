package terminal

import (
	"bytes"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

func TestRendererWriteTurnStatusUsesErrOut(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	renderer := NewRenderer(&out, &errOut)
	renderer.WriteTurnStatus(budget.TurnStatus{
		Stats: budget.TurnStats{
			ElapsedMs:        960,
			PromptTokens:     1340,
			CompletionTokens: 31,
		},
		Budget: &usage.Breakdown{
			MaxContext:     128000,
			EstimatedTotal: 458, // 318+103+37, for PercentUsed() ≈ 0.36% → "<1%"
			Segs: usage.SegmentCounts{
				Conversation: 318,
				Tools:        103,
				System:       37,
			},
		},
	})

	if out.Len() != 0 {
		t.Fatalf("stdout buffer = %q, want empty", out.String())
	}
	want := "[ Stats: elapsed=960ms prompt↑=1340 completion↓=31 ]\n[context <1% 128K | conv 318 | tools 103 | sys 37]\n"
	if got := errOut.String(); got != want {
		t.Fatalf("stderr buffer = %q, want %q", got, want)
	}

	renderer.WriteTurnStatus(budget.TurnStatus{})
	if got := errOut.String(); got != want {
		t.Fatalf("empty status should not write, got %q", got)
	}
}
