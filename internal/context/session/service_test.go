package session

import (
	"context"
	"testing"
	"time"

	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
	contexttracker "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/tracker"
)

func TestStatusFromConversationReturnsStableDTO(t *testing.T) {
	t.Parallel()

	status := statusFromConversation(&contextmodel.Conversation{
		Elapsed:    766 * time.Millisecond,
		Prompt:     318,
		Completion: 37,
		Total:      634,
	}, 318)
	if status.Elapsed != 766*time.Millisecond ||
		status.Prompt != 318 ||
		status.Completion != 37 ||
		status.Total != 634 ||
		status.ContextTokens != 318 {
		t.Fatalf("statusFromConversation() = %#v", status)
	}
}

func TestFinishTurnSeparatesAggregateTotalFromContextTokens(t *testing.T) {
	t.Parallel()

	service := &Service{tracker: contexttracker.New()}
	runCtx := service.BeginTurn(context.Background())
	tracker := contexttracker.FromContext(runCtx)
	tracker.AddTurn(&contextmodel.Turn{Prompt: 10, Completion: 5, Total: 15})
	tracker.AddTurn(&contextmodel.Turn{Prompt: 20, Completion: 10, Total: 30})

	status, err := service.FinishTurn(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("FinishTurn() error = %v", err)
	}
	if status.Prompt != 30 || status.Completion != 15 || status.Total != 45 || status.ContextTokens != 30 {
		t.Fatalf("FinishTurn() status = %#v, want aggregate total 45 and content 30", status)
	}
}
