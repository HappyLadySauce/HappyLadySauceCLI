package usage

import (
	"context"
	"sync"
)

type sessionContextKey struct{}
type skipTrackingContextKey struct{}

// SessionContext stores provider-reported session context occupancy across hops and user turns.
// SessionContext 存储跨 hop 与用户轮次的 provider 会话上下文占用。
type SessionContext struct {
	mu          sync.RWMutex
	totalTokens int
}

// NewSessionContext creates an empty session context tracker.
// NewSessionContext 创建空的会话上下文追踪器。
func NewSessionContext() *SessionContext {
	return &SessionContext{}
}

// TotalTokens returns the latest provider-reported session context occupancy.
// TotalTokens 返回最近一次 provider 报告的会话上下文占用。
func (s *SessionContext) TotalTokens() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalTokens
}

// UpdateFromSnapshot overwrites session occupancy from one model-call usage snapshot.
//
// Prefers TotalTokens; falls back to PromptTokens+CompletionTokens when TotalTokens is zero.
//
// UpdateFromSnapshot 用单次模型调用的用量快照覆盖会话占用。
// 优先使用 TotalTokens；为 0 时回退到 PromptTokens+CompletionTokens。
func (s *SessionContext) UpdateFromSnapshot(snapshot UsageSnapshot) {
	if s == nil || snapshot.IsZero() {
		return
	}
	total := snapshot.TotalTokens
	if total <= 0 {
		total = snapshot.PromptTokens + snapshot.CompletionTokens
	}
	if total <= 0 {
		return
	}
	s.mu.Lock()
	s.totalTokens = total
	s.mu.Unlock()
}

// WithSessionContext attaches a session context tracker to ctx.
// WithSessionContext 将会话上下文追踪器附加到 ctx。
func WithSessionContext(ctx context.Context, session *SessionContext) context.Context {
	if ctx == nil || session == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionContextKey{}, session)
}

// SessionFromContext returns the session context tracker attached to ctx.
// SessionFromContext 返回附加在 ctx 上的会话上下文追踪器。
func SessionFromContext(ctx context.Context) *SessionContext {
	if ctx == nil {
		return nil
	}
	session, _ := ctx.Value(sessionContextKey{}).(*SessionContext)
	return session
}

// WithSkipTracking marks ctx so UsageTrackingChatModel skips session and turn usage updates.
// WithSkipTracking 标记 ctx，使 UsageTrackingChatModel 跳过 session 与回合用量更新。
func WithSkipTracking(ctx context.Context) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, skipTrackingContextKey{}, true)
}

// skipTracking reports whether usage tracking is disabled for this ctx.
// skipTracking 判断当前 ctx 是否禁用用量追踪。
func skipTracking(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	skip, _ := ctx.Value(skipTrackingContextKey{}).(bool)
	return skip
}
