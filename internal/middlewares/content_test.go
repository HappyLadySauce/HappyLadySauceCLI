package middlewares

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	contextx "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context"
)

type fakeChatModel struct {
	response *schema.Message
	err      error
}

func (m *fakeChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *fakeChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{}), nil
}

func TestNewContentMiddlewareRequiresCompactor(t *testing.T) {
	if _, err := NewContentMiddleware(nil); err == nil {
		t.Fatal("NewContentMiddleware(nil) error = nil, want error")
	}
}

func TestBeforeModelRewriteStateReturnsNilAndEmptyState(t *testing.T) {
	middleware := newTestMiddleware(t, &fakeChatModel{response: schema.AssistantMessage("summary", nil)}, 1000, 100)

	_, got, err := middleware.BeforeModelRewriteState(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState(nil) error = %v", err)
	}
	if got != nil {
		t.Fatalf("BeforeModelRewriteState(nil) state = %#v, want nil", got)
	}

	state := &adk.ChatModelAgentState{}
	_, got, err = middleware.BeforeModelRewriteState(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState(empty) error = %v", err)
	}
	if got != state {
		t.Fatal("empty state should return original state")
	}
}

func TestBeforeModelRewriteStateNoopReturnsOriginalState(t *testing.T) {
	middleware := newTestMiddleware(t, &fakeChatModel{response: schema.AssistantMessage("summary", nil)}, 1000, 100)
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{schema.UserMessage("short")}}

	_, got, err := middleware.BeforeModelRewriteState(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState() error = %v", err)
	}
	if got != state {
		t.Fatal("no-op compaction should return original state")
	}
}

func TestBeforeModelRewriteStateCompactsWithCopiedState(t *testing.T) {
	middleware := newTestMiddleware(t, &fakeChatModel{response: schema.AssistantMessage("## Goal\nsummary", nil)}, 180, 20)
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			schema.SystemMessage("system"),
			schema.UserMessage("head user"),
			schema.AssistantMessage("head answer", nil),
			schema.UserMessage(strings.Repeat("middle ", 80)),
			schema.AssistantMessage(strings.Repeat("middle answer ", 80), nil),
			schema.UserMessage("follow up"),
			schema.UserMessage("latest"),
			schema.AssistantMessage("answer", nil),
		},
		ToolInfos: []*schema.ToolInfo{{Name: "lookup", Desc: "lookup data"}},
	}

	_, got, err := middleware.BeforeModelRewriteState(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState() error = %v", err)
	}
	if got == state {
		t.Fatal("compacted state should be copied")
	}
	foundSummary := false
	for _, msg := range got.Messages {
		if msg != nil && strings.Contains(msg.Content, "## Goal\nsummary") {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatalf("summary message not found: %#v", got.Messages)
	}
	if len(got.ToolInfos) != 1 || got.ToolInfos[0].Name != "lookup" {
		t.Fatalf("tool infos changed: %#v", got.ToolInfos)
	}
}

func TestBeforeModelRewriteStateSwallowsCompactionError(t *testing.T) {
	state := &adk.ChatModelAgentState{Messages: longConversation()}
	middleware := newTestMiddleware(t, &fakeChatModel{err: errors.New("model down")}, 160, 20)

	_, got, err := middleware.BeforeModelRewriteState(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState() error = %v, want nil", err)
	}
	if got != state {
		t.Fatal("failed compaction should return original state")
	}
}

func newTestMiddleware(t *testing.T, chatModel model.BaseChatModel, maxContext, maxOutput int) adk.ChatModelAgentMiddleware {
	t.Helper()
	compactor, err := contextx.NewCompactor(contextx.Config{
		Model:           chatModel,
		ModelName:       "unknown-local-model",
		MaxModelContext: maxContext,
		MaxOutputTokens: maxOutput,
	})
	if err != nil {
		t.Fatalf("NewCompactor() error = %v", err)
	}
	middleware, err := NewContentMiddleware(compactor)
	if err != nil {
		t.Fatalf("NewContentMiddleware() error = %v", err)
	}
	return middleware
}

func longConversation() []*schema.Message {
	return []*schema.Message{
		schema.UserMessage("head user"),
		schema.AssistantMessage("head answer", nil),
		schema.UserMessage(strings.Repeat("middle ", 80)),
		schema.AssistantMessage(strings.Repeat("middle answer ", 80), nil),
		schema.UserMessage("latest"),
		schema.AssistantMessage("answer", nil),
		schema.UserMessage("final"),
	}
}
