// Package security provides ADK middleware for capability execution safety.
// Package security 提供 capability 执行安全相关的 ADK middleware。
package security

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/logger"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/security/policy"
)

// ApprovalRequest contains the information shown to a human approver.
// ApprovalRequest 包含展示给人工审批者的信息。
type ApprovalRequest struct {
	ToolName   string
	ToolCallID string
	Capability capability.Descriptor
	Decision   policy.Decision
}

// ApprovalDecision is the human approval result.
// ApprovalDecision 表示人工审批结果。
type ApprovalDecision struct {
	Approved bool
}

// Approver asks the user whether a reviewed capability should run.
// Approver 询问用户是否允许需要确认的 capability 执行。
type Approver interface {
	Approve(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error)
}

// Config contains dependencies for ExecutionSecurityMiddleware.
// Config 包含 ExecutionSecurityMiddleware 所需依赖。
type Config struct {
	Registry *capability.Registry
	Policy   *policy.Engine
	Grants   *policy.SessionGrants
	Approver Approver
}

// ExecutionSecurityMiddleware guards Eino tool execution through policy and approval checks.
// ExecutionSecurityMiddleware 通过策略与审批检查保护 Eino tool 执行。
type ExecutionSecurityMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	registry   *capability.Registry
	policy     *policy.Engine
	grants     *policy.SessionGrants
	approver   Approver
	approvalMu sync.Mutex
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
	return &ExecutionSecurityMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		registry:                     cfg.Registry,
		policy:                       cfg.Policy,
		grants:                       cfg.Grants,
		approver:                     cfg.Approver,
	}, nil
}

// WrapInvokableToolCall guards standard invokable tool calls.
// WrapInvokableToolCall 保护标准 invokable tool 调用。
func (m *ExecutionSecurityMiddleware) WrapInvokableToolCall(ctx context.Context, endpoint adk.InvokableToolCallEndpoint, tCtx *adk.ToolContext) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		if err := m.authorize(ctx, tCtx); err != nil {
			return "", err
		}

		start := time.Now()
		result, err := endpoint(ctx, argumentsInJSON, opts...)
		m.auditExecution(ctx, tCtx, start, err)
		return result, err
	}, nil
}

// WrapStreamableToolCall guards standard streamable tool calls.
// WrapStreamableToolCall 保护标准 streamable tool 调用。
func (m *ExecutionSecurityMiddleware) WrapStreamableToolCall(ctx context.Context, endpoint adk.StreamableToolCallEndpoint, tCtx *adk.ToolContext) (adk.StreamableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		if err := m.authorize(ctx, tCtx); err != nil {
			return nil, err
		}

		start := time.Now()
		result, err := endpoint(ctx, argumentsInJSON, opts...)
		m.auditExecution(ctx, tCtx, start, err)
		return result, err
	}, nil
}

// WrapEnhancedInvokableToolCall guards enhanced invokable tool calls.
// WrapEnhancedInvokableToolCall 保护 enhanced invokable tool 调用。
func (m *ExecutionSecurityMiddleware) WrapEnhancedInvokableToolCall(ctx context.Context, endpoint adk.EnhancedInvokableToolCallEndpoint, tCtx *adk.ToolContext) (adk.EnhancedInvokableToolCallEndpoint, error) {
	return func(ctx context.Context, toolArgument *schema.ToolArgument, opts ...tool.Option) (*schema.ToolResult, error) {
		if err := m.authorize(ctx, tCtx); err != nil {
			return nil, err
		}

		start := time.Now()
		result, err := endpoint(ctx, toolArgument, opts...)
		m.auditExecution(ctx, tCtx, start, err)
		return result, err
	}, nil
}

// WrapEnhancedStreamableToolCall guards enhanced streamable tool calls.
// WrapEnhancedStreamableToolCall 保护 enhanced streamable tool 调用。
func (m *ExecutionSecurityMiddleware) WrapEnhancedStreamableToolCall(ctx context.Context, endpoint adk.EnhancedStreamableToolCallEndpoint, tCtx *adk.ToolContext) (adk.EnhancedStreamableToolCallEndpoint, error) {
	return func(ctx context.Context, toolArgument *schema.ToolArgument, opts ...tool.Option) (*schema.StreamReader[*schema.ToolResult], error) {
		if err := m.authorize(ctx, tCtx); err != nil {
			return nil, err
		}

		start := time.Now()
		result, err := endpoint(ctx, toolArgument, opts...)
		m.auditExecution(ctx, tCtx, start, err)
		return result, err
	}, nil
}

func (m *ExecutionSecurityMiddleware) authorize(ctx context.Context, tCtx *adk.ToolContext) error {
	descriptor, registered := m.descriptorForTool(tCtx)
	decision := m.policy.Evaluate(descriptor, registered)

	switch decision.Action {
	case policy.ActionAllow:
		m.auditDecision(ctx, tCtx, descriptor, decision, "none", "allowed")
		return nil
	case policy.ActionDeny:
		m.auditDecision(ctx, tCtx, descriptor, decision, "none", "denied")
		return fmt.Errorf("capability denied by policy: %s", toolName(tCtx))
	case policy.ActionReview:
		if m.grants.IsAllowed(descriptor) {
			m.auditDecision(ctx, tCtx, descriptor, decision, "session", "allowed")
			return nil
		}
		m.approvalMu.Lock()
		defer m.approvalMu.Unlock()
		if m.grants.IsAllowed(descriptor) {
			m.auditDecision(ctx, tCtx, descriptor, decision, "session", "allowed")
			return nil
		}
		if m.approver == nil {
			m.auditDecision(ctx, tCtx, descriptor, decision, "none", "denied")
			return fmt.Errorf("capability approval required: %s", toolName(tCtx))
		}
		approval, err := m.approver.Approve(ctx, ApprovalRequest{
			ToolName:   toolName(tCtx),
			ToolCallID: toolCallID(tCtx),
			Capability: descriptor,
			Decision:   decision,
		})
		if err != nil {
			m.auditDecision(ctx, tCtx, descriptor, decision, "none", "denied")
			return fmt.Errorf("approve capability: %w", err)
		}
		if !approval.Approved {
			m.auditDecision(ctx, tCtx, descriptor, decision, "none", "denied")
			return fmt.Errorf("capability denied by user: %s", toolName(tCtx))
		}
		m.grants.Allow(descriptor)
		m.auditDecision(ctx, tCtx, descriptor, decision, "session", "approved")
		return nil
	default:
		return fmt.Errorf("unknown policy action: %s", decision.Action)
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

func (m *ExecutionSecurityMiddleware) auditDecision(ctx context.Context, tCtx *adk.ToolContext, descriptor capability.Descriptor, decision policy.Decision, approvalScope, status string) {
	logger.Info(ctx, 1, "Capability policy evaluated",
		"phase", "capability_policy",
		"tool_name", toolName(tCtx),
		"tool_call_id", toolCallID(tCtx),
		"capability_type", string(descriptor.Type),
		"capability_source", descriptor.Source,
		"risk", string(descriptor.Risk),
		"decision", string(decision.Action),
		"decision_reason", decision.Reason,
		"approval_scope", approvalScope,
		"status", status)
}

func (m *ExecutionSecurityMiddleware) auditExecution(ctx context.Context, tCtx *adk.ToolContext, start time.Time, err error) {
	descriptor, _ := m.descriptorForTool(tCtx)
	kvs := []any{
		"phase", "capability_call",
		"tool_name", toolName(tCtx),
		"tool_call_id", toolCallID(tCtx),
		"capability_type", string(descriptor.Type),
		"capability_source", descriptor.Source,
		"risk", string(descriptor.Risk),
		"elapsed_ms", time.Since(start).Milliseconds(),
	}
	if err != nil {
		logger.Error(ctx, err, "Capability execution failed", append(kvs, "status", "error")...)
		return
	}
	logger.Info(ctx, 1, "Capability execution completed", append(kvs, "status", "success")...)
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
