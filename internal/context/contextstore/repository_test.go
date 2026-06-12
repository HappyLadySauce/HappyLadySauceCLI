package contextstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
	"github.com/HappyLadySauce/HappyLadySauceCLI/migrations"
	storagesqlite "github.com/HappyLadySauce/HappyLadySauceCLI/pkg/storage/sqlite"
)

func TestRepositorySavesSessionConversationTurnsAndMessages(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "context.sqlite")
	db, err := storagesqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("sqlite Open() error = %v", err)
	}
	defer db.Close()

	repo := New(db)
	if err := migrations.Apply(ctx, db); err != nil {
		t.Fatalf("migrations Apply() error = %v", err)
	}

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

	verifyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open verification db: %v", err)
	}
	defer verifyDB.Close()

	assertCount(t, verifyDB, "context_sessions", 1)
	assertCount(t, verifyDB, "context_conversations", 1)
	assertCount(t, verifyDB, "context_turns", 1)
	assertCount(t, verifyDB, "context_messages", 1)
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
