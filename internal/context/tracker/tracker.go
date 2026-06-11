// Package tracker keeps the in-memory context usage hierarchy for one CLI session.
// Package tracker 维护一个 CLI session 的内存上下文 usage 层级。
package tracker

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"

	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
)

type contextKey struct{}

// Tracker owns the in-memory Session -> Conversation -> Turn hierarchy.
// It is safe for model stream callbacks and the interactive runner loop to update concurrently.
//
// Tracker 持有内存中的 Session -> Conversation -> Turn 层级。
// 它允许模型流回调与交互式 runner loop 并发更新。
type Tracker struct {
	mu          sync.RWMutex
	session     *contextmodel.Session
	current     *contextmodel.Conversation
	latestTotal int
}

// New creates a tracker for one CLI process lifetime.
// The returned tracker starts with an empty running session.
//
// New 为一个 CLI 进程生命周期创建 tracker。
// 返回的 tracker 带有一个空的 running session。
func New() *Tracker {
	return &Tracker{
		session: contextmodel.NewSession(newID("session"), time.Now()),
	}
}

// WithTracker attaches a tracker to context using one context key.
// Middleware can retrieve it with FromContext without knowing conversation internals.
//
// WithTracker 使用一个 context key 将 tracker 附加到 context。
// middleware 可通过 FromContext 获取它，而无需感知 conversation 内部结构。
func WithTracker(ctx context.Context, tracker *Tracker) context.Context {
	if tracker == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, tracker)
}

// FromContext returns the tracker attached by WithTracker.
// It returns nil when no tracker is attached.
//
// FromContext 返回 WithTracker 附加的 tracker。
// 未附加 tracker 时返回 nil。
func FromContext(ctx context.Context) *Tracker {
	if ctx == nil {
		return nil
	}
	tracker, _ := ctx.Value(contextKey{}).(*Tracker)
	return tracker
}

// BeginConversation starts one ChatModelAgent Run inside the current session.
// It returns a defensive snapshot of the new conversation for observability.
//
// BeginConversation 在当前 session 中开始一次 ChatModelAgent Run。
// 它返回新 conversation 的防御性快照，便于观测。
func (t *Tracker) BeginConversation() *contextmodel.Conversation {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	sequence := len(t.session.Conversations) + 1
	conversation := contextmodel.NewConversation(newID("conversation"), t.session.ID, sequence, time.Now())
	t.session.AddConversation(conversation)
	t.current = conversation
	return cloneConversation(conversation)
}

// AddTurn appends one completed model-call turn to the active conversation.
// The input turn may omit ID, sequence, and conversation ID; tracker fills them.
//
// AddTurn 向当前活跃 conversation 追加一次已完成的模型调用 turn。
// 输入 turn 可省略 ID、sequence 与 conversation ID；tracker 会补齐。
func (t *Tracker) AddTurn(turn *contextmodel.Turn) *contextmodel.Turn {
	if t == nil || turn == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == nil {
		return nil
	}

	next := cloneTurn(turn)
	next.ID = firstNonEmpty(next.ID, newID("turn"))
	next.ConversationID = t.current.ID
	next.Sequence = len(t.current.Turns) + 1
	normalizeTurnTiming(next)

	t.current.AddTurn(next)
	t.session.Recalculate()
	t.latestTotal = next.Total
	return cloneTurn(next)
}

// SetMessages replaces replayable messages for the active conversation.
// Only user, assistant, and tool message snapshots that exist in memory are stored.
//
// SetMessages 替换当前活跃 conversation 的可重放消息。
// 只保存内存中已有的 user、assistant 与 tool 消息快照。
func (t *Tracker) SetMessages(messages []*schema.Message) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == nil {
		return
	}

	records := make([]*contextmodel.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		records = append(records, messageRecordFromSchema(t.current.ID, len(records)+1, msg))
	}
	t.current.SetMessages(records)
}

// FinishConversation closes the active conversation and refreshes session aggregates.
// The returned snapshot is safe to pass to renderers or persistence layers.
//
// FinishConversation 结束当前活跃 conversation，并刷新 session 聚合。
// 返回的快照可安全传给渲染器或持久化层。
func (t *Tracker) FinishConversation(err error) *contextmodel.Conversation {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == nil {
		return nil
	}

	t.current.Finish(err)
	t.session.Finish(err)
	conversation := cloneConversation(t.current)
	t.current = nil
	return conversation
}

// CurrentConversation returns a defensive snapshot of the active conversation.
// It returns nil when no conversation is running.
//
// CurrentConversation 返回当前活跃 conversation 的防御性快照。
// 没有运行中的 conversation 时返回 nil。
func (t *Tracker) CurrentConversation() *contextmodel.Conversation {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return cloneConversation(t.current)
}

// Session returns a defensive snapshot of the current session aggregate.
// The snapshot includes completed and currently running conversations.
//
// Session 返回当前 session 聚合的防御性快照。
// 快照包含已完成与正在运行的 conversations。
func (t *Tracker) Session() *contextmodel.Session {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return cloneSession(t.session)
}

// TotalTokens returns the latest provider-reported context total from a model turn.
// This is the compaction pressure signal and is intentionally not the session aggregate sum.
//
// TotalTokens 返回最近一次模型 turn 中 provider 上报的上下文 total。
// 这是压缩压力信号，故意不等同于 session 聚合总和。
func (t *Tracker) TotalTokens() int {
	if t == nil {
		return 0
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.latestTotal
}

func normalizeTurnTiming(turn *contextmodel.Turn) {
	if turn == nil {
		return
	}
	if turn.Status == "" {
		turn.Status = contextmodel.StatusSucceeded
	}
	if turn.StartedAt.IsZero() {
		if turn.Elapsed > 0 {
			turn.StartedAt = time.Now().Add(-turn.Elapsed)
		} else {
			turn.StartedAt = time.Now()
		}
	}
	if turn.CompletedAt.IsZero() {
		turn.CompletedAt = turn.StartedAt.Add(turn.Elapsed)
		if turn.Elapsed == 0 {
			turn.CompletedAt = turn.StartedAt
		}
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

func firstNonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
