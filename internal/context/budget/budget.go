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
	PromptTokens     int     // last-hop provider prompt = final model-call input / 最后一跳 prompt（最终模型调用输入）
	CompletionTokens int     // accumulated provider completion across hops / 各跳累加 completion（本回合生成量）
	ContextTokens    int     // session context occupancy in window / 会话上下文窗口占用总量
	MaxContext       int     // model context window / 模型上下文窗口
}

// IsZero reports whether the stats carry no displayable values.
// IsZero 表示统计中没有任何可展示的数据。
func (s TurnStats) IsZero() bool {
	return s.ElapsedMs <= 0 && s.PromptTokens <= 0 && s.CompletionTokens <= 0 && s.ContextTokens <= 0
}

// TotalTokens returns session context occupancy shown as total↑↓.
// TotalTokens 返回作为 total↑↓ 展示的会话上下文占用总量。
func (s TurnStats) TotalTokens() int {
	return s.ContextTokens
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
	mu                sync.RWMutex
	stats             TurnStats
	turnStart         time.Time
	lastHopPrompt     int
	lastHopCompletion int
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
	w.lastHopCompletion = 0
}

// AddUsage records provider usage from one model hop within the current turn.
//
// prompt↑ uses only the last hop prompt (session input at the final call).
// completion↓ accumulates all hops (tokens generated this turn).
//
// AddUsage 记录同轮内单次模型调用的 provider 用量。
// prompt↑ 仅取最后一跳 prompt；completion↓ 累加各跳 completion。
func (w *BudgetWriter) AddUsage(snapshot usage.UsageSnapshot) {
	if w == nil || snapshot.IsZero() {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stats.CompletionTokens += snapshot.CompletionTokens
	if snapshot.PromptTokens > 0 {
		w.lastHopPrompt = snapshot.PromptTokens
	}
	if snapshot.CompletionTokens > 0 {
		w.lastHopCompletion = snapshot.CompletionTokens
	}
}

// FinalizeTurn closes elapsed-time tracking and stores context occupancy.
//
// total↑↓ prefers estimatedContextTokens from the post-turn model-visible state
// (reflects compaction). When unavailable, falls back to last-hop prompt + completion.
//
// FinalizeTurn 结束耗时统计并写入上下文占用。
// total↑↓ 优先使用回合结束后模型可见状态的本地估算（反映压缩）；否则回退到最后一跳 prompt+completion。
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
	w.stats.PromptTokens = w.lastHopPrompt
	if w.stats.PromptTokens == 0 {
		w.stats.PromptTokens = estimatedContextTokens
	}

	switch {
	case estimatedContextTokens > 0:
		w.stats.ContextTokens = estimatedContextTokens
	case w.lastHopPrompt > 0:
		w.stats.ContextTokens = w.lastHopPrompt + w.lastHopCompletion
	default:
		w.stats.ContextTokens = 0
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
