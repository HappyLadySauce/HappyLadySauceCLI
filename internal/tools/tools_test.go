package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	execfiles "github.com/HappyLadySauce/HappyLadySauceCLI/internal/execution/files"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
)

func TestNewAgentToolsExposesBuiltinTools(t *testing.T) {
	t.Parallel()

	cfg, err := NewAgentTools(testWorkspaceGuard(t), testFileService())
	if err != nil {
		t.Fatalf("NewAgentTools() error = %v", err)
	}
	if len(cfg.Tools) != 6 {
		t.Fatalf("tool count = %d, want 6", len(cfg.Tools))
	}

	names := make(map[string]bool, len(cfg.Tools))
	for _, registeredTool := range cfg.Tools {
		info, err := registeredTool.Info(context.Background())
		if err != nil {
			t.Fatalf("tool info error: %v", err)
		}
		names[info.Name] = true
	}
	for _, want := range []string{"get_weather", "file_read", "file_list", "file_edit", "file_create", "file_delete"} {
		if !names[want] {
			t.Fatalf("registered tools = %#v, missing %s", names, want)
		}
	}
}

func TestNewOperationBuildersRegistersWeatherBuilder(t *testing.T) {
	t.Parallel()

	builders := NewOperationBuilders()
	builder := builders["get_weather"]
	if builder == nil {
		t.Fatal("expected get_weather operation builder")
	}
	operation, err := builder(context.Background(), securitycore.OperationRequest{
		ToolName:      "get_weather",
		OperationKind: securitycore.OperationNativeTool,
		Risk:          capability.RiskLow,
	}, securitycore.OperationBuildInput{Summary: `{city=北京}`})
	if err != nil {
		t.Fatalf("builder() error = %v", err)
	}
	if operation.OperationKind != "network.weather" {
		t.Fatalf("OperationKind = %q, want network.weather", operation.OperationKind)
	}
	if len(operation.Resources) != 1 || operation.Resources[0].Kind != "url" {
		t.Fatalf("Resources = %#v, want weather URL resource", operation.Resources)
	}
}

func TestOperationBuildersCoverAllRegisteredTools(t *testing.T) {
	t.Parallel()

	cfg, err := NewAgentTools(testWorkspaceGuard(t), testFileService())
	if err != nil {
		t.Fatalf("NewAgentTools() error = %v", err)
	}
	registry, err := NewCapabilityRegistry()
	if err != nil {
		t.Fatalf("NewCapabilityRegistry() error = %v", err)
	}
	builders := NewOperationBuilders()

	for _, registeredTool := range cfg.Tools {
		info, err := registeredTool.Info(context.Background())
		if err != nil {
			t.Fatalf("tool info error: %v", err)
		}
		if _, ok := registry.Get(info.Name); !ok {
			t.Fatalf("tool %q has no capability descriptor", info.Name)
		}
		if builders[info.Name] == nil {
			t.Fatalf("tool %q has no operation builder", info.Name)
		}
	}
}

func TestOperationBuildersHonorNetworkScopeDiscipline(t *testing.T) {
	t.Parallel()

	cfg, err := NewAgentTools(testWorkspaceGuard(t), testFileService())
	if err != nil {
		t.Fatalf("NewAgentTools() error = %v", err)
	}
	registry, err := NewCapabilityRegistry()
	if err != nil {
		t.Fatalf("NewCapabilityRegistry() error = %v", err)
	}
	builders := NewOperationBuilders()

	for _, registeredTool := range cfg.Tools {
		info, err := registeredTool.Info(context.Background())
		if err != nil {
			t.Fatalf("tool info error: %v", err)
		}
		desc, ok := registry.Get(info.Name)
		if !ok {
			t.Fatalf("tool %q has no capability descriptor", info.Name)
		}
		if !capability.HasNetworkScope(desc.Scopes) && !capability.HasHTTPResource(desc.Resources) {
			continue
		}
		builder := builders[info.Name]
		if builder == nil {
			t.Fatalf("tool %q has no operation builder", info.Name)
		}
		operation, err := builder(context.Background(), securitycore.OperationRequest{
			ToolName:   info.Name,
			Capability: desc,
			Registered: true,
			Risk:       desc.Risk,
		}, securitycore.OperationBuildInput{RawJSON: `{}`, Summary: securitycore.SummarizeArguments(`{}`)})
		if err != nil {
			t.Fatalf("builder() error = %v", err)
		}
		if !strings.HasPrefix(operation.OperationKind, "network.") {
			t.Fatalf("tool %q OperationKind = %q, want network.* prefix", info.Name, operation.OperationKind)
		}
		if len(operation.Resources) == 0 {
			t.Fatalf("tool %q builder returned no Resources for network capability", info.Name)
		}
	}
}

func TestNewCapabilityRegistryRegistersWeatherAsAllowedBuiltin(t *testing.T) {
	t.Parallel()

	registry, err := NewCapabilityRegistry()
	if err != nil {
		t.Fatalf("NewCapabilityRegistry() error = %v", err)
	}
	desc, ok := registry.Get("get_weather")
	if !ok {
		t.Fatal("expected get_weather capability")
	}
	if desc.Type != capability.TypeNativeTool || desc.Source != capability.SourceBuiltin {
		t.Fatalf("unexpected capability source: %#v", desc)
	}
	if desc.Risk != capability.RiskLow || desc.DefaultPolicy != capability.DefaultPolicyAllow {
		t.Fatalf("unexpected weather policy: %#v", desc)
	}
}

func TestFileToolDescriptorsUseFileScopesOnly(t *testing.T) {
	t.Parallel()

	registry, err := NewCapabilityRegistry()
	if err != nil {
		t.Fatalf("NewCapabilityRegistry() error = %v", err)
	}
	cases := map[string]struct {
		scope         string
		risk          capability.RiskLevel
		defaultPolicy capability.DefaultPolicy
	}{
		"file_read":   {scope: securitycore.ScopeFileRead, risk: capability.RiskLow, defaultPolicy: capability.DefaultPolicyAllow},
		"file_list":   {scope: securitycore.ScopeFileList, risk: capability.RiskLow, defaultPolicy: capability.DefaultPolicyAllow},
		"file_edit":   {scope: securitycore.ScopeFileWrite, risk: capability.RiskMedium, defaultPolicy: capability.DefaultPolicyReview},
		"file_create": {scope: securitycore.ScopeFileWrite, risk: capability.RiskMedium, defaultPolicy: capability.DefaultPolicyReview},
		"file_delete": {scope: securitycore.ScopeFileDelete, risk: capability.RiskHigh, defaultPolicy: capability.DefaultPolicyReview},
	}
	for name, want := range cases {
		desc, ok := registry.Get(name)
		if !ok {
			t.Fatalf("tool %q has no descriptor", name)
		}
		if capability.HasNetworkScope(desc.Scopes) || capability.HasHTTPResource(desc.Resources) {
			t.Fatalf("file tool %q unexpectedly declares network access: %#v", name, desc)
		}
		if !contains(desc.Scopes, want.scope) || desc.Risk != want.risk || desc.DefaultPolicy != want.defaultPolicy {
			t.Fatalf("descriptor for %q = %#v, want scope=%s risk=%s policy=%s", name, desc, want.scope, want.risk, want.defaultPolicy)
		}
	}
}

func TestFileOperationBuildersMapScopesAndResources(t *testing.T) {
	t.Parallel()

	registry, err := NewCapabilityRegistry()
	if err != nil {
		t.Fatalf("NewCapabilityRegistry() error = %v", err)
	}
	builders := NewOperationBuilders()
	target := filepath.Join(t.TempDir(), "target.txt")
	cases := []struct {
		name         string
		args         string
		operation    string
		resourceKind string
	}{
		{name: "file_read", args: `{"path":"` + slashPath(target) + `"}`, operation: securitycore.OperationFileRead, resourceKind: securitycore.ResourceKindFile},
		{name: "file_list", args: `{"path":"` + slashPath(filepath.Dir(target)) + `"}`, operation: securitycore.OperationFileList, resourceKind: securitycore.ResourceKindPath},
		{name: "file_edit", args: `{"path":"` + slashPath(target) + `","old_text":"secret old body","new_text":"secret new body"}`, operation: securitycore.OperationFileWrite, resourceKind: securitycore.ResourceKindFile},
		{name: "file_create", args: `{"path":"` + slashPath(target) + `","content":"secret created body"}`, operation: securitycore.OperationFileWrite, resourceKind: securitycore.ResourceKindFile},
		{name: "file_delete", args: `{"path":"` + slashPath(target) + `"}`, operation: securitycore.OperationFileDelete, resourceKind: securitycore.ResourceKindFile},
	}
	for _, tc := range cases {
		desc, ok := registry.Get(tc.name)
		if !ok {
			t.Fatalf("tool %q has no descriptor", tc.name)
		}
		builder := builders[tc.name]
		if builder == nil {
			t.Fatalf("tool %q has no builder", tc.name)
		}
		operation, err := builder(context.Background(), securitycore.OperationRequest{
			ToolName:   tc.name,
			Capability: desc,
			Registered: true,
			Risk:       desc.Risk,
		}, securitycore.OperationBuildInput{RawJSON: tc.args, Summary: securitycore.SummarizeArguments(tc.args)})
		if err != nil {
			t.Fatalf("%s builder() error = %v", tc.name, err)
		}
		if operation.OperationKind != tc.operation {
			t.Fatalf("%s OperationKind = %q, want %q", tc.name, operation.OperationKind, tc.operation)
		}
		if len(operation.Resources) != 1 || operation.Resources[0].Kind != tc.resourceKind {
			t.Fatalf("%s Resources = %#v, want kind %s", tc.name, operation.Resources, tc.resourceKind)
		}
		if strings.Contains(operation.SanitizedArgsSummary, "secret") || strings.Contains(operation.SanitizedArgsSummary, "body") {
			t.Fatalf("%s summary leaked raw content: %q", tc.name, operation.SanitizedArgsSummary)
		}
	}
}

func testWorkspaceGuard(t *testing.T) *securitycore.WorkspaceGuard {
	t.Helper()
	guard, err := securitycore.NewWorkspaceGuard([]string{t.TempDir()})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}
	return guard
}

func testFileService() *execfiles.Service {
	return execfiles.NewService(execfiles.Config{})
}

func slashPath(path string) string {
	return strings.ReplaceAll(path, `\`, `\\`)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
