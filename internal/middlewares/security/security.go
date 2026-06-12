// Package security provides ADK middleware for capability execution safety.
// Package security 提供 capability 执行安全相关的 ADK middleware。
package security

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools/toolresult"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/utils/urlscope"
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

type authorization struct {
	operation     securitycore.OperationRequest
	decision      policy.Decision
	approvalScope string
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
		auth, err := m.authorize(ctx, tCtx, operationBuildInput(argumentsInJSON))
		if payload, recovered, authErr := m.finishAuthorize(ctx, auth, err); recovered {
			return payload, nil
		} else if authErr != nil {
			return "", authErr
		}

		ctx = securitycore.WithAuthorizedOperation(ctx, auth.operation)
		ctx, cancel := m.executionContext(ctx, auth.operation)
		defer cancel()
		start := time.Now()
		result, err := endpoint(ctx, argumentsInJSON, opts...)
		outputBytes := toolOutputSize(result)
		if err == nil {
			if limitErr := m.ensureToolOutputWithinLimit(outputBytes); limitErr != nil {
				err = limitErr
				result = ""
				outputBytes = 0
			}
		}
		if err != nil {
			payload := toolresult.FormatError(err)
			m.auditExecution(ctx, auth, start, err, len(payload), true)
			return payload, nil
		}
		m.auditExecution(ctx, auth, start, nil, outputBytes, false)
		return result, nil
	}, nil
}

// WrapStreamableToolCall guards standard streamable tool calls.
// WrapStreamableToolCall 保护标准 streamable tool 调用。
func (m *ExecutionSecurityMiddleware) WrapStreamableToolCall(ctx context.Context, endpoint adk.StreamableToolCallEndpoint, tCtx *adk.ToolContext) (adk.StreamableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		auth, err := m.authorize(ctx, tCtx, operationBuildInput(argumentsInJSON))
		if payload, recovered, authErr := m.finishAuthorize(ctx, auth, err); recovered {
			return schema.StreamReaderFromArray([]string{payload}), nil
		} else if authErr != nil {
			return nil, authErr
		}

		ctx = securitycore.WithAuthorizedOperation(ctx, auth.operation)
		ctx, cancel := m.executionContext(ctx, auth.operation)
		start := time.Now()
		result, err := endpoint(ctx, argumentsInJSON, opts...)
		if err != nil {
			cancel()
			payload := toolresult.FormatError(err)
			m.auditExecution(ctx, auth, start, err, len(payload), true)
			return schema.StreamReaderFromArray([]string{payload}), nil
		}
		m.auditStreamOpened(ctx, auth, start)

		// Wrap the stream so audit fires on actual stream completion (EOF) rather
		// than at stream setup time. This gives accurate elapsed time.
		// 包装 stream 使审计在流实际消费完（EOF）时触发，而非在 stream 建立时，以获取准确的耗时。
		var audited atomic.Bool
		var streamStopped atomic.Bool
		outputBudget := newToolOutputBudget(m.maxToolOutputBytes)
		doAudit := func(streamErr error, recovered bool) {
			if audited.CompareAndSwap(false, true) {
				cancel()
				m.auditExecution(ctx, auth, start, streamErr, int(outputBudget.Used()), recovered)
			}
		}
		return proxyStreamReaderWithFinalize(
			schema.StreamReaderWithConvert(result,
				func(s string) (string, error) {
					if streamStopped.Load() {
						return "", io.EOF
					}
					if limitErr := outputBudget.Add(s); limitErr != nil {
						streamStopped.Store(true)
						doAudit(limitErr, true)
						return toolresult.FormatError(limitErr), nil
					}
					return s, nil
				},
				schema.WithOnEOF(func() (any, error) {
					doAudit(nil, false)
					return nil, io.EOF
				}),
				schema.WithErrWrapper(func(err error) error {
					doAudit(err, false)
					return err
				}),
			),
			func() { doAudit(nil, false) },
		), nil
	}, nil
}

// WrapEnhancedInvokableToolCall guards enhanced invokable tool calls.
// WrapEnhancedInvokableToolCall 保护 enhanced invokable tool 调用。
func (m *ExecutionSecurityMiddleware) WrapEnhancedInvokableToolCall(ctx context.Context, endpoint adk.EnhancedInvokableToolCallEndpoint, tCtx *adk.ToolContext) (adk.EnhancedInvokableToolCallEndpoint, error) {
	return func(ctx context.Context, toolArgument *schema.ToolArgument, opts ...tool.Option) (*schema.ToolResult, error) {
		auth, err := m.authorize(ctx, tCtx, toolArgumentBuildInput(toolArgument))
		if payload, recovered, authErr := m.finishAuthorize(ctx, auth, err); recovered {
			return textToolResult(payload), nil
		} else if authErr != nil {
			return nil, authErr
		}

		ctx = securitycore.WithAuthorizedOperation(ctx, auth.operation)
		ctx, cancel := m.executionContext(ctx, auth.operation)
		defer cancel()
		start := time.Now()
		result, err := endpoint(ctx, toolArgument, opts...)
		outputBytes := toolOutputSize(result)
		if err == nil {
			if limitErr := m.ensureToolOutputWithinLimit(outputBytes); limitErr != nil {
				err = limitErr
				result = nil
				outputBytes = 0
			}
		}
		if err != nil {
			payload := errorToolResult(err)
			m.auditExecution(ctx, auth, start, err, toolOutputSize(payload), true)
			return payload, nil
		}
		m.auditExecution(ctx, auth, start, nil, outputBytes, false)
		return result, nil
	}, nil
}

// WrapEnhancedStreamableToolCall guards enhanced streamable tool calls.
// WrapEnhancedStreamableToolCall 保护 enhanced streamable tool 调用。
func (m *ExecutionSecurityMiddleware) WrapEnhancedStreamableToolCall(ctx context.Context, endpoint adk.EnhancedStreamableToolCallEndpoint, tCtx *adk.ToolContext) (adk.EnhancedStreamableToolCallEndpoint, error) {
	return func(ctx context.Context, toolArgument *schema.ToolArgument, opts ...tool.Option) (*schema.StreamReader[*schema.ToolResult], error) {
		auth, err := m.authorize(ctx, tCtx, toolArgumentBuildInput(toolArgument))
		if payload, recovered, authErr := m.finishAuthorize(ctx, auth, err); recovered {
			return schema.StreamReaderFromArray([]*schema.ToolResult{textToolResult(payload)}), nil
		} else if authErr != nil {
			return nil, authErr
		}

		ctx = securitycore.WithAuthorizedOperation(ctx, auth.operation)
		ctx, cancel := m.executionContext(ctx, auth.operation)
		start := time.Now()
		result, err := endpoint(ctx, toolArgument, opts...)
		if err != nil {
			cancel()
			payload := errorToolResult(err)
			m.auditExecution(ctx, auth, start, err, toolOutputSize(payload), true)
			return schema.StreamReaderFromArray([]*schema.ToolResult{payload}), nil
		}
		m.auditStreamOpened(ctx, auth, start)

		// Wrap the stream so audit fires on actual stream completion (EOF) rather
		// than at stream setup time. This gives accurate elapsed time.
		// 包装 stream 使审计在流实际消费完（EOF）时触发，而非在 stream 建立时，以获取准确的耗时。
		var audited atomic.Bool
		var streamStopped atomic.Bool
		outputBudget := newToolOutputBudget(m.maxToolOutputBytes)
		doAudit := func(streamErr error, recovered bool) {
			if audited.CompareAndSwap(false, true) {
				cancel()
				m.auditExecution(ctx, auth, start, streamErr, int(outputBudget.Used()), recovered)
			}
		}
		return proxyStreamReaderWithFinalize(
			schema.StreamReaderWithConvert(result,
				func(tr *schema.ToolResult) (*schema.ToolResult, error) {
					if streamStopped.Load() {
						return nil, io.EOF
					}
					if limitErr := outputBudget.Add(tr); limitErr != nil {
						streamStopped.Store(true)
						doAudit(limitErr, true)
						return errorToolResult(limitErr), nil
					}
					return tr, nil
				},
				schema.WithOnEOF(func() (any, error) {
					doAudit(nil, false)
					return nil, io.EOF
				}),
				schema.WithErrWrapper(func(err error) error {
					doAudit(err, false)
					return err
				}),
			),
			func() { doAudit(nil, false) },
		), nil
	}, nil
}

func (m *ExecutionSecurityMiddleware) authorize(ctx context.Context, tCtx *adk.ToolContext, input securitycore.OperationBuildInput) (authorization, error) {
	operation, err := m.operationForTool(ctx, tCtx, input)
	if err != nil {
		return authorization{operation: operation}, err
	}
	decision := m.policy.Evaluate(operation)
	auth := authorization{
		operation:     operation,
		decision:      decision,
		approvalScope: securitycore.ApprovalScopeNone,
	}

	switch decision.Action {
	case policy.ActionAllow:
		m.auditDecision(ctx, operation, decision, securitycore.ApprovalScopeNone, "allowed")
		return auth, nil
	case policy.ActionDeny:
		m.auditDecision(ctx, operation, decision, securitycore.ApprovalScopeNone, "denied")
		return auth, securitycore.CapabilityDeniedByPolicyError(toolName(tCtx))
	case policy.ActionReview:
		if m.grants.IsAllowed(operation) {
			auth.approvalScope = securitycore.ApprovalScopeSession
			m.auditDecision(ctx, operation, decision, auth.approvalScope, "allowed")
			return auth, nil
		}
		unlock := m.lockApproval(operation.SessionGrantKey())
		defer unlock()
		if m.grants.IsAllowed(operation) {
			auth.approvalScope = securitycore.ApprovalScopeSession
			m.auditDecision(ctx, operation, decision, auth.approvalScope, "allowed")
			return auth, nil
		}
		if m.approver == nil {
			m.auditDecision(ctx, operation, decision, securitycore.ApprovalScopeNone, "denied")
			return auth, fmt.Errorf("capability approval required: %s", toolName(tCtx))
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
			return auth, fmt.Errorf("approve capability: %w", err)
		}
		if !approval.Approved {
			m.auditDecision(ctx, operation, decision, securitycore.ApprovalScopeNone, "denied")
			return auth, securitycore.CapabilityDeniedByUserError(toolName(tCtx))
		}
		approvalScope := approval.ApprovalScope
		if approvalScope == "" {
			approvalScope = securitycore.ApprovalScopeOnce
		}
		auth.approvalScope = approvalScope
		if approvalScope == securitycore.ApprovalScopeSession {
			m.grants.Allow(operation)
		}
		m.auditDecision(ctx, operation, decision, approvalScope, "approved")
		return auth, nil
	default:
		return auth, fmt.Errorf("unknown policy action: %s", decision.Action)
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

func (m *ExecutionSecurityMiddleware) operationForTool(ctx context.Context, tCtx *adk.ToolContext, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
	descriptor, registered := m.descriptorForTool(tCtx)
	operation := securitycore.OperationRequest{
		ToolName:             toolName(tCtx),
		ToolCallID:           toolCallID(tCtx),
		Capability:           descriptor,
		Registered:           registered,
		OperationKind:        securitycore.OperationNativeTool,
		Risk:                 descriptor.Risk,
		SanitizedArgsSummary: input.Summary,
	}
	for _, resource := range descriptor.Resources {
		operation.Resources = append(operation.Resources, securitycore.OperationResource{Kind: "declared", Value: resource})
	}
	if builder := m.builders[operation.ToolName]; builder != nil {
		operation = builder(ctx, operation, input)
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
	if err := m.validateOperationScopes(operation); err != nil {
		return operation, err
	}
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

func (m *ExecutionSecurityMiddleware) validateOperationScopes(operation securitycore.OperationRequest) error {
	if !requiresNetworkResourceValidation(operation) {
		return nil
	}

	hasURL := false
	for _, resource := range operation.Resources {
		if resource.Kind != "url" {
			continue
		}
		hasURL = true
		if !urlscope.Allowed(resource.Value, operation.Capability.Resources) {
			return fmt.Errorf("network resource is outside descriptor resources: %s", resource.Value)
		}
	}
	if hasURL && len(operation.Capability.Resources) == 0 {
		return fmt.Errorf("network resource requires descriptor resources allowlist")
	}
	return nil
}

func requiresNetworkResourceValidation(operation securitycore.OperationRequest) bool {
	if hasScopePrefix(operation.Capability.Scopes, "network:") {
		return true
	}
	if strings.HasPrefix(operation.OperationKind, "network.") {
		return true
	}
	for _, resource := range operation.Resources {
		if resource.Kind == "url" {
			return true
		}
	}
	return false
}

func (m *ExecutionSecurityMiddleware) finishAuthorize(ctx context.Context, auth authorization, err error) (string, bool, error) {
	if err == nil {
		return "", false, nil
	}
	if !securitycore.IsRecoverableAuthorizationDenial(err) {
		return "", false, err
	}
	reason := securitycore.DenialReasonFor(err)
	payload := toolresult.FormatFailure(err, reason)
	m.auditAuthorizationRecovered(ctx, auth, err, len(payload), reason)
	return payload, true, nil
}

func (m *ExecutionSecurityMiddleware) auditAuthorizationRecovered(ctx context.Context, auth authorization, err error, outputBytes int, reason string) {
	operation := auth.operation
	record := securitycore.NewAuditRecord(operation)
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
		"decision", string(auth.decision.Action),
		"decision_reason", auth.decision.Reason,
		"approval_scope", auth.approvalScope,
		"output_bytes", outputBytes,
		"denial_reason", reason,
	}
	logger.Error(ctx, err, "Capability authorization recovered", append(kvs, "status", "denial_returned", "recovered", true)...)
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

func (m *ExecutionSecurityMiddleware) auditExecution(ctx context.Context, auth authorization, start time.Time, err error, outputBytes int, recovered bool) {
	operation := auth.operation
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
		"decision", string(auth.decision.Action),
		"decision_reason", auth.decision.Reason,
		"approval_scope", auth.approvalScope,
		"output_bytes", outputBytes,
		"elapsed_ms", record.ElapsedMS,
	}
	if err != nil {
		if recovered {
			logger.Error(ctx, err, "Capability execution recovered", append(kvs, "status", "tool_error_returned", "recovered", true)...)
			return
		}
		logger.Error(ctx, err, "Capability execution failed", append(kvs, "status", "error")...)
		return
	}
	logger.Info(ctx, 1, "Capability execution completed", append(kvs, "status", "success")...)
}

func errorToolResult(err error) *schema.ToolResult {
	return textToolResult(toolresult.FormatError(err))
}

func textToolResult(text string) *schema.ToolResult {
	return &schema.ToolResult{
		Parts: []schema.ToolOutputPart{{
			Type: schema.ToolPartTypeText,
			Text: text,
		}},
	}
}

func (m *ExecutionSecurityMiddleware) auditStreamOpened(ctx context.Context, auth authorization, start time.Time) {
	operation := auth.operation
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
		"decision", string(auth.decision.Action),
		"decision_reason", auth.decision.Reason,
		"approval_scope", auth.approvalScope,
		"output_bytes", 0,
		"elapsed_ms", record.ElapsedMS,
		"status", "stream_opened")
}

func (m *ExecutionSecurityMiddleware) executionContext(ctx context.Context, operation securitycore.OperationRequest) (context.Context, context.CancelFunc) {
	if m.commandTimeoutSeconds <= 0 || operation.OperationKind != securitycore.OperationCommandRun {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, time.Duration(m.commandTimeoutSeconds)*time.Second)
}

func (m *ExecutionSecurityMiddleware) ensureToolOutputWithinLimit(size int) error {
	if m.maxToolOutputBytes <= 0 {
		return nil
	}
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

func (b *toolOutputBudget) Used() int64 {
	if b == nil {
		return 0
	}
	return b.used.Load()
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

func hasScopePrefix(scopes []string, prefix string) bool {
	for _, scope := range scopes {
		if len(scope) >= len(prefix) && scope[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func operationBuildInput(rawJSON string) securitycore.OperationBuildInput {
	return securitycore.OperationBuildInput{
		RawJSON: rawJSON,
		Summary: securitycore.SummarizeArguments(rawJSON),
	}
}

func toolArgumentBuildInput(argument *schema.ToolArgument) securitycore.OperationBuildInput {
	if argument == nil {
		return securitycore.OperationBuildInput{}
	}
	return operationBuildInput(argument.Text)
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
