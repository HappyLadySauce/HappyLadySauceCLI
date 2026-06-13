package files

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	execfiles "github.com/HappyLadySauce/HappyLadySauceCLI/internal/execution/files"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
)

func TestNewToolsReturnsFilesystemTools(t *testing.T) {
	t.Parallel()

	tools, err := NewTools(testGuard(t), execfiles.NewService(execfiles.Config{}))
	if err != nil {
		t.Fatalf("NewTools() error = %v", err)
	}
	if len(tools) != 5 {
		t.Fatalf("tool count = %d, want 5", len(tools))
	}
	names := make(map[string]bool, len(tools))
	for _, registeredTool := range tools {
		info, err := registeredTool.Info(context.Background())
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}
		names[info.Name] = true
	}
	for _, want := range []string{toolFileRead, toolFileList, toolFileEdit, toolFileCreate, toolFileDelete} {
		if !names[want] {
			t.Fatalf("tools = %#v, missing %s", names, want)
		}
	}
}

func TestFileReadEndpointRequiresAuthorizedPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	guard, err := securitycore.NewWorkspaceGuard([]string{root})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}
	allowed := filepath.Join(root, "allowed.txt")
	other := filepath.Join(root, "other.txt")
	if err := os.WriteFile(allowed, []byte("allowed\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(allowed) error = %v", err)
	}
	if err := os.WriteFile(other, []byte("other\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(other) error = %v", err)
	}
	normalizedAllowed, err := guard.NormalizePath(allowed)
	if err != nil {
		t.Fatalf("NormalizePath() error = %v", err)
	}
	ctx := securitycore.WithAuthorizedOperation(context.Background(), securitycore.OperationRequest{
		ToolName:      toolFileRead,
		OperationKind: securitycore.OperationFileRead,
		Capability: capability.Descriptor{
			Name:          toolFileRead,
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskLow,
			DefaultPolicy: capability.DefaultPolicyAllow,
			Scopes:        []string{securitycore.ScopeFileRead},
		},
		Resources: []securitycore.OperationResource{{Kind: securitycore.ResourceKindFile, Value: normalizedAllowed}},
	})
	set := &toolSet{guard: guard, service: newTestService()}

	if _, err := set.read(ctx, &FileReadParams{Path: other}); err == nil {
		t.Fatal("read() error = nil, want authorized path mismatch")
	}
	result, err := set.read(ctx, &FileReadParams{Path: allowed})
	if err != nil {
		t.Fatalf("read() authorized path error = %v", err)
	}
	if strings.TrimSpace(result.Content) != "allowed" {
		t.Fatalf("read content = %q", result.Content)
	}
}

func TestFileEditEndpointDoesNotRunWithoutAuthorizedOperation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	set := &toolSet{guard: testGuardForRoot(t, root), service: newTestService()}

	if _, err := set.edit(context.Background(), &FileEditParams{Path: target, OldText: "before", NewText: "after"}); err == nil {
		t.Fatal("edit() error = nil, want missing authorized operation")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "before" {
		t.Fatalf("file changed without authorization: %q", data)
	}
}

func TestOperationBuildersDoNotLeakWriteContent(t *testing.T) {
	t.Parallel()

	builders := OperationBuilders()
	cases := map[string]string{
		toolFileEdit:   `{"path":"notes.txt","old_text":"top secret old text","new_text":"top secret new text"}`,
		toolFileCreate: `{"path":"notes.txt","content":"top secret created text"}`,
	}
	for name, args := range cases {
		builder := builders[name]
		if builder == nil {
			t.Fatalf("builder %s is nil", name)
		}
		operation, err := builder(context.Background(), securitycore.OperationRequest{
			ToolName: name,
			Capability: capability.Descriptor{
				Name:          name,
				Type:          capability.TypeNativeTool,
				Source:        capability.SourceBuiltin,
				Risk:          capability.RiskMedium,
				DefaultPolicy: capability.DefaultPolicyReview,
				Scopes:        []string{securitycore.ScopeFileWrite},
			},
			Registered: true,
			Risk:       capability.RiskMedium,
		}, securitycore.OperationBuildInput{RawJSON: args})
		if err != nil {
			t.Fatalf("%s builder() error = %v", name, err)
		}
		if strings.Contains(operation.SanitizedArgsSummary, "top secret") || strings.Contains(operation.SanitizedArgsSummary, "created text") {
			t.Fatalf("%s leaked content in summary: %q", name, operation.SanitizedArgsSummary)
		}
		if !strings.Contains(operation.SanitizedArgsSummary, "sha256") {
			t.Fatalf("%s summary = %q, want content hash metadata", name, operation.SanitizedArgsSummary)
		}
	}
}

func TestOperationBuilderRejectsInvalidArguments(t *testing.T) {
	t.Parallel()

	builder := OperationBuilders()[toolFileRead]
	if builder == nil {
		t.Fatal("file_read builder is nil")
	}
	if _, err := builder(context.Background(), securitycore.OperationRequest{}, securitycore.OperationBuildInput{RawJSON: `{`}); err == nil {
		t.Fatal("builder() error = nil, want malformed JSON error")
	}
	if _, err := builder(context.Background(), securitycore.OperationRequest{}, securitycore.OperationBuildInput{RawJSON: `{"path":"  "}`}); err == nil {
		t.Fatal("builder() error = nil, want empty path error")
	}
}

func testGuard(t *testing.T) *securitycore.WorkspaceGuard {
	t.Helper()
	return testGuardForRoot(t, t.TempDir())
}

func testGuardForRoot(t *testing.T, root string) *securitycore.WorkspaceGuard {
	t.Helper()
	guard, err := securitycore.NewWorkspaceGuard([]string{root})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}
	return guard
}

func newTestService() *execfiles.Service {
	return execfiles.NewService(execfiles.Config{})
}
