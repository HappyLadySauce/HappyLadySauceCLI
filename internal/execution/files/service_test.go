package files

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestServiceReadTextReturnsBoundedLineRange(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(target, []byte("one\ntwo\nthree\nfour\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, err := NewService().ReadText(context.Background(), ReadRequest{
		Path:      target,
		StartLine: 2,
		MaxLines:  2,
	})
	if err != nil {
		t.Fatalf("ReadText() error = %v", err)
	}
	if result.Content != "two\nthree\n" {
		t.Fatalf("Content = %q", result.Content)
	}
	if result.StartLine != 2 || result.EndLine != 3 || result.TotalLines != 4 || !result.Truncated {
		t.Fatalf("unexpected range metadata: %#v", result)
	}
}

func TestServiceReadTextRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "binary.dat")
	if err := os.WriteFile(target, []byte{0xff, 0xfe, '\n'}, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := NewService().ReadText(context.Background(), ReadRequest{Path: target}); err == nil {
		t.Fatal("ReadText() error = nil, want invalid UTF-8 error")
	}
}

func TestServiceReadTextRejectsInvalidLimits(t *testing.T) {
	t.Parallel()

	_, err := NewService().ReadText(context.Background(), ReadRequest{
		Path:      filepath.Join(t.TempDir(), "missing.txt"),
		StartLine: 1,
		MaxLines:  MaxReadLines + 1,
	})
	if err == nil || !strings.Contains(err.Error(), "max_lines") {
		t.Fatalf("ReadText() error = %v, want max_lines validation", err)
	}
}

func TestServiceListDirectoryReturnsSortedBoundedEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, name := range []string{"z.txt", "a.txt", "m.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}

	result, err := NewService().ListDirectory(context.Background(), ListRequest{
		Path:       root,
		MaxEntries: 2,
	})
	if err != nil {
		t.Fatalf("ListDirectory() error = %v", err)
	}
	if len(result.Entries) != 2 || result.Entries[0].Name != "a.txt" || result.Entries[1].Name != "m.txt" {
		t.Fatalf("Entries = %#v, want sorted first two entries", result.Entries)
	}
	if result.TotalEntries != 3 || !result.Truncated {
		t.Fatalf("unexpected listing metadata: %#v", result)
	}
}

func TestServiceEditTextRequiresUniqueOldText(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(target, []byte("repeat repeat"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := NewService().EditText(context.Background(), EditRequest{
		Path:    target,
		OldText: "repeat",
		NewText: "done",
	})
	if err == nil || !strings.Contains(err.Error(), "matches=2") {
		t.Fatalf("EditText() error = %v, want non-unique match error", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "repeat repeat" {
		t.Fatalf("file content changed after failed edit: %q", data)
	}
}

func TestServiceEditTextReplacesTextAtomicallyAndPreservesMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(target, []byte("hello world"), 0o640); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, err := NewService().EditText(context.Background(), EditRequest{
		Path:    target,
		OldText: "world",
		NewText: "agent",
	})
	if err != nil {
		t.Fatalf("EditText() error = %v", err)
	}
	if result.Replacements != 1 || result.BytesWritten != int64(len("hello agent")) || result.ContentSHA256 == "" {
		t.Fatalf("unexpected edit result: %#v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "hello agent" {
		t.Fatalf("file content = %q", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got := info.Mode().Perm(); got != 0o640 {
			t.Fatalf("mode = %v, want 0640", got)
		}
	}
}

func TestServiceCreateTextFailsWhenTargetExists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := NewService().CreateText(context.Background(), CreateRequest{Path: target, Content: "new"}); err == nil {
		t.Fatal("CreateText() error = nil, want exists error")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "old" {
		t.Fatalf("existing file content changed: %q", data)
	}
}

func TestServiceCreateTextCreatesUTF8File(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "new.txt")
	result, err := NewService().CreateText(context.Background(), CreateRequest{Path: target, Content: "hello"})
	if err != nil {
		t.Fatalf("CreateText() error = %v", err)
	}
	if result.BytesWritten != int64(len("hello")) || result.ContentSHA256 == "" {
		t.Fatalf("unexpected create result: %#v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("created content = %q", data)
	}
}

func TestServiceDeleteFileRejectsDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if _, err := NewService().DeleteFile(context.Background(), DeleteRequest{Path: root}); err == nil {
		t.Fatal("DeleteFile() error = nil, want directory rejection")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("directory should still exist: %v", err)
	}
}

func TestServiceDeleteFileDeletesRegularFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "delete.txt")
	if err := os.WriteFile(target, []byte("remove"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, err := NewService().DeleteFile(context.Background(), DeleteRequest{Path: target})
	if err != nil {
		t.Fatalf("DeleteFile() error = %v", err)
	}
	if !result.Deleted {
		t.Fatalf("Deleted = false")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("Stat() error = %v, want not exists", err)
	}
}
