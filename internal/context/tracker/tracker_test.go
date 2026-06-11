package tracker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
)

func TestTrackerRecordsConversationTurnsAndMessages(t *testing.T) {
	t.Parallel()

	tracker := New()
	tracker.BeginConversation()
	tracker.AddTurn(&contextmodel.Turn{
		Elapsed:    25 * time.Millisecond,
		Prompt:     10,
		Completion: 5,
		Total:      15,
		Status:     contextmodel.StatusSucceeded,
	})
	tracker.SetMessages([]*schema.Message{schema.UserMessage("hello"), schema.AssistantMessage("ok", nil)})

	conversation := tracker.FinishConversation(nil)
	if conversation.Prompt != 10 || conversation.Completion != 5 || conversation.Total != 15 {
		t.Fatalf("conversation usage = prompt:%d completion:%d total:%d", conversation.Prompt, conversation.Completion, conversation.Total)
	}
	if len(conversation.Turns) != 1 || conversation.Turns[0].Elapsed <= 0 {
		t.Fatalf("turns = %#v, want one elapsed turn", conversation.Turns)
	}
	if len(conversation.Messages) != 2 || conversation.Messages[0].Role != string(schema.User) {
		t.Fatalf("messages = %#v, want replay records", conversation.Messages)
	}
	if got := tracker.TotalTokens(); got != 15 {
		t.Fatalf("TotalTokens() = %d, want 15", got)
	}
	session := tracker.Session()
	if len(session.Conversations) != 1 || session.Total != 15 {
		t.Fatalf("session = %#v, want one aggregated conversation", session)
	}
}

func TestTrackerStoresLatestTotalSeparatelyFromAggregate(t *testing.T) {
	t.Parallel()

	tracker := New()
	tracker.BeginConversation()
	tracker.AddTurn(&contextmodel.Turn{Prompt: 10, Completion: 5, Total: 15})
	tracker.AddTurn(&contextmodel.Turn{Prompt: 20, Completion: 10, Total: 30})
	conversation := tracker.FinishConversation(nil)

	if conversation.Total != 45 {
		t.Fatalf("conversation.Total = %d, want aggregate 45", conversation.Total)
	}
	if got := tracker.TotalTokens(); got != 30 {
		t.Fatalf("TotalTokens() = %d, want latest provider total 30", got)
	}
}

func TestTrackerRecordsFailedTurn(t *testing.T) {
	t.Parallel()

	tracker := New()
	tracker.BeginConversation()
	wantErr := errors.New("model down")
	turn := contextmodel.NewTurn("", "", 0, time.Now())
	turn.Finish(time.Millisecond, 0, 0, 0, wantErr)
	tracker.AddTurn(turn)

	conversation := tracker.CurrentConversation()
	if len(conversation.Turns) != 1 {
		t.Fatalf("turns = %d, want one failed turn", len(conversation.Turns))
	}
	if conversation.Turns[0].Status != contextmodel.StatusFailed || conversation.Turns[0].Error != wantErr.Error() {
		t.Fatalf("failed turn = %#v", conversation.Turns[0])
	}
}

func TestWithTrackerRoundTrip(t *testing.T) {
	t.Parallel()

	tracker := New()
	ctx := WithTracker(context.Background(), tracker)
	if got := FromContext(ctx); got != tracker {
		t.Fatal("FromContext() did not return attached tracker")
	}
}
