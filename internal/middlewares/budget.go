package middlewares

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"
	"k8s.io/klog/v2"

	contextcommon "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common"
)

// budgetMiddleware records model-visible context budget snapshots before model calls.
// budgetMiddleware 在模型调用前记录模型可见上下文预算快照。
type budgetMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	estimator        *contextcommon.TokenEstimator
	maxContextTokens int
}

// NewBudgetMiddleware creates a read-only context budget middleware.
// NewBudgetMiddleware 创建只读的上下文预算中间件。
func NewBudgetMiddleware(estimator *contextcommon.TokenEstimator, maxContextTokens int) (adk.ChatModelAgentMiddleware, error) {
	if estimator == nil {
		return nil, errors.New("budget middleware token estimator is required")
	}
	if maxContextTokens <= 0 {
		return nil, errors.New("budget middleware max context tokens must be greater than 0")
	}
	return &budgetMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		estimator:                    estimator,
		maxContextTokens:             maxContextTokens,
	}, nil
}

// BeforeModelRewriteState estimates the current model-visible context budget.
// BeforeModelRewriteState 估算当前模型可见上下文预算。
func (m *budgetMiddleware) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	writer := contextcommon.BudgetWriterFromContext(ctx)
	if writer == nil || state == nil {
		return ctx, state, nil
	}

	budget, err := contextcommon.EstimateBudget(contextcommon.BudgetInput{
		Messages:  state.Messages,
		ToolInfos: state.ToolInfos,
	}, m.estimator, m.maxContextTokens)
	if err != nil {
		klog.Warningf("context budget estimate skipped: %v", err)
		return ctx, state, nil
	}
	writer.Write(budget)
	return ctx, state, nil
}
