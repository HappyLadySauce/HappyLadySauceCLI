// Package sandbox provides command execution sandbox backends.
// Package sandbox 提供命令执行 sandbox 后端。
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	// BackendWSL2 executes commands through wsl.exe and a Linux-side sandbox runtime.
	// BackendWSL2 通过 wsl.exe 与 Linux 侧 sandbox runtime 执行命令。
	BackendWSL2 = "wsl2"
	// NetworkDeny disables network access for sandboxed command processes.
	// NetworkDeny 禁止 sandboxed command 进程访问网络。
	NetworkDeny = "deny"
)

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Config describes command sandbox behavior.
// Config 描述命令 sandbox 行为。
type Config struct {
	Backend         string
	FailClosed      bool
	Network         string
	WSLDistribution string
	AllowedEnvKeys  []string
	WorkspaceRoots  []string
	MaxOutputBytes  int
}

// Status reports whether a sandbox backend is ready to execute commands.
// Status 描述 sandbox 后端是否已准备好执行命令。
type Status struct {
	Backend   string
	Available bool
	Reason    string
}

// Request contains one sandboxed command invocation.
// Request 表示一次 sandboxed command 调用。
type Request struct {
	Command        string
	Args           []string
	WorkDir        string
	Env            map[string]string
	Timeout        time.Duration
	MaxOutputBytes int
}

// Result contains sanitized command execution metadata and bounded output.
// Result 包含命令执行的脱敏元数据与有界输出。
type Result struct {
	Stdout          string
	Stderr          string
	ExitCode        int
	TimedOut        bool
	OutputTruncated bool
	ElapsedMS       int64
}

// Runner probes and executes command requests inside a sandbox backend.
// Runner 在 sandbox 后端中探测并执行命令请求。
type Runner interface {
	Probe(ctx context.Context) Status
	Run(ctx context.Context, request Request) (Result, error)
}

// DefaultAllowedEnvKeys returns the safe default command environment allowlist.
// DefaultAllowedEnvKeys 返回安全的默认命令环境变量 allowlist。
func DefaultAllowedEnvKeys() []string {
	return []string{"PATH", "HOME", "LANG", "LC_ALL", "TERM"}
}

// NormalizeConfig applies defaults and validates sandbox configuration.
// NormalizeConfig 应用默认值并校验 sandbox 配置。
func NormalizeConfig(cfg Config) (Config, error) {
	if strings.TrimSpace(cfg.Backend) == "" {
		cfg.Backend = BackendWSL2
	}
	if strings.TrimSpace(cfg.Network) == "" {
		cfg.Network = NetworkDeny
	}
	if len(cfg.AllowedEnvKeys) == 0 {
		cfg.AllowedEnvKeys = DefaultAllowedEnvKeys()
	}
	if !cfg.FailClosed {
		cfg.FailClosed = true
	}
	cfg.Backend = strings.ToLower(strings.TrimSpace(cfg.Backend))
	cfg.Network = strings.ToLower(strings.TrimSpace(cfg.Network))
	cfg.WSLDistribution = strings.TrimSpace(cfg.WSLDistribution)

	var errs error
	if cfg.Backend != BackendWSL2 {
		errs = errors.Join(errs, fmt.Errorf("unsupported command sandbox backend: %s", cfg.Backend))
	}
	if cfg.Network != NetworkDeny {
		errs = errors.Join(errs, fmt.Errorf("unsupported command sandbox network policy: %s", cfg.Network))
	}
	for _, key := range cfg.AllowedEnvKeys {
		key = strings.TrimSpace(key)
		if !envKeyPattern.MatchString(key) {
			errs = errors.Join(errs, fmt.Errorf("invalid command sandbox env key: %s", key))
		}
	}
	if cfg.MaxOutputBytes < 0 {
		errs = errors.Join(errs, errors.New("command sandbox max output bytes cannot be negative"))
	}
	return cfg, errs
}

// NewRunner creates a sandbox runner from configuration.
// NewRunner 基于配置创建 sandbox runner。
func NewRunner(cfg Config) (Runner, error) {
	normalized, err := NormalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return NewWSL2Runner(normalized, nil), nil
}
