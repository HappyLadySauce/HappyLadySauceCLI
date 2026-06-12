// Package security defines operation-level execution safety primitives.
// Package security 定义操作级执行安全基础类型。
package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
)

const (
	// ResourceKindDeclared represents a descriptor-declared static resource.
	// ResourceKindDeclared 表示 descriptor 声明的静态资源。
	ResourceKindDeclared = "declared"
	// ResourceKindPath represents a local filesystem path resource.
	// ResourceKindPath 表示本地文件系统路径资源。
	ResourceKindPath = "path"
	// ResourceKindFile represents a local filesystem file resource.
	// ResourceKindFile 表示本地文件系统文件资源。
	ResourceKindFile = "file"
	// ResourceKindURL represents a network URL resource.
	// ResourceKindURL 表示网络 URL 资源。
	ResourceKindURL = "url"
)

const (
	// OperationNativeTool represents a built-in tool call without a narrower operation kind.
	// OperationNativeTool 表示尚未细分操作类型的内置工具调用。
	OperationNativeTool = "native.tool"
	// OperationFileRead is reserved for future file read tools.
	// OperationFileRead 预留给未来的文件读取工具。
	OperationFileRead = "file.read"
	// OperationFileList is reserved for future file listing tools.
	// OperationFileList 预留给未来的文件列表工具。
	OperationFileList = "file.list"
	// OperationFileWrite is reserved for future file write tools.
	// OperationFileWrite 预留给未来的文件写入工具。
	OperationFileWrite = "file.write"
	// OperationFileDelete is reserved for future file deletion tools.
	// OperationFileDelete 预留给未来的文件删除工具。
	OperationFileDelete = "file.delete"
	// OperationCommandRun is reserved for future command execution tools.
	// OperationCommandRun 预留给未来的命令执行工具。
	OperationCommandRun = "command.run"
)

const (
	// ApprovalScopeNone means no approval was requested or reused.
	// ApprovalScopeNone 表示未请求或复用审批。
	ApprovalScopeNone = "none"
	// ApprovalScopeOnce means approval applies only to the current operation call.
	// ApprovalScopeOnce 表示审批只作用于当前操作调用。
	ApprovalScopeOnce = "once"
	// ApprovalScopeSession means approval can be reused for the same grant key in this process.
	// ApprovalScopeSession 表示审批可在当前进程中复用于相同授权 key。
	ApprovalScopeSession = "session"
)

// OperationResource describes one normalized resource touched by a capability call.
// OperationResource 描述一次 capability 调用涉及的规范化资源。
type OperationResource struct {
	Kind  string
	Value string
}

// OperationRequest is the policy input for one concrete tool operation.
// OperationRequest 是单次具体工具操作的策略输入。
type OperationRequest struct {
	ToolName             string
	ToolCallID           string
	Capability           capability.Descriptor
	Registered           bool
	OperationKind        string
	Resources            []OperationResource
	Risk                 capability.RiskLevel
	SanitizedArgsSummary string
}

// OperationBuildInput carries sanitized and raw tool arguments for builders.
// OperationBuildInput 携带供 builder 使用的脱敏与原始工具参数。
type OperationBuildInput struct {
	RawJSON string
	Summary string
}

// OperationBuilder enriches the default operation request from tool input.
// OperationBuilder 基于工具输入补充默认操作请求。
type OperationBuilder func(ctx context.Context, request OperationRequest, input OperationBuildInput) OperationRequest

// GrantKey returns the stable reusable approval key for the operation.
// GrantKey 返回该操作对应的稳定可复用授权 key。
func (r OperationRequest) GrantKey() string {
	return r.grantKey(r.includeArgsSummaryInGrantKey())
}

// SessionGrantKey returns the approval key stored when the user chooses session scope.
// SessionGrantKey 返回用户选择 session 审批范围时写入的授权 key。
func (r OperationRequest) SessionGrantKey() string {
	return r.grantKey(r.includeArgsSummaryInSessionGrantKey())
}

func (r OperationRequest) grantKey(includeArgsSummary bool) string {
	parts := []string{
		escapeGrantKeyComponent(string(r.Capability.Type)),
		escapeGrantKeyComponent(r.Capability.Source),
		escapeGrantKeyComponent(r.Capability.Name),
		escapeGrantKeyComponent(r.OperationKind),
		escapeGrantKeyComponent(string(r.Risk)),
	}
	for _, resource := range sortedResources(r.Resources) {
		parts = append(parts, escapeGrantKeyComponent(resource.Kind)+"="+escapeGrantKeyComponent(resource.Value))
	}
	if includeArgsSummary {
		parts = append(parts, "args_sha="+sha256Hex(SanitizeText(r.SanitizedArgsSummary)))
	}
	return strings.Join(parts, "|")
}

// ResourceSummary returns a safe, compact description for prompts and logs.
// ResourceSummary 返回可用于提示和日志的安全简短资源描述。
func (r OperationRequest) ResourceSummary() string {
	if len(r.Resources) == 0 {
		return "[]"
	}
	values := make([]string, 0, len(r.Resources))
	for _, resource := range sortedResources(r.Resources) {
		if resource.Kind == "" && resource.Value == "" {
			continue
		}
		values = append(values, resource.Kind+"="+resource.Value)
	}
	if len(values) == 0 {
		return "[]"
	}
	return "[" + strings.Join(values, ",") + "]"
}

// HasResourceKind reports whether the operation touches at least one resource of kind.
// HasResourceKind 判断 operation 是否涉及至少一个指定 kind 的资源。
func (r OperationRequest) HasResourceKind(kind string) bool {
	for _, resource := range r.Resources {
		if resource.Kind == kind {
			return true
		}
	}
	return false
}

// IsNetworkOperation reports whether policy should treat the operation as network access.
// IsNetworkOperation 判断策略是否应将 operation 视为网络访问。
func (r OperationRequest) IsNetworkOperation() bool {
	return strings.HasPrefix(r.OperationKind, "network.") || capability.HasNetworkScope(r.Capability.Scopes)
}

// RequiresNetworkResourceValidation reports whether URL allowlist validation is required.
// RequiresNetworkResourceValidation 判断是否需要进行 URL 白名单校验。
func (r OperationRequest) RequiresNetworkResourceValidation() bool {
	return r.IsNetworkOperation() || r.HasResourceKind(ResourceKindURL)
}

func sortedResources(resources []OperationResource) []OperationResource {
	if len(resources) == 0 {
		return nil
	}
	next := append([]OperationResource(nil), resources...)
	sort.Slice(next, func(i, j int) bool {
		left := next[i].Kind + "=" + next[i].Value
		right := next[j].Kind + "=" + next[j].Value
		return left < right
	})
	return next
}

func escapeGrantKeyComponent(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `|`, `\|`, `=`, `\=`)
	return replacer.Replace(value)
}

func (r OperationRequest) includeArgsSummaryInGrantKey() bool {
	return r.Risk == capability.RiskHigh ||
		r.OperationKind == OperationCommandRun ||
		strings.HasPrefix(r.OperationKind, "network.")
}

func (r OperationRequest) includeArgsSummaryInSessionGrantKey() bool {
	if strings.HasPrefix(r.OperationKind, "network.") {
		return false
	}
	return r.Risk == capability.RiskHigh || r.OperationKind == OperationCommandRun
}

// AuditRecord is the sanitized metadata emitted around policy and execution events.
// AuditRecord 是围绕策略与执行事件输出的脱敏元数据。
type AuditRecord struct {
	ToolName       string
	ToolCallID     string
	OperationKind  string
	Resources      string
	ArgsSummary    string
	ArgsSummarySHA string
	Risk           string
	Decision       string
	DecisionReason string
	ApprovalScope  string
	Status         string
	ElapsedMS      int64
}

// NewAuditRecord creates sanitized audit metadata for an operation.
// NewAuditRecord 为一次操作创建脱敏审计元数据。
func NewAuditRecord(operation OperationRequest) AuditRecord {
	argsSummary := SanitizeText(operation.SanitizedArgsSummary)
	return AuditRecord{
		ToolName:       operation.ToolName,
		ToolCallID:     operation.ToolCallID,
		OperationKind:  operation.OperationKind,
		Resources:      operation.ResourceSummary(),
		ArgsSummary:    argsSummary,
		ArgsSummarySHA: sha256Hex(argsSummary),
		Risk:           string(operation.Risk),
	}
}

func sha256Hex(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
