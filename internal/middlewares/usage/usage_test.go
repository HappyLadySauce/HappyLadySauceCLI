package usage

import (
	"context"
	"io"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	contextusage "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/usage"
)

type fakeUsageModel struct {
	response *schema.Message
	stream   *schema.StreamReader[*schema.Message]
}

func (m *fakeUsageModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	return m.response, nil
}

func (m *fakeUsageModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.stream, nil
}

func TestWrapModelGenerateRecordsUsage(t *testing.T) {
	t.Parallel()

	middleware := NewUsageMiddleware()
	response := usageMessage(11, 7, 18)
	wrapped, err := middleware.WrapModel(context.Background(), &fakeUsageModel{response: response}, nil)
	if err != nil {
		t.Fatalf("WrapModel() error = %v", err)
	}

	session := contextusage.NewSessionContext()
	recorder := session.BeginConversation()
	ctx := contextusage.WithConversationRecorder(contextusage.WithSessionContext(context.Background(), session), recorder)

	if _, err := wrapped.Generate(ctx, []*schema.Message{schema.UserMessage("hello")}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	conversation := recorder.Snapshot()
	if len(conversation.Turns) != 1 || conversation.Total != 18 {
		t.Fatalf("conversation = %#v, want one recorded turn", conversation)
	}
}

func TestWrapModelStreamRecordsUsageOnEOF(t *testing.T) {
	t.Parallel()

	chunks := []*schema.Message{
		{Role: schema.Assistant, Content: "hel"},
		usageMessage(11, 7, 18),
	}
	middleware := NewUsageMiddleware()
	wrapped, err := middleware.WrapModel(context.Background(), &fakeUsageModel{stream: schema.StreamReaderFromArray(chunks)}, nil)
	if err != nil {
		t.Fatalf("WrapModel() error = %v", err)
	}

	session := contextusage.NewSessionContext()
	recorder := session.BeginConversation()
	ctx := contextusage.WithConversationRecorder(contextusage.WithSessionContext(context.Background(), session), recorder)

	stream, err := wrapped.Stream(ctx, []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	for {
		_, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			t.Fatalf("Recv() error = %v", recvErr)
		}
	}
	stream.Close()

	conversation := recorder.Snapshot()
	if len(conversation.Turns) != 1 || conversation.Total != 18 {
		t.Fatalf("conversation = %#v, want one recorded turn", conversation)
	}
}

func usageMessage(prompt, completion, total int) *schema.Message {
	msg := schema.AssistantMessage("lo", nil)
	msg.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
	}}
	return msg
}
