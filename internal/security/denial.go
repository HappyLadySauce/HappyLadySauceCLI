package security

import (
	"errors"
	"fmt"
)

const (
	// DenialReasonUserDenied marks a human approval rejection payload.
	// DenialReasonUserDenied 标记人工审批拒绝 payload。
	DenialReasonUserDenied = "user_denied"
	// DenialReasonPolicyDenied marks a policy engine denial payload.
	// DenialReasonPolicyDenied 标记策略引擎拒绝 payload。
	DenialReasonPolicyDenied = "policy_denied"
)

var (
	// ErrCapabilityDeniedByUser indicates the human approver rejected the capability.
	// ErrCapabilityDeniedByUser 表示人工审批者拒绝了 capability。
	ErrCapabilityDeniedByUser = errors.New("capability denied by user")
	// ErrCapabilityDeniedByPolicy indicates the policy engine denied the capability.
	// ErrCapabilityDeniedByPolicy 表示策略引擎拒绝了 capability。
	ErrCapabilityDeniedByPolicy = errors.New("capability denied by policy")
)

// CapabilityDeniedByUserError builds a recoverable user-denial error for toolName.
// CapabilityDeniedByUserError 为 toolName 构建可恢复的用户拒绝错误。
func CapabilityDeniedByUserError(toolName string) error {
	return fmt.Errorf("%w: %s", ErrCapabilityDeniedByUser, toolName)
}

// CapabilityDeniedByPolicyError builds a recoverable policy-denial error for toolName.
// CapabilityDeniedByPolicyError 为 toolName 构建可恢复的策略拒绝错误。
func CapabilityDeniedByPolicyError(toolName string) error {
	return fmt.Errorf("%w: %s", ErrCapabilityDeniedByPolicy, toolName)
}

// IsRecoverableAuthorizationDenial reports whether err should be returned to the model as tool output.
// IsRecoverableAuthorizationDenial 判断 err 是否应作为 tool output 回传给模型。
func IsRecoverableAuthorizationDenial(err error) bool {
	return errors.Is(err, ErrCapabilityDeniedByUser) || errors.Is(err, ErrCapabilityDeniedByPolicy)
}

// IsStructuredDenialReason reports whether reason is a known authorization denial payload reason.
// IsStructuredDenialReason 判断 reason 是否为已知的授权拒绝 payload 原因。
func IsStructuredDenialReason(reason string) bool {
	return reason == DenialReasonUserDenied || reason == DenialReasonPolicyDenied
}

// DenialReasonFor returns the structured denial reason for recoverable authorization errors.
// DenialReasonFor 返回可恢复授权错误对应的结构化拒绝原因。
func DenialReasonFor(err error) string {
	switch {
	case errors.Is(err, ErrCapabilityDeniedByUser):
		return DenialReasonUserDenied
	case errors.Is(err, ErrCapabilityDeniedByPolicy):
		return DenialReasonPolicyDenied
	default:
		return ""
	}
}
