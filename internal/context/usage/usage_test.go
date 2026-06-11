package usage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestSessionConversationRecordsTurnsAndMessages(t *testing.T) {
	t.Parallel()

	session := NewSessionContext()
	recorder := session.BeginConversation()
	ctx := WithConversationRecorder(WithSessionContext(context.Background(), session), recorder)

	msg := schema.AssistantMessage("ok", nil)
	msg.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
	}}
	RecordModelUsage(ctx, 25*time.Millisecond, msg, nil)
	recorder.SetMessages([]*schema.Message{schema.UserMessage("hello"), msg})

	conversation, err := session.FinishConversation(ctx, recorder, nil)
	if err != nil {
		t.Fatalf("FinishConversation() error = %v", err)
	}
	if conversation.Prompt != 10 || conversation.Completion != 5 || conversation.Total != 15 {
		t.Fatalf("conversation usage = prompt:%d completion:%d total:%d", conversation.Prompt, conversation.Completion, conversation.Total)
	}
	if len(conversation.Turns) != 1 || conversation.Turns[0].Elapsed <= 0 {
		t.Fatalf("turns = %#v, want one elapsed turn", conversation.Turns)
	}
	if len(conversation.Messages) != 2 || conversation.Messages[0].Role != string(schema.User) {
		t.Fatalf("messages = %#v, want replay records", conversation.Messages)
	}
	if got := session.TotalTokens(); got != 15 {
		t.Fatalf("TotalTokens() = %d, want 15", got)
	}
}

func TestRecordModelUsageHonorsSkipTracking(t *testing.T) {
	t.Parallel()

	session := NewSessionContext()
	recorder := session.BeginConversation()
	ctx := WithSkipTracking(WithConversationRecorder(WithSessionContext(context.Background(), session), recorder))
	msg := schema.AssistantMessage("summary", nil)
	msg.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{TotalTokens: 100}}

	RecordModelUsage(ctx, time.Millisecond, msg, nil)
	conversation := recorder.Snapshot()
	if len(conversation.Turns) != 0 {
		t.Fatalf("turns = %d, want no tracking for skipped context", len(conversation.Turns))
	}
	if got := session.TotalTokens(); got != 0 {
		t.Fatalf("TotalTokens() = %d, want 0", got)
	}
}

func TestRecordModelUsageRecordsErrorsWithoutUsage(t *testing.T) {
	t.Parallel()

	session := NewSessionContext()
	recorder := session.BeginConversation()
	ctx := WithConversationRecorder(WithSessionContext(context.Background(), session), recorder)
	wantErr := errors.New("model down")

	RecordModelUsage(ctx, time.Millisecond, nil, wantErr)
	conversation := recorder.Snapshot()
	if len(conversation.Turns) != 1 {
		t.Fatalf("turns = %d, want one failed turn", len(conversation.Turns))
	}
	if conversation.Turns[0].Status != "failed" || conversation.Turns[0].Error != wantErr.Error() {
		t.Fatalf("failed turn = %#v", conversation.Turns[0])
	}
}
