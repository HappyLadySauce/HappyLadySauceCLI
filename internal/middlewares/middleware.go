package middlewares

import (
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	compactmiddleware "github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares/compact"
	securitymiddleware "github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares/security"
	usagemiddleware "github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares/usage"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/security/policy"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/context/compact"
)

// ChatModelAgentMiddlewareConfig groups dependencies for the default agent middleware chain.
// ChatModelAgentMiddlewareConfig 聚合默认 agent middleware 链所需依赖。
type ChatModelAgentMiddlewareConfig struct {
	Model              model.BaseChatModel
	ModelName          string
	MaxModelContext    int
	MaxOutputTokens    int
	CapabilityRegistry *capability.Registry
	Approver           securitymiddleware.Approver
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

	securityMiddleware, err := securitymiddleware.NewExecutionSecurityMiddleware(securitymiddleware.Config{
		Registry: cfg.CapabilityRegistry,
		Policy:   policy.NewEngine(),
		Grants:   policy.NewSessionGrants(),
		Approver: cfg.Approver,
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
