package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"k8s.io/klog/v2"

	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/appdirs"
)

const defaultLogFilename = "happyladysaucecli.log"

// ConfigureDefaultFile redirects klog output to <home>/logs/happyladysaucecli.log.
// The returned closer must be closed after klog has been flushed.
//
// ConfigureDefaultFile 将 klog 输出重定向到 <home>/logs/happyladysaucecli.log。
// 调用方必须在 flush klog 后关闭返回的 closer。
func ConfigureDefaultFile() (io.Closer, string, error) {
	logDir, err := appdirs.LogsDir()
	if err != nil {
		return nil, "", err
	}
	return configureFile(logDir)
}

func configureFile(logDir string) (io.Closer, string, error) {
	if logDir == "" {
		return nil, "", fmt.Errorf("log directory is required")
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, "", fmt.Errorf("create log directory: %w", err)
	}

	logPath := filepath.Join(logDir, defaultLogFilename)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, "", fmt.Errorf("open log file: %w", err)
	}

	klog.LogToStderr(false)
	klog.SetOutput(file)
	return file, logPath, nil
}
