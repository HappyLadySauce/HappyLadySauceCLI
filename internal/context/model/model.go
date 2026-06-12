package model

import "time"

const (
	// StatusRunning marks a record that has started but has not reached a terminal state.
	// StatusRunning 表示记录已经开始但尚未进入终态。
	StatusRunning = "running"

	// StatusSucceeded marks a record that completed without an execution error.
	// StatusSucceeded 表示记录已成功完成。
	StatusSucceeded = "succeeded"

	// StatusFailed marks a record that completed with an execution error.
	// StatusFailed 表示记录因执行错误结束。
	StatusFailed = "failed"
)

// Turn records one ChatModel invocation inside an Eino ReAct loop.
// A streaming model call is closed when the stream reaches EOF.
//
// Example:
//
//	turn := NewTurn("turn-1", "conversation-1", 1, time.Now())
//	turn.Finish(time.Second, 128, 32, 160, nil)
//
// Turn 记录 Eino ReAct 循环中的一次 ChatModel 调用。
// 流式模型调用在 stream 到达 EOF 时结束。
type Turn struct {
	ID             string        // Stable turn id. / 稳定的 turn 标识。
	ConversationID string        // Parent conversation id. / 所属 conversation 标识。
	Sequence       int           // 1-based order within the conversation. / conversation 内从 1 开始的顺序。
	StartedAt      time.Time     // Model call start time. / 模型调用开始时间。
	CompletedAt    time.Time     // Model call completion time. / 模型调用完成时间。
	Elapsed        time.Duration // Model call latency. / 模型调用耗时。
	Prompt         int           // Provider prompt tokens. / provider 返回的输入 token 数。
	Completion     int           // Provider completion tokens. / provider 返回的输出 token 数。
	Total          int           // Provider total/context tokens for this model call. / 本次模型调用的总上下文 token 数。
	Status         string        // running, succeeded, or failed. / 运行中、成功或失败状态。
	Error          string        // Error text when the model call fails. / 模型调用失败时的错误文本。
}

// NewTurn creates a running turn record.
// NewTurn 创建一个运行中的 turn 记录。
func NewTurn(id, conversationID string, sequence int, startedAt time.Time) *Turn {
	return &Turn{
		ID:             id,
		ConversationID: conversationID,
		Sequence:       sequence,
		StartedAt:      startedAt,
		Status:         StatusRunning,
	}
}

// Finish closes the turn and writes provider usage.
// Finish 结束 turn 并写入 provider usage。
func (t *Turn) Finish(elapsed time.Duration, prompt, completion, total int, err error) {
	if t == nil {
		return
	}
	t.CompletedAt = t.StartedAt.Add(elapsed)
	if t.StartedAt.IsZero() {
		t.CompletedAt = time.Now()
	}
	t.Elapsed = elapsed
	t.Prompt = prompt
	t.Completion = completion
	t.Total = total
	t.Status = StatusSucceeded
	t.Error = ""
	if err != nil {
		t.Status = StatusFailed
		t.Error = err.Error()
	}
}

// Message records a replayable, persistence-safe schema.Message snapshot for one conversation.
// RawJSON preserves sanitized provider-specific fields when content persistence is enabled.
//
// Message 记录一条可用于会话重现且适合持久化的 schema.Message 快照。
// RawJSON 在启用内容持久化时保留脱敏后的 provider 特定字段。
type Message struct {
	ID             string    // Stable message id. / 稳定的 message 标识。
	ConversationID string    // Parent conversation id. / 所属 conversation 标识。
	Sequence       int       // 1-based order within the conversation. / conversation 内从 1 开始的顺序。
	Role           string    // Message role. / 消息角色。
	Content        string    // Visible content. / 可见正文。
	Reasoning      string    // Reasoning content when returned by the model. / 模型返回的推理正文。
	ToolName       string    // Tool name for tool messages. / 工具消息对应的工具名。
	ToolCallID     string    // Tool call id for tool messages. / 工具调用标识。
	RawJSON        string    // Sanitized serialized message, empty in metadata-only mode. / 脱敏序列化消息，metadata-only 模式下为空。
	CreatedAt      time.Time // Capture time. / 捕获时间。
}

// Conversation records one ChatModelAgent Run, i.e. one user interaction that may contain multiple model turns.
//
// Example:
//
//	conversation := NewConversation("conversation-1", "session-1", 1, time.Now())
//	conversation.AddTurn(turn)
//	conversation.Finish(nil)
//
// Conversation 记录一次 ChatModelAgent Run，即一次用户交互，内部可包含多次模型 turn。
type Conversation struct {
	ID          string        // Stable conversation id. / 稳定的 conversation 标识。
	SessionID   string        // Parent session id. / 所属 session 标识。
	Sequence    int           // 1-based order within the session. / session 内从 1 开始的顺序。
	Turns       []*Turn       // Model-call turns in this conversation. / 本次 conversation 内的模型调用列表。
	Messages    []*Message    // Replayable user, assistant, and tool messages. / 可重现的用户、助手与工具消息。
	StartedAt   time.Time     // Conversation start time. / conversation 开始时间。
	CompletedAt time.Time     // Conversation completion time. / conversation 完成时间。
	Elapsed     time.Duration // Total conversation latency. / 本次 conversation 总耗时。
	Prompt      int           // Sum of prompt tokens across turns. / 所有 turn 的输入 token 总数。
	Completion  int           // Sum of completion tokens across turns. / 所有 turn 的输出 token 总数。
	Total       int           // Sum of total tokens across turns. / 所有 turn 的总上下文 token 数。
	Status      string        // running, succeeded, or failed. / 运行中、成功或失败状态。
	Error       string        // Error text when the conversation fails. / conversation 失败时的错误文本。
}

// NewConversation creates a running conversation record.
// NewConversation 创建一个运行中的 conversation 记录。
func NewConversation(id, sessionID string, sequence int, startedAt time.Time) *Conversation {
	return &Conversation{
		ID:        id,
		SessionID: sessionID,
		Sequence:  sequence,
		StartedAt: startedAt,
		Status:    StatusRunning,
	}
}

// AddTurn appends a model-call turn and refreshes aggregate usage.
// AddTurn 追加一次模型调用 turn，并刷新聚合 usage。
func (c *Conversation) AddTurn(turn *Turn) {
	if c == nil || turn == nil {
		return
	}
	c.Turns = append(c.Turns, turn)
	c.Recalculate()
}

// SetMessages replaces replayable messages with a defensive copy of records.
// SetMessages 用防御性拷贝替换可重现消息列表。
func (c *Conversation) SetMessages(messages []*Message) {
	if c == nil {
		return
	}
	c.Messages = append([]*Message(nil), messages...)
}

// Finish closes the conversation and refreshes aggregate usage.
// Finish 结束 conversation 并刷新聚合 usage。
func (c *Conversation) Finish(err error) {
	if c == nil {
		return
	}
	c.CompletedAt = time.Now()
	if !c.StartedAt.IsZero() {
		c.Elapsed = c.CompletedAt.Sub(c.StartedAt)
	}
	c.Recalculate()
	c.Status = StatusSucceeded
	c.Error = ""
	if err != nil {
		c.Status = StatusFailed
		c.Error = err.Error()
	}
}

// Recalculate rebuilds conversation token totals from turns.
// Recalculate 根据 turns 重新计算 conversation token 总量。
func (c *Conversation) Recalculate() {
	if c == nil {
		return
	}
	c.Prompt = 0
	c.Completion = 0
	c.Total = 0
	for _, turn := range c.Turns {
		if turn == nil {
			continue
		}
		c.Prompt += turn.Prompt
		c.Completion += turn.Completion
		c.Total += turn.Total
	}
}

// Session records the full interactive CLI process lifetime.
//
// Example:
//
//	session := NewSession("session-1", time.Now())
//	session.AddConversation(conversation)
//
// Session 记录完整交互式 CLI 进程生命周期。
type Session struct {
	ID            string          // Stable session id. / 稳定的 session 标识。
	Conversations []*Conversation // Conversations in this session. / 本会话的 conversation 列表。
	StartedAt     time.Time       // Session start time. / session 开始时间。
	CompletedAt   time.Time       // Session latest completion time. / session 最近完成时间。
	Elapsed       time.Duration   // Total session elapsed time. / 本会话总耗时。
	Prompt        int             // Sum of prompt tokens across conversations. / 所有 conversation 的输入 token 总数。
	Completion    int             // Sum of completion tokens across conversations. / 所有 conversation 的输出 token 总数。
	Total         int             // Sum of total tokens across conversations. / 所有 conversation 的总上下文 token 数。
	Status        string          // running, succeeded, or failed. / 运行中、成功或失败状态。
	Error         string          // Error text when the session fails. / session 失败时的错误文本。
}

// NewSession creates a running session record.
// NewSession 创建一个运行中的 session 记录。
func NewSession(id string, startedAt time.Time) *Session {
	return &Session{
		ID:        id,
		StartedAt: startedAt,
		Status:    StatusRunning,
	}
}

// AddConversation appends a conversation and refreshes aggregate usage.
// AddConversation 追加 conversation 并刷新聚合 usage。
func (s *Session) AddConversation(conversation *Conversation) {
	if s == nil || conversation == nil {
		return
	}
	s.Conversations = append(s.Conversations, conversation)
	s.Recalculate()
}

// Finish refreshes session totals and marks the latest completion state.
// Finish 刷新 session 总量并标记最近完成状态。
func (s *Session) Finish(err error) {
	if s == nil {
		return
	}
	s.CompletedAt = time.Now()
	if !s.StartedAt.IsZero() {
		s.Elapsed = s.CompletedAt.Sub(s.StartedAt)
	}
	s.Recalculate()
	s.Status = StatusSucceeded
	s.Error = ""
	if err != nil {
		s.Status = StatusFailed
		s.Error = err.Error()
	}
}

// Recalculate rebuilds session token totals from conversations.
// Recalculate 根据 conversations 重新计算 session token 总量。
func (s *Session) Recalculate() {
	if s == nil {
		return
	}
	s.Prompt = 0
	s.Completion = 0
	s.Total = 0
	for _, conversation := range s.Conversations {
		if conversation == nil {
			continue
		}
		s.Prompt += conversation.Prompt
		s.Completion += conversation.Completion
		s.Total += conversation.Total
	}
}
