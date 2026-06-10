// Package usage provides refined token counting that combines local estimation with
// provider-reported usage, while classifying tokens by context segment in a single pass.
//
// Package usage 提供精细化 token 计算，结合本地估算与 provider 用量，并在单次遍历中按上下文分段分类。
package usage

import (
	"math"

	"github.com/cloudwego/eino/schema"
)

// Segment identifies a model-visible context budget bucket.
// Segment 标识一个模型可见上下文预算分段。
type Segment string

const (
	// SegmentSystem is the system prompt and instruction segment.
	// SegmentSystem 表示 system prompt 与 instruction 分段。
	SegmentSystem Segment = "system"
	// SegmentConversation is user/assistant dialogue without tool calls.
	// SegmentConversation 表示不含 tool call 的 user/assistant 对话。
	SegmentConversation Segment = "conversation"
	// SegmentTools is tool schemas (ToolInfos) plus ReAct tool trace messages.
	// SegmentTools 表示工具 schema 与 ReAct 工具轨迹消息。
	SegmentTools Segment = "tools"
)

// SegmentCounts holds token counts for the three model-visible context categories.
// SegmentCounts 保存三个模型可见上下文分类的 token 计数。
type SegmentCounts struct {
	System       int // system prompt + instruction / 系统指令
	Conversation int // user/assistant dialogue / 对话
	Tools        int // tool schemas + tool trace messages / 工具定义与轨迹
}

// IsZero reports whether all segment counts are zero.
// IsZero 判断所有分段计数是否为零。
func (s SegmentCounts) IsZero() bool {
	return s.System <= 0 && s.Conversation <= 0 && s.Tools <= 0
}

// Total returns the sum of all segment counts.
// Total 返回所有分段计数的总和。
func (s SegmentCounts) Total() int {
	total := 0
	if s.System > 0 {
		total += s.System
	}
	if s.Conversation > 0 {
		total += s.Conversation
	}
	if s.Tools > 0 {
		total += s.Tools
	}
	return total
}

// Breakdown is a categorized token snapshot for one model call.
// Segments are scaled proportionally when provider usage is applied.
// Breakdown 表示一次模型调用的分类 token 快照；当 provider 用量应用后分段按比例缩放。
type Breakdown struct {
	Segs            SegmentCounts // per-category token counts（有 provider 时已缩放）/ 各分段 token
	EstimatedTotal  int           // original estimated total before scaling / 缩放前原始估算总量
	ActualPrompt    int           // provider reported prompt tokens / provider 回报 prompt tokens
	ActualOutput    int           // provider reported completion tokens / provider 回报 completion tokens
	CachedTokens    int           // provider reported cached prompt tokens / provider 回报缓存 prompt tokens
	ReasoningTokens int           // provider reported reasoning tokens / provider 回报推理 tokens
	Source          string        // "provider" | "estimated" / 数据来源
	MaxContext      int           // model context window ceiling / 模型上下文窗口上限
}

// Total returns the best available token count: provider actual if available, otherwise estimated.
// Total 返回最佳可用 token 数：优先 provider 实际值，否则用估算值。
func (b *Breakdown) Total() int {
	if b == nil {
		return 0
	}
	if b.ActualPrompt > 0 {
		return b.ActualPrompt
	}
	return b.EstimatedTotal
}

// PercentUsed returns the percentage of the context window that is consumed.
// PercentUsed 返回上下文窗口使用百分比。
func (b *Breakdown) PercentUsed() float64 {
	if b == nil || b.MaxContext <= 0 {
		return 0
	}
	return float64(b.Total()) / float64(b.MaxContext) * 100
}

// IsZero reports whether the breakdown carries any token signal.
// IsZero 判断 breakdown 是否包含任何 token 信号。
func (b *Breakdown) IsZero() bool {
	if b == nil {
		return true
	}
	return b.EstimatedTotal <= 0 && b.ActualPrompt <= 0 && b.ActualOutput <= 0
}

// ApplyProvider merges provider usage into the breakdown.
// Segments are scaled proportionally: scale = PromptTokens / EstimatedTotal.
//
// ApplyProvider 将 provider 用量合并到 breakdown。
// 分段按比例缩放：scale = PromptTokens / EstimatedTotal。
func (b *Breakdown) ApplyProvider(s UsageSnapshot) {
	if b == nil || s.IsZero() {
		return
	}
	b.ActualPrompt = s.PromptTokens
	b.ActualOutput = s.CompletionTokens
	b.CachedTokens = s.CachedTokens
	b.ReasoningTokens = s.ReasoningTokens
	if s.Source != "" {
		b.Source = s.Source
	}
	if b.EstimatedTotal <= 0 || s.PromptTokens <= 0 {
		return
	}
	b.Segs = scaleSegmentCounts(b.Segs, b.EstimatedTotal, s.PromptTokens)
}

// scaleSegmentCounts proportionally scales segment counts and absorbs rounding
// remainder into the segment with the largest value.
//
// scaleSegmentCounts 按比例缩放各分段计数，舍入余量分配到值最大的分段。
func scaleSegmentCounts(counts SegmentCounts, estimatedTotal, targetTotal int) SegmentCounts {
	if counts.IsZero() || estimatedTotal <= 0 || targetTotal <= 0 {
		return counts
	}

	scale := float64(targetTotal) / float64(estimatedTotal)

	// Collect non-zero fields with their scaled values.
	type entry struct {
		ptr    *int
		raw    float64
		value  int
	}
	entries := make([]entry, 0, 3)
	add := func(ptr *int, val int) {
		if val <= 0 {
			return
		}
		raw := float64(val) * scale
		rounded := int(math.Round(raw))
		if rounded < 1 {
			rounded = 1
		}
		entries = append(entries, entry{ptr: ptr, raw: raw, value: rounded})
	}
	add(&counts.System, counts.System)
	add(&counts.Conversation, counts.Conversation)
	add(&counts.Tools, counts.Tools)

	if len(entries) == 0 {
		return counts
	}

	sum := 0
	for _, e := range entries {
		sum += e.value
	}
	delta := targetTotal - sum

	// Absorb rounding delta into the entry with the largest current value.
	if delta != 0 {
		best := 0
		for i := 1; i < len(entries); i++ {
			if entries[i].value > entries[best].value {
				best = i
			}
		}
		entries[best].value += delta
	}

	for _, e := range entries {
		*e.ptr = e.value
	}
	return counts
}

// Calculator combines local token estimation with per-message classification.
// Calculator 组合本地 token 估算与逐消息分类。
type Calculator struct {
	estimator  *TokenEstimator
	maxContext int
}

// NewCalculator creates a token calculator with the given model and context window.
// NewCalculator 基于模型名和上下文窗口创建 token 计算器。
func NewCalculator(modelName string, maxContextTokens int) *Calculator {
	return &Calculator{
		estimator:  NewTokenEstimator(modelName),
		maxContext: maxContextTokens,
	}
}

// Estimator returns the underlying local token estimator.
// Estimator 返回底层本地 token 估算器。
func (c *Calculator) Estimator() *TokenEstimator {
	if c == nil {
		return nil
	}
	return c.estimator
}

// CountInput groups all model-visible context parts for a single count pass.
// CountInput 聚合一次计数所需的模型可见上下文。
type CountInput struct {
	Messages            []*schema.Message
	ToolInfos           []*schema.ToolInfo
	DeferredToolInfos   []*schema.ToolInfo
	Instruction         string
}

// Count estimates and classifies tokens in a single pass through messages.
//
// Classification aligns with ChatModelAgent BeforeModelRewriteState inputs:
//   - system messages + Instruction → Segs.System
//   - non-system, non-tool messages → Segs.Conversation
//   - tool trace + ToolInfos + DeferredToolInfos → Segs.Tools
//
// Count 单次遍历消息，同时完成估算与分类。
func (c *Calculator) Count(input CountInput) *Breakdown {
	if c == nil || c.estimator == nil {
		return &Breakdown{MaxContext: c.maxContext}
	}

	var segs SegmentCounts
	estimatedTotal := 0

	for _, msg := range input.Messages {
		if msg == nil {
			continue
		}
		tokens := c.estimator.CountMessage(msg)
		if tokens <= 0 {
			continue
		}
		estimatedTotal += tokens
		switch {
		case msg.Role == schema.System:
			segs.System += tokens
		case msg.Role == schema.Tool || len(msg.ToolCalls) > 0:
			segs.Tools += tokens
		default:
			segs.Conversation += tokens
		}
	}

	// ToolInfos and DeferredToolInfos are model-visible on every call.
	if len(input.ToolInfos) > 0 || len(input.DeferredToolInfos) > 0 {
		if toolTokens, err := c.estimator.CountModelToolContext(input.ToolInfos, input.DeferredToolInfos); err == nil && toolTokens > 0 {
			segs.Tools += toolTokens
			estimatedTotal += toolTokens
		}
	}

	// Instruction injected as system context.
	if input.Instruction != "" {
		instTokens := c.estimator.CountText(input.Instruction)
		if instTokens > 0 {
			segs.System += instTokens
			estimatedTotal += instTokens
		}
	}

	// Reply priming goes to the largest segment (chat framing priority).
	if estimatedTotal > 0 {
		segs = addReplyPriming(segs)
		estimatedTotal += ReplyPrimingTokens
	}

	return &Breakdown{
		Segs:           segs,
		EstimatedTotal: estimatedTotal,
		Source:         UsageSourceEstimated,
		MaxContext:     c.maxContext,
	}
}

// addReplyPriming allocates reply priming overhead to the largest segment.
// addReplyPriming 将 reply priming 加到值最大的分段上。
func addReplyPriming(segs SegmentCounts) SegmentCounts {
	switch {
	case segs.Conversation >= segs.Tools && segs.Conversation >= segs.System:
		segs.Conversation += ReplyPrimingTokens
	case segs.Tools >= segs.System:
		segs.Tools += ReplyPrimingTokens
	default:
		segs.System += ReplyPrimingTokens
	}
	return segs
}
