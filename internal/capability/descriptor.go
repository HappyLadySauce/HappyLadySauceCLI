// Package capability describes executable agent capabilities and their policy metadata.
// Package capability 描述 agent 可执行能力及其策略元数据。
package capability

import (
	"errors"
	"fmt"
	"strings"
)

// CapabilityType identifies where a capability comes from.
// CapabilityType 标识能力来源类型。
type CapabilityType string

const (
	// TypeNativeTool is a built-in Go/Eino tool.
	// TypeNativeTool 表示内置 Go/Eino 工具。
	TypeNativeTool CapabilityType = "native_tool"
	// TypeMCPTool is a tool exposed by an MCP server.
	// TypeMCPTool 表示 MCP server 暴露的 tool。
	TypeMCPTool CapabilityType = "mcp_tool"
	// TypeMCPResource is a resource exposed by an MCP server.
	// TypeMCPResource 表示 MCP server 暴露的 resource。
	TypeMCPResource CapabilityType = "mcp_resource"
	// TypeMCPPrompt is a prompt exposed by an MCP server.
	// TypeMCPPrompt 表示 MCP server 暴露的 prompt。
	TypeMCPPrompt CapabilityType = "mcp_prompt"
	// TypeSkill is a capability derived from a skill.
	// TypeSkill 表示由 skill 派生的能力。
	TypeSkill CapabilityType = "skill"
	// TypeUnknown is used when a registered executable has no descriptor.
	// TypeUnknown 表示可执行项缺少 descriptor 时的安全占位类型。
	TypeUnknown CapabilityType = "unknown"
)

// RiskLevel describes the expected blast radius of a capability.
// RiskLevel 描述能力的预期影响范围。
type RiskLevel string

const (
	// RiskLow is safe to execute by policy when explicitly allowed.
	// RiskLow 表示显式允许时可直接执行的低风险能力。
	RiskLow RiskLevel = "low"
	// RiskMedium usually requires user review unless explicitly allowed.
	// RiskMedium 表示通常需要用户确认的中风险能力。
	RiskMedium RiskLevel = "medium"
	// RiskHigh always requires review unless policy denies it outright.
	// RiskHigh 表示除直接拒绝外总是需要确认的高风险能力。
	RiskHigh RiskLevel = "high"
)

// DefaultPolicy is the default decision requested by a capability descriptor.
// DefaultPolicy 表示能力 descriptor 请求的默认策略。
type DefaultPolicy string

const (
	// DefaultPolicyAllow asks the policy engine to allow low/medium risk calls.
	// DefaultPolicyAllow 表示低/中风险调用默认允许。
	DefaultPolicyAllow DefaultPolicy = "allow"
	// DefaultPolicyReview asks the policy engine to require user review.
	// DefaultPolicyReview 表示默认需要用户确认。
	DefaultPolicyReview DefaultPolicy = "review"
	// DefaultPolicyDeny asks the policy engine to deny the call.
	// DefaultPolicyDeny 表示默认拒绝调用。
	DefaultPolicyDeny DefaultPolicy = "deny"
)

// SourceBuiltin identifies capabilities shipped with this CLI.
// SourceBuiltin 标识 CLI 内置能力来源。
const SourceBuiltin = "builtin"

// Descriptor describes a capability for policy, approval, and audit decisions.
// Descriptor 描述一个用于策略、审批和审计判断的能力。
type Descriptor struct {
	Name          string
	Type          CapabilityType
	Source        string
	Risk          RiskLevel
	DefaultPolicy DefaultPolicy
	Scopes        []string
	Resources     []string
}

// Validate checks whether a descriptor is suitable for registry use.
// Validate 检查 descriptor 是否可用于 registry。
func (d Descriptor) Validate() error {
	var errs []error
	if strings.TrimSpace(d.Name) == "" {
		errs = append(errs, errors.New("capability name is required"))
	}
	if !validCapabilityType(d.Type) {
		errs = append(errs, fmt.Errorf("invalid capability type: %s", d.Type))
	}
	if strings.TrimSpace(d.Source) == "" {
		errs = append(errs, errors.New("capability source is required"))
	}
	if !validRiskLevel(d.Risk) {
		errs = append(errs, fmt.Errorf("invalid risk level: %s", d.Risk))
	}
	if !validDefaultPolicy(d.DefaultPolicy) {
		errs = append(errs, fmt.Errorf("invalid default policy: %s", d.DefaultPolicy))
	}
	return errors.Join(errs...)
}

// UnknownDescriptor returns a review-only descriptor for unregistered capabilities.
// UnknownDescriptor 为未注册能力返回仅用于确认的安全 descriptor。
func UnknownDescriptor(name string) Descriptor {
	return Descriptor{
		Name:          name,
		Type:          TypeUnknown,
		Source:        "unknown",
		Risk:          RiskHigh,
		DefaultPolicy: DefaultPolicyReview,
	}
}

func validCapabilityType(value CapabilityType) bool {
	switch value {
	case TypeNativeTool, TypeMCPTool, TypeMCPResource, TypeMCPPrompt, TypeSkill:
		return true
	default:
		return false
	}
}

func validRiskLevel(value RiskLevel) bool {
	switch value {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	default:
		return false
	}
}

func validDefaultPolicy(value DefaultPolicy) bool {
	switch value {
	case DefaultPolicyAllow, DefaultPolicyReview, DefaultPolicyDeny:
		return true
	default:
		return false
	}
}
