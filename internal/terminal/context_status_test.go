package terminal

import (
	"bytes"
	"testing"

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

func TestRendererWriteContextStatusUsesErrOut(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	renderer := NewRenderer(&out, &errOut)
	renderer.WriteContextStatus(&contextbudget.ContextBudget{
		MaxTokens:   1000,
		PercentFull: 1,
		Segs:        usage.SegmentCounts{Conversation: 10},
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
