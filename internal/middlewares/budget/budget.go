package budget

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"
	"k8s.io/klog/v2"

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

// budgetMiddleware records model-visible context budget snapshots before model calls.
// budgetMiddleware 在模型调用前记录模型可见上下文预算快照。
type budgetMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	calculator *usage.Calculator
}

// NewBudgetMiddleware creates a read-only context budget middleware.
// NewBudgetMiddleware 创建只读的上下文预算中间件。
func NewBudgetMiddleware(calculator *usage.Calculator) (adk.ChatModelAgentMiddleware, error) {
	if calculator == nil {
		return nil, errors.New("budget middleware token calculator is required")
	}
	return &budgetMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		calculator:                   calculator,
	}, nil
}

// BeforeModelRewriteState estimates the current model-visible context budget.
// BeforeModelRewriteState 估算当前模型可见上下文预算。
func (m *budgetMiddleware) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	writer := contextbudget.BudgetWriterFromContext(ctx)
	if writer == nil || state == nil {
		return ctx, state, nil
	}

	budget, err := contextbudget.EstimateBudget(contextbudget.BudgetInput{
		Messages:  state.Messages,
		ToolInfos: state.ToolInfos,
	}, m.calculator)
	if err != nil {
		klog.Warningf("context budget estimate skipped: %v", err)
		return ctx, state, nil
	}
	writer.Write(budget)
	return ctx, state, nil
}
