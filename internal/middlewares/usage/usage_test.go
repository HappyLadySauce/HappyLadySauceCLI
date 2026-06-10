package usage

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestAttachKimiUsageFromRaw(t *testing.T) {
	t.Parallel()

	msg := schema.AssistantMessage("hello", nil)
	raw := []byte(`{"choices":[{"delta":{},"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14,"prompt_token_details":{"cached_tokens":2},"completion_token_details":{"reasoning_tokens":3}}}]}`)

	got := attachKimiUsageFromRaw(msg, raw)
	if got.ResponseMeta == nil || got.ResponseMeta.Usage == nil {
		t.Fatal("usage was not attached")
	}
	usage := got.ResponseMeta.Usage
	if usage.PromptTokens != 10 || usage.CompletionTokens != 4 || usage.TotalTokens != 14 ||
		usage.PromptTokenDetails.CachedTokens != 2 || usage.CompletionTokensDetails.ReasoningTokens != 3 {
		t.Fatalf("usage = %#v, want Kimi usage fields", usage)
	}
}

func TestAttachKimiUsageDoesNotOverrideStandardUsage(t *testing.T) {
	t.Parallel()

	msg := schema.AssistantMessage("hello", nil)
	msg.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 99}}
	raw := []byte(`{"choices":[{"usage":{"prompt_tokens":10}}]}`)

	got := attachKimiUsageFromRaw(msg, raw)
	if got.ResponseMeta.Usage.PromptTokens != 99 {
		t.Fatalf("PromptTokens = %d, want existing usage", got.ResponseMeta.Usage.PromptTokens)
	}
}

func TestAttachKimiUsageIgnoresMalformedJSON(t *testing.T) {
	t.Parallel()

	msg := schema.AssistantMessage("hello", nil)
	got := attachKimiUsageFromRaw(msg, []byte(`{`))
	if got.ResponseMeta != nil && got.ResponseMeta.Usage != nil {
		t.Fatalf("usage = %#v, want nil", got.ResponseMeta.Usage)
	}
}
