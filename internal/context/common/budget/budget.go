package budget

import (
	"errors"

	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

// BudgetInput groups all model-visible context parts for one estimate.
// BudgetInput 聚合一次预算估算所需的模型可见上下文。
type BudgetInput struct {
	Messages            []*schema.Message
	ToolInfos           []*schema.ToolInfo
	FallbackInstruction string
}

// ContextBudget is a segmented token snapshot before one model call.
// ContextBudget 表示一次模型调用前的分段 token 快照。
type ContextBudget struct {
	MaxTokens              int
	TotalTokens            int
	EstimatedTotalTokens   int
	ActualPromptTokens     int
	ActualCompletionTokens int
	CachedTokens           int
	ReasoningTokens        int
	UsageSource            string
	Segs                   usage.SegmentCounts
	PercentFull            float64
}

// EstimateBudget delegates to usage.Calculator for single-pass classified token counting.
// EstimateBudget 委托 usage.Calculator 进行单遍分类 token 计数。
func EstimateBudget(input BudgetInput, calc *usage.Calculator) (*ContextBudget, error) {
	if calc == nil {
		return nil, errors.New("token calculator is required")
	}

	breakdown := calc.Count(usage.CountInput{
		Messages:    input.Messages,
		ToolInfos:   input.ToolInfos,
		Instruction: input.FallbackInstruction,
	})

	return breakdownToContextBudget(breakdown), nil
}

// BreakdownFromContextBudget converts a stored budget snapshot into a usage breakdown.
// BreakdownFromContextBudget 将已存储的预算快照转换为 usage breakdown。
func BreakdownFromContextBudget(budget *ContextBudget) *usage.Breakdown {
	if budget == nil {
		return nil
	}
	return &usage.Breakdown{
		Segs:            budget.Segs,
		EstimatedTotal:  budget.EstimatedTotalTokens,
		ActualPrompt:    budget.ActualPromptTokens,
		ActualOutput:    budget.ActualCompletionTokens,
		CachedTokens:    budget.CachedTokens,
		ReasoningTokens: budget.ReasoningTokens,
		Source:          budget.UsageSource,
		MaxContext:      budget.MaxTokens,
	}
}

func breakdownToContextBudget(breakdown *usage.Breakdown) *ContextBudget {
	if breakdown == nil {
		return nil
	}
	return &ContextBudget{
		MaxTokens:              breakdown.MaxContext,
		TotalTokens:            breakdown.Total(),
		EstimatedTotalTokens:   breakdown.EstimatedTotal,
		ActualPromptTokens:     breakdown.ActualPrompt,
		ActualCompletionTokens: breakdown.ActualOutput,
		CachedTokens:           breakdown.CachedTokens,
		ReasoningTokens:        breakdown.ReasoningTokens,
		UsageSource:            breakdown.Source,
		Segs:                   breakdown.Segs,
		PercentFull:            breakdown.PercentUsed(),
	}
}

// RecalculateBudgetTotals refreshes total tokens and percent from segments.
// RecalculateBudgetTotals 根据分段 token 重新计算总量和占比。
func RecalculateBudgetTotals(budget *ContextBudget) {
	if budget == nil {
		return
	}
	budget.TotalTokens = budget.Segs.Total()
	if budget.MaxTokens > 0 {
		budget.PercentFull = float64(budget.TotalTokens) / float64(budget.MaxTokens) * 100
		return
	}
	budget.PercentFull = 0
}
