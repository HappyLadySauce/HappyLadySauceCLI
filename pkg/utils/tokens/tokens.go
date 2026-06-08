package tokens

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/cloudwego/eino/schema"
	tiktoken "github.com/pkoukk/tiktoken-go"
)

const fallbackEncoding = "cl100k_base"

// TokenCounter estimates message and tool prompt tokens with a tiktoken encoding.
// TokenCounter 使用 tiktoken encoding 估算消息和工具 prompt token。
type TokenCounter struct {
	encoding *tiktoken.Tiktoken
}

// NewTokenCounter creates a tokenizer-backed token counter.
// NewTokenCounter 创建基于 tokenizer 的 token 计数器。
func NewTokenCounter(modelName, tokenizerModel string) (*TokenCounter, error) {
	name := strings.TrimSpace(tokenizerModel)
	if name == "" {
		name = strings.TrimSpace(modelName)
	}

	var encoding *tiktoken.Tiktoken
	var err error
	if name != "" {
		encoding, err = tiktoken.EncodingForModel(name)
	}
	if encoding == nil || err != nil {
		encoding, err = tiktoken.GetEncoding(fallbackEncoding)
	}
	if err != nil {
		return nil, err
	}
	return &TokenCounter{encoding: encoding}, nil
}

// CountMessages estimates prompt tokens for messages and tool definitions.
// CountMessages 估算消息和工具定义的 prompt token。
func (c *TokenCounter) CountMessages(messages []*schema.Message, tools []*schema.ToolInfo) (int, error) {
	if c == nil || c.encoding == nil {
		return 0, errors.New("token counter is not initialized")
	}

	total := 0
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		total += 4
		total += c.countText(string(msg.Role))
		total += c.countText(msg.Name)
		total += c.countText(msg.Content)
		total += c.countText(msg.ReasoningContent)
		total += c.countText(msg.ToolCallID)
		total += c.countText(msg.ToolName)
		for _, part := range msg.MultiContent {
			total += c.countJSON(part)
		}
		for _, part := range msg.UserInputMultiContent {
			total += c.countJSON(part)
		}
		for _, part := range msg.AssistantGenMultiContent {
			total += c.countJSON(part)
		}
		for _, call := range msg.ToolCalls {
			total += c.countJSON(call)
		}
	}
	for _, tool := range tools {
		total += c.countJSON(tool)
	}
	return total, nil
}

func (c *TokenCounter) countText(text string) int {
	if text == "" {
		return 0
	}
	return len(c.encoding.Encode(text, nil, nil))
}

func (c *TokenCounter) countJSON(value any) int {
	data, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return c.countText(string(data))
}
