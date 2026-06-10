package budget

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

// budgetMiddleware records post-turn API usage and context occupancy.
// budgetMiddleware 在模型完成最终回复后记录 API 用量与上下文占用。
type budgetMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	maxContext int
	estimator  *usage.TokenEstimator
}

// NewBudgetMiddleware creates a read-only turn stats middleware.
// NewBudgetMiddleware 创建只读的回合统计中间件。
func NewBudgetMiddleware(maxContext int, estimator *usage.TokenEstimator) (adk.ChatModelAgentMiddleware, error) {
	if maxContext <= 0 {
		return nil, errors.New("budget middleware max context must be greater than 0")
	}
	if estimator == nil {
		return nil, errors.New("budget middleware token estimator is required")
	}
	return &budgetMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		maxContext:                   maxContext,
		estimator:                    estimator,
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

// AfterModelRewriteState accumulates provider usage from each model hop.
// AfterModelRewriteState 聚合同一轮内每次模型调用的 provider 用量。
func (m *budgetMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	writer := budget.BudgetWriterFromContext(ctx)
	if writer == nil || state == nil {
		return ctx, state, nil
	}
	if msg := lastAssistantMessage(state.Messages); msg != nil {
		if snapshot, ok := usage.SnapshotFromMessage(msg); ok {
			writer.AddUsage(snapshot)
		}
	}
	return ctx, state, nil
}

// AfterAgent estimates context occupancy and finalizes the post-turn snapshot.
// AfterAgent 估算上下文占用并完成回合结束快照。
func (m *budgetMiddleware) AfterAgent(ctx context.Context, state *adk.ChatModelAgentState) (context.Context, error) {
	writer := budget.BudgetWriterFromContext(ctx)
	if writer == nil || state == nil {
		return ctx, nil
	}

	estimated, _ := m.estimator.EstimateVisiblePromptTokens(
		state.Messages,
		state.ToolInfos,
		state.DeferredToolInfos,
	)
	writer.FinalizeTurn(m.maxContext, estimated)
	return ctx, nil
}

func lastAssistantMessage(messages []*schema.Message) *schema.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg != nil && msg.Role == schema.Assistant {
			return msg
		}
	}
	return nil
}
