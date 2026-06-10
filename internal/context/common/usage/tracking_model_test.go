package usage

import (
	"context"
	"io"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeInnerModel struct {
	response *schema.Message
	stream   []*schema.Message
}

func (m *fakeInnerModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return m.response, nil
}

func (m *fakeInnerModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray(m.stream), nil
}

func TestTrackingChatModelGenerateUpdatesSessionAndTurnRecorder(t *testing.T) {
	t.Parallel()

	inner := &fakeInnerModel{response: &schema.Message{
		Role:    schema.Assistant,
		Content: "hi",
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{PromptTokens: 318, CompletionTokens: 40, TotalTokens: 358},
		},
	}}
	tracked := NewTrackingChatModel(inner)

	session := NewSessionContext()
	writer := &recordingTurnRecorder{}
	ctx := WithSessionContext(context.Background(), session)
	ctx = WithTurnRecorder(ctx, writer)

	_, err := tracked.Generate(ctx, []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got, want := session.TotalTokens(), 358; got != want {
		t.Fatalf("session TotalTokens = %d, want %d", got, want)
	}
	if len(writer.snapshots) != 1 || writer.snapshots[0].PromptTokens != 318 {
		t.Fatalf("turn recorder snapshots = %#v, want one prompt=318 snapshot", writer.snapshots)
	}
}

func TestTrackingChatModelGenerateSkipsWhenMarked(t *testing.T) {
	t.Parallel()

	inner := &fakeInnerModel{response: &schema.Message{
		Role:    schema.Assistant,
		Content: "summary",
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
		},
	}}
	tracked := NewTrackingChatModel(inner)

	session := NewSessionContext()
	writer := &recordingTurnRecorder{}
	ctx := WithSkipTracking(context.Background())
	ctx = WithSessionContext(ctx, session)
	ctx = WithTurnRecorder(ctx, writer)

	_, err := tracked.Generate(ctx, []*schema.Message{schema.UserMessage("compact")})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if session.TotalTokens() != 0 || len(writer.snapshots) != 0 {
		t.Fatalf("skip tracking should not record usage: session=%d snapshots=%d", session.TotalTokens(), len(writer.snapshots))
	}
}

func TestTrackingChatModelStreamForwardsChunksAndRecordsUsage(t *testing.T) {
	t.Parallel()

	inner := &fakeInnerModel{stream: []*schema.Message{
		{Role: schema.Assistant, Content: "hel"},
		{
			Role:    schema.Assistant,
			Content: "lo",
			ResponseMeta: &schema.ResponseMeta{
				Usage: &schema.TokenUsage{PromptTokens: 50, CompletionTokens: 5, TotalTokens: 55},
			},
		},
	}}
	tracked := NewTrackingChatModel(inner)

	session := NewSessionContext()
	writer := &recordingTurnRecorder{}
	ctx := WithSessionContext(context.Background(), session)
	ctx = WithTurnRecorder(ctx, writer)

	stream, err := tracked.Stream(ctx, []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	chunks := make([]string, 0, 2)
	for {
		chunk, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			t.Fatalf("Recv() error = %v", recvErr)
		}
		if chunk != nil {
			chunks = append(chunks, chunk.Content)
		}
	}
	if len(chunks) != 2 || chunks[0] != "hel" || chunks[1] != "lo" {
		t.Fatalf("stream chunks = %#v, want forwarded hel/lo", chunks)
	}
	if got, want := session.TotalTokens(), 55; got != want {
		t.Fatalf("session TotalTokens = %d, want %d", got, want)
	}
	if len(writer.snapshots) != 1 {
		t.Fatalf("turn recorder snapshots = %#v, want one merged usage snapshot", writer.snapshots)
	}
}

type recordingTurnRecorder struct {
	snapshots []UsageSnapshot
}

func (r *recordingTurnRecorder) AddUsage(snapshot UsageSnapshot) {
	r.snapshots = append(r.snapshots, snapshot)
}
