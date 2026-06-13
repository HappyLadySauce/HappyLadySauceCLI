package security

import (
	"strings"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
)

func TestValidateNetworkResourcesRejectsURLWithoutAllowlist(t *testing.T) {
	t.Parallel()

	err := ValidateNetworkResources(OperationRequest{
		Capability: capability.Descriptor{
			Name: "fetch",
		},
		Resources: []OperationResource{{Kind: ResourceKindURL, Value: "https://example.com/api"}},
	})
	if err == nil {
		t.Fatal("expected missing allowlist error")
	}
	if !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("error = %v, want allowlist context", err)
	}
}

func TestValidateNetworkResourcesAllowsDescriptorURL(t *testing.T) {
	t.Parallel()

	err := ValidateNetworkResources(OperationRequest{
		OperationKind: "network.fetch",
		Capability: capability.Descriptor{
			Name:      "fetch",
			Resources: []string{"https://example.com/api"},
		},
		Resources: []OperationResource{{Kind: ResourceKindURL, Value: "https://example.com/api"}},
	})
	if err != nil {
		t.Fatalf("ValidateNetworkResources() error = %v", err)
	}
}

func TestValidateFileResourcesRequiresMatchingScope(t *testing.T) {
	t.Parallel()

	err := ValidateFileResources(OperationRequest{
		OperationKind: OperationFileRead,
		Capability: capability.Descriptor{
			Name:   "read_file",
			Scopes: []string{ScopeFileWrite},
		},
		Resources: []OperationResource{{Kind: ResourceKindPath, Value: "/workspace/a.txt"}},
	})
	if err == nil {
		t.Fatal("expected scope mismatch error")
	}
	if !strings.Contains(err.Error(), ScopeFileRead) {
		t.Fatalf("error = %v, want required scope context", err)
	}
}

func TestValidateFileResourcesRequiresPathResource(t *testing.T) {
	t.Parallel()

	err := ValidateFileResources(OperationRequest{
		OperationKind: OperationFileList,
		Capability: capability.Descriptor{
			Name:   "list_files",
			Scopes: []string{ScopeFileList},
		},
	})
	if err == nil {
		t.Fatal("expected missing file resource error")
	}
	if !strings.Contains(err.Error(), "path or file resource") {
		t.Fatalf("error = %v, want path/file resource context", err)
	}
}

func TestValidateFileResourcesRejectsFileScopeWithNativeOperation(t *testing.T) {
	t.Parallel()

	err := ValidateFileResources(OperationRequest{
		OperationKind: OperationNativeTool,
		Capability: capability.Descriptor{
			Name:   "misconfigured",
			Scopes: []string{ScopeFileRead},
		},
	})
	if err == nil {
		t.Fatal("expected file scope mismatch error")
	}
	if !strings.Contains(err.Error(), "file operation kind") {
		t.Fatalf("error = %v, want operation kind context", err)
	}
}

func TestValidateFileResourcesAllowsMatchingFileOperation(t *testing.T) {
	t.Parallel()

	err := ValidateFileResources(OperationRequest{
		OperationKind: OperationFileRead,
		Capability: capability.Descriptor{
			Name:   "read_file",
			Scopes: []string{ScopeFileRead},
		},
		Resources: []OperationResource{{Kind: ResourceKindFile, Value: "/workspace/a.txt"}},
	})
	if err != nil {
		t.Fatalf("ValidateFileResources() error = %v", err)
	}
}
