package messages

import (
	"fmt"

	"github.com/cloudwego/eino/schema"

	tokenutils "github.com/HappyLadySauce/HappyLadySauceCLI/pkg/utils/tokens"
)

// TrimByMessageCount keeps the latest messages up to max.
// TrimByMessageCount 按 max 保留最新消息。
func TrimByMessageCount(messages []*schema.Message, max int) []*schema.Message {
	if max <= 0 || len(messages) <= max {
		return messages
	}
	return messages[len(messages)-max:]
}

// TrimByTokenBudget removes the oldest removable messages until token usage fits the budget.
// TrimByTokenBudget 删除最旧的可移除消息，直到 token 使用量进入预算。
func TrimByTokenBudget(messages []*schema.Message, tools []*schema.ToolInfo, budget int, counter *tokenutils.TokenCounter) ([]*schema.Message, error) {
	trimmed := append([]*schema.Message(nil), messages...)
	for {
		total, err := counter.CountMessages(trimmed, tools)
		if err != nil {
			return messages, err
		}
		if total <= budget {
			return trimmed, nil
		}

		index := firstRemovableMessageIndex(trimmed)
		if index < 0 {
			return trimmed, fmt.Errorf("message token budget exceeded after trimming: estimated=%d budget=%d", total, budget)
		}
		trimmed = append(trimmed[:index], trimmed[index+1:]...)
	}
}

// LatestAssistantUsage returns token usage from the latest assistant message.
// LatestAssistantUsage 返回最新 assistant 消息中的 token usage。
func LatestAssistantUsage(messages []*schema.Message) *schema.TokenUsage {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg == nil || msg.Role != schema.Assistant {
			continue
		}
		if msg.ResponseMeta == nil {
			return nil
		}
		return msg.ResponseMeta.Usage
	}
	return nil
}

// SameMessageSlice compares two message slices by pointer identity.
// SameMessageSlice 按消息指针身份比较两个消息切片。
func SameMessageSlice(left, right []*schema.Message) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func firstRemovableMessageIndex(messages []*schema.Message) int {
	latestUser := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil && messages[i].Role == schema.User {
			latestUser = i
			break
		}
	}
	for i, msg := range messages {
		if msg == nil {
			return i
		}
		if i == latestUser {
			continue
		}
		return i
	}
	return -1
}
