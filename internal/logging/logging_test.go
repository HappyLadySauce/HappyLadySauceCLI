package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/klog/v2"

	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/appdirs"
)

func TestConfigureFileRedirectsKlogToLogFile(t *testing.T) {
	state := klog.CaptureState()
	defer state.Restore()

	logDir := filepath.Join(t.TempDir(), "logs")
	closer, logPath, err := configureFile(logDir)
	if err != nil {
		t.Fatalf("configureFile() error = %v", err)
	}
	defer closer.Close()

	klog.Info("test log file routing")
	klog.Flush()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", logPath, err)
	}
	if !strings.Contains(string(data), "test log file routing") {
		t.Fatalf("log file content = %q, want routed klog message", string(data))
	}
	if filepath.Base(logPath) != defaultLogFilename {
		t.Fatalf("logPath = %q, want base %q", logPath, defaultLogFilename)
	}
}

func TestConfigureDefaultFileUsesConfiguredAppHome(t *testing.T) {
	state := klog.CaptureState()
	defer state.Restore()

	home := t.TempDir()
	if err := appdirs.SetHomeDir(home); err != nil {
		t.Fatalf("SetHomeDir() error = %v", err)
	}
	t.Cleanup(func() { _ = appdirs.SetHomeDir("") })

	closer, logPath, err := ConfigureDefaultFile()
	if err != nil {
		t.Fatalf("ConfigureDefaultFile() error = %v", err)
	}
	defer closer.Close()

	want := filepath.Join(home, "logs", defaultLogFilename)
	if logPath != want {
		t.Fatalf("ConfigureDefaultFile() logPath = %q, want %q", logPath, want)
	}
}
