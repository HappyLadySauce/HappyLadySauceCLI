package security

import (
	"strings"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
)

func TestOperationGrantKeySortsResources(t *testing.T) {
	t.Parallel()

	base := OperationRequest{
		Capability: capability.Descriptor{
			Name:   "read_file",
			Type:   capability.TypeNativeTool,
			Source: capability.SourceBuiltin,
		},
		OperationKind: OperationFileRead,
		Risk:          capability.RiskHigh,
		Resources: []OperationResource{
			{Kind: "path", Value: "C:/workspace/b.txt"},
			{Kind: "path", Value: "C:/workspace/a.txt"},
		},
	}
	other := base
	other.Resources = []OperationResource{
		{Kind: "path", Value: "C:/workspace/a.txt"},
		{Kind: "path", Value: "C:/workspace/b.txt"},
	}

	if base.GrantKey() != other.GrantKey() {
		t.Fatalf("GrantKey() differs for reordered resources: %q vs %q", base.GrantKey(), other.GrantKey())
	}
	if !strings.Contains(base.ResourceSummary(), "a.txt") || strings.Index(base.ResourceSummary(), "a.txt") > strings.Index(base.ResourceSummary(), "b.txt") {
		t.Fatalf("ResourceSummary() is not sorted: %q", base.ResourceSummary())
	}
}
