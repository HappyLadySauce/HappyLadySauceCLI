package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/input"
	securitymiddleware "github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares/security"
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

	a.renderer.ApprovalPrompt(fmt.Sprintf(
		"Approve capability %s (risk=%s reason=%s)? [y/N]: ",
		req.ToolName,
		req.Capability.Risk,
		req.Decision.Reason,
	))
	result, ok := a.promptReader.Receive(ctx)
	if !ok {
		return securitymiddleware.ApprovalDecision{}, errors.New("approval input closed")
	}
	if result.Error != nil {
		return securitymiddleware.ApprovalDecision{}, result.Error
	}

	answer := strings.ToLower(strings.TrimSpace(result.Text))
	return securitymiddleware.ApprovalDecision{Approved: answer == "y" || answer == "yes"}, nil
}
