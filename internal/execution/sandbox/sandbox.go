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
	// DefaultProbeTimeout bounds WSL2 readiness checks before policy evaluation.
	// DefaultProbeTimeout 限制策略评估前 WSL2 就绪探测耗时。
	DefaultProbeTimeout = 5 * time.Second
	// DefaultProbeCacheTTL avoids repeated wsl.exe probes for adjacent command calls.
	// DefaultProbeCacheTTL 避免相邻 command 调用重复执行 wsl.exe 探测。
	DefaultProbeCacheTTL = 30 * time.Second
	// DefaultMaxEnvValueBytes bounds one allowlisted environment value.
	// DefaultMaxEnvValueBytes 限制单个允许透传环境变量值长度。
	DefaultMaxEnvValueBytes = 4 << 10
)

var (
	envKeyPattern          = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	wslDistributionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// Config describes command sandbox behavior.
// Config 描述命令 sandbox 行为。
type Config struct {
	Backend          string
	Network          string
	WSLDistribution  string
	AllowedEnvKeys   []string
	WorkspaceRoots   []string
	MaxOutputBytes   int
	ProbeTimeout     time.Duration
	ProbeCacheTTL    time.Duration
	MaxEnvValueBytes int
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
	cfg.Backend = strings.ToLower(strings.TrimSpace(cfg.Backend))
	cfg.Network = strings.ToLower(strings.TrimSpace(cfg.Network))
	cfg.WSLDistribution = strings.TrimSpace(cfg.WSLDistribution)
	if cfg.ProbeTimeout == 0 {
		cfg.ProbeTimeout = DefaultProbeTimeout
	}
	if cfg.ProbeCacheTTL == 0 {
		cfg.ProbeCacheTTL = DefaultProbeCacheTTL
	}
	if cfg.MaxEnvValueBytes == 0 {
		cfg.MaxEnvValueBytes = DefaultMaxEnvValueBytes
	}

	var errs error
	if cfg.Backend != BackendWSL2 {
		errs = errors.Join(errs, fmt.Errorf("unsupported command sandbox backend: %s", cfg.Backend))
	}
	if cfg.Network != NetworkDeny {
		errs = errors.Join(errs, fmt.Errorf("unsupported command sandbox network policy: %s", cfg.Network))
	}
	if cfg.WSLDistribution != "" && !wslDistributionPattern.MatchString(cfg.WSLDistribution) {
		errs = errors.Join(errs, fmt.Errorf("invalid WSL2 distribution name: %s", cfg.WSLDistribution))
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
	if cfg.ProbeTimeout < 0 {
		errs = errors.Join(errs, errors.New("command sandbox probe timeout cannot be negative"))
	}
	if cfg.ProbeCacheTTL < 0 {
		errs = errors.Join(errs, errors.New("command sandbox probe cache ttl cannot be negative"))
	}
	if cfg.MaxEnvValueBytes < 0 {
		errs = errors.Join(errs, errors.New("command sandbox max env value bytes cannot be negative"))
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
