package usage

import (
	"context"
	"testing"
)

func TestSessionContextUpdatePrefersTotalTokens(t *testing.T) {
	t.Parallel()

	session := NewSessionContext()
	session.UpdateFromSnapshot(UsageSnapshot{
		PromptTokens:     318,
		CompletionTokens: 40,
		TotalTokens:      358,
	})
	if got, want := session.TotalTokens(), 358; got != want {
		t.Fatalf("TotalTokens() = %d, want %d", got, want)
	}
}

func TestSessionContextUpdateFallsBackToPromptPlusCompletion(t *testing.T) {
	t.Parallel()

	session := NewSessionContext()
	session.UpdateFromSnapshot(UsageSnapshot{
		PromptTokens:     318,
		CompletionTokens: 40,
	})
	if got, want := session.TotalTokens(), 358; got != want {
		t.Fatalf("TotalTokens() = %d, want %d", got, want)
	}
}

func TestSessionContextRoundTrip(t *testing.T) {
	t.Parallel()

	session := NewSessionContext()
	ctx := WithSessionContext(context.Background(), session)
	if got := SessionFromContext(ctx); got != session {
		t.Fatalf("SessionFromContext() = %#v, want original session", got)
	}
	if got := SessionFromContext(context.Background()); got != nil {
		t.Fatalf("SessionFromContext(empty) = %#v, want nil", got)
	}
}

func TestSkipTrackingRoundTrip(t *testing.T) {
	t.Parallel()

	if skipTracking(context.Background()) {
		t.Fatal("skipTracking(default) = true, want false")
	}
	ctx := WithSkipTracking(context.Background())
	if !skipTracking(ctx) {
		t.Fatal("skipTracking(marked) = false, want true")
	}
}
