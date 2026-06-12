package migrations

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	storagesqlite "github.com/HappyLadySauce/HappyLadySauceCLI/pkg/storage/sqlite"
)

func TestApplyCreatesContextTables(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "context.sqlite")
	db, err := storagesqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("sqlite Open() error = %v", err)
	}
	defer db.Close()

	if err := Apply(ctx, db); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	verifyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open verification db: %v", err)
	}
	defer verifyDB.Close()

	for _, table := range []string{
		"context_sessions",
		"context_conversations",
		"context_turns",
		"context_messages",
	} {
		assertTableExists(t, verifyDB, table)
	}
}

func assertTableExists(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var name string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
		t.Fatalf("table %s does not exist: %v", table, err)
	}
}
