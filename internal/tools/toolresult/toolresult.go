// Package toolresult formats tool execution errors as structured payloads for the model.
// Package toolresult 将工具执行错误格式化为供模型解析的结构化 payload。
package toolresult

import (
	"encoding/json"
	"errors"
	"strings"
)

const (
	// ReasonUserDenied marks a human approval rejection payload.
	// ReasonUserDenied 标记人工审批拒绝 payload。
	ReasonUserDenied = "user_denied"
	// ReasonPolicyDenied marks a policy engine denial payload.
	// ReasonPolicyDenied 标记策略引擎拒绝 payload。
	ReasonPolicyDenied = "policy_denied"
)

type errorPayload struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error"`
	Reason string `json:"reason,omitempty"`
}

// FormatError serializes an execution error into a stable JSON tool result string.
// FormatError 将执行错误序列化为稳定的 JSON tool result 字符串。
func FormatError(err error) string {
	return FormatFailure(err, "")
}

// FormatFailure serializes a failure into JSON with an optional structured reason.
// FormatFailure 将失败序列化为带可选结构化 reason 的 JSON。
func FormatFailure(err error, reason string) string {
	if err == nil {
		return `{"ok":false,"error":"unknown tool error"}`
	}
	payload := errorPayload{
		OK:     false,
		Error:  rootMessage(err),
		Reason: strings.TrimSpace(reason),
	}
	data, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return `{"ok":false,"error":"failed to format tool error"}`
	}
	return string(data)
}

// IsErrorPayload reports whether content is a structured tool error payload.
// IsErrorPayload 判断 content 是否为结构化 tool error payload。
func IsErrorPayload(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	var payload errorPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return false
	}
	return !payload.OK && payload.Error != ""
}

// IsDeniedPayload reports whether content is a structured authorization denial payload.
// IsDeniedPayload 判断 content 是否为结构化授权拒绝 payload。
func IsDeniedPayload(content string) bool {
	reason := DenialReason(content)
	return reason == ReasonUserDenied || reason == ReasonPolicyDenied
}

// DenialReason returns the structured denial reason from a tool payload, if any.
// DenialReason 从 tool payload 返回结构化拒绝原因（如有）。
func DenialReason(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	var payload errorPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return ""
	}
	if payload.OK || payload.Error == "" {
		return ""
	}
	return payload.Reason
}

func rootMessage(err error) string {
	if err == nil {
		return ""
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		wrapped := joined.Unwrap()
		if len(wrapped) > 0 {
			return rootMessage(wrapped[len(wrapped)-1])
		}
	}
	current := err
	for {
		next := errors.Unwrap(current)
		if next == nil {
			return current.Error()
		}
		current = next
	}
}
