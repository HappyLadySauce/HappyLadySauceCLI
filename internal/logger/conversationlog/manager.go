// Package conversationlog writes session-scoped JSONL conversation detail logs.
// Package conversationlog 写入按 session 分隔的 JSONL 对话明细日志。
package conversationlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// Entry is a single JSONL line recording one conversation event.
// Entry 是记录单次对话事件的 JSONL 行。
type Entry struct {
	TS             time.Time `json:"ts"`
	Type           string    `json:"type"`
	SessionID      string    `json:"session_id,omitempty"`
	ConversationID string    `json:"conversation_id,omitempty"`
	UserTurnSeq    int       `json:"user_turn_seq,omitempty"`
	ModelCall      int       `json:"model_call,omitempty"`
	Kind           string    `json:"kind,omitempty"`
	ToolName       string    `json:"tool_name,omitempty"`
	UserPrompt     string    `json:"user_prompt,omitempty"`
	Content        string    `json:"content,omitempty"`
	Prompt         int       `json:"prompt,omitempty"`
	Completion     int       `json:"completion,omitempty"`
	Total          int       `json:"total,omitempty"`
	Error          string    `json:"error,omitempty"`
}

// Manager owns a single-session JSONL file and provides thread-safe append.
// Manager 持有单个 session 的 JSONL 文件，提供线程安全写入。
type Manager struct {
	mu        sync.Mutex
	dir       string
	file      *os.File
	encoder   *json.Encoder
	sessionID string
}

// NewManager creates a Manager that writes JSONL files to logDir.
// NewManager 创建向 logDir 写入 JSONL 文件的 Manager。
func NewManager(logDir string) *Manager {
	return &Manager{dir: logDir}
}

// OpenSession creates or opens the session JSONL file and removes old session files.
// Creates the file first, then cleans up — so a crash between steps leaves the current
// file intact.
//
// OpenSession 创建或打开 session JSONL 文件，并清理旧 session 文件。
// 先创建文件再清理——确保中间崩溃时当前文件不丢。
func (m *Manager) OpenSession(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session ID is required")
	}
	if strings.ContainsAny(sessionID, "/\\") {
		return fmt.Errorf("session ID contains invalid characters: %q", sessionID)
	}

	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return fmt.Errorf("create session log directory: %w", err)
	}

	logPath := filepath.Join(m.dir, sessionID+".jsonl")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open session log file: %w", err)
	}

	if m.file != nil {
		m.file.Close()
	}

	m.mu.Lock()
	m.file = file
	m.encoder = json.NewEncoder(file)
	m.sessionID = sessionID
	m.mu.Unlock()

	entries, err := os.ReadDir(m.dir)
	if err != nil {
		klog.Warningf("read session log directory for cleanup: %v", err)
		return nil
	}

	target := sessionID + ".jsonl"
	cleaned := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == target || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		if err := os.Remove(filepath.Join(m.dir, name)); err != nil {
			klog.Warningf("remove old session log %s: %v", name, err)
			continue
		}
		cleaned++
	}
	if cleaned > 0 {
		klog.Infof("cleaned %d old session log(s)", cleaned)
	}

	return nil
}

// Append writes one JSONL entry to the current session file.
// Append 向当前 session 文件追加写入一条 JSONL 条目。
func (m *Manager) Append(entry Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.encoder == nil {
		return fmt.Errorf("conversation log not opened")
	}
	entry.TS = time.Now().UTC()
	return m.encoder.Encode(entry)
}

// Path returns the relative log path for diagnostic log reference.
// Returns empty string if no session is open.
//
// Path 返回相对日志路径，供诊断日志引用。
// 若未打开 session 则返回空字符串。
func (m *Manager) Path() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessionID == "" {
		return ""
	}
	return "session/" + m.sessionID + ".jsonl"
}

// Close flushes and closes the current session file.
// Close 刷新并关闭当前 session 文件。
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.file == nil {
		return nil
	}
	err := m.file.Close()
	m.file = nil
	m.encoder = nil
	return err
}
