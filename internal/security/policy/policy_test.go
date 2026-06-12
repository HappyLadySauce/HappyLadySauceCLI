package policy

import (
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
)

func TestEngineAllowsLowRiskAllowCapability(t *testing.T) {
	t.Parallel()

	decision := NewEngine().Evaluate(securitycore.OperationRequest{
		Registered: true,
		Capability: capability.Descriptor{
			Name:          "get_weather",
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskLow,
			DefaultPolicy: capability.DefaultPolicyAllow,
		},
	})

	if decision.Action != ActionAllow {
		t.Fatalf("decision action = %s, want %s", decision.Action, ActionAllow)
	}
}

func TestEngineReviewsHighRiskCapabilityEvenWhenDefaultAllow(t *testing.T) {
	t.Parallel()

	decision := NewEngine().Evaluate(securitycore.OperationRequest{
		Registered: true,
		Capability: capability.Descriptor{
			Name:          "run_shell",
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskHigh,
			DefaultPolicy: capability.DefaultPolicyAllow,
		},
	})

	if decision.Action != ActionReview {
		t.Fatalf("decision action = %s, want %s", decision.Action, ActionReview)
	}
}

func TestEngineReviewsOperationRiskEvenWhenDescriptorAllows(t *testing.T) {
	t.Parallel()

	decision := NewEngine().Evaluate(securitycore.OperationRequest{
		Registered: true,
		Risk:       capability.RiskHigh,
		Capability: capability.Descriptor{
			Name:          "future_write",
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskLow,
			DefaultPolicy: capability.DefaultPolicyAllow,
		},
	})

	if decision.Action != ActionReview || decision.Reason != "high_risk" {
		t.Fatalf("decision = %#v, want high-risk review", decision)
	}
}

func TestEngineReviewsCommandOperations(t *testing.T) {
	t.Parallel()

	decision := NewEngine().Evaluate(securitycore.OperationRequest{
		Registered:    true,
		OperationKind: securitycore.OperationCommandRun,
		Risk:          capability.RiskLow,
		Capability: capability.Descriptor{
			Name:          "run_command",
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskLow,
			DefaultPolicy: capability.DefaultPolicyAllow,
		},
	})

	if decision.Action != ActionReview || decision.Reason != "command_run" {
		t.Fatalf("decision = %#v, want command review", decision)
	}
}

func TestEngineAllowsLowRiskNetworkAllowCapability(t *testing.T) {
	t.Parallel()

	decision := NewEngine(Config{ApprovalDefault: "review"}).Evaluate(securitycore.OperationRequest{
		Registered:    true,
		OperationKind: "network.weather",
		Risk:          capability.RiskLow,
		Capability: capability.Descriptor{
			Name:          "get_weather",
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskLow,
			DefaultPolicy: capability.DefaultPolicyAllow,
			Scopes:        []string{"network:weather"},
		},
	})

	if decision.Action != ActionAllow || decision.Reason != "default_policy_allow" {
		t.Fatalf("decision = %#v, want low-risk network allow", decision)
	}
}

func TestEngineReviewsNetworkOperationsWhenNotLowRiskAllow(t *testing.T) {
	t.Parallel()

	decision := NewEngine(Config{ApprovalDefault: "review"}).Evaluate(securitycore.OperationRequest{
		Registered:    true,
		OperationKind: "network.weather",
		Risk:          capability.RiskMedium,
		Capability: capability.Descriptor{
			Name:          "fetch_data",
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskMedium,
			DefaultPolicy: capability.DefaultPolicyAllow,
			Scopes:        []string{"network:api"},
		},
	})

	if decision.Action != ActionReview || decision.Reason != "network_operation" {
		t.Fatalf("decision = %#v, want network review", decision)
	}
}

func TestEngineReviewsMissingDescriptor(t *testing.T) {
	t.Parallel()

	decision := NewEngine().Evaluate(securitycore.OperationRequest{
		Capability: capability.Descriptor{Name: "unregistered"},
		Registered: false,
	})
	if decision.Action != ActionReview {
		t.Fatalf("decision action = %s, want %s", decision.Action, ActionReview)
	}
}

func TestEngineUsesApprovalDefaultForReviewDecisions(t *testing.T) {
	t.Parallel()

	decision := NewEngine(Config{ApprovalDefault: "deny"}).Evaluate(securitycore.OperationRequest{
		Capability: capability.Descriptor{Name: "unregistered"},
		Registered: false,
	})

	if decision.Action != ActionDeny || decision.Reason != "approval_default_unsupported" {
		t.Fatalf("decision = %#v, want fail-closed approval default denial", decision)
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

	readOperation := securitycore.OperationRequest{
		Capability:    readFile,
		OperationKind: securitycore.OperationFileRead,
		Risk:          readFile.Risk,
		Resources:     []securitycore.OperationResource{{Kind: "path", Value: "D:/Code/project/a.txt"}},
	}
	otherOperation := readOperation
	otherOperation.Capability = otherFile
	otherOperation.Resources = []securitycore.OperationResource{{Kind: "path", Value: "D:/Code/project/b.txt"}}

	grants.Allow(readOperation)
	if !grants.IsAllowed(readOperation) {
		t.Fatal("expected same descriptor to be allowed")
	}
	if grants.IsAllowed(otherOperation) {
		t.Fatal("session grant leaked to a different resource")
	}
}

func TestSessionGrantsReuseNetworkSessionKeyAcrossArgs(t *testing.T) {
	t.Parallel()

	grants := NewSessionGrants()
	zhOperation := securitycore.OperationRequest{
		Capability: capability.Descriptor{
			Name:   "get_weather",
			Type:   capability.TypeNativeTool,
			Source: capability.SourceBuiltin,
		},
		OperationKind:        "network.weather",
		Risk:                 capability.RiskLow,
		Resources:            []securitycore.OperationResource{{Kind: "url", Value: "https://uapis.cn/api/v1/misc/weather"}},
		SanitizedArgsSummary: "{city=重庆,lang=zh}",
	}
	jaOperation := zhOperation
	jaOperation.SanitizedArgsSummary = "{city=重庆,lang=ja}"

	grants.Allow(zhOperation)
	if !grants.IsAllowed(jaOperation) {
		t.Fatal("expected session grant to cover different network args for same resource")
	}
}
