package options

import (
	"path/filepath"
	"testing"
)

func TestSecurityOptionsValidateAppliesDefaults(t *testing.T) {
	t.Parallel()

	opts := &SecurityOptions{}
	if err := opts.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(opts.WorkspaceRoots) != 1 || !filepath.IsAbs(opts.WorkspaceRoots[0]) {
		t.Fatalf("WorkspaceRoots = %#v, want one absolute default root", opts.WorkspaceRoots)
	}
	if opts.ApprovalDefault != ApprovalDefaultReview {
		t.Fatalf("ApprovalDefault = %q, want review", opts.ApprovalDefault)
	}
	if opts.PersistContent != PersistContentSanitized {
		t.Fatalf("PersistContent = %q, want sanitized", opts.PersistContent)
	}
	if opts.CommandTimeoutSeconds != 30 || opts.MaxToolOutputBytes != 1<<20 {
		t.Fatalf("timeout/output defaults = %d/%d", opts.CommandTimeoutSeconds, opts.MaxToolOutputBytes)
	}
}

func TestSecurityOptionsValidateRejectsInvalidPersistenceMode(t *testing.T) {
	t.Parallel()

	opts := NewSecurityOptions()
	opts.PersistContent = "raw"
	if err := opts.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid persistence mode")
	}
}
