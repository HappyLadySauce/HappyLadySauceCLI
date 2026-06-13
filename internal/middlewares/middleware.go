package middlewares

import (
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	commandsandbox "github.com/HappyLadySauce/HappyLadySauceCLI/internal/execution/sandbox"
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
	WorkspaceGuard     *securitycore.WorkspaceGuard
	CommandSandbox     commandsandbox.Runner
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

	workspaceGuard, err := middlewareWorkspaceGuard(cfg)
	if err != nil {
		return nil, err
	}
	commandSandbox, err := middlewareCommandSandbox(cfg)
	if err != nil {
		return nil, err
	}

	securityMiddleware, err := securitymiddleware.NewExecutionSecurityMiddleware(securitymiddleware.Config{
		Registry:              cfg.CapabilityRegistry,
		Policy:                policy.NewEngine(),
		Grants:                policy.NewSessionGrants(),
		Approver:              cfg.Approver,
		Builders:              cfg.OperationBuilders,
		WorkspaceGuard:        workspaceGuard,
		CommandSandbox:        commandSandbox,
		CommandTimeoutSeconds: securityCommandTimeout(cfg.Security),
		FileTimeoutSeconds:    securityFileTimeout(cfg.Security),
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

func middlewareWorkspaceGuard(cfg ChatModelAgentMiddlewareConfig) (*securitycore.WorkspaceGuard, error) {
	if cfg.WorkspaceGuard != nil {
		return cfg.WorkspaceGuard, nil
	}
	// Production runtime injects the shared guard; this fallback is for isolated tests.
	// 生产 runtime 会注入共享 guard；此 fallback 仅用于孤立测试。
	return newWorkspaceGuard(cfg.Security)
}

func middlewareCommandSandbox(cfg ChatModelAgentMiddlewareConfig) (commandsandbox.Runner, error) {
	if cfg.CommandSandbox != nil {
		return cfg.CommandSandbox, nil
	}
	// Production runtime injects the shared runner; this fallback is for isolated tests.
	// 生产 runtime 会注入共享 runner；此 fallback 仅用于孤立测试。
	securityOpts := cfg.Security
	if securityOpts == nil {
		securityOpts = options.NewSecurityOptions()
	}
	return commandsandbox.NewRunner(commandsandbox.Config{
		Backend:         securityOpts.CommandSandbox.Backend,
		Network:         securityOpts.CommandSandbox.Network,
		WSLDistribution: securityOpts.CommandSandbox.WSLDistribution,
		AllowedEnvKeys:  securityOpts.CommandSandbox.AllowedEnvKeys,
		WorkspaceRoots:  securityOpts.WorkspaceRoots,
		MaxOutputBytes:  securityOpts.MaxToolOutputBytes,
	})
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

func securityFileTimeout(opts *options.SecurityOptions) int {
	if opts == nil {
		return options.NewSecurityOptions().FileOperationTimeoutSeconds
	}
	return opts.FileOperationTimeoutSeconds
}

func securityMaxToolOutputBytes(opts *options.SecurityOptions) int {
	if opts == nil {
		return options.NewSecurityOptions().MaxToolOutputBytes
	}
	return opts.MaxToolOutputBytes
}
