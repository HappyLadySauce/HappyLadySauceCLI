package usage

import (
	"errors"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
)

func TestTurnFromMessageExtractsProviderUsage(t *testing.T) {
	t.Parallel()

	msg := schema.AssistantMessage("ok", nil)
	msg.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
	}}

	turn := TurnFromMessage(25*time.Millisecond, msg, nil)
	if turn.Prompt != 10 || turn.Completion != 5 || turn.Total != 15 {
		t.Fatalf("turn usage = prompt:%d completion:%d total:%d", turn.Prompt, turn.Completion, turn.Total)
	}
	if turn.Elapsed != 25*time.Millisecond || turn.Status != contextmodel.StatusSucceeded {
		t.Fatalf("turn = %#v, want elapsed succeeded turn", turn)
	}
}

func TestTurnFromMessageRecordsErrorWithoutUsage(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("model down")
	turn := TurnFromMessage(time.Millisecond, nil, wantErr)
	if turn.Prompt != 0 || turn.Completion != 0 || turn.Total != 0 {
		t.Fatalf("turn usage = prompt:%d completion:%d total:%d, want zeros", turn.Prompt, turn.Completion, turn.Total)
	}
	if turn.Status != contextmodel.StatusFailed || turn.Error != wantErr.Error() {
		t.Fatalf("failed turn = %#v", turn)
	}
}
