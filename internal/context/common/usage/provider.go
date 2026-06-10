package usage

import "github.com/cloudwego/eino/schema"

const (
	// UsageSourceProvider marks token usage returned by the model provider.
	// UsageSourceProvider 表示 token 用量来自模型服务商返回值。
	UsageSourceProvider = "provider"
	// UsageSourceEstimated marks token usage estimated locally.
	// UsageSourceEstimated 表示 token 用量来自本地估算。
	UsageSourceEstimated = "estimated"
)

// UsageSnapshot is a normalized token usage snapshot for one model call.
// UsageSnapshot 表示一次模型调用的标准化 token 用量快照。
type UsageSnapshot struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
	ReasoningTokens  int
	Source           string
}

// IsZero reports whether the snapshot carries any usage signal.
// IsZero 判断快照是否包含任何用量信号。
func (s UsageSnapshot) IsZero() bool {
	return s.PromptTokens <= 0 &&
		s.CompletionTokens <= 0 &&
		s.TotalTokens <= 0 &&
		s.CachedTokens <= 0 &&
		s.ReasoningTokens <= 0
}

// SnapshotFromMessage extracts provider usage from an Eino message.
// SnapshotFromMessage 从 Eino message 中提取服务商返回的用量。
func SnapshotFromMessage(msg *schema.Message) (UsageSnapshot, bool) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return UsageSnapshot{}, false
	}
	return SnapshotFromTokenUsage(msg.ResponseMeta.Usage, UsageSourceProvider)
}

// SnapshotFromTokenUsage normalizes schema.TokenUsage.
// SnapshotFromTokenUsage 标准化 schema.TokenUsage。
func SnapshotFromTokenUsage(tokenUsage *schema.TokenUsage, source string) (UsageSnapshot, bool) {
	if tokenUsage == nil {
		return UsageSnapshot{}, false
	}
	if source == "" {
		source = UsageSourceProvider
	}
	snapshot := UsageSnapshot{
		PromptTokens:     tokenUsage.PromptTokens,
		CompletionTokens: tokenUsage.CompletionTokens,
		TotalTokens:      tokenUsage.TotalTokens,
		CachedTokens:     tokenUsage.PromptTokenDetails.CachedTokens,
		ReasoningTokens:  tokenUsage.CompletionTokensDetails.ReasoningTokens,
		Source:           source,
	}
	return snapshot, !snapshot.IsZero()
}
