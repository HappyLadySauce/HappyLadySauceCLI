package budget

import (
	"context"
	"sync"
	"time"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

type budgetWriterContextKey struct{}

// TurnStats captures per-turn latency and aggregated provider token usage.
// TurnStats 记录单轮耗时与聚合后的 provider token 用量。
type TurnStats struct {
	ElapsedMs        int64
	PromptTokens     int
	CompletionTokens int
}

// TurnStatus is the post-turn snapshot shown after the model finishes responding.
// TurnStatus 表示模型完成最终回复后用于展示的快照。
type TurnStatus struct {
	Stats  TurnStats
	Budget *ContextBudget
}

// BudgetWriter stores the latest context budget snapshot for one runner turn.
// BudgetWriter 存储单轮 runner 的最新上下文预算快照。
type BudgetWriter struct {
	mu            sync.RWMutex
	data          *ContextBudget
	turnStart     time.Time
	stats         TurnStats
	lastHopPrompt int // last model hop prompt for context occupancy / 最后一跳 prompt，用于上下文占用
}

// NewBudgetWriter creates an empty budget snapshot writer.
// NewBudgetWriter 创建空的预算快照写入器。
func NewBudgetWriter() *BudgetWriter {
	return &BudgetWriter{}
}

// BeginTurn marks the start of one runner turn for elapsed-time tracking.
// BeginTurn 标记单轮 runner 开始，用于统计耗时。
func (w *BudgetWriter) BeginTurn() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.turnStart = time.Now()
	w.stats = TurnStats{}
	w.lastHopPrompt = 0
}

// AddUsage accumulates provider usage from one model hop within the current turn.
// AddUsage 聚合同一轮内单次模型调用的 provider 用量。
func (w *BudgetWriter) AddUsage(snapshot usage.UsageSnapshot) {
	if w == nil || snapshot.IsZero() {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stats.PromptTokens += snapshot.PromptTokens
	w.stats.CompletionTokens += snapshot.CompletionTokens
	if snapshot.PromptTokens > 0 {
		// Context line uses the final hop prompt as current session occupancy.
		// Context 行以最后一跳 prompt 表示当前会话占用。
		w.lastHopPrompt = snapshot.PromptTokens
	}
}

// FinalizeTurn stores the post-turn context budget and closes elapsed-time tracking.
//
// Context line policy:
//   - Prefer the last model hop's provider prompt_tokens for window occupancy.
//   - Scale locally classified segments proportionally to that prompt total.
//   - Fall back to local estimates when provider usage is unavailable.
//
// Stats line continues to use accumulated provider usage from AddUsage.
//
// FinalizeTurn 写入回合结束后的上下文预算并结束耗时统计。
//
// Context 行策略：
//   - 优先用最后一跳 provider prompt_tokens 表示当前会话占用；
//   - 本地分类分段按比例缩放到该 prompt 总量；
//   - 无 provider 用量时回退到本地估算。
func (w *BudgetWriter) FinalizeTurn(budget *ContextBudget) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.turnStart.IsZero() {
		w.stats.ElapsedMs = time.Since(w.turnStart).Milliseconds()
	}

	if budget != nil {
		if w.lastHopPrompt > 0 && budget.EstimatedTotalTokens > 0 {
			breakdown := BreakdownFromContextBudget(budget)
			if breakdown == nil {
				breakdown = &usage.Breakdown{MaxContext: budget.MaxTokens}
			}
			breakdown.ApplyProvider(usage.UsageSnapshot{
				PromptTokens: w.lastHopPrompt,
				Source:       usage.UsageSourceProvider,
			})
			budget = breakdownToContextBudget(breakdown)
		} else {
			RecalculateBudgetTotals(budget)
			if budget.UsageSource == "" {
				budget.UsageSource = usage.UsageSourceEstimated
			}
		}
		budget.ActualCompletionTokens = w.stats.CompletionTokens
	}

	w.data = cloneContextBudget(budget)
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

// ApplyUsage merges provider token usage into the latest budget snapshot via usage.Breakdown.
// ApplyUsage 通过 usage.Breakdown 将服务商 token 用量合并进最新预算快照。
func (w *BudgetWriter) ApplyUsage(snapshot usage.UsageSnapshot) {
	if w == nil || snapshot.IsZero() {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.data == nil {
		w.data = &ContextBudget{}
	}

	breakdown := BreakdownFromContextBudget(w.data)
	if breakdown == nil {
		breakdown = &usage.Breakdown{MaxContext: w.data.MaxTokens}
	}
	breakdown.ApplyProvider(snapshot)
	w.data = breakdownToContextBudget(breakdown)
}

// ReadTurnStatus returns the post-turn stats and context budget snapshot.
// ReadTurnStatus 返回回合结束后的统计与上下文预算快照。
func (w *BudgetWriter) ReadTurnStatus() TurnStatus {
	if w == nil {
		return TurnStatus{}
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return TurnStatus{
		Stats:  w.stats,
		Budget: cloneContextBudget(w.data),
	}
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
	copy := *budget // Segs is a value type (SegmentCounts), copied by value.
	return &copy
}
