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
	if opts.PersistContent != PersistContentSanitized {
		t.Fatalf("PersistContent = %q, want sanitized", opts.PersistContent)
	}
	if opts.CommandTimeoutSeconds != 30 || opts.MaxToolOutputBytes != 1<<20 {
		t.Fatalf("timeout/output defaults = %d/%d", opts.CommandTimeoutSeconds, opts.MaxToolOutputBytes)
	}
	if opts.CommandSandbox.Backend != CommandSandboxBackendWSL2 || !opts.CommandSandbox.FailClosed || opts.CommandSandbox.Network != CommandSandboxNetworkDeny {
		t.Fatalf("CommandSandbox defaults = %#v, want wsl2/fail-closed/deny", opts.CommandSandbox)
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

func TestSecurityOptionsValidateRejectsInvalidCommandSandboxBackend(t *testing.T) {
	t.Parallel()

	opts := NewSecurityOptions()
	opts.CommandSandbox.Backend = "native"
	if err := opts.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid command sandbox backend")
	}
}

func TestSecurityOptionsValidateRejectsInvalidCommandSandboxNetworkPolicy(t *testing.T) {
	t.Parallel()

	opts := NewSecurityOptions()
	opts.CommandSandbox.Network = "allow"
	if err := opts.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid command sandbox network policy")
	}
}

func TestSecurityOptionsValidateRejectsInvalidCommandSandboxEnvKey(t *testing.T) {
	t.Parallel()

	opts := NewSecurityOptions()
	opts.CommandSandbox.AllowedEnvKeys = []string{"PATH", "API-KEY"}
	if err := opts.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid command sandbox env key")
	}
}
