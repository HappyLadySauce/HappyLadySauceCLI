// Package policy evaluates capability execution decisions.
// Package policy 负责评估 capability 执行策略。
package policy

import (
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

// Engine evaluates operation requests.
// Engine 评估操作请求。
type Engine struct{}

// NewEngine creates a policy engine.
// NewEngine 创建 policy engine。
func NewEngine() *Engine {
	return &Engine{}
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
//	network.* + builtin + RiskLow + DefaultPolicyAllow + resources → ActionAllow (default_policy_allow)
//	network.* (otherwise)   → ActionReview  (network_operation)
//	DefaultPolicyReview     → ActionReview  (default_policy_review)
//	otherwise               → ActionAllow   (default_policy_allow)
//
// Note: RiskMedium + DefaultPolicyAllow results in ActionAllow for non-network tools only.
// Medium-risk network tools always require review even when DefaultPolicyAllow is declared.
// 注意：RiskMedium + DefaultPolicyAllow 对非 network 工具为 ActionAllow；中等风险 network 工具一律 Review。
func (e *Engine) Evaluate(request securitycore.OperationRequest) PolicyDecision {
	if !request.Registered {
		return review("descriptor_missing")
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
		return review("high_risk")
	}
	if request.OperationKind == securitycore.OperationCommandRun {
		return review("command_run")
	}
	if request.IsNetworkOperation() {
		if allowLowRiskBuiltinNetwork(request, descriptor, risk) {
			return PolicyDecision{Action: ActionAllow, Reason: "default_policy_allow"}
		}
		return review("network_operation")
	}
	if descriptor.DefaultPolicy == capability.DefaultPolicyReview {
		return review("default_policy_review")
	}
	return PolicyDecision{Action: ActionAllow, Reason: "default_policy_allow"}
}

func review(reason string) PolicyDecision {
	return PolicyDecision{Action: ActionReview, Reason: reason}
}

func allowLowRiskBuiltinNetwork(request securitycore.OperationRequest, descriptor capability.Descriptor, risk capability.RiskLevel) bool {
	if risk != capability.RiskLow || descriptor.DefaultPolicy != capability.DefaultPolicyAllow {
		return false
	}
	if descriptor.Source != capability.SourceBuiltin {
		return false
	}
	if len(request.Resources) == 0 && len(descriptor.Resources) == 0 {
		return false
	}
	return true
}
