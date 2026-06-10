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
	}}

	got, ok := snapshotFromMessage(msg)
	if !ok {
		t.Fatal("snapshotFromMessage() ok = false, want true")
	}
	if got.PromptTokens != 10 || got.CompletionTokens != 5 || got.TotalTokens != 15 {
		t.Fatalf("snapshot = %#v, want provider usage values", got)
	}
}

func TestSnapshotFromMessageMissingUsage(t *testing.T) {
	t.Parallel()

	if got, ok := snapshotFromMessage(schema.AssistantMessage("done", nil)); ok || !got.IsZero() {
		t.Fatalf("snapshotFromMessage() = %#v, %v, want zero false", got, ok)
	}
}
