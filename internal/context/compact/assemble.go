package compact

import "github.com/cloudwego/eino/schema"

// assembleCompactedMessages builds [head] + [summary] + [tail] without mutating the inputs.
// assembleCompactedMessages 组装 [头部] + [摘要] + [尾部]，不修改入参消息。
func assembleCompactedMessages(head []*schema.Message, summary *schema.Message, tail []*schema.Message) []*schema.Message {
	total := len(head) + len(tail)
	if summary != nil {
		total++
	}
	messages := make([]*schema.Message, 0, total)
	messages = append(messages, head...)
	if summary != nil {
		messages = append(messages, summary)
	}
	messages = append(messages, tail...)
	return messages
}
