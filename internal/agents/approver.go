package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/input"
	securitymiddleware "github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares/security"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal"
)

type terminalApprover struct {
	promptReader *input.PromptReader
	renderer     *terminal.Renderer
	mu           sync.Mutex
}

func newTerminalApprover(promptReader *input.PromptReader, renderer *terminal.Renderer) securitymiddleware.Approver {
	return &terminalApprover{
		promptReader: promptReader,
		renderer:     renderer,
	}
}

func (a *terminalApprover) Approve(ctx context.Context, req securitymiddleware.ApprovalRequest) (securitymiddleware.ApprovalDecision, error) {
	if a == nil || a.promptReader == nil || a.renderer == nil {
		return securitymiddleware.ApprovalDecision{}, errors.New("terminal approver is incomplete")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	argsSummary := req.Operation.SanitizedArgsSummary
	if argsSummary == "" {
		argsSummary = "none"
	}
	argsSHA := securitycore.NewAuditRecord(req.Operation).ArgsSummarySHA
	if argsSHA == "" {
		argsSHA = "none"
	}
	a.renderer.ApprovalPrompt(fmt.Sprintf(
		"Approve capability %s (operation=%s risk=%s reason=%s resources=%s args_sha=%s args_len=%d)? [y=once/s=session/N]: ",
		req.ToolName,
		req.Operation.OperationKind,
		req.Capability.Risk,
		req.Decision.Reason,
		req.Operation.ResourceSummary(),
		argsSHA,
		len(argsSummary),
	))
	result, ok := a.promptReader.Receive(ctx)
	if !ok {
		return securitymiddleware.ApprovalDecision{}, errors.New("approval input closed")
	}
	if result.Error != nil {
		return securitymiddleware.ApprovalDecision{}, result.Error
	}

	answer := strings.ToLower(strings.TrimSpace(result.Text))
	switch answer {
	case "y", "yes":
		return securitymiddleware.ApprovalDecision{Approved: true, ApprovalScope: securitycore.ApprovalScopeOnce}, nil
	case "s", "session":
		return securitymiddleware.ApprovalDecision{Approved: true, ApprovalScope: securitycore.ApprovalScopeSession}, nil
	default:
		return securitymiddleware.ApprovalDecision{}, nil
	}
}
