package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPathUsesHiddenHappyLadySauceDirectory(t *testing.T) {
	t.Parallel()

	path, err := DefaultPath("context.sqlite")
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

func TestDefaultPathRejectsNestedFilename(t *testing.T) {
	t.Parallel()

	if _, err := DefaultPath(filepath.Join("nested", "context.sqlite")); err == nil {
		t.Fatal("DefaultPath() error = nil, want nested filename rejection")
	}
}

func TestOpenCreatesDatabase(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "context.sqlite")
	db, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("PingContext() error = %v", err)
	}
}
