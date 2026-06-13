package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeExecutor struct {
	results []ExecuteResult
	errs    []error
	calls   []fakeExecCall
}

type fakeExecCall struct {
	name string
	args []string
	opts ExecuteOptions
}

func (e *fakeExecutor) Execute(ctx context.Context, name string, args []string, opts ExecuteOptions) (ExecuteResult, error) {
	e.calls = append(e.calls, fakeExecCall{
		name: name,
		args: append([]string(nil), args...),
		opts: opts,
	})
	index := len(e.calls) - 1
	if index < len(e.errs) && e.errs[index] != nil {
		return ExecuteResult{}, e.errs[index]
	}
	if index < len(e.results) {
		return e.results[index], nil
	}
	return ExecuteResult{}, nil
}

func TestWSL2RunnerProbeSuccess(t *testing.T) {
	t.Parallel()

	executor := &fakeExecutor{results: []ExecuteResult{{ExitCode: 0}}}
	runner := NewWSL2Runner(Config{Backend: BackendWSL2, Network: NetworkDeny}, executor)

	status := runner.Probe(context.Background())
	if !status.Available || status.Backend != BackendWSL2 {
		t.Fatalf("Probe() = %#v, want available wsl2", status)
	}
	if len(executor.calls) != 1 || executor.calls[0].name != "wsl.exe" {
		t.Fatalf("executor calls = %#v, want wsl.exe probe", executor.calls)
	}
}

func TestWSL2RunnerProbeFailure(t *testing.T) {
	t.Parallel()

	executor := &fakeExecutor{errs: []error{errors.New("wsl missing")}}
	runner := NewWSL2Runner(Config{Backend: BackendWSL2, Network: NetworkDeny}, executor)

	status := runner.Probe(context.Background())
	if status.Available {
		t.Fatalf("Probe() = %#v, want unavailable", status)
	}
	if !strings.Contains(status.Reason, "wsl missing") {
		t.Fatalf("Probe() reason = %q, want executor error", status.Reason)
	}
}

func TestWSL2RunnerRunUsesBubblewrapAndReportsExitCode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	executor := &fakeExecutor{
		results: []ExecuteResult{
			{ExitCode: 0},
			{Stdout: "done", Stderr: "warn", ExitCode: 7},
		},
	}
	runner := NewWSL2Runner(Config{
		Backend:        BackendWSL2,
		Network:        NetworkDeny,
		WorkspaceRoots: []string{root},
		AllowedEnvKeys: []string{"PATH"},
	}, executor)

	result, err := runner.Run(context.Background(), Request{
		Command: "go",
		Args:    []string{"test", "./..."},
		WorkDir: root,
		Env:     map[string]string{"PATH": "/usr/bin", "SECRET": "nope"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 7 || result.Stdout != "done" || result.Stderr != "warn" {
		t.Fatalf("Run() result = %#v, want nonzero command result", result)
	}
	if len(executor.calls) != 2 {
		t.Fatalf("executor call count = %d, want probe + run", len(executor.calls))
	}
	runArgs := strings.Join(executor.calls[1].args, " ")
	for _, want := range []string{"bwrap", "--unshare-net", "--clearenv", "--ro-bind", "-- go test ./..."} {
		if !strings.Contains(runArgs, want) {
			t.Fatalf("run args = %q, missing %q", runArgs, want)
		}
	}
	if strings.Contains(runArgs, "SECRET") {
		t.Fatalf("run args leaked non-allowlisted env: %q", runArgs)
	}
}

func TestWSL2RunnerRunReusesCachedProbe(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	executor := &fakeExecutor{
		results: []ExecuteResult{
			{ExitCode: 0},
			{Stdout: "done"},
		},
	}
	runner := NewWSL2Runner(Config{
		Backend:        BackendWSL2,
		Network:        NetworkDeny,
		WorkspaceRoots: []string{root},
		ProbeCacheTTL:  time.Minute,
	}, executor)

	status := runner.Probe(context.Background())
	if !status.Available {
		t.Fatalf("Probe() = %#v, want available", status)
	}
	if _, err := runner.Run(context.Background(), Request{Command: "true", WorkDir: root}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(executor.calls) != 2 {
		t.Fatalf("executor call count = %d, want one probe plus one run", len(executor.calls))
	}
}

func TestWSL2RunnerProbeTimeout(t *testing.T) {
	t.Parallel()

	runner := NewWSL2Runner(Config{
		Backend:      BackendWSL2,
		Network:      NetworkDeny,
		ProbeTimeout: time.Millisecond,
	}, blockingExecutor{})

	status := runner.Probe(context.Background())
	if status.Available || status.Reason != "probe_timeout" {
		t.Fatalf("Probe() = %#v, want probe_timeout unavailable status", status)
	}
}

func TestWSL2RunnerRunReportsTimeoutAndTruncation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	executor := &fakeExecutor{
		results: []ExecuteResult{
			{ExitCode: 0},
			{Stdout: "partial", TimedOut: true, OutputTruncated: true},
		},
	}
	runner := NewWSL2Runner(Config{
		Backend:        BackendWSL2,
		Network:        NetworkDeny,
		WorkspaceRoots: []string{root},
	}, executor)

	result, err := runner.Run(context.Background(), Request{
		Command:        "sleep",
		Args:           []string{"10"},
		WorkDir:        root,
		MaxOutputBytes: 8,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.TimedOut || !result.OutputTruncated {
		t.Fatalf("Run() result = %#v, want timeout and truncation", result)
	}
	if executor.calls[1].opts.MaxOutputBytes != 8 {
		t.Fatalf("MaxOutputBytes = %d, want request override", executor.calls[1].opts.MaxOutputBytes)
	}
}

func TestWSL2RunnerRejectsOversizedAllowlistedEnvValue(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	executor := &fakeExecutor{results: []ExecuteResult{{ExitCode: 0}}}
	runner := NewWSL2Runner(Config{
		Backend:          BackendWSL2,
		Network:          NetworkDeny,
		WorkspaceRoots:   []string{root},
		AllowedEnvKeys:   []string{"PATH"},
		MaxEnvValueBytes: 4,
	}, executor)

	_, err := runner.Run(context.Background(), Request{
		Command: "go",
		WorkDir: root,
		Env:     map[string]string{"PATH": "/too/long"},
	})
	if err == nil || !strings.Contains(err.Error(), "env value") {
		t.Fatalf("Run() error = %v, want env value length error", err)
	}
	if len(executor.calls) != 1 {
		t.Fatalf("executor call count = %d, want only probe call", len(executor.calls))
	}
}

func TestNormalizeConfigRejectsInvalidDistribution(t *testing.T) {
	t.Parallel()

	_, err := NormalizeConfig(Config{
		Backend:         BackendWSL2,
		Network:         NetworkDeny,
		WSLDistribution: "Ubuntu;rm",
	})
	if err == nil || !strings.Contains(err.Error(), "distribution") {
		t.Fatalf("NormalizeConfig() error = %v, want invalid distribution error", err)
	}
}

type blockingExecutor struct{}

func (blockingExecutor) Execute(ctx context.Context, name string, args []string, opts ExecuteOptions) (ExecuteResult, error) {
	<-ctx.Done()
	return ExecuteResult{}, ctx.Err()
}
