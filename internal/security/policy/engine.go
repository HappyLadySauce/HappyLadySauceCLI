// Package policy evaluates capability execution decisions.
// Package policy 负责评估 capability 执行策略。
package policy

import (
	"strings"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
)

// Action is the policy decision action.
// Action 表示策略决策动作。
type Action string

const (
	// ActionAllow permits execution without user approval.
	// ActionAllow 表示无需用户确认即可执行。
	ActionAllow Action = "allow"
	// ActionReview requires user approval before execution.
	// ActionReview 表示执行前需要用户确认。
	ActionReview Action = "review"
	// ActionDeny blocks execution.
	// ActionDeny 表示阻止执行。
	ActionDeny Action = "deny"
)

// PolicyDecision is the result of evaluating an operation call.
// PolicyDecision 表示一次操作调用的策略评估结果。
type PolicyDecision struct {
	Action Action
	Reason string
}

// Decision is kept as an alias for middleware and approver call sites.
// Decision 作为别名保留，供 middleware 与 approver 调用点使用。
type Decision = PolicyDecision

// Config controls policy defaults.
// Config 控制策略默认值。
type Config struct {
	ApprovalDefault string
}

const approvalDefaultReview = "review"

// Engine evaluates operation requests.
// Engine 评估操作请求。
type Engine struct {
	approvalDefault string
}

// NewEngine creates a policy engine with repository defaults.
// NewEngine 创建使用项目默认策略的 policy engine。
func NewEngine(configs ...Config) *Engine {
	cfg := Config{ApprovalDefault: approvalDefaultReview}
	if len(configs) > 0 && strings.TrimSpace(configs[0].ApprovalDefault) != "" {
		cfg.ApprovalDefault = strings.TrimSpace(configs[0].ApprovalDefault)
	}
	return &Engine{approvalDefault: cfg.ApprovalDefault}
}

// Evaluate returns the policy decision for one concrete operation.
// Evaluate 返回一次具体操作对应的策略决策。
//
// Decision matrix (first match wins) — 决策矩阵（优先匹配）：
//
//	unregistered            → ActionReview  (descriptor_missing)
//	DefaultPolicyDeny       → ActionDeny    (default_policy_deny)
//	Operation RiskHigh      → ActionReview  (high_risk)          — overrides DefaultPolicyAllow
//	command.run             → ActionReview  (command_run)
//	network.*               → ActionReview  (network_operation)
//	DefaultPolicyReview     → ActionReview  (default_policy_review)
//	otherwise               → ActionAllow   (default_policy_allow)
//
// Note: RiskMedium + DefaultPolicyAllow results in ActionAllow. This is deliberate:
// medium-risk tools that declare Allow are trusted to run without approval, while
// medium-risk tools that declare Review will prompt the user.
// 注意：RiskMedium + DefaultPolicyAllow 的组合结果为 ActionAllow。这是刻意设计——
// 声明 Allow 的中等风险工具被信任可直接执行，声明 Review 的中等风险工具则会提示用户确认。
func (e *Engine) Evaluate(request securitycore.OperationRequest) PolicyDecision {
	if !request.Registered {
		return e.review("descriptor_missing")
	}
	descriptor := request.Capability
	if descriptor.DefaultPolicy == capability.DefaultPolicyDeny {
		return PolicyDecision{Action: ActionDeny, Reason: "default_policy_deny"}
	}
	risk := request.Risk
	if risk == "" {
		risk = descriptor.Risk
	}
	if risk == capability.RiskHigh {
		return e.review("high_risk")
	}
	if request.OperationKind == securitycore.OperationCommandRun {
		return e.review("command_run")
	}
	if strings.HasPrefix(request.OperationKind, "network.") || hasScopePrefix(descriptor.Scopes, "network:") {
		return e.review("network_operation")
	}
	if descriptor.DefaultPolicy == capability.DefaultPolicyReview {
		return e.review("default_policy_review")
	}
	return PolicyDecision{Action: ActionAllow, Reason: "default_policy_allow"}
}

func (e *Engine) review(reason string) PolicyDecision {
	if e == nil || e.approvalDefault == approvalDefaultReview {
		return PolicyDecision{Action: ActionReview, Reason: reason}
	}
	return PolicyDecision{Action: ActionDeny, Reason: "approval_default_unsupported"}
}

func hasScopePrefix(scopes []string, prefix string) bool {
	for _, scope := range scopes {
		if strings.HasPrefix(scope, prefix) {
			return true
		}
	}
	return false
}
