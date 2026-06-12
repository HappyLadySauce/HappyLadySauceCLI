// Package policy evaluates capability execution decisions.
// Package policy 负责评估 capability 执行策略。
package policy

import "github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"

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

// Decision is the result of evaluating a capability call.
// Decision 表示一次 capability 调用的策略评估结果。
type Decision struct {
	Action Action
	Reason string
}

// Engine evaluates capability descriptors.
// Engine 评估 capability descriptor。
type Engine struct{}

// NewEngine creates a policy engine with repository defaults.
// NewEngine 创建使用项目默认策略的 policy engine。
func NewEngine() *Engine {
	return &Engine{}
}

// Evaluate returns the policy decision for a descriptor.
// Evaluate 返回 descriptor 对应的策略决策。
func (e *Engine) Evaluate(descriptor capability.Descriptor, registered bool) Decision {
	if !registered {
		return Decision{Action: ActionReview, Reason: "descriptor_missing"}
	}
	if descriptor.DefaultPolicy == capability.DefaultPolicyDeny {
		return Decision{Action: ActionDeny, Reason: "default_policy_deny"}
	}
	if descriptor.Risk == capability.RiskHigh {
		return Decision{Action: ActionReview, Reason: "high_risk"}
	}
	if descriptor.DefaultPolicy == capability.DefaultPolicyReview {
		return Decision{Action: ActionReview, Reason: "default_policy_review"}
	}
	return Decision{Action: ActionAllow, Reason: "default_policy_allow"}
}
