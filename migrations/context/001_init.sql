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
