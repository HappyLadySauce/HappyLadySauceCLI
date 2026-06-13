package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const defaultLinuxPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// Executor runs a host command and returns bounded process output.
// Executor 运行宿主命令并返回有界进程输出。
type Executor interface {
	Execute(ctx context.Context, name string, args []string, opts ExecuteOptions) (ExecuteResult, error)
}

// ExecuteOptions configures one host command execution.
// ExecuteOptions 配置一次宿主命令执行。
type ExecuteOptions struct {
	Env            []string
	MaxOutputBytes int
}

// ExecuteResult contains one host command result.
// ExecuteResult 包含一次宿主命令执行结果。
type ExecuteResult struct {
	Stdout          string
	Stderr          string
	ExitCode        int
	TimedOut        bool
	OutputTruncated bool
}

// WSL2Runner executes commands through wsl.exe and bubblewrap.
// WSL2Runner 通过 wsl.exe 与 bubblewrap 执行命令。
type WSL2Runner struct {
	cfg         Config
	executor    Executor
	probeMu     sync.Mutex
	probeStatus Status
	probeAt     time.Time
}

// NewWSL2Runner creates a WSL2 sandbox runner.
// NewWSL2Runner 创建 WSL2 sandbox runner。
func NewWSL2Runner(cfg Config, executor Executor) *WSL2Runner {
	if executor == nil {
		executor = osExecutor{}
	}
	if normalized, err := NormalizeConfig(cfg); err == nil {
		cfg = normalized
	}
	return &WSL2Runner{cfg: cfg, executor: executor}
}

// Probe verifies that WSL2 and the Linux-side sandbox runtime are available.
// Probe 校验 WSL2 与 Linux 侧 sandbox runtime 是否可用。
func (r *WSL2Runner) Probe(ctx context.Context) Status {
	status := Status{Backend: BackendWSL2}
	if r == nil || r.executor == nil {
		status.Reason = "sandbox runner is incomplete"
		return status
	}
	if cached, ok := r.cachedProbeStatus(time.Now()); ok {
		return cached
	}
	r.probeMu.Lock()
	defer r.probeMu.Unlock()
	now := time.Now()
	if cached, ok := r.cachedProbeStatusLocked(now); ok {
		return cached
	}
	probeCtx := ctx
	cancel := func() {}
	if r.cfg.ProbeTimeout > 0 {
		probeCtx, cancel = context.WithTimeout(ctx, r.cfg.ProbeTimeout)
	}
	defer cancel()
	args := r.wslArgs("sh", "-lc", "command -v bwrap >/dev/null 2>&1")
	result, err := r.executor.Execute(probeCtx, "wsl.exe", args, ExecuteOptions{MaxOutputBytes: 4096})
	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			status.Reason = "probe_timeout"
		} else {
			status.Reason = err.Error()
		}
		r.storeProbeStatus(status, now)
		return status
	}
	if errors.Is(probeCtx.Err(), context.DeadlineExceeded) || result.TimedOut {
		status.Reason = "probe_timeout"
		r.storeProbeStatus(status, now)
		return status
	}
	if result.ExitCode != 0 {
		status.Reason = "bubblewrap is unavailable in WSL2 distribution"
		r.storeProbeStatus(status, now)
		return status
	}
	status.Available = true
	status.Reason = "ready"
	r.storeProbeStatus(status, now)
	return status
}

// Run executes one command inside the WSL2 sandbox.
// Run 在 WSL2 sandbox 中执行一次命令。
func (r *WSL2Runner) Run(ctx context.Context, request Request) (Result, error) {
	start := time.Now()
	status := r.Probe(ctx)
	if !status.Available {
		return Result{}, fmt.Errorf("command sandbox unavailable: %s", status.Reason)
	}
	command := strings.TrimSpace(request.Command)
	if command == "" {
		return Result{}, errors.New("command is required")
	}
	workDir, err := r.resolveWorkDir(request.WorkDir)
	if err != nil {
		return Result{}, err
	}
	args, err := r.bwrapArgs(command, request.Args, workDir, request.Env)
	if err != nil {
		return Result{}, err
	}
	execCtx := ctx
	cancel := func() {}
	if request.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, request.Timeout)
	}
	defer cancel()

	maxOutputBytes := request.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = r.cfg.MaxOutputBytes
	}
	hostResult, err := r.executor.Execute(execCtx, "wsl.exe", r.wslArgs(args...), ExecuteOptions{
		MaxOutputBytes: maxOutputBytes,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{
		Stdout:          hostResult.Stdout,
		Stderr:          hostResult.Stderr,
		ExitCode:        hostResult.ExitCode,
		TimedOut:        hostResult.TimedOut,
		OutputTruncated: hostResult.OutputTruncated,
		ElapsedMS:       time.Since(start).Milliseconds(),
	}, nil
}

func (r *WSL2Runner) wslArgs(args ...string) []string {
	next := make([]string, 0, len(args)+3)
	if strings.TrimSpace(r.cfg.WSLDistribution) != "" {
		next = append(next, "-d", r.cfg.WSLDistribution)
	}
	next = append(next, "--")
	next = append(next, args...)
	return next
}

func (r *WSL2Runner) bwrapArgs(command string, commandArgs []string, workDir string, env map[string]string) ([]string, error) {
	if r.cfg.Network != NetworkDeny {
		return nil, fmt.Errorf("unsupported command sandbox network policy: %s", r.cfg.Network)
	}
	args := []string{
		"bwrap",
		"--die-with-parent",
		"--unshare-net",
		"--clearenv",
		"--setenv", "PATH", defaultLinuxPath,
		"--setenv", "HOME", "/tmp",
	}
	envArgs, err := r.allowedEnvArgs(env)
	if err != nil {
		return nil, err
	}
	args = append(args, envArgs...)
	for _, path := range []string{"/bin", "/usr", "/lib", "/lib64", "/etc"} {
		args = append(args, "--ro-bind-try", path, path)
	}
	for _, root := range r.cfg.WorkspaceRoots {
		wslRoot, err := windowsPathToWSL(root)
		if err != nil {
			return nil, fmt.Errorf("convert workspace root for WSL2: %w", err)
		}
		args = append(args, "--ro-bind", wslRoot, wslRoot)
	}
	args = append(args, "--tmpfs", "/tmp")
	if workDir != "" {
		args = append(args, "--chdir", workDir)
	}
	args = append(args, "--", command)
	args = append(args, commandArgs...)
	return args, nil
}

func (r *WSL2Runner) allowedEnvArgs(env map[string]string) ([]string, error) {
	if len(env) == 0 || len(r.cfg.AllowedEnvKeys) == 0 {
		return nil, nil
	}
	allowed := make(map[string]struct{}, len(r.cfg.AllowedEnvKeys))
	for _, key := range r.cfg.AllowedEnvKeys {
		allowed[strings.TrimSpace(key)] = struct{}{}
	}
	args := make([]string, 0, len(env)*3)
	for key, value := range env {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if r.cfg.MaxEnvValueBytes > 0 && len(value) > r.cfg.MaxEnvValueBytes {
			return nil, fmt.Errorf("command sandbox env value for %s exceeds max length: %d > %d", key, len(value), r.cfg.MaxEnvValueBytes)
		}
		args = append(args, "--setenv", key, value)
	}
	return args, nil
}

func (r *WSL2Runner) cachedProbeStatus(now time.Time) (Status, bool) {
	r.probeMu.Lock()
	defer r.probeMu.Unlock()
	return r.cachedProbeStatusLocked(now)
}

func (r *WSL2Runner) cachedProbeStatusLocked(now time.Time) (Status, bool) {
	if r.cfg.ProbeCacheTTL <= 0 || r.probeAt.IsZero() {
		return Status{}, false
	}
	if now.Sub(r.probeAt) > r.cfg.ProbeCacheTTL {
		return Status{}, false
	}
	return r.probeStatus, true
}

func (r *WSL2Runner) storeProbeStatus(status Status, now time.Time) {
	if r.cfg.ProbeCacheTTL <= 0 {
		return
	}
	r.probeStatus = status
	r.probeAt = now
}

func (r *WSL2Runner) resolveWorkDir(workDir string) (string, error) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		if len(r.cfg.WorkspaceRoots) == 0 {
			return "", nil
		}
		return windowsPathToWSL(r.cfg.WorkspaceRoots[0])
	}
	if len(r.cfg.WorkspaceRoots) > 0 && !pathWithinAnyRoot(workDir, r.cfg.WorkspaceRoots) {
		return "", fmt.Errorf("command workdir escapes workspace roots: %s", workDir)
	}
	return windowsPathToWSL(workDir)
}

func pathWithinAnyRoot(path string, roots []string) bool {
	path = filepath.Clean(path)
	for _, root := range roots {
		root = filepath.Clean(root)
		if sameHostPath(path, root) {
			return true
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			continue
		}
		if rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func sameHostPath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func windowsPathToWSL(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return "", errors.New("path is required")
	}
	volume := filepath.VolumeName(path)
	if volume == "" {
		return filepath.ToSlash(path), nil
	}
	drive := strings.TrimSuffix(volume, ":")
	if len(drive) != 1 {
		return "", fmt.Errorf("unsupported Windows volume: %s", volume)
	}
	rest := strings.TrimPrefix(path, volume)
	rest = strings.TrimLeft(rest, `\/`)
	rest = filepath.ToSlash(rest)
	if rest == "" {
		return "/mnt/" + strings.ToLower(drive), nil
	}
	return "/mnt/" + strings.ToLower(drive) + "/" + rest, nil
}

type osExecutor struct{}

func (osExecutor) Execute(ctx context.Context, name string, args []string, opts ExecuteOptions) (ExecuteResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(opts.Env) > 0 {
		cmd.Env = append([]string(nil), opts.Env...)
	}
	stdout := newBoundedBuffer(opts.MaxOutputBytes)
	stderr := newBoundedBuffer(opts.MaxOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	result := ExecuteResult{
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		OutputTruncated: stdout.Truncated() || stderr.Truncated(),
	}
	if ctx.Err() != nil {
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		return result, nil
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, err
}

type boundedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newBoundedBuffer(limit int) *boundedBuffer {
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b == nil {
		return len(p), nil
	}
	if b.limit <= 0 {
		_, _ = b.buf.Write(p)
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	if b == nil {
		return ""
	}
	return b.buf.String()
}

func (b *boundedBuffer) Truncated() bool {
	return b != nil && b.truncated
}
