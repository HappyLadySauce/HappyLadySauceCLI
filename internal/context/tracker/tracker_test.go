package tracker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/options"
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

func TestTrackerSanitizesPersistedMessagesByDefault(t *testing.T) {
	tracker := New()
	tracker.BeginConversation()
	tracker.SetMessages([]*schema.Message{
		schema.UserMessage("api_key=secret password=hunter2"),
	})

	conversation := tracker.FinishConversation(nil)
	if len(conversation.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(conversation.Messages))
	}
	message := conversation.Messages[0]
	for _, leaked := range []string{"secret", "hunter2"} {
		if strings.Contains(message.Content, leaked) || strings.Contains(message.RawJSON, leaked) {
			t.Fatalf("message leaked %q: %#v", leaked, message)
		}
	}
}

func TestTrackerMetadataOnlyPersistenceDropsMessageBodies(t *testing.T) {
	tracker := New()
	tracker.persistMode = options.PersistContentMetadataOnly
	tracker.BeginConversation()
	tracker.SetMessages([]*schema.Message{
		schema.UserMessage("sensitive prompt"),
		schema.ToolMessage("sensitive tool result", "call-1", schema.WithToolName("tool")),
	})

	conversation := tracker.FinishConversation(nil)
	if len(conversation.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(conversation.Messages))
	}
	for _, message := range conversation.Messages {
		if message.Content != "" || message.Reasoning != "" || message.RawJSON != "" {
			t.Fatalf("metadata-only message retained body: %#v", message)
		}
	}
	if conversation.Messages[1].ToolName != "tool" || conversation.Messages[1].ToolCallID != "call-1" {
		t.Fatalf("metadata-only message lost tool metadata: %#v", conversation.Messages[1])
	}
}
