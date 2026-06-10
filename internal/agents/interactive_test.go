package agents

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestApplyProviderModelMetadataUsesProviderWhenNotConfigured(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("request path = %s, want /models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"unknown-local-model","context_length":4096}]}`))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Model.BaseURL = server.URL
	cfg.Model.MaxModelContext = 128000
	cfg.Model.MaxModelContextConfigured = false

	applyProviderModelMetadata(context.Background(), cfg)

	if got, want := cfg.Model.MaxModelContext, 4096; got != want {
		t.Fatalf("MaxModelContext = %d, want provider value %d", got, want)
	}
}

func TestApplyProviderModelMetadataKeepsConfiguredContext(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"unknown-local-model","context_length":4096}]}`))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Model.BaseURL = server.URL
	cfg.Model.MaxModelContext = 777
	cfg.Model.MaxModelContextConfigured = true

	applyProviderModelMetadata(context.Background(), cfg)

	if got, want := cfg.Model.MaxModelContext, 777; got != want {
		t.Fatalf("MaxModelContext = %d, want configured value %d", got, want)
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
