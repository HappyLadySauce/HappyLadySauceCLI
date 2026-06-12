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
