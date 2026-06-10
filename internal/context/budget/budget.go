// Package budget provides per-turn token usage and context occupancy tracking.
//
// Package budget 提供单轮 token 用量与上下文占用追踪。
package budget

import (
	"context"
	"sync"
	"time"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

type budgetWriterContextKey struct{}

// TurnStats captures post-turn latency, API usage, and context window occupancy.
// TurnStats 记录回合结束后的耗时、API 用量与上下文窗口占用。
type TurnStats struct {
	ElapsedMs        int64
	PromptTokens     int     // accumulated provider prompt across hops / 各跳累加 prompt
	CompletionTokens int     // accumulated provider completion across hops / 各跳累加 completion
	ContextTokens    int     // last-hop provider prompt, or local estimate / 最后一跳 prompt 或本地估算
	MaxContext       int     // model context window / 模型上下文窗口
}

// IsZero reports whether the stats carry no displayable values.
// IsZero 表示统计中没有任何可展示的数据。
func (s TurnStats) IsZero() bool {
	return s.ElapsedMs <= 0 && s.PromptTokens <= 0 && s.CompletionTokens <= 0 && s.ContextTokens <= 0
}

// PercentUsed returns the percentage of MaxContext consumed by ContextTokens.
// PercentUsed 返回 ContextTokens 占 MaxContext 的百分比。
func (s TurnStats) PercentUsed() float64 {
	if s.MaxContext <= 0 || s.ContextTokens <= 0 {
		return 0
	}
	return float64(s.ContextTokens) / float64(s.MaxContext) * 100
}

// BudgetWriter stores the latest turn snapshot for one runner turn.
// BudgetWriter 存储单轮 runner 的最新回合快照。
type BudgetWriter struct {
	mu            sync.RWMutex
	stats         TurnStats
	turnStart     time.Time
	lastHopPrompt int
}

// NewBudgetWriter creates an empty budget writer.
// NewBudgetWriter 创建空的预算写入器。
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
		w.lastHopPrompt = snapshot.PromptTokens
	}
}

// FinalizeTurn closes elapsed-time tracking and stores context occupancy.
//
// Context occupancy prefers the last model hop's provider prompt_tokens.
// When provider usage is unavailable, estimatedContextTokens is used instead.
//
// FinalizeTurn 结束耗时统计并写入上下文占用。
// 优先使用最后一跳 provider prompt_tokens；无 provider 时回退到 estimatedContextTokens。
func (w *BudgetWriter) FinalizeTurn(maxContext, estimatedContextTokens int) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.turnStart.IsZero() {
		w.stats.ElapsedMs = time.Since(w.turnStart).Milliseconds()
	}
	w.stats.MaxContext = maxContext
	w.stats.ContextTokens = w.lastHopPrompt
	if w.stats.ContextTokens == 0 {
		w.stats.ContextTokens = estimatedContextTokens
	}
}

// ReadTurnStatus returns the post-turn snapshot.
// ReadTurnStatus 返回回合结束后的快照。
func (w *BudgetWriter) ReadTurnStatus() TurnStats {
	if w == nil {
		return TurnStats{}
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.stats
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
