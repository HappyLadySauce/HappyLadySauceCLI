package compact

import "github.com/cloudwego/eino/schema"

// assembleCompactedMessages builds [system] + [head] + [summary] + [tail] without mutating the inputs.
// assembleCompactedMessages 组装 [system] + [头部] + [摘要] + [尾部]，不修改入参消息。
func assembleCompactedMessages(systemMessages, head []*schema.Message, summary *schema.Message, tail []*schema.Message) []*schema.Message {
	total := len(systemMessages) + len(head) + len(tail)
	if summary != nil {
		total++
	}
	messages := make([]*schema.Message, 0, total)
	messages = append(messages, systemMessages...)
	messages = append(messages, head...)
	if summary != nil {
		messages = append(messages, summary)
	}
	messages = append(messages, tail...)
	return messages
}
