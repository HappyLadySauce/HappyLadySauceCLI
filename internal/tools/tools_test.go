package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
)

func TestNewAgentToolsExposesWeatherTool(t *testing.T) {
	t.Parallel()

	cfg, err := NewAgentTools()
	if err != nil {
		t.Fatalf("NewAgentTools() error = %v", err)
	}
	if len(cfg.Tools) != 1 {
		t.Fatalf("tool count = %d, want 1", len(cfg.Tools))
	}

	info, err := cfg.Tools[0].Info(context.Background())
	if err != nil {
		t.Fatalf("tool info error: %v", err)
	}
	if info.Name != "get_weather" {
		t.Fatalf("tool name = %s, want get_weather", info.Name)
	}
}

func TestNewOperationBuildersRegistersWeatherBuilder(t *testing.T) {
	t.Parallel()

	builders := NewOperationBuilders()
	builder := builders["get_weather"]
	if builder == nil {
		t.Fatal("expected get_weather operation builder")
	}
	operation := builder(context.Background(), securitycore.OperationRequest{
		ToolName:      "get_weather",
		OperationKind: securitycore.OperationNativeTool,
		Risk:          capability.RiskLow,
	}, securitycore.OperationBuildInput{Summary: `{city=北京}`})
	if operation.OperationKind != "network.weather" {
		t.Fatalf("OperationKind = %q, want network.weather", operation.OperationKind)
	}
	if len(operation.Resources) != 1 || operation.Resources[0].Kind != "url" {
		t.Fatalf("Resources = %#v, want weather URL resource", operation.Resources)
	}
}

func TestOperationBuildersCoverAllRegisteredTools(t *testing.T) {
	t.Parallel()

	cfg, err := NewAgentTools()
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

	cfg, err := NewAgentTools()
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
		operation := builder(context.Background(), securitycore.OperationRequest{
			ToolName:   info.Name,
			Capability: desc,
			Registered: true,
			Risk:       desc.Risk,
		}, securitycore.OperationBuildInput{RawJSON: `{}`, Summary: securitycore.SummarizeArguments(`{}`)})
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
