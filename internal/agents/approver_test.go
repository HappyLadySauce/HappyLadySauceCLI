package agents

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/input"
	securitymiddleware "github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares/security"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/security/policy"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal"
)

func TestTerminalApproverApprovesYesResponse(t *testing.T) {
	ctx := context.Background()
	reader := input.NewPromptReader(ctx, strings.NewReader("y\n"))
	var out bytes.Buffer
	var errOut bytes.Buffer
	approver := newTerminalApprover(reader, terminal.NewRenderer(&out, &errOut))

	decision, err := approver.Approve(ctx, securitymiddleware.ApprovalRequest{
		ToolName: "run_shell",
		Capability: capability.Descriptor{
			Name:          "run_shell",
			Type:          capability.TypeNativeTool,
			Source:        capability.SourceBuiltin,
			Risk:          capability.RiskHigh,
			DefaultPolicy: capability.DefaultPolicyReview,
		},
		Decision: policy.Decision{Action: policy.ActionReview, Reason: "high_risk"},
		Operation: securitycore.OperationRequest{
			OperationKind:        securitycore.OperationCommandRun,
			Risk:                 capability.RiskHigh,
			SanitizedArgsSummary: "cmd=[REDACTED]",
		},
	})
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if !decision.Approved {
		t.Fatal("expected approval")
	}
	if decision.ApprovalScope != securitycore.ApprovalScopeOnce {
		t.Fatalf("approval scope = %q, want once", decision.ApprovalScope)
	}
	if !strings.Contains(errOut.String(), "Approve capability run_shell") || !strings.Contains(errOut.String(), "operation=command.run") {
		t.Fatalf("approval prompt not rendered: %q", errOut.String())
	}
}

func TestTerminalApproverApprovesSessionResponse(t *testing.T) {
	ctx := context.Background()
	reader := input.NewPromptReader(ctx, strings.NewReader("s\n"))
	var out bytes.Buffer
	var errOut bytes.Buffer
	approver := newTerminalApprover(reader, terminal.NewRenderer(&out, &errOut))

	decision, err := approver.Approve(ctx, securitymiddleware.ApprovalRequest{ToolName: "run_shell"})
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if !decision.Approved || decision.ApprovalScope != securitycore.ApprovalScopeSession {
		t.Fatalf("approval decision = %#v, want session approval", decision)
	}
}

func TestTerminalApproverDeniesDefaultResponse(t *testing.T) {
	ctx := context.Background()
	reader := input.NewPromptReader(ctx, strings.NewReader("\n"))
	var out bytes.Buffer
	var errOut bytes.Buffer
	approver := newTerminalApprover(reader, terminal.NewRenderer(&out, &errOut))

	decision, err := approver.Approve(ctx, securitymiddleware.ApprovalRequest{ToolName: "danger"})
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if decision.Approved {
		t.Fatal("expected default denial")
	}
}
