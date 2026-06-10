package agents

import (
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/config"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/options"
)

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
