package middlewares

import (
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"

	compactmiddleware "github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares/compact"
	usagemiddleware "github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares/usage"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/context/compact"
)

// ChatModelAgentMiddlewareConfig groups dependencies for the default agent middleware chain.
// ChatModelAgentMiddlewareConfig 聚合默认 agent middleware 链所需依赖。
type ChatModelAgentMiddlewareConfig struct {
	Model           model.BaseChatModel
	ModelName       string
	MaxModelContext int
	MaxOutputTokens int
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

	compactMiddleware, err := compactmiddleware.NewCompactMiddleware(compactor)
	if err != nil {
		return nil, fmt.Errorf("new compact middleware: %w", err)
	}
	usageMiddleware := usagemiddleware.NewUsageMiddleware()

	return []adk.ChatModelAgentMiddleware{compactMiddleware, usageMiddleware}, nil
}
