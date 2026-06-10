package usage

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestSnapshotFromMessageExtractsProviderUsage(t *testing.T) {
	t.Parallel()

	msg := schema.AssistantMessage("done", nil)
	msg.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
		PromptTokenDetails: schema.PromptTokenDetails{
			CachedTokens: 3,
		},
		CompletionTokensDetails: schema.CompletionTokensDetails{
			ReasoningTokens: 2,
		},
	}}

	got, ok := SnapshotFromMessage(msg)
	if !ok {
		t.Fatal("SnapshotFromMessage() ok = false, want true")
	}
	if got.PromptTokens != 10 || got.CompletionTokens != 5 || got.TotalTokens != 15 ||
		got.CachedTokens != 3 || got.ReasoningTokens != 2 || got.Source != UsageSourceProvider {
		t.Fatalf("snapshot = %#v, want provider usage values", got)
	}
}

func TestSnapshotFromMessageMissingUsage(t *testing.T) {
	t.Parallel()

	if got, ok := SnapshotFromMessage(schema.AssistantMessage("done", nil)); ok || !got.IsZero() {
		t.Fatalf("SnapshotFromMessage() = %#v, %v, want zero false", got, ok)
	}
}
