package context

import "github.com/cloudwego/eino/schema"

// compactionBoundary splits a transcript into head, middle, and tail segments for summarization.
// compactionBoundary 将对话切分为头部、中间段和尾部，供摘要压缩使用。
type compactionBoundary struct {
	head   []*schema.Message
	middle []*schema.Message
	tail   []*schema.Message
	ok     bool
}

// selectBoundary chooses a safe head/middle/tail split without breaking tool call pairs.
// selectBoundary 选择安全的头/中/尾切分点，避免拆开 tool_call 与 tool_result。
func selectBoundary(messages []*schema.Message) compactionBoundary {
	messages = withoutSystemMessages(messages)
	if len(messages) < defaultHeadMessages+defaultTailMessages+1 {
		return compactionBoundary{}
	}

	headEnd := defaultHeadMessages
	if headEnd > len(messages) {
		headEnd = len(messages)
	}

	tailStart := len(messages) - defaultTailMessages
	if tailStart <= headEnd {
		headEnd = tailStart - 1
		if headEnd < 0 {
			headEnd = 0
		}
	}
	if tailStart < headEnd+1 {
		return compactionBoundary{}
	}
	tailStart = adjustTailStartForToolPairs(messages, tailStart)
	if tailStart <= headEnd {
		headEnd = tailStart - 1
		if headEnd < 0 {
			return compactionBoundary{}
		}
	}
	if tailStart <= headEnd {
		return compactionBoundary{}
	}

	head := cloneMessages(messages[:headEnd])
	middle := cloneMessages(messages[headEnd:tailStart])
	tail := cloneMessages(messages[tailStart:])
	if len(middle) == 0 || !hasSafeToolPairs(tail) {
		return compactionBoundary{}
	}

	return compactionBoundary{
		head:   head,
		middle: middle,
		tail:   tail,
		ok:     true,
	}
}

// withoutSystemMessages removes system messages from compaction candidates.
// ChatModelAgent injects Instruction separately, so compacted history should not duplicate it.
// withoutSystemMessages 从压缩候选中移除 system 消息。
// ChatModelAgent 会单独注入 Instruction，因此压缩后的历史不应重复携带 system 信息。
func withoutSystemMessages(messages []*schema.Message) []*schema.Message {
	filtered := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil || msg.Role == schema.System {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

// adjustTailStartForToolPairs walks tailStart backward to include parent assistant tool calls.
// adjustTailStartForToolPairs 将 tailStart 向前扩展，以包含尾部 tool_result 对应的 assistant tool_call。
func adjustTailStartForToolPairs(messages []*schema.Message, tailStart int) int {
	if tailStart < 0 {
		tailStart = 0
	}
	if tailStart >= len(messages) {
		return len(messages)
	}

	changed := true
	for changed {
		changed = false
		for i := tailStart; i < len(messages); i++ {
			msg := messages[i]
			if msg == nil || msg.Role != schema.Tool || msg.ToolCallID == "" {
				continue
			}
			callIndex := findAssistantToolCall(messages, msg.ToolCallID, i-1)
			if callIndex >= 0 && callIndex < tailStart {
				tailStart = callIndex
				changed = true
				break
			}
		}
	}

	return tailStart
}

// findAssistantToolCall locates the assistant message that owns the given tool call ID.
// findAssistantToolCall 查找拥有指定 tool call ID 的 assistant 消息索引。
func findAssistantToolCall(messages []*schema.Message, toolCallID string, before int) int {
	for i := before; i >= 0; i-- {
		msg := messages[i]
		if msg == nil || msg.Role != schema.Assistant {
			continue
		}
		for _, call := range msg.ToolCalls {
			if call.ID == toolCallID {
				return i
			}
		}
	}
	return -1
}

// hasSafeToolPairs reports whether every tool result in messages has a matching assistant call.
// hasSafeToolPairs 判断消息列表中的 tool_result 是否都有对应的 assistant tool_call。
func hasSafeToolPairs(messages []*schema.Message) bool {
	seenCalls := map[string]struct{}{}
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.Role == schema.Assistant {
			for _, call := range msg.ToolCalls {
				if call.ID != "" {
					seenCalls[call.ID] = struct{}{}
				}
			}
		}
		if msg.Role == schema.Tool && msg.ToolCallID != "" {
			if _, ok := seenCalls[msg.ToolCallID]; !ok {
				return false
			}
		}
	}
	return true
}

// cloneMessages returns a shallow copy of messages with duplicated ToolCalls and Extra maps.
// cloneMessages 浅拷贝消息列表，并复制 ToolCalls 与 Extra 字段。
func cloneMessages(messages []*schema.Message) []*schema.Message {
	if len(messages) == 0 {
		return nil
	}
	cloned := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			cloned = append(cloned, nil)
			continue
		}
		next := *msg
		if msg.ToolCalls != nil {
			next.ToolCalls = append([]schema.ToolCall(nil), msg.ToolCalls...)
		}
		if msg.Extra != nil {
			next.Extra = make(map[string]any, len(msg.Extra))
			for k, v := range msg.Extra {
				next.Extra[k] = v
			}
		}
		cloned = append(cloned, &next)
	}
	return cloned
}
