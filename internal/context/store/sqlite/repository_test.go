package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
)

func TestRepositorySavesSessionConversationTurnsAndMessages(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "context.sqlite")
	repo, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer repo.Close()

	session := contextmodel.NewSession("session_test", time.Now())
	conversation := contextmodel.NewConversation("conversation_test", session.ID, 1, time.Now())
	turn := contextmodel.NewTurn("turn_test", conversation.ID, 1, time.Now())
	turn.Finish(25*time.Millisecond, 10, 5, 15, nil)
	conversation.AddTurn(turn)
	conversation.SetMessages([]*contextmodel.Message{{
		ID:             "message_test",
		ConversationID: conversation.ID,
		Sequence:       1,
		Role:           "user",
		Content:        "hello",
		RawJSON:        `{"role":"user","content":"hello"}`,
		CreatedAt:      time.Now(),
	}})
	conversation.Finish(nil)
	session.AddConversation(conversation)
	session.Finish(nil)

	if err := repo.SaveSession(ctx, session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := repo.SaveConversation(ctx, conversation); err != nil {
		t.Fatalf("SaveConversation() error = %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open verification db: %v", err)
	}
	defer db.Close()

	assertCount(t, db, "context_sessions", 1)
	assertCount(t, db, "context_conversations", 1)
	assertCount(t, db, "context_turns", 1)
	assertCount(t, db, "context_messages", 1)
}

func TestDefaultPathUsesHiddenHappyLadySauceDirectory(t *testing.T) {
	t.Parallel()

	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	if !strings.Contains(filepath.ToSlash(path), "/.HAPPLADYSAUCECLI/") {
		t.Fatalf("DefaultPath() = %q, want hidden .HAPPLADYSAUCECLI directory", path)
	}
	if filepath.Base(path) != "context.sqlite" {
		t.Fatalf("DefaultPath() base = %q, want context.sqlite", filepath.Base(path))
	}
}

func assertCount(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}
