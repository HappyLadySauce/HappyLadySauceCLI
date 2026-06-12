package appdirs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDirUsesHiddenHappyLadySauceDirectory(t *testing.T) {
	resetHomeForTest(t)

	dir, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir() error = %v", err)
	}
	if filepath.Base(dir) != defaultDirName {
		t.Fatalf("DefaultDir() = %q, want base %q", dir, defaultDirName)
	}
}

func TestLogsDirUsesLogsSubdirectory(t *testing.T) {
	resetHomeForTest(t)

	dir, err := LogsDir()
	if err != nil {
		t.Fatalf("LogsDir() error = %v", err)
	}
	if filepath.Base(dir) != "logs" {
		t.Fatalf("LogsDir() = %q, want logs subdirectory", dir)
	}
	if filepath.Base(filepath.Dir(dir)) != defaultDirName {
		t.Fatalf("LogsDir() = %q, want parent %q", dir, defaultDirName)
	}
}

func TestSetHomeDirUsesConfiguredDirectory(t *testing.T) {
	resetHomeForTest(t)

	want := filepath.Join(t.TempDir(), "custom-home")
	if err := SetHomeDir(want); err != nil {
		t.Fatalf("SetHomeDir() error = %v", err)
	}
	got, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir() error = %v", err)
	}
	if got != filepath.Clean(want) {
		t.Fatalf("DefaultDir() = %q, want %q", got, filepath.Clean(want))
	}

	logsDir, err := LogsDir()
	if err != nil {
		t.Fatalf("LogsDir() error = %v", err)
	}
	if logsDir != filepath.Join(filepath.Clean(want), "logs") {
		t.Fatalf("LogsDir() = %q, want logs under configured home", logsDir)
	}
}

func TestResolveHomeDirResolvesRelativePathAgainstCWD(t *testing.T) {
	resetHomeForTest(t)

	cwd := t.TempDir()
	oldResolveCwd := resolveCwd
	resolveCwd = func() (string, error) { return cwd, nil }
	t.Cleanup(func() { resolveCwd = oldResolveCwd })

	got, err := ResolveHomeDir(".HAPPLADYSAUCECLI")
	if err != nil {
		t.Fatalf("ResolveHomeDir() error = %v", err)
	}
	want := filepath.Join(cwd, ".HAPPLADYSAUCECLI")
	if got != want {
		t.Fatalf("ResolveHomeDir() = %q, want %q", got, want)
	}
}

func TestResolveHomeDirExpandsTilde(t *testing.T) {
	resetHomeForTest(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := ResolveHomeDir("~/dev-home")
	if err != nil {
		t.Fatalf("ResolveHomeDir() error = %v", err)
	}
	want := filepath.Join(home, "dev-home")
	if got != want {
		t.Fatalf("ResolveHomeDir() = %q, want %q", got, want)
	}
}

func resetHomeForTest(t *testing.T) {
	t.Helper()

	homeMu.Lock()
	oldHome := homeDir
	homeDir = ""
	homeMu.Unlock()
	t.Cleanup(func() {
		homeMu.Lock()
		homeDir = oldHome
		homeMu.Unlock()
	})

	oldResolveCwd := resolveCwd
	resolveCwd = os.Getwd
	t.Cleanup(func() { resolveCwd = oldResolveCwd })
}
