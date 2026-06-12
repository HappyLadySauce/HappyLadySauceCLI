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

func TestOperationGrantKeyIncludesArgsForNetworkOperations(t *testing.T) {
	t.Parallel()

	base := OperationRequest{
		Capability: capability.Descriptor{
			Name:   "get_weather",
			Type:   capability.TypeNativeTool,
			Source: capability.SourceBuiltin,
		},
		OperationKind:        "network.weather",
		Risk:                 capability.RiskLow,
		Resources:            []OperationResource{{Kind: "url", Value: "https://uapis.cn/api/v1/misc/weather"}},
		SanitizedArgsSummary: "{city=北京}",
	}
	other := base
	other.SanitizedArgsSummary = "{city=上海}"

	if base.GrantKey() == other.GrantKey() {
		t.Fatalf("GrantKey() should include args hash for network operations: %q", base.GrantKey())
	}
	if !strings.Contains(base.GrantKey(), "args_sha=") {
		t.Fatalf("GrantKey() missing args hash: %q", base.GrantKey())
	}
}

func TestOperationSessionGrantKeyOmitsArgsForNetworkOperations(t *testing.T) {
	t.Parallel()

	base := OperationRequest{
		Capability: capability.Descriptor{
			Name:   "get_weather",
			Type:   capability.TypeNativeTool,
			Source: capability.SourceBuiltin,
		},
		OperationKind:        "network.weather",
		Risk:                 capability.RiskLow,
		Resources:            []OperationResource{{Kind: "url", Value: "https://uapis.cn/api/v1/misc/weather"}},
		SanitizedArgsSummary: "{city=北京,lang=zh}",
	}
	other := base
	other.SanitizedArgsSummary = "{city=重庆,lang=ja}"

	if base.SessionGrantKey() != other.SessionGrantKey() {
		t.Fatalf("SessionGrantKey() = %q, other = %q; want same network session key", base.SessionGrantKey(), other.SessionGrantKey())
	}
	if strings.Contains(base.SessionGrantKey(), "args_sha=") {
		t.Fatalf("SessionGrantKey() should omit args hash for network operations: %q", base.SessionGrantKey())
	}
}
