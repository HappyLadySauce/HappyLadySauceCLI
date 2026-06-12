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

func TestOperationGrantKeyEscapesSeparators(t *testing.T) {
	t.Parallel()

	left := OperationRequest{
		Capability: capability.Descriptor{
			Name:   "tool|kind",
			Type:   capability.TypeNativeTool,
			Source: capability.SourceBuiltin,
		},
		OperationKind: "read",
		Risk:          capability.RiskHigh,
		Resources:     []OperationResource{{Kind: "path", Value: "C:/workspace/a=b.txt"}},
	}
	right := left
	right.Capability.Name = "tool"
	right.OperationKind = "kind|read"

	if left.GrantKey() == right.GrantKey() {
		t.Fatalf("GrantKey() collision for escaped separators: %q", left.GrantKey())
	}
	if !strings.Contains(left.GrantKey(), `tool\|kind`) || !strings.Contains(left.GrantKey(), `a\=b.txt`) {
		t.Fatalf("GrantKey() did not escape separators: %q", left.GrantKey())
	}
}
