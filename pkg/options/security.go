package options

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/pflag"
)

const (
	// CommandSandboxBackendWSL2 runs command operations through WSL2.
	// CommandSandboxBackendWSL2 表示通过 WSL2 执行命令操作。
	CommandSandboxBackendWSL2 = "wsl2"
	// CommandSandboxNetworkDeny disables command sandbox network access.
	// CommandSandboxNetworkDeny 表示禁止 command sandbox 网络访问。
	CommandSandboxNetworkDeny = "deny"
	// PersistContentSanitized stores sanitized message content and raw JSON.
	// PersistContentSanitized 表示保存脱敏后的消息内容与 raw JSON。
	PersistContentSanitized = "sanitized"
	// PersistContentMetadataOnly stores replay metadata without message bodies.
	// PersistContentMetadataOnly 表示只保存消息元数据，不保存正文。
	PersistContentMetadataOnly = "metadata_only"
)

var (
	commandSandboxEnvKeyPattern          = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	commandSandboxWSLDistributionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// CommandSandboxOptions configures runtime isolation for command.run operations.
// CommandSandboxOptions 配置 command.run 操作的运行时隔离。
type CommandSandboxOptions struct {
	Backend         string   `mapstructure:"backend"`
	FailClosed      bool     `mapstructure:"fail_closed"`
	Network         string   `mapstructure:"network"`
	WSLDistribution string   `mapstructure:"wsl_distribution"`
	AllowedEnvKeys  []string `mapstructure:"allowed_env_keys"`
}

// SecurityOptions configures execution safety and persistence redaction.
// SecurityOptions 配置执行安全与持久化脱敏策略。
type SecurityOptions struct {
	WorkspaceRoots              []string              `mapstructure:"workspace_roots"`
	PersistContent              string                `mapstructure:"persist_content"`
	CommandTimeoutSeconds       int                   `mapstructure:"command_timeout_seconds"`
	FileOperationTimeoutSeconds int                   `mapstructure:"file_operation_timeout_seconds"`
	FileMaxBytes                int                   `mapstructure:"file_max_bytes"`
	FileMaxLineBytes            int                   `mapstructure:"file_max_line_bytes"`
	MaxToolOutputBytes          int                   `mapstructure:"max_tool_output_bytes"`
	CommandSandbox              CommandSandboxOptions `mapstructure:"command_sandbox"`
}

// NewSecurityOptions returns secure defaults for the current process.
// NewSecurityOptions 返回当前进程的安全默认配置。
func NewSecurityOptions() *SecurityOptions {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	return &SecurityOptions{
		WorkspaceRoots:              []string{cwd},
		PersistContent:              PersistContentSanitized,
		CommandTimeoutSeconds:       30,
		FileOperationTimeoutSeconds: 30,
		FileMaxBytes:                16 << 20,
		FileMaxLineBytes:            64 << 10,
		MaxToolOutputBytes:          1 << 20,
		CommandSandbox: CommandSandboxOptions{
			Backend:        CommandSandboxBackendWSL2,
			FailClosed:     true,
			Network:        CommandSandboxNetworkDeny,
			AllowedEnvKeys: []string{"PATH", "HOME", "LANG", "LC_ALL", "TERM"},
		},
	}
}

// Validate normalizes and validates security options.
// Validate 规范化并校验安全配置。
func (o *SecurityOptions) Validate() error {
	if o == nil {
		return nil
	}
	defaults := NewSecurityOptions()
	if len(o.WorkspaceRoots) == 0 {
		o.WorkspaceRoots = append([]string(nil), defaults.WorkspaceRoots...)
	}
	if strings.TrimSpace(o.PersistContent) == "" {
		o.PersistContent = defaults.PersistContent
	}
	if o.CommandTimeoutSeconds == 0 {
		o.CommandTimeoutSeconds = defaults.CommandTimeoutSeconds
	}
	if o.FileOperationTimeoutSeconds == 0 {
		o.FileOperationTimeoutSeconds = defaults.FileOperationTimeoutSeconds
	}
	if o.FileMaxBytes == 0 {
		o.FileMaxBytes = defaults.FileMaxBytes
	}
	if o.FileMaxLineBytes == 0 {
		o.FileMaxLineBytes = defaults.FileMaxLineBytes
	}
	if o.MaxToolOutputBytes == 0 {
		o.MaxToolOutputBytes = defaults.MaxToolOutputBytes
	}
	o.applyCommandSandboxDefaults(defaults.CommandSandbox)

	var errs error
	for i, root := range o.WorkspaceRoots {
		normalized, err := normalizeWorkspaceRoot(root)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		o.WorkspaceRoots[i] = normalized
	}
	switch o.PersistContent {
	case PersistContentSanitized, PersistContentMetadataOnly:
	default:
		errs = errors.Join(errs, fmt.Errorf("security.persist_content must be %q or %q", PersistContentSanitized, PersistContentMetadataOnly))
	}
	if o.CommandTimeoutSeconds <= 0 {
		errs = errors.Join(errs, errors.New("security.command_timeout_seconds must be greater than 0"))
	}
	if o.FileOperationTimeoutSeconds <= 0 {
		errs = errors.Join(errs, errors.New("security.file_operation_timeout_seconds must be greater than 0"))
	}
	if o.FileMaxBytes <= 0 {
		errs = errors.Join(errs, errors.New("security.file_max_bytes must be greater than 0"))
	}
	if o.FileMaxLineBytes <= 0 {
		errs = errors.Join(errs, errors.New("security.file_max_line_bytes must be greater than 0"))
	}
	if o.MaxToolOutputBytes <= 0 {
		errs = errors.Join(errs, errors.New("security.max_tool_output_bytes must be greater than 0"))
	}
	switch o.CommandSandbox.Backend {
	case CommandSandboxBackendWSL2:
	default:
		errs = errors.Join(errs, fmt.Errorf("security.command_sandbox.backend must be %q", CommandSandboxBackendWSL2))
	}
	if !o.CommandSandbox.FailClosed {
		errs = errors.Join(errs, errors.New("security.command_sandbox.fail_closed must be true"))
	}
	switch o.CommandSandbox.Network {
	case CommandSandboxNetworkDeny:
	default:
		errs = errors.Join(errs, fmt.Errorf("security.command_sandbox.network must be %q", CommandSandboxNetworkDeny))
	}
	if o.CommandSandbox.WSLDistribution != "" && !commandSandboxWSLDistributionPattern.MatchString(o.CommandSandbox.WSLDistribution) {
		errs = errors.Join(errs, fmt.Errorf("security.command_sandbox.wsl_distribution contains invalid characters: %q", o.CommandSandbox.WSLDistribution))
	}
	for i, key := range o.CommandSandbox.AllowedEnvKeys {
		key = strings.TrimSpace(key)
		o.CommandSandbox.AllowedEnvKeys[i] = key
		if !commandSandboxEnvKeyPattern.MatchString(key) {
			errs = errors.Join(errs, fmt.Errorf("security.command_sandbox.allowed_env_keys contains invalid key %q", key))
		}
	}
	return errs
}

// AddFlags registers security options.
// AddFlags 注册安全配置命令行参数。
func (o *SecurityOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringSliceVar(&o.WorkspaceRoots, "security-workspace-roots", o.WorkspaceRoots, "Allowed workspace roots for path/file operation resources")
	fs.StringVar(&o.PersistContent, "security-persist-content", o.PersistContent, "Context persistence mode: sanitized or metadata_only")
	fs.IntVar(&o.CommandTimeoutSeconds, "security-command-timeout-seconds", o.CommandTimeoutSeconds, "Default timeout for command.run operations")
	fs.IntVar(&o.FileOperationTimeoutSeconds, "security-file-operation-timeout-seconds", o.FileOperationTimeoutSeconds, "Default timeout for file.* operations")
	fs.IntVar(&o.FileMaxBytes, "security-file-max-bytes", o.FileMaxBytes, "Maximum bytes for one file tool input file")
	fs.IntVar(&o.FileMaxLineBytes, "security-file-max-line-bytes", o.FileMaxLineBytes, "Maximum bytes for one line returned by file_read")
	fs.IntVar(&o.MaxToolOutputBytes, "security-max-tool-output-bytes", o.MaxToolOutputBytes, "Maximum tool output bytes")
	fs.StringVar(&o.CommandSandbox.Backend, "security-command-sandbox-backend", o.CommandSandbox.Backend, "Command sandbox backend; only wsl2 is supported")
	fs.BoolVar(&o.CommandSandbox.FailClosed, "security-command-sandbox-fail-closed", o.CommandSandbox.FailClosed, "Reject command.run when the sandbox is unavailable")
	fs.StringVar(&o.CommandSandbox.Network, "security-command-sandbox-network", o.CommandSandbox.Network, "Command sandbox network policy; only deny is supported")
	fs.StringVar(&o.CommandSandbox.WSLDistribution, "security-command-sandbox-wsl-distribution", o.CommandSandbox.WSLDistribution, "Optional WSL2 distribution for command sandbox execution")
	fs.StringSliceVar(&o.CommandSandbox.AllowedEnvKeys, "security-command-sandbox-allowed-env-keys", o.CommandSandbox.AllowedEnvKeys, "Allowed environment variable names for command sandbox execution")
}

func (o *SecurityOptions) applyCommandSandboxDefaults(defaults CommandSandboxOptions) {
	if strings.TrimSpace(o.CommandSandbox.Backend) == "" {
		o.CommandSandbox.Backend = defaults.Backend
	}
	if strings.TrimSpace(o.CommandSandbox.Network) == "" {
		o.CommandSandbox.Network = defaults.Network
	}
	if len(o.CommandSandbox.AllowedEnvKeys) == 0 {
		o.CommandSandbox.AllowedEnvKeys = append([]string(nil), defaults.AllowedEnvKeys...)
	}
	if !o.CommandSandbox.FailClosed {
		o.CommandSandbox.FailClosed = defaults.FailClosed
	}
	o.CommandSandbox.Backend = strings.ToLower(strings.TrimSpace(o.CommandSandbox.Backend))
	o.CommandSandbox.Network = strings.ToLower(strings.TrimSpace(o.CommandSandbox.Network))
	o.CommandSandbox.WSLDistribution = strings.TrimSpace(o.CommandSandbox.WSLDistribution)
}

func normalizeWorkspaceRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("security.workspace_roots cannot contain empty values")
	}
	if !filepath.IsAbs(root) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
		root = filepath.Join(cwd, root)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root %q: %w", root, err)
	}
	return filepath.Clean(absolute), nil
}
