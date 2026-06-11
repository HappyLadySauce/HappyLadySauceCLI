// Package context implements Hermes-style semantic compaction for long agent conversations.
// Package context 实现 Hermes 风格的对话语义压缩。
package compact

import (
	stdcontext "context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/context/estimate"
)

const (
	// compactionTriggerPercent is the safe context budget fraction that starts compaction.
	// compactionTriggerPercent 为触发压缩的安全上下文预算比例。
	compactionTriggerPercent = 80
	// defaultHeadMessages is the head message count kept from non-system context.
	// defaultHeadMessages 为非 system 上下文保留的头部消息数。
	defaultHeadMessages = 2
	// defaultTailMessages is the minimum recent messages protected in the tail.
	// defaultTailMessages 为尾部至少保留的最近消息条数。
	defaultTailMessages = 4
	// defaultSummaryTokens caps auxiliary summary output when model max tokens is large.
	// 4k strikes a balance: enough for a detailed six-section summary of a large middle segment,
	// but not so large that the summary itself becomes a compaction trigger.
	// defaultSummaryTokens 在模型 max tokens 较大时限制辅助摘要输出上限。
	// 4k 是一个平衡点：足够容纳大规模 middle 段的六段式详细摘要，又不至于让摘要自身触发压缩。
	defaultSummaryTokens = 4096
	// minimumSummaryTokens keeps structured six-section summaries usable when max output is small.
	// minimumSummaryTokens 在 max output 较小时保证六段式摘要仍有可用输出空间。
	minimumSummaryTokens = 512
)

// ErrUnsafeBoundary indicates that compaction would break message ordering constraints.
// ErrUnsafeBoundary 表示压缩会破坏消息顺序约束。
var ErrUnsafeBoundary = errors.New("unsafe context compaction boundary")

// Config contains the internal compactor settings derived from model options.
// Config 包含从模型配置派生出的压缩器内部设置。
type Config struct {
	Model           model.BaseChatModel
	ModelName       string
	MaxModelContext int
	MaxOutputTokens int
}

// Compactor rewrites message history when context pressure is high.
// Compactor 在上下文压力过高时重写消息历史。
type Compactor struct {
	model            model.BaseChatModel
	maxContextTokens int
	maxOutputTokens  int
	estimator        *estimate.TokenEstimator
}

// NewCompactor creates a context compactor from model options.
// NewCompactor 基于模型配置创建上下文压缩器。
func NewCompactor(cfg Config) (*Compactor, error) {
	if cfg.Model == nil {
		return nil, errors.New("compactor model is required")
	}
	if cfg.MaxModelContext <= 0 {
		return nil, errors.New("max model context must be greater than 0")
	}
	if cfg.MaxOutputTokens <= 0 {
		return nil, errors.New("max output tokens must be greater than 0")
	}
	if cfg.MaxModelContext <= cfg.MaxOutputTokens {
		return nil, errors.New("max model context must be greater than max output tokens")
	}

	return &Compactor{
		model:            cfg.Model,
		maxContextTokens: cfg.MaxModelContext,
		maxOutputTokens:  cfg.MaxOutputTokens,
		estimator:        estimate.NewTokenEstimator(cfg.ModelName),
	}, nil
}

// CompactIfNeeded checks prompt pressure using the provider-reported sessionTotal,
// summarizes the middle segment when needed, and returns [head, summary, tail].
// On summary failure it returns the original messages unchanged.
// CompactIfNeeded 使用 provider 上报的 sessionTotal 检查 prompt 压力，必要时摘要中间段并返回 [头部, 摘要, 尾部]；摘要失败时原样返回输入。
func (c *Compactor) CompactIfNeeded(ctx stdcontext.Context, messages []*schema.Message, sessionTotal int) ([]*schema.Message, bool, error) {
	if c == nil {
		return messages, false, errors.New("compactor is nil")
	}
	if len(messages) == 0 {
		return messages, false, nil
	}

	if sessionTotal < c.triggerTokens() {
		return messages, false, nil
	}

	systemMessages, contextMessages := splitSystemAndContextMessages(messages)
	boundary := selectBoundary(contextMessages)
	if !boundary.ok {
		return messages, false, ErrUnsafeBoundary
	}

	middleTokens := c.estimator.CountMessages(boundary.middle)
	summary, err := c.generateSummary(ctx, boundary.middle, middleTokens)
	if err != nil {
		return messages, false, err
	}

	next := assembleCompactedMessages(systemMessages, boundary.head, summary, boundary.tail)
	return next, true, nil
}

// triggerTokens returns the session total watermark that starts compaction.
// triggerTokens 返回开始压缩的 session total 水位线。
func (c *Compactor) triggerTokens() int {
	return c.safePromptBudget() * compactionTriggerPercent / 100
}

// summaryTokenLimit bounds the auxiliary model output for middle-turn summarization.
// summaryTokenLimit 限制中间段摘要时辅助模型的输出 token 上限。
func (c *Compactor) summaryTokenLimit() int {
	limit := c.maxOutputTokens / 4
	if limit <= 0 {
		return defaultSummaryTokens
	}
	if limit < minimumSummaryTokens {
		return minimumSummaryTokens
	}
	if limit > defaultSummaryTokens {
		return defaultSummaryTokens
	}
	return limit
}

// safePromptBudget is context window minus reserved output tokens.
// safePromptBudget 为上下文窗口减去预留给输出的 token。
func (c *Compactor) safePromptBudget() int {
	return c.maxContextTokens - c.maxOutputTokens
}

// generateSummary calls the auxiliary model to produce a structured middle-turn summary.
// generateSummary 调用辅助模型生成中间段的结构化摘要。
func (c *Compactor) generateSummary(ctx stdcontext.Context, middle []*schema.Message, estimatedTokens int) (*schema.Message, error) {
	targetTokens := c.summaryTokenLimit()
	input := []*schema.Message{
		schema.SystemMessage(summarySystemPrompt),
		schema.UserMessage(summaryUserPrompt(summaryPromptInput{
			EstimatedTokens: estimatedTokens,
			TargetTokens:    targetTokens,
			Messages:        middle,
		})),
	}

	resp, err := c.model.Generate(ctx, input, model.WithMaxTokens(targetTokens))
	if err != nil {
		return nil, fmt.Errorf("generate context summary: %w", err)
	}
	if resp == nil {
		return nil, errors.New("generate context summary: empty response")
	}
	if len(resp.ToolCalls) > 0 {
		return nil, errors.New("generate context summary: model returned tool calls")
	}

	content := strings.TrimSpace(resp.Content)
	if content == "" {
		return nil, errors.New("generate context summary: empty content")
	}

	// Use a user message so later compaction passes preserve the accumulated summary.
	// 使用 user message，确保后续压缩轮次不会像 system message 一样过滤掉累计摘要。
	return schema.UserMessage(summaryPrefix + content), nil
}
