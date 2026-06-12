package tools

import (
	"context"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
)

func TestNewAgentToolsExposesWeatherTool(t *testing.T) {
	t.Parallel()

	cfg := NewAgentTools()
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

func TestNewCapabilityRegistryRegistersWeatherAsAllowedBuiltin(t *testing.T) {
	t.Parallel()

	registry := NewCapabilityRegistry()
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
