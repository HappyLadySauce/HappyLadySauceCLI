package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceGuardRejectsPathTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	guard, err := NewWorkspaceGuard([]string{root})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}

	if _, err := guard.NormalizePath(filepath.Join(root, "..", "outside.txt")); err == nil {
		t.Fatal("expected path traversal rejection")
	}
}

func TestWorkspaceGuardRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	guard, err := NewWorkspaceGuard([]string{root})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}
	if _, err := guard.NormalizePath(link); err == nil {
		t.Fatal("expected symlink escape rejection")
	}
}
