// Package security provides ADK middleware for capability execution safety.
// Package security 提供 capability 执行安全相关的 ADK middleware。
package security

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/logger"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/security/policy"
)

// ApprovalRequest contains the information shown to a human approver.
// ApprovalRequest 包含展示给人工审批者的信息。
type ApprovalRequest struct {
	ToolName   string
	ToolCallID string
	Capability capability.Descriptor
	Decision   policy.Decision
	Operation  securitycore.OperationRequest
}

// ApprovalDecision is the human approval result.
// ApprovalDecision 表示人工审批结果。
type ApprovalDecision struct {
	Approved      bool
	ApprovalScope string
}

// Approver asks the user whether a reviewed capability should run.
// Approver 询问用户是否允许需要确认的 capability 执行。
type Approver interface {
	Approve(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error)
}

// Config contains dependencies for ExecutionSecurityMiddleware.
// Config 包含 ExecutionSecurityMiddleware 所需依赖。
type Config struct {
	Registry              *capability.Registry
	Policy                *policy.Engine
	Grants                *policy.SessionGrants
	Approver              Approver
	Builders              map[string]securitycore.OperationBuilder
	WorkspaceGuard        *securitycore.WorkspaceGuard
	CommandTimeoutSeconds int
	MaxToolOutputBytes    int
}

// ExecutionSecurityMiddleware guards Eino tool execution through policy and approval checks.
// ExecutionSecurityMiddleware 通过策略与审批检查保护 Eino tool 执行。
type ExecutionSecurityMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	registry              *capability.Registry
	policy                *policy.Engine
	grants                *policy.SessionGrants
	approver              Approver
	builders              map[string]securitycore.OperationBuilder
	workspaceGuard        *securitycore.WorkspaceGuard
	commandTimeoutSeconds int
	maxToolOutputBytes    int
	approvalLocksMu       sync.Mutex
	approvalLocks         map[string]*approvalLockEntry
}

type approvalLockEntry struct {
	mu   sync.Mutex
	refs int
}

// NewExecutionSecurityMiddleware creates an execution security middleware.
// NewExecutionSecurityMiddleware 创建执行安全 middleware。
func NewExecutionSecurityMiddleware(cfg Config) (*ExecutionSecurityMiddleware, error) {
	if cfg.Registry == nil {
		return nil, errors.New("capability registry is required")
	}
	if cfg.Policy == nil {
		cfg.Policy = policy.NewEngine()
	}
	if cfg.Grants == nil {
		cfg.Grants = policy.NewSessionGrants()
	}
	if cfg.WorkspaceGuard == nil {
		guard, err := securitycore.NewWorkspaceGuard(nil)
		if err != nil {
			return nil, fmt.Errorf("new default workspace guard: %w", err)
		}
		cfg.WorkspaceGuard = guard
	}
	return &ExecutionSecurityMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		registry:                     cfg.Registry,
		policy:                       cfg.Policy,
		grants:                       cfg.Grants,
		approver:                     cfg.Approver,
		builders:                     cfg.Builders,
		workspaceGuard:               cfg.WorkspaceGuard,
		commandTimeoutSeconds:        cfg.CommandTimeoutSeconds,
		maxToolOutputBytes:           cfg.MaxToolOutputBytes,
		approvalLocks:                make(map[string]*approvalLockEntry),
	}, nil
}

// WrapInvokableToolCall guards standard invokable tool calls.
// WrapInvokableToolCall 保护标准 invokable tool 调用。
func (m *ExecutionSecurityMiddleware) WrapInvokableToolCall(ctx context.Context, endpoint adk.InvokableToolCallEndpoint, tCtx *adk.ToolContext) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		operation, err := m.authorize(ctx, tCtx, securitycore.SummarizeArguments(argumentsInJSON))
		if err != nil {
			return "", err
		}

		ctx, cancel := m.executionContext(ctx)
		defer cancel()
		start := time.Now()
		result, err := endpoint(ctx, argumentsInJSON, opts...)
		if err == nil {
			if limitErr := m.ensureToolOutputWithinLimit(result); limitErr != nil {
				err = limitErr
				result = ""
			}
		}
		m.auditExecution(ctx, operation, start, err)
		return result, err
	}, nil
}

// WrapStreamableToolCall guards standard streamable tool calls.
// WrapStreamableToolCall 保护标准 streamable tool 调用。
func (m *ExecutionSecurityMiddleware) WrapStreamableToolCall(ctx context.Context, endpoint adk.StreamableToolCallEndpoint, tCtx *adk.ToolContext) (adk.StreamableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		operation, err := m.authorize(ctx, tCtx, securitycore.SummarizeArguments(argumentsInJSON))
		if err != nil {
			return nil, err
		}

		ctx, cancel := m.executionContext(ctx)
		start := time.Now()
		result, err := endpoint(ctx, argumentsInJSON, opts...)
		if err != nil {
			cancel()
			m.auditExecution(ctx, operation, start, err)
			return nil, err
		}
		m.auditStreamOpened(ctx, operation, start)

		// Wrap the stream so audit fires on actual stream completion (EOF) rather
		// than at stream setup time. This gives accurate elapsed time.
		// 包装 stream 使审计在流实际消费完（EOF）时触发，而非在 stream 建立时，以获取准确的耗时。
		var audited atomic.Bool
		outputBudget := newToolOutputBudget(m.maxToolOutputBytes)
		doAudit := func(streamErr error) {
			if audited.CompareAndSwap(false, true) {
				cancel()
				m.auditExecution(ctx, operation, start, streamErr)
			}
		}
		return schema.StreamReaderWithConvert(result,
			func(s string) (string, error) {
				if limitErr := outputBudget.Add(s); limitErr != nil {
					doAudit(limitErr)
					return "", limitErr
				}
				return s, nil
			},
			schema.WithOnEOF(func() (any, error) {
				doAudit(nil)
				return nil, io.EOF
			}),
			schema.WithErrWrapper(func(err error) error {
				doAudit(err)
				return err
			}),
		), nil
	}, nil
}

// WrapEnhancedInvokableToolCall guards enhanced invokable tool calls.
// WrapEnhancedInvokableToolCall 保护 enhanced invokable tool 调用。
func (m *ExecutionSecurityMiddleware) WrapEnhancedInvokableToolCall(ctx context.Context, endpoint adk.EnhancedInvokableToolCallEndpoint, tCtx *adk.ToolContext) (adk.EnhancedInvokableToolCallEndpoint, error) {
	return func(ctx context.Context, toolArgument *schema.ToolArgument, opts ...tool.Option) (*schema.ToolResult, error) {
		operation, err := m.authorize(ctx, tCtx, toolArgumentSummary(toolArgument))
		if err != nil {
			return nil, err
		}

		ctx, cancel := m.executionContext(ctx)
		defer cancel()
		start := time.Now()
		result, err := endpoint(ctx, toolArgument, opts...)
		if err == nil {
			if limitErr := m.ensureToolOutputWithinLimit(result); limitErr != nil {
				err = limitErr
				result = nil
			}
		}
		m.auditExecution(ctx, operation, start, err)
		return result, err
	}, nil
}

// WrapEnhancedStreamableToolCall guards enhanced streamable tool calls.
// WrapEnhancedStreamableToolCall 保护 enhanced streamable tool 调用。
func (m *ExecutionSecurityMiddleware) WrapEnhancedStreamableToolCall(ctx context.Context, endpoint adk.EnhancedStreamableToolCallEndpoint, tCtx *adk.ToolContext) (adk.EnhancedStreamableToolCallEndpoint, error) {
	return func(ctx context.Context, toolArgument *schema.ToolArgument, opts ...tool.Option) (*schema.StreamReader[*schema.ToolResult], error) {
		operation, err := m.authorize(ctx, tCtx, toolArgumentSummary(toolArgument))
		if err != nil {
			return nil, err
		}

		ctx, cancel := m.executionContext(ctx)
		start := time.Now()
		result, err := endpoint(ctx, toolArgument, opts...)
		if err != nil {
			cancel()
			m.auditExecution(ctx, operation, start, err)
			return nil, err
		}
		m.auditStreamOpened(ctx, operation, start)

		// Wrap the stream so audit fires on actual stream completion (EOF) rather
		// than at stream setup time. This gives accurate elapsed time.
		// 包装 stream 使审计在流实际消费完（EOF）时触发，而非在 stream 建立时，以获取准确的耗时。
		var audited atomic.Bool
		outputBudget := newToolOutputBudget(m.maxToolOutputBytes)
		doAudit := func(streamErr error) {
			if audited.CompareAndSwap(false, true) {
				cancel()
				m.auditExecution(ctx, operation, start, streamErr)
			}
		}
		return schema.StreamReaderWithConvert(result,
			func(tr *schema.ToolResult) (*schema.ToolResult, error) {
				if limitErr := outputBudget.Add(tr); limitErr != nil {
					doAudit(limitErr)
					return nil, limitErr
				}
				return tr, nil
			},
			schema.WithOnEOF(func() (any, error) {
				doAudit(nil)
				return nil, io.EOF
			}),
			schema.WithErrWrapper(func(err error) error {
				doAudit(err)
				return err
			}),
		), nil
	}, nil
}

func (m *ExecutionSecurityMiddleware) authorize(ctx context.Context, tCtx *adk.ToolContext, argsSummary string) (securitycore.OperationRequest, error) {
	operation, err := m.operationForTool(ctx, tCtx, argsSummary)
	if err != nil {
		return operation, err
	}
	decision := m.policy.Evaluate(operation)

	switch decision.Action {
	case policy.ActionAllow:
		m.auditDecision(ctx, operation, decision, securitycore.ApprovalScopeNone, "allowed")
		return operation, nil
	case policy.ActionDeny:
		m.auditDecision(ctx, operation, decision, securitycore.ApprovalScopeNone, "denied")
		return operation, fmt.Errorf("capability denied by policy: %s", toolName(tCtx))
	case policy.ActionReview:
		if m.grants.IsAllowed(operation) {
			m.auditDecision(ctx, operation, decision, securitycore.ApprovalScopeSession, "allowed")
			return operation, nil
		}
		unlock := m.lockApproval(operation.GrantKey())
		defer unlock()
		if m.grants.IsAllowed(operation) {
			m.auditDecision(ctx, operation, decision, securitycore.ApprovalScopeSession, "allowed")
			return operation, nil
		}
		if m.approver == nil {
			m.auditDecision(ctx, operation, decision, securitycore.ApprovalScopeNone, "denied")
			return operation, fmt.Errorf("capability approval required: %s", toolName(tCtx))
		}
		approval, err := m.approver.Approve(ctx, ApprovalRequest{
			ToolName:   toolName(tCtx),
			ToolCallID: toolCallID(tCtx),
			Capability: operation.Capability,
			Decision:   decision,
			Operation:  operation,
		})
		if err != nil {
			m.auditDecision(ctx, operation, decision, securitycore.ApprovalScopeNone, "denied")
			return operation, fmt.Errorf("approve capability: %w", err)
		}
		if !approval.Approved {
			m.auditDecision(ctx, operation, decision, securitycore.ApprovalScopeNone, "denied")
			return operation, fmt.Errorf("capability denied by user: %s", toolName(tCtx))
		}
		approvalScope := approval.ApprovalScope
		if approvalScope == "" {
			approvalScope = securitycore.ApprovalScopeOnce
		}
		if approvalScope == securitycore.ApprovalScopeSession {
			m.grants.Allow(operation)
		}
		m.auditDecision(ctx, operation, decision, approvalScope, "approved")
		return operation, nil
	default:
		return operation, fmt.Errorf("unknown policy action: %s", decision.Action)
	}
}

func (m *ExecutionSecurityMiddleware) lockApproval(grantKey string) func() {
	m.approvalLocksMu.Lock()
	if m.approvalLocks == nil {
		m.approvalLocks = make(map[string]*approvalLockEntry)
	}
	entry := m.approvalLocks[grantKey]
	if entry == nil {
		entry = &approvalLockEntry{}
		m.approvalLocks[grantKey] = entry
	}
	entry.refs++
	m.approvalLocksMu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		m.approvalLocksMu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(m.approvalLocks, grantKey)
		}
		m.approvalLocksMu.Unlock()
	}
}

func (m *ExecutionSecurityMiddleware) descriptorForTool(tCtx *adk.ToolContext) (capability.Descriptor, bool) {
	name := toolName(tCtx)
	descriptor, ok := m.registry.Get(name)
	if ok {
		return descriptor, true
	}
	return capability.UnknownDescriptor(name), false
}

func (m *ExecutionSecurityMiddleware) operationForTool(ctx context.Context, tCtx *adk.ToolContext, argsSummary string) (securitycore.OperationRequest, error) {
	descriptor, registered := m.descriptorForTool(tCtx)
	operation := securitycore.OperationRequest{
		ToolName:             toolName(tCtx),
		ToolCallID:           toolCallID(tCtx),
		Capability:           descriptor,
		Registered:           registered,
		OperationKind:        securitycore.OperationNativeTool,
		Risk:                 descriptor.Risk,
		SanitizedArgsSummary: argsSummary,
	}
	for _, resource := range descriptor.Resources {
		operation.Resources = append(operation.Resources, securitycore.OperationResource{Kind: "declared", Value: resource})
	}
	if builder := m.builders[operation.ToolName]; builder != nil {
		operation = builder(ctx, operation, argsSummary)
	}
	if operation.Risk == "" {
		operation.Risk = operation.Capability.Risk
	}
	if operation.OperationKind == "" {
		operation.OperationKind = securitycore.OperationNativeTool
	}
	operation.SanitizedArgsSummary = securitycore.SanitizeText(operation.SanitizedArgsSummary)
	normalized, err := m.normalizeOperationResources(operation.Resources)
	if err != nil {
		return operation, err
	}
	operation.Resources = normalized
	return operation, nil
}

func (m *ExecutionSecurityMiddleware) normalizeOperationResources(resources []securitycore.OperationResource) ([]securitycore.OperationResource, error) {
	if len(resources) == 0 {
		return nil, nil
	}
	next := make([]securitycore.OperationResource, 0, len(resources))
	for _, resource := range resources {
		switch resource.Kind {
		case "path", "file":
			normalized, err := m.workspaceGuard.NormalizePath(resource.Value)
			if err != nil {
				return nil, fmt.Errorf("normalize operation resource: %w", err)
			}
			resource.Value = normalized
		}
		next = append(next, resource)
	}
	return next, nil
}

func (m *ExecutionSecurityMiddleware) auditDecision(ctx context.Context, operation securitycore.OperationRequest, decision policy.Decision, approvalScope, status string) {
	record := securitycore.NewAuditRecord(operation)
	record.Decision = string(decision.Action)
	record.DecisionReason = decision.Reason
	record.ApprovalScope = approvalScope
	record.Status = status
	logger.Info(ctx, 1, "Capability policy evaluated",
		"phase", "capability_policy",
		"tool_name", record.ToolName,
		"tool_call_id", record.ToolCallID,
		"capability_type", string(operation.Capability.Type),
		"capability_source", operation.Capability.Source,
		"operation_kind", record.OperationKind,
		"resources", record.Resources,
		"args_summary_sha", record.ArgsSummarySHA,
		"risk", record.Risk,
		"decision", record.Decision,
		"decision_reason", record.DecisionReason,
		"approval_scope", record.ApprovalScope,
		"status", record.Status)
}

func (m *ExecutionSecurityMiddleware) auditExecution(ctx context.Context, operation securitycore.OperationRequest, start time.Time, err error) {
	record := securitycore.NewAuditRecord(operation)
	record.ElapsedMS = time.Since(start).Milliseconds()
	kvs := []any{
		"phase", "capability_call",
		"tool_name", record.ToolName,
		"tool_call_id", record.ToolCallID,
		"capability_type", string(operation.Capability.Type),
		"capability_source", operation.Capability.Source,
		"operation_kind", record.OperationKind,
		"resources", record.Resources,
		"args_summary_sha", record.ArgsSummarySHA,
		"risk", record.Risk,
		"elapsed_ms", record.ElapsedMS,
	}
	if err != nil {
		logger.Error(ctx, err, "Capability execution failed", append(kvs, "status", "error")...)
		return
	}
	logger.Info(ctx, 1, "Capability execution completed", append(kvs, "status", "success")...)
}

func (m *ExecutionSecurityMiddleware) auditStreamOpened(ctx context.Context, operation securitycore.OperationRequest, start time.Time) {
	record := securitycore.NewAuditRecord(operation)
	record.ElapsedMS = time.Since(start).Milliseconds()
	logger.Info(ctx, 1, "Capability stream opened",
		"phase", "capability_call",
		"tool_name", record.ToolName,
		"tool_call_id", record.ToolCallID,
		"capability_type", string(operation.Capability.Type),
		"capability_source", operation.Capability.Source,
		"operation_kind", record.OperationKind,
		"resources", record.Resources,
		"args_summary_sha", record.ArgsSummarySHA,
		"risk", record.Risk,
		"elapsed_ms", record.ElapsedMS,
		"status", "stream_opened")
}

func (m *ExecutionSecurityMiddleware) executionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if m.commandTimeoutSeconds <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, time.Duration(m.commandTimeoutSeconds)*time.Second)
}

func (m *ExecutionSecurityMiddleware) ensureToolOutputWithinLimit(value any) error {
	if m.maxToolOutputBytes <= 0 {
		return nil
	}
	size := toolOutputSize(value)
	if size > m.maxToolOutputBytes {
		return fmt.Errorf("tool output exceeds security.max_tool_output_bytes: %d > %d", size, m.maxToolOutputBytes)
	}
	return nil
}

type toolOutputBudget struct {
	max  int
	used atomic.Int64
}

func newToolOutputBudget(max int) *toolOutputBudget {
	return &toolOutputBudget{max: max}
}

func (b *toolOutputBudget) Add(value any) error {
	if b == nil || b.max <= 0 {
		return nil
	}
	used := b.used.Add(int64(toolOutputSize(value)))
	if used > int64(b.max) {
		return fmt.Errorf("stream tool output exceeds security.max_tool_output_bytes: %d > %d", used, b.max)
	}
	return nil
}

func toolOutputSize(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case string:
		return len(typed)
	case *schema.ToolResult:
		data, err := json.Marshal(typed)
		if err == nil {
			return len(data)
		}
	}
	return len(fmt.Sprint(value))
}

func toolName(tCtx *adk.ToolContext) string {
	if tCtx == nil {
		return ""
	}
	return tCtx.Name
}

func toolCallID(tCtx *adk.ToolContext) string {
	if tCtx == nil {
		return ""
	}
	return tCtx.CallID
}

func toolArgumentSummary(argument *schema.ToolArgument) string {
	if argument == nil {
		return ""
	}
	return securitycore.SummarizeArguments(argument.Text)
}
