package security

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	execfiles "github.com/HappyLadySauce/HappyLadySauceCLI/internal/execution/files"
	commandsandbox "github.com/HappyLadySauce/HappyLadySauceCLI/internal/execution/sandbox"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/security/policy"
	filetools "github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools/files"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools/toolresult"
)

type fakeApprover struct {
	approve bool
	scope   string
	calls   atomic.Int32
	last    ApprovalRequest
}

type fakeSandboxRunner struct {
	status commandsandbox.Status
}

func (r fakeSandboxRunner) Probe(ctx context.Context) commandsandbox.Status {
	return r.status
}

func (r fakeSandboxRunner) Run(ctx context.Context, request commandsandbox.Request) (commandsandbox.Result, error) {
	return commandsandbox.Result{}, nil
}

func TestWrapEnhancedToolCallsAuthorizeBeforeExecution(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "danger",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskHigh,
		DefaultPolicy: capability.DefaultPolicyDeny,
	})
	middleware := newTestMiddleware(t, registry, nil)

	var invokableCalled atomic.Bool
	wrappedInvokable, err := middleware.WrapEnhancedInvokableToolCall(context.Background(), func(ctx context.Context, toolArgument *schema.ToolArgument, opts ...tool.Option) (*schema.ToolResult, error) {
		invokableCalled.Store(true)
		return &schema.ToolResult{}, nil
	}, &adk.ToolContext{Name: "danger", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapEnhancedInvokableToolCall() error = %v", err)
	}
	result, err := wrappedInvokable(context.Background(), &schema.ToolArgument{Text: `{}`})
	if err != nil {
		t.Fatalf("enhanced invokable returned error: %v", err)
	}
	if !toolresult.IsDeniedPayload(result.Parts[0].Text) {
		t.Fatalf("expected enhanced invokable denial payload, got %#v", result)
	}
	if invokableCalled.Load() {
		t.Fatal("denied enhanced invokable endpoint should not be called")
	}

	var streamableCalled atomic.Bool
	wrappedStreamable, err := middleware.WrapEnhancedStreamableToolCall(context.Background(), func(ctx context.Context, toolArgument *schema.ToolArgument, opts ...tool.Option) (*schema.StreamReader[*schema.ToolResult], error) {
		streamableCalled.Store(true)
		return schema.StreamReaderFromArray([]*schema.ToolResult{{}}), nil
	}, &adk.ToolContext{Name: "danger", CallID: "call-2"})
	if err != nil {
		t.Fatalf("WrapEnhancedStreamableToolCall() error = %v", err)
	}
	stream, err := wrappedStreamable(context.Background(), &schema.ToolArgument{Text: `{}`})
	if err != nil {
		t.Fatalf("enhanced streamable returned error: %v", err)
	}
	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("enhanced streamable Recv() error = %v", err)
	}
	if !toolresult.IsDeniedPayload(first.Parts[0].Text) {
		t.Fatalf("expected enhanced streamable denial payload, got %#v", first)
	}
	if streamableCalled.Load() {
		t.Fatal("denied enhanced streamable endpoint should not be called")
	}
}

func TestWrapStreamableToolCallAuditsOnEOF(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "stream_tool",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	})
	middleware := newTestMiddleware(t, registry, nil)

	wrapped, err := middleware.WrapStreamableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		return schema.StreamReaderFromArray([]string{"ok"}), nil
	}, &adk.ToolContext{Name: "stream_tool", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapStreamableToolCall() error = %v", err)
	}
	reader, err := wrapped(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("wrapped streamable returned error: %v", err)
	}
	defer reader.Close()
	if got, err := reader.Recv(); err != nil || got != "ok" {
		t.Fatalf("Recv() = %q, %v; want ok, nil", got, err)
	}
	if _, err := reader.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv() EOF error = %v, want io.EOF", err)
	}
}

func TestWrapStreamableToolCallCanBeClosedWithoutConsumption(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "stream_tool",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	})
	middleware := newTestMiddleware(t, registry, nil)

	wrapped, err := middleware.WrapStreamableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		return schema.StreamReaderFromArray([]string{"ok"}), nil
	}, &adk.ToolContext{Name: "stream_tool", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapStreamableToolCall() error = %v", err)
	}
	reader, err := wrapped(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("wrapped streamable returned error: %v", err)
	}
	reader.Close()
	// capability_call audit on Close-without-Recv is guaranteed by proxyStreamReaderWithFinalize;
	// see TestProxyStreamReaderWithFinalizeRunsOnCloseWithoutConsumption.
}

func (a *fakeApprover) Approve(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error) {
	a.calls.Add(1)
	a.last = req
	return ApprovalDecision{Approved: a.approve, ApprovalScope: a.scope}, nil
}

func TestWrapInvokableToolCallAllowsPolicyAllowedCapability(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "get_weather",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	})
	middleware := newTestMiddleware(t, registry, nil)

	var called atomic.Bool
	endpoint := func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		called.Store(true)
		return "sunny", nil
	}
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), endpoint, &adk.ToolContext{Name: "get_weather", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	got, err := wrapped(context.Background(), `{"city":"北京"}`)
	if err != nil {
		t.Fatalf("wrapped endpoint returned error: %v", err)
	}
	if got != "sunny" || !called.Load() {
		t.Fatalf("wrapped endpoint result = %q called=%v", got, called.Load())
	}
}

func TestWrapInvokableToolCallRejectsOversizedOutput(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "small_output",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	})
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry:           registry,
		Policy:             policy.NewEngine(),
		Grants:             policy.NewSessionGrants(),
		MaxToolOutputBytes: 4,
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		return "too large", nil
	}, &adk.ToolContext{Name: "small_output", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	got, err := wrapped(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("wrapped endpoint returned error: %v", err)
	}
	if !toolresult.IsErrorPayload(got) {
		t.Fatalf("expected soft-fail payload, got %q", got)
	}
}

func TestWrapInvokableToolCallSoftFailsEndpointExecutionError(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "get_weather",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	})
	middleware := newTestMiddleware(t, registry, nil)
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		return "", errors.New("lang must be zh or en")
	}, &adk.ToolContext{Name: "get_weather", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	got, err := wrapped(context.Background(), `{"city":"重庆","lang":"ja"}`)
	if err != nil {
		t.Fatalf("wrapped endpoint returned error: %v", err)
	}
	if !toolresult.IsErrorPayload(got) {
		t.Fatalf("expected soft-fail payload, got %q", got)
	}
}

func TestWrapStreamableToolCallRejectsOversizedOutput(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "stream_tool",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	})
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry:           registry,
		Policy:             policy.NewEngine(),
		Grants:             policy.NewSessionGrants(),
		MaxToolOutputBytes: 2,
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}
	wrapped, err := middleware.WrapStreamableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		return schema.StreamReaderFromArray([]string{"ok", "too-large"}), nil
	}, &adk.ToolContext{Name: "stream_tool", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapStreamableToolCall() error = %v", err)
	}
	reader, err := wrapped(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("wrapped streamable returned error: %v", err)
	}
	defer reader.Close()
	if got, err := reader.Recv(); err != nil || got != "ok" {
		t.Fatalf("first Recv() = %q, %v; want ok, nil", got, err)
	}
	got, err := reader.Recv()
	if err != nil {
		t.Fatalf("second Recv() returned error: %v", err)
	}
	if !toolresult.IsErrorPayload(got) {
		t.Fatalf("expected soft-fail payload, got %q", got)
	}
	if _, err := reader.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("third Recv() error = %v, want io.EOF", err)
	}
}

func TestWrapStreamableToolCallSoftFailsEndpointSetupError(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "stream_tool",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	})
	middleware := newTestMiddleware(t, registry, nil)
	wrapped, err := middleware.WrapStreamableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		return nil, errors.New("network timeout")
	}, &adk.ToolContext{Name: "stream_tool", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapStreamableToolCall() error = %v", err)
	}
	reader, err := wrapped(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("wrapped streamable returned error: %v", err)
	}
	defer reader.Close()
	got, err := reader.Recv()
	if err != nil {
		t.Fatalf("Recv() returned error: %v", err)
	}
	if !toolresult.IsErrorPayload(got) {
		t.Fatalf("expected soft-fail payload, got %q", got)
	}
}

func TestWrapInvokableToolCallRejectsEscapingPathResource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "read_file",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	})
	guard, err := securitycore.NewWorkspaceGuard([]string{root})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry:       registry,
		Policy:         policy.NewEngine(),
		Grants:         policy.NewSessionGrants(),
		WorkspaceGuard: guard,
		Builders: map[string]securitycore.OperationBuilder{
			"read_file": func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
				request.OperationKind = securitycore.OperationFileRead
				request.Resources = []securitycore.OperationResource{{Kind: "path", Value: filepath.Join(outside, "secret.txt")}}
				return request, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}

	var called atomic.Bool
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		called.Store(true)
		return "", nil
	}, &adk.ToolContext{Name: "read_file", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	if _, err := wrapped(context.Background(), `{}`); err == nil {
		t.Fatal("expected path containment error")
	}
	if called.Load() {
		t.Fatal("endpoint should not run for escaping path resource")
	}
}

func TestWrapInvokableToolCallRejectsFileScopeOperationMismatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "notes.txt")
	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "file_tool",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskMedium,
		DefaultPolicy: capability.DefaultPolicyReview,
		Scopes:        []string{securitycore.ScopeFileWrite},
	})
	guard, err := securitycore.NewWorkspaceGuard([]string{root})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry:       registry,
		Policy:         policy.NewEngine(),
		Grants:         policy.NewSessionGrants(),
		WorkspaceGuard: guard,
		Builders: map[string]securitycore.OperationBuilder{
			"file_tool": func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
				request.OperationKind = securitycore.OperationFileRead
				request.Resources = []securitycore.OperationResource{{Kind: securitycore.ResourceKindFile, Value: target}}
				return request, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}

	var called atomic.Bool
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		called.Store(true)
		return "ok", nil
	}, &adk.ToolContext{Name: "file_tool", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	if _, err := wrapped(context.Background(), `{}`); err == nil {
		t.Fatal("expected file scope mismatch error")
	}
	if called.Load() {
		t.Fatal("endpoint should not run for file scope mismatch")
	}
}

func TestWrapInvokableToolCallRejectsNetworkResourceOutsideScope(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "network_tool",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
		Scopes:        []string{"network:test"},
		Resources:     []string{"https://example.com/allowed"},
	})
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry: registry,
		Policy:   policy.NewEngine(),
		Grants:   policy.NewSessionGrants(),
		Builders: map[string]securitycore.OperationBuilder{
			"network_tool": func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
				request.OperationKind = "network.test"
				request.Resources = []securitycore.OperationResource{{Kind: "url", Value: "https://example.com/other"}}
				return request, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}

	var called atomic.Bool
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		called.Store(true)
		return "ok", nil
	}, &adk.ToolContext{Name: "network_tool", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	if _, err := wrapped(context.Background(), `{}`); err == nil {
		t.Fatal("expected network scope error")
	}
	if called.Load() {
		t.Fatal("endpoint should not run for disallowed network resource")
	}
}

func TestWrapInvokableToolCallRejectsNetworkResourceWithoutNetworkScope(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "network_tool",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	})
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry: registry,
		Policy:   policy.NewEngine(),
		Grants:   policy.NewSessionGrants(),
		Builders: map[string]securitycore.OperationBuilder{
			"network_tool": func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
				request.OperationKind = "network.test"
				request.Resources = []securitycore.OperationResource{{Kind: "url", Value: "https://example.com/other"}}
				return request, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}

	var called atomic.Bool
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		called.Store(true)
		return "ok", nil
	}, &adk.ToolContext{Name: "network_tool", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	if _, err := wrapped(context.Background(), `{}`); err == nil {
		t.Fatal("expected network resource error without network scope")
	}
	if called.Load() {
		t.Fatal("endpoint should not run for disallowed network resource")
	}
}

func TestWrapInvokableToolCallInjectsAuthorizedOperationIntoContext(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "get_weather",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
		Scopes:        []string{"network:weather"},
		Resources:     []string{"https://uapis.cn/api/v1/misc/weather"},
	})
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry: registry,
		Policy:   policy.NewEngine(),
		Grants:   policy.NewSessionGrants(),
		Builders: map[string]securitycore.OperationBuilder{
			"get_weather": func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
				request.OperationKind = "network.weather"
				request.Resources = []securitycore.OperationResource{{Kind: "url", Value: "https://uapis.cn/api/v1/misc/weather"}}
				return request, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}

	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		operation, ok := securitycore.AuthorizedOperationFromContext(ctx)
		if !ok {
			t.Fatal("expected authorized operation in context")
		}
		if operation.OperationKind != "network.weather" {
			t.Fatalf("OperationKind = %q, want network.weather", operation.OperationKind)
		}
		if len(operation.Resources) == 0 {
			t.Fatal("expected authorized resources")
		}
		return "ok", nil
	}, &adk.ToolContext{Name: "get_weather", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	if _, err := wrapped(context.Background(), `{"city":"北京"}`); err != nil {
		t.Fatalf("wrapped endpoint returned error: %v", err)
	}
}

func TestWrapInvokableToolCallRunsRegisteredFileReadWithinWorkspace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(target, []byte("hello\nworld\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	guard, err := securitycore.NewWorkspaceGuard([]string{root})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}
	descriptors := filetools.CapabilityDescriptors()
	registry := newTestRegistry(t, descriptors...)
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry:       registry,
		Policy:         policy.NewEngine(),
		Grants:         policy.NewSessionGrants(),
		WorkspaceGuard: guard,
		Builders:       filetools.OperationBuilders(),
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}
	registeredTools, err := filetools.NewTools(guard, execfiles.NewService(execfiles.Config{}))
	if err != nil {
		t.Fatalf("NewTools() error = %v", err)
	}
	var readTool tool.InvokableTool
	for _, candidate := range registeredTools {
		info, err := candidate.Info(context.Background())
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}
		if info.Name == "file_read" {
			var ok bool
			readTool, ok = candidate.(tool.InvokableTool)
			if !ok {
				t.Fatalf("file_read tool does not implement InvokableTool")
			}
		}
	}
	if readTool == nil {
		t.Fatal("file_read tool not found")
	}
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), readTool.InvokableRun, &adk.ToolContext{Name: "file_read", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	got, err := wrapped(context.Background(), `{"path":"`+escapeJSONPath(target)+`","max_lines":1}`)
	if err != nil {
		t.Fatalf("wrapped file_read returned error: %v", err)
	}
	if !strings.Contains(got, `"content":"hello\n"`) || !strings.Contains(got, `"truncated":true`) {
		t.Fatalf("file_read result = %q, want content and truncated metadata", got)
	}
}

func TestWrapInvokableToolCallReturnsRecoverableInvalidFileArguments(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	guard, err := securitycore.NewWorkspaceGuard([]string{root})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}
	registry := newTestRegistry(t, filetools.CapabilityDescriptors()...)
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry:       registry,
		Policy:         policy.NewEngine(),
		Grants:         policy.NewSessionGrants(),
		WorkspaceGuard: guard,
		Builders:       filetools.OperationBuilders(),
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}
	var called atomic.Bool
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		called.Store(true)
		return "should not run", nil
	}, &adk.ToolContext{Name: "file_read", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	got, err := wrapped(context.Background(), `{`)
	if err != nil {
		t.Fatalf("wrapped file_read returned error: %v", err)
	}
	if called.Load() {
		t.Fatal("endpoint should not run for invalid file arguments")
	}
	if !toolresult.IsErrorPayload(got) || toolresult.DenialReason(got) != securitycore.ToolFailureReasonInvalidArguments {
		t.Fatalf("expected invalid argument payload, got %q", got)
	}
}

func TestWrapInvokableToolCallDeniesPolicyDeniedCapability(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "danger",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskHigh,
		DefaultPolicy: capability.DefaultPolicyDeny,
	})
	middleware := newTestMiddleware(t, registry, nil)

	var called atomic.Bool
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		called.Store(true)
		return "", nil
	}, &adk.ToolContext{Name: "danger", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	got, err := wrapped(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("wrapped endpoint returned error: %v", err)
	}
	if !toolresult.IsDeniedPayload(got) {
		t.Fatalf("expected denial payload, got %q", got)
	}
	if toolresult.DenialReason(got) != securitycore.DenialReasonPolicyDenied {
		t.Fatalf("DenialReason() = %q, want %q", toolresult.DenialReason(got), securitycore.DenialReasonPolicyDenied)
	}
	if called.Load() {
		t.Fatal("denied endpoint should not be called")
	}
}

func TestWrapInvokableToolCallCachesSessionApproval(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "high_risk",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskHigh,
		DefaultPolicy: capability.DefaultPolicyReview,
	})
	approver := &fakeApprover{approve: true, scope: "session"}
	middleware := newTestMiddleware(t, registry, approver)

	var called atomic.Int32
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		called.Add(1)
		return "ok", nil
	}, &adk.ToolContext{Name: "high_risk", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	for i := 0; i < 2; i++ {
		if _, err := wrapped(context.Background(), `{}`); err != nil {
			t.Fatalf("wrapped endpoint call %d returned error: %v", i+1, err)
		}
	}
	if approver.calls.Load() != 1 {
		t.Fatalf("approver calls = %d, want 1", approver.calls.Load())
	}
	if called.Load() != 2 {
		t.Fatalf("endpoint calls = %d, want 2", called.Load())
	}
}

func TestWrapInvokableToolCallReusesSessionApprovalForDifferentNetworkArgs(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "get_weather",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyReview,
		Scopes:        []string{"network:weather"},
		Resources:     []string{"https://uapis.cn/api/v1/misc/weather"},
	})
	approver := &fakeApprover{approve: true, scope: "session"}
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry: registry,
		Policy:   policy.NewEngine(),
		Grants:   policy.NewSessionGrants(),
		Approver: approver,
		Builders: map[string]securitycore.OperationBuilder{
			"get_weather": func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
				request.OperationKind = "network.weather"
				request.Resources = []securitycore.OperationResource{{Kind: "url", Value: "https://uapis.cn/api/v1/misc/weather"}}
				return request, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}

	var called atomic.Int32
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		called.Add(1)
		return "ok", nil
	}, &adk.ToolContext{Name: "get_weather", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	if _, err := wrapped(context.Background(), `{"city":"重庆","lang":"zh"}`); err != nil {
		t.Fatalf("first wrapped endpoint returned error: %v", err)
	}
	if _, err := wrapped(context.Background(), `{"city":"重庆","lang":"ja"}`); err != nil {
		t.Fatalf("second wrapped endpoint returned error: %v", err)
	}
	if approver.calls.Load() != 1 {
		t.Fatalf("approver calls = %d, want 1", approver.calls.Load())
	}
	if called.Load() != 2 {
		t.Fatalf("endpoint calls = %d, want 2", called.Load())
	}
}

func TestWrapInvokableToolCallApprovalDefaultsToOneOperation(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "high_risk",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskHigh,
		DefaultPolicy: capability.DefaultPolicyReview,
	})
	approver := &fakeApprover{approve: true}
	middleware := newTestMiddleware(t, registry, approver)

	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		return "ok", nil
	}, &adk.ToolContext{Name: "high_risk", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	for i := 0; i < 2; i++ {
		if _, err := wrapped(context.Background(), `{}`); err != nil {
			t.Fatalf("wrapped endpoint call %d returned error: %v", i+1, err)
		}
	}
	if approver.calls.Load() != 2 {
		t.Fatalf("approver calls = %d, want 2", approver.calls.Load())
	}
}

func TestWrapInvokableToolCallCleansApprovalLock(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "high_risk_once",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskHigh,
		DefaultPolicy: capability.DefaultPolicyReview,
	})
	middleware := newTestMiddleware(t, registry, &fakeApprover{approve: true})

	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		return "ok", nil
	}, &adk.ToolContext{Name: "high_risk_once", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}
	if _, err := wrapped(context.Background(), `{}`); err != nil {
		t.Fatalf("wrapped endpoint returned error: %v", err)
	}

	middleware.approvalLocksMu.Lock()
	defer middleware.approvalLocksMu.Unlock()
	if len(middleware.approvalLocks) != 0 {
		t.Fatalf("approval lock count = %d, want 0", len(middleware.approvalLocks))
	}
}

func TestWrapInvokableToolCallAppliesTimeout(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "slow_tool",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	})
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry:              registry,
		Policy:                policy.NewEngine(),
		Grants:                policy.NewSessionGrants(),
		Approver:              &fakeApprover{approve: true},
		CommandSandbox:        fakeSandboxRunner{status: commandsandbox.Status{Backend: commandsandbox.BackendWSL2, Available: true, Reason: "ready"}},
		CommandTimeoutSeconds: 1,
		Builders: map[string]securitycore.OperationBuilder{
			"slow_tool": func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
				request.OperationKind = securitycore.OperationCommandRun
				return request, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}, &adk.ToolContext{Name: "slow_tool", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	got, err := wrapped(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("wrapped endpoint returned error: %v", err)
	}
	if !toolresult.IsErrorPayload(got) {
		t.Fatalf("expected soft-fail payload, got %q", got)
	}
}

func TestWrapInvokableToolCallInjectsCommandSandboxRunner(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "run_shell",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	})
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry:       registry,
		Policy:         policy.NewEngine(),
		Grants:         policy.NewSessionGrants(),
		Approver:       &fakeApprover{approve: true},
		CommandSandbox: fakeSandboxRunner{status: commandsandbox.Status{Backend: commandsandbox.BackendWSL2, Available: true, Reason: "ready"}},
		Builders: map[string]securitycore.OperationBuilder{
			"run_shell": func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
				request.OperationKind = securitycore.OperationCommandRun
				return request, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		if _, ok := commandsandbox.RunnerFromContext(ctx); !ok {
			t.Fatal("RunnerFromContext() ok = false, want injected sandbox runner")
		}
		if _, err := commandsandbox.RunFromContext(ctx, commandsandbox.Request{Command: "true"}); err != nil {
			t.Fatalf("RunFromContext() error = %v", err)
		}
		return "ok", nil
	}, &adk.ToolContext{Name: "run_shell", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	got, err := wrapped(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("wrapped endpoint returned error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("wrapped endpoint result = %q, want ok", got)
	}
}

func TestWrapInvokableToolCallAppliesFileTimeout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "notes.txt")
	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "file_read",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
		Scopes:        []string{securitycore.ScopeFileRead},
	})
	guard, err := securitycore.NewWorkspaceGuard([]string{root})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry:              registry,
		Policy:                policy.NewEngine(),
		Grants:                policy.NewSessionGrants(),
		WorkspaceGuard:        guard,
		FileTimeoutSeconds:    1,
		MaxToolOutputBytes:    1 << 20,
		CommandSandbox:        fakeSandboxRunner{status: commandsandbox.Status{Backend: commandsandbox.BackendWSL2, Available: true, Reason: "ready"}},
		CommandTimeoutSeconds: 30,
		Builders: map[string]securitycore.OperationBuilder{
			"file_read": func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
				request.OperationKind = securitycore.OperationFileRead
				request.Resources = []securitycore.OperationResource{{Kind: securitycore.ResourceKindFile, Value: target}}
				return request, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}, &adk.ToolContext{Name: "file_read", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	got, err := wrapped(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("wrapped endpoint returned error: %v", err)
	}
	if !toolresult.IsErrorPayload(got) {
		t.Fatalf("expected soft-fail payload, got %q", got)
	}
}

func TestWrapInvokableToolCallDeniesCommandWhenSandboxUnavailable(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "run_shell",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	})
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry:       registry,
		Policy:         policy.NewEngine(),
		Grants:         policy.NewSessionGrants(),
		Approver:       &fakeApprover{approve: true},
		CommandSandbox: fakeSandboxRunner{status: commandsandbox.Status{Backend: commandsandbox.BackendWSL2, Reason: "missing bwrap"}},
		Builders: map[string]securitycore.OperationBuilder{
			"run_shell": func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
				request.OperationKind = securitycore.OperationCommandRun
				return request, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}
	var called atomic.Bool
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		called.Store(true)
		return "ok", nil
	}, &adk.ToolContext{Name: "run_shell", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	got, err := wrapped(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("wrapped endpoint returned error: %v", err)
	}
	if called.Load() {
		t.Fatal("endpoint should not run when command sandbox is unavailable")
	}
	if !toolresult.IsDeniedPayload(got) || toolresult.DenialReason(got) != securitycore.DenialReasonCommandSandboxUnavailable {
		t.Fatalf("expected command sandbox denial payload, got %q", got)
	}
}

func TestWrapInvokableToolCallRejectsFileScopeMismatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "notes.txt")
	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "file_bad",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
		Scopes:        []string{securitycore.ScopeFileWrite},
	})
	guard, err := securitycore.NewWorkspaceGuard([]string{root})
	if err != nil {
		t.Fatalf("NewWorkspaceGuard() error = %v", err)
	}
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry:       registry,
		Policy:         policy.NewEngine(),
		Grants:         policy.NewSessionGrants(),
		WorkspaceGuard: guard,
		Builders: map[string]securitycore.OperationBuilder{
			"file_bad": func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
				request.OperationKind = securitycore.OperationFileRead
				request.Resources = []securitycore.OperationResource{{Kind: securitycore.ResourceKindFile, Value: target}}
				return request, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}
	var called atomic.Bool
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		called.Store(true)
		return "ok", nil
	}, &adk.ToolContext{Name: "file_bad", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	if _, err := wrapped(context.Background(), `{}`); err == nil {
		t.Fatal("wrapped endpoint error = nil, want file scope mismatch hard-fail")
	}
	if called.Load() {
		t.Fatal("endpoint should not run when file scope and operation kind mismatch")
	}
}

func TestWrapInvokableToolCallReviewsUnknownCapability(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t)
	approver := &fakeApprover{approve: true}
	middleware := newTestMiddleware(t, registry, approver)

	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		return "ok", nil
	}, &adk.ToolContext{Name: "unknown_tool", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	if _, err := wrapped(context.Background(), `{"api_key":"secret"}`); err != nil {
		t.Fatalf("wrapped endpoint returned error: %v", err)
	}
	if approver.calls.Load() != 1 {
		t.Fatalf("approver calls = %d, want 1", approver.calls.Load())
	}
	if approver.last.Operation.Registered {
		t.Fatal("expected unknown capability to be unregistered")
	}
	if approver.last.Operation.SanitizedArgsSummary == "" || approver.last.Operation.SanitizedArgsSummary == `{"api_key":"secret"}` {
		t.Fatalf("arguments were not summarized safely: %q", approver.last.Operation.SanitizedArgsSummary)
	}
}

func TestWrapInvokableToolCallStopsWhenReviewDenied(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "review_tool",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskMedium,
		DefaultPolicy: capability.DefaultPolicyReview,
	})
	middleware := newTestMiddleware(t, registry, &fakeApprover{approve: false})

	var called atomic.Bool
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		called.Store(true)
		return "", errors.New("endpoint should not run")
	}, &adk.ToolContext{Name: "review_tool", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	got, err := wrapped(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("wrapped endpoint returned error: %v", err)
	}
	if called.Load() {
		t.Fatal("endpoint should not run when review is denied")
	}
	if !toolresult.IsDeniedPayload(got) {
		t.Fatalf("expected denial payload, got %q", got)
	}
	if toolresult.DenialReason(got) != securitycore.DenialReasonUserDenied {
		t.Fatalf("DenialReason() = %q, want %q", toolresult.DenialReason(got), securitycore.DenialReasonUserDenied)
	}
}

func TestWrapInvokableToolCallSerializesConcurrentApproval(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "review_parallel",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskHigh,
		DefaultPolicy: capability.DefaultPolicyReview,
	})
	approver := &slowApprover{approve: true, scope: "session"}
	middleware := newTestMiddleware(t, registry, approver)

	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		return "ok", nil
	}, &adk.ToolContext{Name: "review_parallel", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	const calls = 8
	var wg sync.WaitGroup
	errs := make(chan error, calls)
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := wrapped(context.Background(), `{}`)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("wrapped endpoint returned error: %v", err)
		}
	}

	if approver.calls.Load() != 1 {
		t.Fatalf("approver calls = %d, want 1", approver.calls.Load())
	}
	if approver.maxActive.Load() != 1 {
		t.Fatalf("max concurrent approvals = %d, want 1", approver.maxActive.Load())
	}
}

func TestWrapInvokableToolCallAllowsConcurrentApprovalForDifferentTools(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t,
		capability.Descriptor{
			Name:          "review_a",
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskHigh,
			DefaultPolicy: capability.DefaultPolicyReview,
		},
		capability.Descriptor{
			Name:          "review_b",
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskHigh,
			DefaultPolicy: capability.DefaultPolicyReview,
		},
	)
	approver := &slowApprover{approve: true}
	middleware := newTestMiddleware(t, registry, approver)

	wrappedA, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		return "a", nil
	}, &adk.ToolContext{Name: "review_a", CallID: "call-a"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall(review_a) error = %v", err)
	}
	wrappedB, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		return "b", nil
	}, &adk.ToolContext{Name: "review_b", CallID: "call-b"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall(review_b) error = %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := wrappedA(context.Background(), `{}`)
		errs <- err
	}()
	go func() {
		defer wg.Done()
		_, err := wrappedB(context.Background(), `{}`)
		errs <- err
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("wrapped endpoint returned error: %v", err)
		}
	}

	if approver.calls.Load() != 2 {
		t.Fatalf("approver calls = %d, want 2", approver.calls.Load())
	}
	if approver.maxActive.Load() != 2 {
		t.Fatalf("max concurrent approvals = %d, want 2", approver.maxActive.Load())
	}
}

func TestWrapInvokableToolCallRequiresApproverForReviewedCapability(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, capability.Descriptor{
		Name:          "review_tool",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskMedium,
		DefaultPolicy: capability.DefaultPolicyReview,
	})
	middleware := newTestMiddleware(t, registry, nil)

	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		return "ok", nil
	}, &adk.ToolContext{Name: "review_tool", CallID: "call-1"})
	if err != nil {
		t.Fatalf("WrapInvokableToolCall() error = %v", err)
	}

	_, err = wrapped(context.Background(), `{}`)
	if err == nil {
		t.Fatal("expected approval required error")
	}
}

func newTestRegistry(t *testing.T, descriptors ...capability.Descriptor) *capability.Registry {
	t.Helper()
	registry, err := capability.NewRegistry(descriptors...)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}

func newTestMiddleware(t *testing.T, registry *capability.Registry, approver Approver) *ExecutionSecurityMiddleware {
	t.Helper()
	middleware, err := NewExecutionSecurityMiddleware(Config{
		Registry: registry,
		Policy:   policy.NewEngine(),
		Grants:   policy.NewSessionGrants(),
		Approver: approver,
	})
	if err != nil {
		t.Fatalf("NewExecutionSecurityMiddleware() error = %v", err)
	}
	return middleware
}

func escapeJSONPath(path string) string {
	return strings.ReplaceAll(path, `\`, `\\`)
}

type slowApprover struct {
	approve   bool
	scope     string
	calls     atomic.Int32
	active    atomic.Int32
	maxActive atomic.Int32
}

func (a *slowApprover) Approve(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error) {
	a.calls.Add(1)
	active := a.active.Add(1)
	for {
		currentMax := a.maxActive.Load()
		if active <= currentMax || a.maxActive.CompareAndSwap(currentMax, active) {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	a.active.Add(-1)
	return ApprovalDecision{Approved: a.approve, ApprovalScope: a.scope}, nil
}
