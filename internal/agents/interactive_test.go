package agents

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/config"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/options"
)

type fakeAgentChatModel struct{}

func (m *fakeAgentChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage("summary", nil), nil
}

func (m *fakeAgentChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{}), nil
}

func TestNewChatModelConfigSetsMaxCompletionTokens(t *testing.T) {
	cfg := testConfig()
	modelConfig := newChatModelConfig(cfg)
	if modelConfig.MaxCompletionTokens == nil {
		t.Fatal("MaxCompletionTokens is nil")
	}
	if got, want := *modelConfig.MaxCompletionTokens, cfg.Model.MaxOutputTokens; got != want {
		t.Fatalf("MaxCompletionTokens = %d, want %d", got, want)
	}
}

func TestNewAgentHandlersRegistersContentMiddleware(t *testing.T) {
	handlers, err := newAgentHandlers(&fakeAgentChatModel{}, testConfig())
	if err != nil {
		t.Fatalf("newAgentHandlers() error = %v", err)
	}
	if len(handlers) != 1 || handlers[0] == nil {
		t.Fatalf("handlers = %#v, want one handler", handlers)
	}
}

func testConfig() *config.Config {
	return &config.Config{
		Model: &options.ModelOptions{
			APIKey:          "test-key",
			BaseURL:         "http://localhost:11434",
			Model:           "unknown-local-model",
			MaxOutputTokens: 20,
			MaxModelContext: 180,
		},
	}
}
