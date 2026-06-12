package logger

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"k8s.io/klog/v2"

	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/appdirs"
)

const (
	defaultLogFilename      = "happyladysaucecli.log"
	defaultErrorLogFilename = "happyladysaucecli.error.log"
)

// FilePaths holds the on-disk diagnostic log destinations.
// FilePaths 保存磁盘诊断日志路径。
type FilePaths struct {
	InfoPath  string
	ErrorPath string
}

// ConfigureDefaultFile redirects klog output to <home>/logs/ and disables terminal logging.
// The returned closer must be closed after klog has been flushed.
//
// ConfigureDefaultFile 将 klog 输出重定向到 <home>/logs/，并禁用终端日志输出。
// 调用方必须在 flush klog 后关闭返回的 closer。
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
		InfoPath:  filepath.Join(logDir, defaultLogFilename),
		ErrorPath: filepath.Join(logDir, defaultErrorLogFilename),
	}

	infoFile, err := os.OpenFile(paths.InfoPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, FilePaths{}, fmt.Errorf("open info log file: %w", err)
	}
	errorFile, err := os.OpenFile(paths.ErrorPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_ = infoFile.Close()
		return nil, FilePaths{}, fmt.Errorf("open error log file: %w", err)
	}

	klog.LogToStderr(false)
	klog.SetOutputBySeverity("INFO", infoFile)
	klog.SetOutputBySeverity("ERROR", errorFile)

	return &multiCloser{closers: []io.Closer{infoFile, errorFile}}, paths, nil
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

type multiCloser struct {
	closers []io.Closer
}

func (m *multiCloser) Close() error {
	if m == nil {
		return nil
	}
	var joined error
	for _, closer := range m.closers {
		if closer == nil {
			continue
		}
		joined = errors.Join(joined, closer.Close())
	}
	return joined
}
