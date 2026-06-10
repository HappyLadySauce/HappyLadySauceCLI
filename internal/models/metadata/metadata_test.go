package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolverParsesKimiContextLength(t *testing.T) {
	t.Parallel()

	server := newMetadataServer(t, `{"data":[{"id":"kimi-k2","owned_by":"moonshot","context_length":131072,"max_output_tokens":8192,"supports_reasoning":true,"supports_image_in":true}]}`)
	resolver := newTestResolver(t, server.URL)

	got, err := resolver.Resolve(context.Background(), "kimi-k2")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.MaxContextTokens != 131072 || got.MaxOutputTokens != 8192 {
		t.Fatalf("metadata tokens = context %d output %d, want 131072/8192", got.MaxContextTokens, got.MaxOutputTokens)
	}
	if !got.SupportsReasoning || !got.SupportsImageIn || got.SupportsVideoIn {
		t.Fatalf("metadata capabilities = %#v, want reasoning and image only", got)
	}
}

func TestResolverKeepsDeepSeekMissingContextEmpty(t *testing.T) {
	t.Parallel()

	server := newMetadataServer(t, `{"data":[{"id":"deepseek-chat","object":"model","owned_by":"deepseek"}]}`)
	resolver := newTestResolver(t, server.URL)

	got, err := resolver.Resolve(context.Background(), "deepseek-chat")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.MaxContextTokens != 0 {
		t.Fatalf("MaxContextTokens = %d, want 0", got.MaxContextTokens)
	}
	if got.OwnedBy != "deepseek" {
		t.Fatalf("OwnedBy = %q, want deepseek", got.OwnedBy)
	}
}

func TestResolverParsesLMStudioContextAliases(t *testing.T) {
	t.Parallel()

	server := newMetadataServer(t, `{"data":[{"id":"local-model","max_model_len":"32768"}]}`)
	resolver := newTestResolver(t, server.URL)

	got, err := resolver.Resolve(context.Background(), "local-model")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.MaxContextTokens != 32768 {
		t.Fatalf("MaxContextTokens = %d, want 32768", got.MaxContextTokens)
	}
}

func TestResolverReturnsNotFound(t *testing.T) {
	t.Parallel()

	server := newMetadataServer(t, `{"data":[{"id":"other"}]}`)
	resolver := newTestResolver(t, server.URL)

	if _, err := resolver.Resolve(context.Background(), "missing"); err == nil {
		t.Fatal("Resolve() error = nil, want not found")
	}
}

func newMetadataServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("request path = %s, want /models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func newTestResolver(t *testing.T, baseURL string) *Resolver {
	t.Helper()
	resolver, err := NewResolver(ResolverConfig{BaseURL: baseURL})
	if err != nil {
		t.Fatalf("NewResolver() error = %v", err)
	}
	return resolver
}
