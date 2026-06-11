package usage

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/estimate"
	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
)

// TokenEstimator estimates model-visible prompt tokens for compaction summaries.
// TokenEstimator 估算模型可见 prompt token，供压缩摘要提示使用。
type TokenEstimator = estimate.TokenEstimator

// NewTokenEstimator creates a token estimator from the API model name.
// NewTokenEstimator 基于 API 模型名创建 token 估算器。
func NewTokenEstimator(modelName string) *TokenEstimator {
	return estimate.NewTokenEstimator(modelName)
}

// UsageSnapshot is a normalized provider token usage snapshot for one model call.
// UsageSnapshot 表示一次模型调用的标准化 provider token 用量快照。
type UsageSnapshot struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Store persists session hierarchy snapshots.
// Store 持久化 session 层次快照。
type Store interface {
	SaveSession(ctx context.Context, session *contextmodel.Session) error
	SaveConversation(ctx context.Context, conversation *contextmodel.Conversation) error
	Close() error
}

type sessionContextKey struct{}
type conversationRecorderContextKey struct{}
type skipTrackingContextKey struct{}

// SessionOption configures a SessionContext.
// SessionOption 配置 SessionContext。
type SessionOption func(*SessionContext)

// WithStore attaches a durable store to the session context.
// WithStore 为 session context 附加持久化存储。
func WithStore(store Store) SessionOption {
	return func(s *SessionContext) {
		s.store = store
	}
}

// SessionContext owns the interactive session aggregate and latest provider context total.
// It is safe for model stream callbacks and the runner loop to update concurrently.
//
// SessionContext 持有交互式 session 聚合与最近一次 provider context total。
// 它允许模型流回调与 runner loop 并发更新。
type SessionContext struct {
	mu          sync.RWMutex
	session     *contextmodel.Session
	latestTotal int
	store       Store
}

// NewSessionContext creates a session-level usage recorder.
// NewSessionContext 创建 session 级 usage 记录器。
func NewSessionContext(opts ...SessionOption) *SessionContext {
	session := contextmodel.NewSession(newID("session"), time.Now())
	out := &SessionContext{session: session}
	for _, opt := range opts {
		if opt != nil {
			opt(out)
		}
	}
	return out
}

// Session returns a defensive snapshot of the current session aggregate.
// Session 返回当前 session 聚合的防御性快照。
func (s *SessionContext) Session() *contextmodel.Session {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSession(s.session)
}

// TotalTokens returns the latest provider-reported total tokens for compaction pressure checks.
// TotalTokens 返回最近一次 provider 上报的 total tokens，供压缩压力判断使用。
func (s *SessionContext) TotalTokens() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latestTotal
}

// UpdateFromSnapshot refreshes the latest provider context total.
// UpdateFromSnapshot 刷新最近一次 provider context total。
func (s *SessionContext) UpdateFromSnapshot(snapshot UsageSnapshot) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latestTotal = snapshot.TotalTokens
}

// BeginConversation creates a recorder for one ChatModelAgent Run.
// BeginConversation 为一次 ChatModelAgent Run 创建 recorder。
func (s *SessionContext) BeginConversation() *ConversationRecorder {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sequence := len(s.session.Conversations) + 1
	conversation := contextmodel.NewConversation(newID("conversation"), s.session.ID, sequence, time.Now())
	s.session.AddConversation(conversation)
	return &ConversationRecorder{conversation: conversation}
}

// FinishConversation closes a conversation, refreshes the session, and persists both snapshots.
// FinishConversation 结束 conversation、刷新 session，并持久化两个快照。
func (s *SessionContext) FinishConversation(ctx context.Context, recorder *ConversationRecorder, runErr error) (*contextmodel.Conversation, error) {
	if s == nil || recorder == nil {
		return nil, nil
	}
	conversation := recorder.Finish(runErr)

	s.mu.Lock()
	s.session.Recalculate()
	s.session.Finish(nil)
	sessionSnapshot := cloneSession(s.session)
	s.mu.Unlock()

	if s.store == nil {
		return conversation, nil
	}
	if err := s.store.SaveSession(ctx, sessionSnapshot); err != nil {
		return conversation, err
	}
	if err := s.store.SaveConversation(ctx, conversation); err != nil {
		return conversation, err
	}
	return conversation, nil
}

// Close releases the attached durable store.
// Close 释放已附加的持久化存储。
func (s *SessionContext) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Close()
}

// ConversationRecorder records one user interaction and all model turns inside it.
// ConversationRecorder 记录一次用户交互及其内部所有模型 turn。
type ConversationRecorder struct {
	mu           sync.Mutex
	conversation *contextmodel.Conversation
}

// AddTurn appends one model-call turn.
// AddTurn 追加一次模型调用 turn。
func (r *ConversationRecorder) AddTurn(elapsed time.Duration, snapshot UsageSnapshot, err error) *contextmodel.Turn {
	if r == nil || r.conversation == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	sequence := len(r.conversation.Turns) + 1
	startedAt := time.Now().Add(-elapsed)
	turn := contextmodel.NewTurn(newID("turn"), r.conversation.ID, sequence, startedAt)
	turn.Finish(elapsed, snapshot.PromptTokens, snapshot.CompletionTokens, snapshot.TotalTokens, err)
	r.conversation.AddTurn(turn)
	return cloneTurn(turn)
}

// SetMessages stores replayable user, assistant, and tool messages for this conversation.
// SetMessages 保存本次 conversation 可重现的用户、助手与工具消息。
func (r *ConversationRecorder) SetMessages(messages []*schema.Message) {
	if r == nil || r.conversation == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	records := make([]*contextmodel.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		records = append(records, messageRecordFromSchema(r.conversation.ID, len(records)+1, msg))
	}
	r.conversation.SetMessages(records)
}

// Snapshot returns a defensive copy of the current conversation.
// Snapshot 返回当前 conversation 的防御性拷贝。
func (r *ConversationRecorder) Snapshot() *contextmodel.Conversation {
	if r == nil || r.conversation == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneConversation(r.conversation)
}

// Finish closes the conversation and returns its snapshot.
// Finish 结束 conversation 并返回快照。
func (r *ConversationRecorder) Finish(err error) *contextmodel.Conversation {
	if r == nil || r.conversation == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conversation.Finish(err)
	return cloneConversation(r.conversation)
}

// WithSessionContext attaches a session recorder to context.
// WithSessionContext 将 session recorder 附加到 context。
func WithSessionContext(ctx context.Context, session *SessionContext) context.Context {
	if session == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionContextKey{}, session)
}

// SessionFromContext returns the attached session recorder.
// SessionFromContext 返回附加在 context 上的 session recorder。
func SessionFromContext(ctx context.Context) *SessionContext {
	if ctx == nil {
		return nil
	}
	session, _ := ctx.Value(sessionContextKey{}).(*SessionContext)
	return session
}

// WithConversationRecorder attaches a conversation recorder to context.
// WithConversationRecorder 将 conversation recorder 附加到 context。
func WithConversationRecorder(ctx context.Context, recorder *ConversationRecorder) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, conversationRecorderContextKey{}, recorder)
}

// ConversationFromContext returns the attached conversation recorder.
// ConversationFromContext 返回附加在 context 上的 conversation recorder。
func ConversationFromContext(ctx context.Context) *ConversationRecorder {
	if ctx == nil {
		return nil
	}
	recorder, _ := ctx.Value(conversationRecorderContextKey{}).(*ConversationRecorder)
	return recorder
}

// WithSkipTracking disables usage recording for auxiliary model calls such as summarization.
// WithSkipTracking 对摘要等辅助模型调用禁用 usage 记录。
func WithSkipTracking(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipTrackingContextKey{}, true)
}

// ShouldSkipTracking reports whether usage tracking is disabled for this context.
// ShouldSkipTracking 判断当前 context 是否禁用了 usage tracking。
func ShouldSkipTracking(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	skip, _ := ctx.Value(skipTrackingContextKey{}).(bool)
	return skip
}

// SnapshotFromMessage extracts provider token usage from an Eino schema.Message.
// SnapshotFromMessage 从 Eino schema.Message 中提取 provider token usage。
func SnapshotFromMessage(msg *schema.Message) (UsageSnapshot, bool) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return UsageSnapshot{}, false
	}
	usage := msg.ResponseMeta.Usage
	return UsageSnapshot{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}, true
}

// RecordModelUsage records one completed model call if tracking is enabled.
// RecordModelUsage 在 tracking 启用时记录一次已完成模型调用。
func RecordModelUsage(ctx context.Context, elapsed time.Duration, msg *schema.Message, callErr error) {
	if ShouldSkipTracking(ctx) {
		return
	}

	snapshot, ok := SnapshotFromMessage(msg)
	if ok {
		if session := SessionFromContext(ctx); session != nil {
			session.UpdateFromSnapshot(snapshot)
		}
	}
	if recorder := ConversationFromContext(ctx); recorder != nil {
		recorder.AddTurn(elapsed, snapshot, callErr)
	}
}

func messageRecordFromSchema(conversationID string, sequence int, msg *schema.Message) *contextmodel.Message {
	raw, _ := json.Marshal(msg)
	return &contextmodel.Message{
		ID:             newID("message"),
		ConversationID: conversationID,
		Sequence:       sequence,
		Role:           string(msg.Role),
		Content:        msg.Content,
		Reasoning:      msg.ReasoningContent,
		ToolName:       msg.ToolName,
		ToolCallID:     msg.ToolCallID,
		RawJSON:        string(raw),
		CreatedAt:      time.Now(),
	}
}

func cloneSession(in *contextmodel.Session) *contextmodel.Session {
	if in == nil {
		return nil
	}
	out := *in
	out.Conversations = make([]*contextmodel.Conversation, 0, len(in.Conversations))
	for _, conversation := range in.Conversations {
		out.Conversations = append(out.Conversations, cloneConversation(conversation))
	}
	return &out
}

func cloneConversation(in *contextmodel.Conversation) *contextmodel.Conversation {
	if in == nil {
		return nil
	}
	out := *in
	out.Turns = make([]*contextmodel.Turn, 0, len(in.Turns))
	for _, turn := range in.Turns {
		out.Turns = append(out.Turns, cloneTurn(turn))
	}
	out.Messages = make([]*contextmodel.Message, 0, len(in.Messages))
	for _, message := range in.Messages {
		if message == nil {
			continue
		}
		next := *message
		out.Messages = append(out.Messages, &next)
	}
	return &out
}

func cloneTurn(in *contextmodel.Turn) *contextmodel.Turn {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
