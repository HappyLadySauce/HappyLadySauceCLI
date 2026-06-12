package logger

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"k8s.io/klog/v2"

	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/appdirs"
)

const defaultLogFilename = "happyladysaucecli.log"

// FilePaths holds the on-disk diagnostic log destination.
// FilePaths 保存磁盘诊断日志路径。
type FilePaths struct {
	Path string
}

// ConfigureDefaultFile redirects klog output to <home>/logs/happyladysaucecli.log and disables terminal logging.
// INFO and ERROR severities share the same file. The returned closer must be closed after klog has been flushed.
//
// ConfigureDefaultFile 将 klog 输出重定向到 <home>/logs/happyladysaucecli.log，并禁用终端日志输出。
// INFO 与 ERROR 级别写入同一文件。调用方必须在 flush klog 后关闭返回的 closer。
func ConfigureDefaultFile() (io.Closer, FilePaths, error) {
	logDir, err := appdirs.LogsDir()
	if err != nil {
		return nil, FilePaths{}, err
	}
	return configureFile(logDir)
}

func configureFile(logDir string) (io.Closer, FilePaths, error) {
	if logDir == "" {
		return nil, FilePaths{}, fmt.Errorf("log directory is required")
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, FilePaths{}, fmt.Errorf("create log directory: %w", err)
	}
	if err := applyKlogFileOnly(); err != nil {
		return nil, FilePaths{}, err
	}

	paths := FilePaths{
		Path: filepath.Join(logDir, defaultLogFilename),
	}

	logFile, err := os.OpenFile(paths.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, FilePaths{}, fmt.Errorf("open log file: %w", err)
	}

	klog.LogToStderr(false)
	klog.SetOutputBySeverity("INFO", logFile)
	klog.SetOutputBySeverity("ERROR", logFile)

	return logFile, paths, nil
}

func applyKlogFileOnly() error {
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	settings := map[string]string{
		"logtostderr":     "false",
		"alsologtostderr": "false",
		"stderrthreshold": "FATAL",
		"one_output":      "true",
	}
	for name, value := range settings {
		if err := fs.Set(name, value); err != nil {
			return fmt.Errorf("set klog flag %s: %w", name, err)
		}
	}
	return nil
}
