package budget

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

// budgetMiddleware records post-turn API usage and context occupancy.
// budgetMiddleware 在模型完成最终回复后记录 API 用量与上下文占用。
type budgetMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	maxContext int
}

// NewBudgetMiddleware creates a read-only turn stats middleware.
// NewBudgetMiddleware 创建只读的回合统计中间件。
func NewBudgetMiddleware(maxContext int) (adk.ChatModelAgentMiddleware, error) {
	if maxContext <= 0 {
		return nil, errors.New("budget middleware max context must be greater than 0")
	}
	return &budgetMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		maxContext:                   maxContext,
	}, nil
}

// BeforeAgent starts per-turn timing and usage aggregation.
// BeforeAgent 开始单轮耗时与用量聚合。
func (m *budgetMiddleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	if writer := budget.BudgetWriterFromContext(ctx); writer != nil {
		writer.BeginTurn()
	}
	return ctx, runCtx, nil
}

// AfterAgent finalizes the post-turn snapshot from provider session total.
// AfterAgent 根据 provider session total 完成回合结束快照。
func (m *budgetMiddleware) AfterAgent(ctx context.Context, state *adk.ChatModelAgentState) (context.Context, error) {
	writer := budget.BudgetWriterFromContext(ctx)
	if writer == nil {
		return ctx, nil
	}

	sessionTotal := 0
	if session := usage.SessionFromContext(ctx); session != nil {
		sessionTotal = session.TotalTokens()
	}
	writer.FinalizeTurn(m.maxContext, sessionTotal)
	return ctx, nil
}
