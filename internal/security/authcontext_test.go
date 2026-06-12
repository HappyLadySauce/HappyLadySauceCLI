package security

import (
	"context"
	"testing"
)

func TestAuthorizedOperationFromContextMissing(t *testing.T) {
	t.Parallel()

	if _, ok := AuthorizedOperationFromContext(context.Background()); ok {
		t.Fatal("expected missing authorized operation")
	}
}

func TestWithAuthorizedOperationRoundTrip(t *testing.T) {
	t.Parallel()

	want := OperationRequest{ToolName: "get_weather", OperationKind: "network.weather"}
	ctx := WithAuthorizedOperation(context.Background(), want)
	got, ok := AuthorizedOperationFromContext(ctx)
	if !ok {
		t.Fatal("expected authorized operation")
	}
	if got.ToolName != want.ToolName || got.OperationKind != want.OperationKind {
		t.Fatalf("got = %#v, want %#v", got, want)
	}
}
