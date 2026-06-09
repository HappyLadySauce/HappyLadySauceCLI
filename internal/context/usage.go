package context

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	tiktoken "github.com/pkoukk/tiktoken-go"
)

// fallbackCharsPerToken is the char-to-token ratio when tiktoken has no model encoding.
// fallbackCharsPerToken 为 tiktoken 无模型编码时使用的字符/token 换算比。
const fallbackCharsPerToken = 4

// TokenEstimator estimates model-visible prompt tokens.
// TokenEstimator 估算模型可见 prompt token 数。
type TokenEstimator struct {
	// encoding is nil for unknown models; CountText then uses character fallback.
	// encoding 在未知模型时为 nil，此时 CountText 回退到字符估算。
	encoding *tiktoken.Tiktoken
}

// NewTokenEstimator creates a token estimator. Unknown models fall back to character estimation.
// NewTokenEstimator 创建 token 估算器；未知模型会回退到字符估算。
func NewTokenEstimator(modelName string) *TokenEstimator {
	encoding, err := tiktoken.EncodingForModel(modelName)
	if err != nil {
		encoding = nil
	}
	return &TokenEstimator{encoding: encoding}
}

// CountMessages estimates tokens for all messages.
// CountMessages 估算全部消息的 token 数。
func (e *TokenEstimator) CountMessages(messages []*schema.Message) int {
	total := 0
	for _, msg := range messages {
		total += e.CountMessage(msg)
	}
	return total
}

// CountMessage estimates tokens for one message.
// CountMessage 估算单条消息的 token 数。
func (e *TokenEstimator) CountMessage(msg *schema.Message) int {
	if msg == nil {
		return 0
	}
	total := 4 + e.CountText(string(msg.Role))
	total += e.CountText(msg.Name)
	total += e.CountText(msg.Content)
	total += e.CountText(msg.ReasoningContent)
	total += e.CountText(msg.ToolCallID)
	total += e.CountText(msg.ToolName)

	for _, call := range msg.ToolCalls {
		total += e.CountText(call.ID)
		total += e.CountText(call.Type)
		total += e.CountText(call.Function.Name)
		total += e.CountText(call.Function.Arguments)
	}

	if len(msg.UserInputMultiContent) > 0 {
		total += e.countJSON(msg.UserInputMultiContent)
	}
	if len(msg.AssistantGenMultiContent) > 0 {
		total += e.countJSON(msg.AssistantGenMultiContent)
	}
	if len(msg.MultiContent) > 0 {
		total += e.countJSON(msg.MultiContent)
	}

	return total
}

// CountTools estimates tokens for tool schemas.
// CountTools 估算工具 schema 的 token 数。
func (e *TokenEstimator) CountTools(tools []*schema.ToolInfo) (int, error) {
	total := 0
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		data, err := json.Marshal(tool)
		if err != nil {
			return 0, err
		}
		total += e.CountText(string(data))
	}
	return total, nil
}

// CountText estimates tokens for text.
// CountText 估算文本 token 数。
func (e *TokenEstimator) CountText(text string) int {
	if text == "" {
		return 0
	}
	if e != nil && e.encoding != nil {
		return len(e.encoding.Encode(text, nil, nil))
	}
	runes := utf8.RuneCountInString(text)
	tokens := runes / fallbackCharsPerToken
	if runes%fallbackCharsPerToken != 0 {
		tokens++
	}
	if tokens == 0 {
		return 1
	}
	return tokens
}

// countJSON estimates tokens for a JSON-serializable message part.
// countJSON 估算可 JSON 序列化消息片段的 token 数。
func (e *TokenEstimator) countJSON(value any) int {
	data, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return e.CountText(string(data))
}
