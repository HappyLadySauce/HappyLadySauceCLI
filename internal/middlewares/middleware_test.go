package middlewares

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools"
)

type fakeChatModel struct{}

func (m *fakeChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage("summary", nil), nil
}

func (m *fakeChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{}), nil
}

func TestNewChatModelAgentMiddlewaresRegistersDefaultChain(t *testing.T) {
	t.Parallel()

	handlers, err := NewChatModelAgentMiddlewares(ChatModelAgentMiddlewareConfig{
		Model:              &fakeChatModel{},
		ModelName:          "unknown-local-model",
		MaxModelContext:    180,
		MaxOutputTokens:    20,
		CapabilityRegistry: tools.NewCapabilityRegistry(),
	})
	if err != nil {
		t.Fatalf("NewChatModelAgentMiddlewares() error = %v", err)
	}
	if len(handlers) != 3 || handlers[0] == nil || handlers[1] == nil || handlers[2] == nil {
		t.Fatalf("handlers = %#v, want security, compact, and usage handlers", handlers)
	}
}
