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

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/prompts"
)

const (
	// compactionTriggerPercent is the prompt budget fraction that starts compaction.
	// compactionTriggerPercent 为触发压缩的 prompt 预算比例。
	compactionTriggerPercent = 80
	// defaultHeadMessages is the non-system head message count kept after system messages are filtered.
	// defaultHeadMessages 为过滤 system 消息后保留的头部消息数。
	defaultHeadMessages = 2
	// defaultTailMessages is the minimum recent messages protected in the tail.
	// defaultTailMessages 为尾部至少保留的最近消息条数。
	defaultTailMessages = 4
	// defaultSummaryTokens caps auxiliary summary output when model max tokens is large.
	// defaultSummaryTokens 在模型 max tokens 较大时限制辅助摘要输出上限。
	defaultSummaryTokens = 2048
	// minimumSummaryTokens keeps structured six-section summaries usable when max output is small.
	// minimumSummaryTokens 在 max output 较小时保证六段式摘要仍有可用输出空间。
	minimumSummaryTokens = 256
)

// ErrUnsafeBoundary indicates that compaction would break message ordering constraints.
// ErrUnsafeBoundary 表示压缩会破坏消息顺序约束。
var ErrUnsafeBoundary = errors.New("unsafe context compaction boundary")

// Config contains the internal compactor settings derived from model options.
// Config 包含从模型配置派生出的压缩器内部设置。
type Config struct {
	Model           model.BaseChatModel
	ModelName       string
	Instruction     string
	MaxModelContext int
	MaxOutputTokens int
}

// Compactor rewrites message history when context pressure is high.
// Compactor 在上下文压力过高时重写消息历史。
type Compactor struct {
	model             model.BaseChatModel
	estimator         *common.TokenEstimator
	instructionTokens int
	maxContextTokens  int
	maxOutputTokens   int
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

	estimator := common.NewTokenEstimator(cfg.ModelName)
	return &Compactor{
		model:             cfg.Model,
		estimator:         estimator,
		instructionTokens: estimator.CountInstruction(cfg.Instruction),
		maxContextTokens:  cfg.MaxModelContext,
		maxOutputTokens:   cfg.MaxOutputTokens,
	}, nil
}

// CompactIfNeeded checks prompt pressure, summarizes the middle segment when needed,
// and returns [head, summary, tail]. On summary failure it returns the original messages unchanged.
// CompactIfNeeded 检查 prompt 压力，必要时摘要中间段并返回 [头部, 摘要, 尾部]；摘要失败时原样返回输入。
func (c *Compactor) CompactIfNeeded(ctx stdcontext.Context, messages []*schema.Message, tools []*schema.ToolInfo) ([]*schema.Message, bool, error) {
	if c == nil {
		return messages, false, errors.New("compactor is nil")
	}
	if len(messages) == 0 {
		return messages, false, nil
	}

	totalTokens, err := c.estimateTotalTokens(messages, tools)
	if err != nil {
		return messages, false, err
	}
	if totalTokens < c.triggerTokens() {
		return messages, false, nil
	}

	boundary := selectBoundary(messages)
	if !boundary.ok {
		return messages, false, ErrUnsafeBoundary
	}

	middleTokens := c.estimator.CountMessages(boundary.middle)
	summary, err := c.generateSummary(ctx, boundary.middle, middleTokens)
	if err != nil {
		return messages, false, err
	}

	next := assembleCompactedMessages(boundary.head, summary, boundary.tail)
	return next, true, nil
}

// estimateTotalTokens sums message and tool-schema tokens for compaction decisions.
// estimateTotalTokens 汇总消息与工具 schema token，用于压缩决策。
func (c *Compactor) estimateTotalTokens(messages []*schema.Message, tools []*schema.ToolInfo) (int, error) {
	total := c.instructionTokens + c.estimator.CountMessages(messages)
	toolTokens, err := c.estimator.CountTools(tools)
	if err != nil {
		return 0, fmt.Errorf("estimate tool tokens: %w", err)
	}
	return total + toolTokens, nil
}

// triggerTokens returns the prompt token watermark that starts compaction.
// triggerTokens 返回开始压缩的 prompt token 水位线。
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
		schema.SystemMessage(prompts.ContextCompactionSystemPrompt),
		schema.UserMessage(prompts.ContextCompactionUserPrompt(prompts.ContextCompactionPromptInput{
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
	return schema.UserMessage(prompts.ContextCompactionSummaryPrefix + content), nil
}
