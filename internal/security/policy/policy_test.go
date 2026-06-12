package policy

import (
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
)

func TestEngineAllowsLowRiskAllowCapability(t *testing.T) {
	t.Parallel()

	decision := NewEngine().Evaluate(capability.Descriptor{
		Name:          "get_weather",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
	}, true)

	if decision.Action != ActionAllow {
		t.Fatalf("decision action = %s, want %s", decision.Action, ActionAllow)
	}
}

func TestEngineReviewsHighRiskCapabilityEvenWhenDefaultAllow(t *testing.T) {
	t.Parallel()

	decision := NewEngine().Evaluate(capability.Descriptor{
		Name:          "run_shell",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskHigh,
		DefaultPolicy: capability.DefaultPolicyAllow,
	}, true)

	if decision.Action != ActionReview {
		t.Fatalf("decision action = %s, want %s", decision.Action, ActionReview)
	}
}

func TestEngineReviewsMissingDescriptor(t *testing.T) {
	t.Parallel()

	decision := NewEngine().Evaluate(capability.Descriptor{Name: "unregistered"}, false)
	if decision.Action != ActionReview {
		t.Fatalf("decision action = %s, want %s", decision.Action, ActionReview)
	}
}

func TestSessionGrantsAreScopedToDescriptorKey(t *testing.T) {
	t.Parallel()

	grants := NewSessionGrants()
	readFile := capability.Descriptor{
		Name:          "read_file",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskHigh,
		DefaultPolicy: capability.DefaultPolicyReview,
		Resources:     []string{"D:/Code/project/a.txt"},
	}
	otherFile := readFile
	otherFile.Resources = []string{"D:/Code/project/b.txt"}

	grants.Allow(readFile)
	if !grants.IsAllowed(readFile) {
		t.Fatal("expected same descriptor to be allowed")
	}
	if grants.IsAllowed(otherFile) {
		t.Fatal("session grant leaked to a different resource")
	}
}
