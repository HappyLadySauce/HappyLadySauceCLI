// Package appdirs provides shared application directory paths.
// Package appdirs 提供共享的应用目录路径。
package appdirs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const defaultDirName = ".HAPPLADYSAUCECLI"

var (
	homeMu     sync.RWMutex
	homeDir    string
	resolveCwd = os.Getwd
)

// DefaultDir returns the shared application data directory under the user home.
// It returns ~/.HAPPLADYSAUCECLI and does not create the directory.
//
// DefaultDir 返回用户 home 下的共享应用数据目录。
// 它返回 ~/.HAPPLADYSAUCECLI，但不会创建目录。
func DefaultDir() (string, error) {
	homeMu.RLock()
	configured := homeDir
	homeMu.RUnlock()
	if configured != "" {
		return configured, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home directory: %w", err)
	}
	return filepath.Join(homeDir, defaultDirName), nil
}

// SetHomeDir configures the application home directory used by data and logs.
// Relative paths are resolved against the current working directory.
//
// SetHomeDir 配置数据与日志使用的应用 home 目录。
// 相对路径会基于当前工作目录解析。
func SetHomeDir(path string) error {
	if strings.TrimSpace(path) == "" {
		homeMu.Lock()
		homeDir = ""
		homeMu.Unlock()
		return nil
	}
	resolved, err := ResolveHomeDir(path)
	if err != nil {
		return err
	}
	homeMu.Lock()
	homeDir = resolved
	homeMu.Unlock()
	return nil
}

// ResolveHomeDir returns an absolute application home directory.
// Empty input resolves to the default ~/.HAPPLADYSAUCECLI directory.
//
// ResolveHomeDir 返回绝对应用 home 目录。
// 空输入会解析为默认 ~/.HAPPLADYSAUCECLI 目录。
func ResolveHomeDir(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return defaultHomeDir()
	}
	if path == "~" || strings.HasPrefix(path, "~"+string(filepath.Separator)) || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get user home directory: %w", err)
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"+string(filepath.Separator)), "~/"))
		}
	}
	if !filepath.IsAbs(path) {
		cwd, err := resolveCwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path), nil
}

// LogsDir returns the default diagnostic log directory under the application data directory.
// It returns ~/.HAPPLADYSAUCECLI/logs and does not create the directory.
//
// LogsDir 返回应用数据目录下的默认诊断日志目录。
// 它返回 ~/.HAPPLADYSAUCECLI/logs，但不会创建目录。
func LogsDir() (string, error) {
	defaultDir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(defaultDir, "logs"), nil
}

func defaultHomeDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home directory: %w", err)
	}
	return filepath.Join(homeDir, defaultDirName), nil
}
