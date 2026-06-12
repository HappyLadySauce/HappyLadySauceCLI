package middlewares

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/options"
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

	capRegistry, err := tools.NewCapabilityRegistry()
	if err != nil {
		t.Fatalf("NewCapabilityRegistry() error = %v", err)
	}

	handlers, err := NewChatModelAgentMiddlewares(ChatModelAgentMiddlewareConfig{
		Model:              &fakeChatModel{},
		ModelName:          "unknown-local-model",
		MaxModelContext:    180,
		MaxOutputTokens:    20,
		CapabilityRegistry: capRegistry,
	})
	if err != nil {
		t.Fatalf("NewChatModelAgentMiddlewares() error = %v", err)
	}
	if len(handlers) != 3 || handlers[0] == nil || handlers[1] == nil || handlers[2] == nil {
		t.Fatalf("handlers = %#v, want security, compact, and usage handlers", handlers)
	}
}

func TestNewChatModelAgentMiddlewaresRejectsInvalidWorkspaceRoot(t *testing.T) {
	t.Parallel()

	capRegistry, err := tools.NewCapabilityRegistry()
	if err != nil {
		t.Fatalf("NewCapabilityRegistry() error = %v", err)
	}

	_, err = NewChatModelAgentMiddlewares(ChatModelAgentMiddlewareConfig{
		Model:              &fakeChatModel{},
		ModelName:          "unknown-local-model",
		MaxModelContext:    180,
		MaxOutputTokens:    20,
		CapabilityRegistry: capRegistry,
		Security: &options.SecurityOptions{
			WorkspaceRoots:        []string{""},
			CommandTimeoutSeconds: 30,
			MaxToolOutputBytes:    1024,
		},
	})
	if err == nil {
		t.Fatal("expected invalid workspace root error")
	}
}
