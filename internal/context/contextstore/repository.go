// Package contextstore persists context sessions, conversations, turns, and messages.
// Package contextstore 持久化上下文 session、conversation、turn 与消息。
package contextstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
)

// Repository stores context hierarchy records in a caller-owned SQL database.
// The current schema is SQLite-oriented; connection creation belongs to internal/storage/sqlite.
//
// Repository 将上下文层级记录存入调用方持有的 SQL 数据库。
// 当前 schema 面向 SQLite；连接创建由 internal/storage/sqlite 负责。
type Repository struct {
	db *sql.DB
}

// New creates a context repository from an existing database connection.
// The caller must run Migrate before saving records and must close the database.
//
// New 基于已有数据库连接创建上下文 repository。
// 调用方必须先执行 Migrate 再保存记录，并负责关闭数据库。
func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Migrate applies the context persistence schema.
// It is safe to call multiple times during startup.
//
// Migrate 应用上下文持久化 schema。
// 启动阶段可安全重复调用。
func (r *Repository) Migrate(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("context store database is nil")
	}
	if _, err := r.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS context_sessions (
	id TEXT PRIMARY KEY,
	started_at TEXT NOT NULL,
	completed_at TEXT,
	elapsed_ms INTEGER NOT NULL DEFAULT 0,
	prompt_tokens INTEGER NOT NULL DEFAULT 0,
	completion_tokens INTEGER NOT NULL DEFAULT 0,
	total_tokens INTEGER NOT NULL DEFAULT 0,
	conversation_count INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS context_conversations (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	sequence INTEGER NOT NULL,
	started_at TEXT NOT NULL,
	completed_at TEXT,
	elapsed_ms INTEGER NOT NULL DEFAULT 0,
	prompt_tokens INTEGER NOT NULL DEFAULT 0,
	completion_tokens INTEGER NOT NULL DEFAULT 0,
	total_tokens INTEGER NOT NULL DEFAULT 0,
	turn_count INTEGER NOT NULL DEFAULT 0,
	message_count INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL,
	FOREIGN KEY(session_id) REFERENCES context_sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_context_conversations_session_sequence
	ON context_conversations(session_id, sequence);

CREATE TABLE IF NOT EXISTS context_turns (
	id TEXT PRIMARY KEY,
	conversation_id TEXT NOT NULL,
	sequence INTEGER NOT NULL,
	started_at TEXT NOT NULL,
	completed_at TEXT,
	elapsed_ms INTEGER NOT NULL DEFAULT 0,
	prompt_tokens INTEGER NOT NULL DEFAULT 0,
	completion_tokens INTEGER NOT NULL DEFAULT 0,
	total_tokens INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	FOREIGN KEY(conversation_id) REFERENCES context_conversations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_context_turns_conversation_sequence
	ON context_turns(conversation_id, sequence);

CREATE TABLE IF NOT EXISTS context_messages (
	id TEXT PRIMARY KEY,
	conversation_id TEXT NOT NULL,
	sequence INTEGER NOT NULL,
	role TEXT NOT NULL,
	content TEXT NOT NULL DEFAULT '',
	reasoning TEXT NOT NULL DEFAULT '',
	tool_name TEXT NOT NULL DEFAULT '',
	tool_call_id TEXT NOT NULL DEFAULT '',
	raw_json TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	FOREIGN KEY(conversation_id) REFERENCES context_conversations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_context_messages_conversation_sequence
	ON context_messages(conversation_id, sequence);
`); err != nil {
		return fmt.Errorf("migrate context store: %w", err)
	}
	return nil
}

// SaveSession upserts the session aggregate.
// It stores only aggregate counters; child records are saved through SaveConversation.
//
// SaveSession upsert session 聚合记录。
// 它只保存聚合计数；子记录通过 SaveConversation 保存。
func (r *Repository) SaveSession(ctx context.Context, session *contextmodel.Session) error {
	if r == nil || r.db == nil || session == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO context_sessions (
	id, started_at, completed_at, elapsed_ms, prompt_tokens, completion_tokens,
	total_tokens, conversation_count, status, error, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	started_at = excluded.started_at,
	completed_at = excluded.completed_at,
	elapsed_ms = excluded.elapsed_ms,
	prompt_tokens = excluded.prompt_tokens,
	completion_tokens = excluded.completion_tokens,
	total_tokens = excluded.total_tokens,
	conversation_count = excluded.conversation_count,
	status = excluded.status,
	error = excluded.error,
	updated_at = excluded.updated_at
`, session.ID, formatTime(session.StartedAt), nullableTime(session.CompletedAt), session.Elapsed.Milliseconds(),
		session.Prompt, session.Completion, session.Total, len(session.Conversations), session.Status, session.Error, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("save context session: %w", err)
	}
	return nil
}

// SaveConversation upserts a conversation and replaces its turns and replay messages.
// The operation runs in a single transaction to keep replay data consistent.
//
// SaveConversation upsert conversation，并替换其 turns 与可重放消息。
// 该操作在单个事务中执行，确保重放数据一致。
func (r *Repository) SaveConversation(ctx context.Context, conversation *contextmodel.Conversation) error {
	if r == nil || r.db == nil || conversation == nil {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin context conversation transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = saveConversation(ctx, tx, conversation); err != nil {
		return err
	}
	if err = replaceTurns(ctx, tx, conversation); err != nil {
		return err
	}
	if err = replaceMessages(ctx, tx, conversation); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit context conversation transaction: %w", err)
	}
	return nil
}

func saveConversation(ctx context.Context, tx *sql.Tx, conversation *contextmodel.Conversation) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO context_conversations (
	id, session_id, sequence, started_at, completed_at, elapsed_ms, prompt_tokens,
	completion_tokens, total_tokens, turn_count, message_count, status, error, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	session_id = excluded.session_id,
	sequence = excluded.sequence,
	started_at = excluded.started_at,
	completed_at = excluded.completed_at,
	elapsed_ms = excluded.elapsed_ms,
	prompt_tokens = excluded.prompt_tokens,
	completion_tokens = excluded.completion_tokens,
	total_tokens = excluded.total_tokens,
	turn_count = excluded.turn_count,
	message_count = excluded.message_count,
	status = excluded.status,
	error = excluded.error,
	updated_at = excluded.updated_at
`, conversation.ID, conversation.SessionID, conversation.Sequence, formatTime(conversation.StartedAt),
		nullableTime(conversation.CompletedAt), conversation.Elapsed.Milliseconds(), conversation.Prompt,
		conversation.Completion, conversation.Total, len(conversation.Turns), len(conversation.Messages),
		conversation.Status, conversation.Error, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("save context conversation: %w", err)
	}
	return nil
}

func replaceTurns(ctx context.Context, tx *sql.Tx, conversation *contextmodel.Conversation) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM context_turns WHERE conversation_id = ?`, conversation.ID); err != nil {
		return fmt.Errorf("delete context turns: %w", err)
	}
	for _, turn := range conversation.Turns {
		if turn == nil {
			continue
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO context_turns (
	id, conversation_id, sequence, started_at, completed_at, elapsed_ms,
	prompt_tokens, completion_tokens, total_tokens, status, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, turn.ID, conversation.ID, turn.Sequence, formatTime(turn.StartedAt), nullableTime(turn.CompletedAt),
			turn.Elapsed.Milliseconds(), turn.Prompt, turn.Completion, turn.Total, turn.Status, turn.Error)
		if err != nil {
			return fmt.Errorf("save context turn: %w", err)
		}
	}
	return nil
}

func replaceMessages(ctx context.Context, tx *sql.Tx, conversation *contextmodel.Conversation) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM context_messages WHERE conversation_id = ?`, conversation.ID); err != nil {
		return fmt.Errorf("delete context messages: %w", err)
	}
	for _, message := range conversation.Messages {
		if message == nil {
			continue
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO context_messages (
	id, conversation_id, sequence, role, content, reasoning, tool_name,
	tool_call_id, raw_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, message.ID, conversation.ID, message.Sequence, message.Role, message.Content, message.Reasoning,
			message.ToolName, message.ToolCallID, message.RawJSON, formatTime(message.CreatedAt))
		if err != nil {
			return fmt.Errorf("save context message: %w", err)
		}
	}
	return nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatTime(t)
}
