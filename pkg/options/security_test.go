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
	if opts.CommandTimeoutSeconds != 30 || opts.FileOperationTimeoutSeconds != 30 || opts.MaxToolOutputBytes != 1<<20 {
		t.Fatalf("timeout/output defaults = %d/%d/%d", opts.CommandTimeoutSeconds, opts.FileOperationTimeoutSeconds, opts.MaxToolOutputBytes)
	}
	if opts.FileMaxBytes != 16<<20 || opts.FileMaxLineBytes != 64<<10 {
		t.Fatalf("file limit defaults = %d/%d", opts.FileMaxBytes, opts.FileMaxLineBytes)
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

func TestSecurityOptionsValidateRejectsInvalidFileLimits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*SecurityOptions)
	}{
		{name: "timeout", mutate: func(opts *SecurityOptions) { opts.FileOperationTimeoutSeconds = -1 }},
		{name: "max bytes", mutate: func(opts *SecurityOptions) { opts.FileMaxBytes = -1 }},
		{name: "max line bytes", mutate: func(opts *SecurityOptions) { opts.FileMaxLineBytes = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := NewSecurityOptions()
			tc.mutate(opts)
			if err := opts.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want invalid file limit")
			}
		})
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

func TestSecurityOptionsValidateRejectsInvalidCommandSandboxDistribution(t *testing.T) {
	t.Parallel()

	opts := NewSecurityOptions()
	opts.CommandSandbox.WSLDistribution = "Ubuntu;rm"
	if err := opts.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid command sandbox distribution")
	}
}
