package execguard

import (
	"context"
	"path/filepath"
	"testing"

	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
)

func TestMatchAuthorizedURL(t *testing.T) {
	t.Parallel()

	operation := securitycore.OperationRequest{
		Resources: []securitycore.OperationResource{
			{Kind: "url", Value: "https://example.com/allowed"},
		},
	}
	if !MatchAuthorizedURL(operation, "https://Example.COM:443/allowed/") {
		t.Fatal("expected resolved url to match authorized resource")
	}
	if MatchAuthorizedURL(operation, "https://example.com/other") {
		t.Fatal("expected disallowed url to be rejected")
	}
}

func TestRequireAuthorizedPathMatchesContextOperation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	guard, err := securitycore.NewWorkspaceGuard([]string{root})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}
	target := filepath.Join(root, "allowed.txt")
	normalized, err := guard.NormalizePath(target)
	if err != nil {
		t.Fatalf("NormalizePath() error = %v", err)
	}
	ctx := securitycore.WithAuthorizedOperation(context.Background(), securitycore.OperationRequest{
		Resources: []securitycore.OperationResource{
			{Kind: securitycore.ResourceKindFile, Value: normalized},
		},
	})

	got, err := RequireAuthorizedPath(ctx, guard, target)
	if err != nil {
		t.Fatalf("RequireAuthorizedPath() error = %v", err)
	}
	if got != normalized {
		t.Fatalf("RequireAuthorizedPath() = %q, want %q", got, normalized)
	}
}

func TestRequireAuthorizedPathRejectsDifferentTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	guard, err := securitycore.NewWorkspaceGuard([]string{root})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}
	allowed, err := guard.NormalizePath(filepath.Join(root, "allowed.txt"))
	if err != nil {
		t.Fatalf("NormalizePath() error = %v", err)
	}
	ctx := securitycore.WithAuthorizedOperation(context.Background(), securitycore.OperationRequest{
		Resources: []securitycore.OperationResource{
			{Kind: securitycore.ResourceKindFile, Value: allowed},
		},
	})

	if _, err := RequireAuthorizedPath(ctx, guard, filepath.Join(root, "other.txt")); err == nil {
		t.Fatal("expected unauthorized path error")
	}
}
