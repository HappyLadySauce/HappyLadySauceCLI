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
//
// Decision matrix (first match wins) — 决策矩阵（优先匹配）：
//
//	unregistered            → ActionReview  (descriptor_missing)
//	DefaultPolicyDeny       → ActionDeny    (default_policy_deny)
//	RiskHigh                → ActionReview  (high_risk)          — overrides DefaultPolicyAllow
//	DefaultPolicyReview     → ActionReview  (default_policy_review)
//	otherwise               → ActionAllow   (default_policy_allow)
//
// Note: RiskMedium + DefaultPolicyAllow results in ActionAllow. This is deliberate:
// medium-risk tools that declare Allow are trusted to run without approval, while
// medium-risk tools that declare Review will prompt the user.
// 注意：RiskMedium + DefaultPolicyAllow 的组合结果为 ActionAllow。这是刻意设计——
// 声明 Allow 的中等风险工具被信任可直接执行，声明 Review 的中等风险工具则会提示用户确认。
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
