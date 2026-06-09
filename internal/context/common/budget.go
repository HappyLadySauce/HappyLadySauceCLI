package common

import (
	"errors"

	"github.com/cloudwego/eino/schema"
)

// Segment identifies one model-visible context budget bucket.
// Segment 标识一个模型可见上下文预算分段。
type Segment string

const (
	// SegmentSystem is the injected system prompt segment.
	// SegmentSystem 表示注入的 system prompt 分段。
	SegmentSystem Segment = "system"
	// SegmentTools is the actual tool call and tool result message segment.
	// SegmentTools 表示实际工具调用与工具结果消息分段。
	SegmentTools Segment = "tools"
	// SegmentRules is reserved for future rule files.
	// SegmentRules 预留给未来规则文件分段。
	SegmentRules Segment = "rules"
	// SegmentSkills is reserved for future skill prompts.
	// SegmentSkills 预留给未来技能提示分段。
	SegmentSkills Segment = "skills"
	// SegmentMCP is reserved for future MCP context.
	// SegmentMCP 预留给未来 MCP 上下文分段。
	SegmentMCP Segment = "mcp"
	// SegmentSubagents is reserved for future subagent context.
	// SegmentSubagents 预留给未来 subagent 上下文分段。
	SegmentSubagents Segment = "subagents"
	// SegmentConversation is the non-system conversation history segment.
	// SegmentConversation 表示非 system 对话历史分段。
	SegmentConversation Segment = "conversation"
)

// BudgetInput groups all model-visible context parts for one estimate.
// BudgetInput 聚合一次预算估算所需的模型可见上下文。
type BudgetInput struct {
	Messages            []*schema.Message
	ToolInfos           []*schema.ToolInfo
	FallbackInstruction string
	RulesText           string
	SkillsText          string
	MCPText             string
	SubagentsText       string
}

// ContextBudget is a segmented token snapshot before one model call.
// ContextBudget 表示一次模型调用前的分段 token 快照。
type ContextBudget struct {
	MaxTokens   int
	TotalTokens int
	Segments    map[Segment]int
	PercentFull float64
}

// EstimateBudget estimates segmented model-visible prompt tokens.
// EstimateBudget 估算模型可见 prompt 的分段 token。
func EstimateBudget(input BudgetInput, estimator *TokenEstimator, maxContextTokens int) (*ContextBudget, error) {
	if estimator == nil {
		return nil, errors.New("token estimator is required")
	}

	segments := map[Segment]int{}
	systemMessages, conversationMessages, toolMessages := splitBudgetMessages(input.Messages)
	systemTokens := countMessageBodies(estimator, systemMessages)
	if systemTokens == 0 && input.FallbackInstruction != "" {
		systemTokens = estimator.CountMessage(schema.SystemMessage(input.FallbackInstruction))
	}

	conversationTokens := countMessageBodies(estimator, conversationMessages)
	toolMessageTokens := countMessageBodies(estimator, toolMessages)
	switch {
	case len(conversationMessages) > 0:
		conversationTokens += replyPrimingTokens
	case len(toolMessages) > 0:
		toolMessageTokens += replyPrimingTokens
	case systemTokens > 0:
		systemTokens += replyPrimingTokens
	}

	addSegment(segments, SegmentSystem, systemTokens)
	addSegment(segments, SegmentConversation, conversationTokens)
	addSegment(segments, SegmentTools, toolMessageTokens)

	toolDefinitionTokens, err := estimator.CountTools(input.ToolInfos)
	if err != nil {
		return nil, err
	}
	systemTokens += toolDefinitionTokens
	if toolDefinitionTokens > 0 {
		segments[SegmentSystem] = systemTokens
	}
	addSegment(segments, SegmentRules, estimator.CountText(input.RulesText))
	addSegment(segments, SegmentSkills, estimator.CountText(input.SkillsText))
	addSegment(segments, SegmentMCP, estimator.CountText(input.MCPText))
	addSegment(segments, SegmentSubagents, estimator.CountText(input.SubagentsText))

	total := 0
	for _, tokens := range segments {
		total += tokens
	}
	budget := &ContextBudget{
		MaxTokens:   maxContextTokens,
		TotalTokens: total,
		Segments:    segments,
	}
	if maxContextTokens > 0 {
		budget.PercentFull = float64(total) / float64(maxContextTokens) * 100
	}
	return budget, nil
}

// RecalculateBudgetTotals refreshes total tokens and percent from segments.
// RecalculateBudgetTotals 根据分段 token 重新计算总量和占比。
func RecalculateBudgetTotals(budget *ContextBudget) {
	if budget == nil {
		return
	}
	total := 0
	for _, tokens := range budget.Segments {
		if tokens > 0 {
			total += tokens
		}
	}
	budget.TotalTokens = total
	if budget.MaxTokens > 0 {
		budget.PercentFull = float64(total) / float64(budget.MaxTokens) * 100
		return
	}
	budget.PercentFull = 0
}

func addSegment(segments map[Segment]int, segment Segment, tokens int) {
	if tokens > 0 {
		segments[segment] = tokens
	}
}

func splitBudgetMessages(messages []*schema.Message) ([]*schema.Message, []*schema.Message, []*schema.Message) {
	systemMessages := make([]*schema.Message, 0, 1)
	conversationMessages := make([]*schema.Message, 0, len(messages))
	toolMessages := make([]*schema.Message, 0, 2)
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.Role == schema.System {
			systemMessages = append(systemMessages, msg)
			continue
		}
		if isToolInteractionMessage(msg) {
			toolMessages = append(toolMessages, msg)
			continue
		}
		conversationMessages = append(conversationMessages, msg)
	}
	return systemMessages, conversationMessages, toolMessages
}

func isToolInteractionMessage(msg *schema.Message) bool {
	if msg == nil {
		return false
	}
	return msg.Role == schema.Tool || len(msg.ToolCalls) > 0 || msg.ToolCallID != "" || msg.ToolName != ""
}

func countMessageBodies(estimator *TokenEstimator, messages []*schema.Message) int {
	total := 0
	for _, msg := range messages {
		total += estimator.CountMessage(msg)
	}
	return total
}
