package middlewares

import (
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	compactmiddleware "github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares/compact"
	securitymiddleware "github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares/security"
	usagemiddleware "github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares/usage"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/security/policy"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/context/compact"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/options"
)

// ChatModelAgentMiddlewareConfig groups dependencies for the default agent middleware chain.
// ChatModelAgentMiddlewareConfig 聚合默认 agent middleware 链所需依赖。
type ChatModelAgentMiddlewareConfig struct {
	Model              model.BaseChatModel
	ModelName          string
	MaxModelContext    int
	MaxOutputTokens    int
	CapabilityRegistry *capability.Registry
	OperationBuilders  map[string]securitycore.OperationBuilder
	Approver           securitymiddleware.Approver
	Security           *options.SecurityOptions
}

// NewChatModelAgentMiddlewares builds the default ChatModelAgent middleware chain.
// NewChatModelAgentMiddlewares 构建默认 ChatModelAgent middleware 链。
func NewChatModelAgentMiddlewares(cfg ChatModelAgentMiddlewareConfig) ([]adk.ChatModelAgentMiddleware, error) {
	compactor, err := compact.NewCompactor(compact.Config{
		Model:           cfg.Model,
		ModelName:       cfg.ModelName,
		MaxModelContext: cfg.MaxModelContext,
		MaxOutputTokens: cfg.MaxOutputTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("new context compactor: %w", err)
	}

	workspaceGuard, err := newWorkspaceGuard(cfg.Security)
	if err != nil {
		return nil, err
	}

	securityMiddleware, err := securitymiddleware.NewExecutionSecurityMiddleware(securitymiddleware.Config{
		Registry:              cfg.CapabilityRegistry,
		Policy:                policy.NewEngine(policy.Config{ApprovalDefault: securityApprovalDefault(cfg.Security)}),
		Grants:                policy.NewSessionGrants(),
		Approver:              cfg.Approver,
		Builders:              cfg.OperationBuilders,
		WorkspaceGuard:        workspaceGuard,
		CommandTimeoutSeconds: securityCommandTimeout(cfg.Security),
		MaxToolOutputBytes:    securityMaxToolOutputBytes(cfg.Security),
	})
	if err != nil {
		return nil, fmt.Errorf("new execution security middleware: %w", err)
	}

	compactMiddleware, err := compactmiddleware.NewCompactMiddleware(compactor)
	if err != nil {
		return nil, fmt.Errorf("new compact middleware: %w", err)
	}
	usageMiddleware := usagemiddleware.NewUsageMiddleware()

	return []adk.ChatModelAgentMiddleware{securityMiddleware, compactMiddleware, usageMiddleware}, nil
}

func newWorkspaceGuard(opts *options.SecurityOptions) (*securitycore.WorkspaceGuard, error) {
	var roots []string
	if opts != nil {
		roots = opts.WorkspaceRoots
	}
	guard, err := securitycore.NewWorkspaceGuard(roots)
	if err != nil {
		return nil, fmt.Errorf("new workspace guard: %w", err)
	}
	return guard, nil
}

func securityCommandTimeout(opts *options.SecurityOptions) int {
	if opts == nil {
		return options.NewSecurityOptions().CommandTimeoutSeconds
	}
	return opts.CommandTimeoutSeconds
}

func securityApprovalDefault(opts *options.SecurityOptions) string {
	if opts == nil {
		return options.NewSecurityOptions().ApprovalDefault
	}
	return opts.ApprovalDefault
}

func securityMaxToolOutputBytes(opts *options.SecurityOptions) int {
	if opts == nil {
		return options.NewSecurityOptions().MaxToolOutputBytes
	}
	return opts.MaxToolOutputBytes
}
