package usage

import "github.com/cloudwego/eino/schema"

// UsageSnapshot is a normalized token usage snapshot for one model call.
// UsageSnapshot 表示一次模型调用的标准化 token 用量快照。
type UsageSnapshot struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// IsZero reports whether the snapshot carries any usage signal.
// IsZero 判断快照是否包含任何用量信号。
func (s UsageSnapshot) IsZero() bool {
	return s.PromptTokens <= 0 &&
		s.CompletionTokens <= 0 &&
		s.TotalTokens <= 0
}

// snapshotFromMessage extracts provider usage from an Eino message.
// snapshotFromMessage 从 Eino message 中提取服务商返回的用量。
func snapshotFromMessage(msg *schema.Message) (UsageSnapshot, bool) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return UsageSnapshot{}, false
	}
	return snapshotFromTokenUsage(msg.ResponseMeta.Usage)
}

// snapshotFromTokenUsage normalizes schema.TokenUsage.
// snapshotFromTokenUsage 标准化 schema.TokenUsage。
func snapshotFromTokenUsage(tokenUsage *schema.TokenUsage) (UsageSnapshot, bool) {
	if tokenUsage == nil {
		return UsageSnapshot{}, false
	}
	snapshot := UsageSnapshot{
		PromptTokens:     tokenUsage.PromptTokens,
		CompletionTokens: tokenUsage.CompletionTokens,
		TotalTokens:      tokenUsage.TotalTokens,
	}
	return snapshot, !snapshot.IsZero()
}
