package budget

import (
	"context"
	"sync"
)

type budgetWriterContextKey struct{}

// BudgetWriter stores the latest context budget snapshot for one runner turn.
// BudgetWriter 存储单轮 runner 的最新上下文预算快照。
type BudgetWriter struct {
	mu   sync.RWMutex
	data *ContextBudget
}

// NewBudgetWriter creates an empty budget snapshot writer.
// NewBudgetWriter 创建空的预算快照写入器。
func NewBudgetWriter() *BudgetWriter {
	return &BudgetWriter{}
}

// Write stores a defensive copy of budget.
// Write 写入 budget 的防御性副本。
func (w *BudgetWriter) Write(budget *ContextBudget) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = cloneContextBudget(budget)
}

// Read returns a defensive copy of the latest budget snapshot.
// Read 返回最新预算快照的防御性副本。
func (w *BudgetWriter) Read() *ContextBudget {
	if w == nil {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return cloneContextBudget(w.data)
}

// Clear removes the latest budget snapshot.
// Clear 清除最新预算快照。
func (w *BudgetWriter) Clear() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = nil
}

// WithBudgetWriter attaches a budget writer to ctx.
// WithBudgetWriter 将预算写入器附加到 ctx。
func WithBudgetWriter(ctx context.Context, writer *BudgetWriter) context.Context {
	if ctx == nil || writer == nil {
		return ctx
	}
	return context.WithValue(ctx, budgetWriterContextKey{}, writer)
}

// BudgetWriterFromContext returns the budget writer attached to ctx.
// BudgetWriterFromContext 返回附加在 ctx 上的预算写入器。
func BudgetWriterFromContext(ctx context.Context) *BudgetWriter {
	if ctx == nil {
		return nil
	}
	writer, _ := ctx.Value(budgetWriterContextKey{}).(*BudgetWriter)
	return writer
}

func cloneContextBudget(budget *ContextBudget) *ContextBudget {
	if budget == nil {
		return nil
	}
	next := *budget
	if budget.Segments != nil {
		next.Segments = make(map[Segment]int, len(budget.Segments))
		for segment, tokens := range budget.Segments {
			next.Segments[segment] = tokens
		}
	}
	return &next
}
