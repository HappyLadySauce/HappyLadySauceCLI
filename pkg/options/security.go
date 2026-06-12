package options

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
)

const (
	// ApprovalDefaultReview requires interactive confirmation for reviewed operations.
	// ApprovalDefaultReview 表示需要对 review 操作进行交互确认。
	ApprovalDefaultReview = "review"

	// PersistContentSanitized stores sanitized message content and raw JSON.
	// PersistContentSanitized 表示保存脱敏后的消息内容与 raw JSON。
	PersistContentSanitized = "sanitized"
	// PersistContentMetadataOnly stores replay metadata without message bodies.
	// PersistContentMetadataOnly 表示只保存消息元数据，不保存正文。
	PersistContentMetadataOnly = "metadata_only"
)

// SecurityOptions configures execution safety and persistence redaction.
// SecurityOptions 配置执行安全与持久化脱敏策略。
type SecurityOptions struct {
	WorkspaceRoots        []string `mapstructure:"workspace_roots"`
	ApprovalDefault       string   `mapstructure:"approval_default"`
	PersistContent        string   `mapstructure:"persist_content"`
	CommandTimeoutSeconds int      `mapstructure:"command_timeout_seconds"`
	MaxToolOutputBytes    int      `mapstructure:"max_tool_output_bytes"`
}

// NewSecurityOptions returns secure defaults for the current process.
// NewSecurityOptions 返回当前进程的安全默认配置。
func NewSecurityOptions() *SecurityOptions {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	return &SecurityOptions{
		WorkspaceRoots:        []string{cwd},
		ApprovalDefault:       ApprovalDefaultReview,
		PersistContent:        PersistContentSanitized,
		CommandTimeoutSeconds: 30,
		MaxToolOutputBytes:    1 << 20,
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
	if strings.TrimSpace(o.ApprovalDefault) == "" {
		o.ApprovalDefault = defaults.ApprovalDefault
	}
	if strings.TrimSpace(o.PersistContent) == "" {
		o.PersistContent = defaults.PersistContent
	}
	if o.CommandTimeoutSeconds == 0 {
		o.CommandTimeoutSeconds = defaults.CommandTimeoutSeconds
	}
	if o.MaxToolOutputBytes == 0 {
		o.MaxToolOutputBytes = defaults.MaxToolOutputBytes
	}

	var errs error
	for i, root := range o.WorkspaceRoots {
		normalized, err := normalizeWorkspaceRoot(root)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		o.WorkspaceRoots[i] = normalized
	}
	switch o.ApprovalDefault {
	case ApprovalDefaultReview:
	default:
		errs = errors.Join(errs, fmt.Errorf("security.approval_default must be %q", ApprovalDefaultReview))
	}
	switch o.PersistContent {
	case PersistContentSanitized, PersistContentMetadataOnly:
	default:
		errs = errors.Join(errs, fmt.Errorf("security.persist_content must be %q or %q", PersistContentSanitized, PersistContentMetadataOnly))
	}
	if o.CommandTimeoutSeconds <= 0 {
		errs = errors.Join(errs, errors.New("security.command_timeout_seconds must be greater than 0"))
	}
	if o.MaxToolOutputBytes <= 0 {
		errs = errors.Join(errs, errors.New("security.max_tool_output_bytes must be greater than 0"))
	}
	return errs
}

// AddFlags registers security options.
// AddFlags 注册安全配置命令行参数。
func (o *SecurityOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringSliceVar(&o.WorkspaceRoots, "security-workspace-roots", o.WorkspaceRoots, "Allowed workspace roots for path/file operation resources")
	fs.StringVar(&o.ApprovalDefault, "security-approval-default", o.ApprovalDefault, "Default approval mode for reviewed operations")
	fs.StringVar(&o.PersistContent, "security-persist-content", o.PersistContent, "Context persistence mode: sanitized or metadata_only")
	fs.IntVar(&o.CommandTimeoutSeconds, "security-command-timeout-seconds", o.CommandTimeoutSeconds, "Default timeout for command.run operations")
	fs.IntVar(&o.MaxToolOutputBytes, "security-max-tool-output-bytes", o.MaxToolOutputBytes, "Maximum tool output bytes")
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
