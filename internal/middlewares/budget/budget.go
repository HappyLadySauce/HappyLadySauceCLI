package budget

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"k8s.io/klog/v2"

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

// budgetMiddleware records post-turn context and provider usage after the model finishes.
// budgetMiddleware 在模型完成最终回复后记录上下文分段与 provider 用量。
type budgetMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	calculator  *usage.Calculator
	instruction string
}

// NewBudgetMiddleware creates a read-only context budget middleware.
// NewBudgetMiddleware 创建只读的上下文预算中间件。
func NewBudgetMiddleware(calculator *usage.Calculator, instruction string) (adk.ChatModelAgentMiddleware, error) {
	if calculator == nil {
		return nil, errors.New("budget middleware token calculator is required")
	}
	return &budgetMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		calculator:                   calculator,
		instruction:                  instruction,
	}, nil
}

// BeforeAgent starts per-turn timing and usage aggregation.
// BeforeAgent 开始单轮耗时与用量聚合。
func (m *budgetMiddleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	if writer := contextbudget.BudgetWriterFromContext(ctx); writer != nil {
		writer.BeginTurn()
	}
	return ctx, runCtx, nil
}

// AfterModelRewriteState accumulates provider usage from each model hop.
// AfterModelRewriteState 聚合同一轮内每次模型调用的 provider 用量。
func (m *budgetMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	writer := contextbudget.BudgetWriterFromContext(ctx)
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

// AfterAgent estimates the final model-visible context after the turn completes.
// AfterAgent 在回合结束后估算最终模型可见上下文。
func (m *budgetMiddleware) AfterAgent(ctx context.Context, state *adk.ChatModelAgentState) (context.Context, error) {
	writer := contextbudget.BudgetWriterFromContext(ctx)
	if writer == nil || state == nil {
		return ctx, nil
	}

	budget, err := contextbudget.EstimateBudget(contextbudget.BudgetInput{
		Messages:          state.Messages,
		ToolInfos:         state.ToolInfos,
		DeferredToolInfos: state.DeferredToolInfos,
		FallbackInstruction: m.instruction,
	}, m.calculator)
	if err != nil {
		klog.Warningf("context budget estimate skipped: %v", err)
		return ctx, nil
	}
	contextbudget.RecalculateBudgetTotals(budget)
	writer.FinalizeTurn(budget)
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
