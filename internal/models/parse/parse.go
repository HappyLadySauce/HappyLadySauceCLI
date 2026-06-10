// Package parse normalizes, validates, and classifies AI model identifiers.
// Package parse 规范化、校验并分类 AI 模型标识符。
package parse

import (
	"errors"
	"strings"
)

// Validate checks that the model name is not empty after trimming whitespace.
// Validate 校验模型名去除空白后非空。
func Validate(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New("model name is required")
	}
	return nil
}

// IsValid returns true if Validate(raw) passes.
// IsValid 返回 Validate(raw) 是否通过。
func IsValid(raw string) bool {
	return Validate(raw) == nil
}

// Normalize lowercases and replaces separators (/, :, _, whitespace) with a single dash.
// Repeated separators are collapsed and leading/trailing dashes are trimmed.
// Normalize 将模型名转小写，并将分隔符统一折叠为单个短横线。
func Normalize(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if lower == "" {
		return ""
	}

	var b strings.Builder
	lastWasDash := false
	for _, r := range lower {
		switch r {
		case '/', ':', '_', '-', ' ', '\t', '\n', '\r':
			if !lastWasDash {
				b.WriteByte('-')
				lastWasDash = true
			}
		default:
			b.WriteRune(r)
			lastWasDash = false
		}
	}

	return strings.Trim(b.String(), "-")
}

// LastSegment returns the final /-separated segment, stripping any provider prefix.
// LastSegment 返回最后一个 / 分隔的段，去除厂商路径前缀。
func LastSegment(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		return trimmed[idx+1:]
	}
	return trimmed
}

// Tokens splits a normalized model name into word-level tokens on common separators.
// Tokens 按常见分隔符将模型名拆分为词元。
func Tokens(name string) []string {
	fields := strings.FieldsFunc(name, func(r rune) bool {
		switch r {
		case '/', ':', '_', '-', '.', ' ', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	})

	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			tokens = append(tokens, f)
		}
	}

	// Preserve "meta-llama" as a compound token for fuzzy matching.
	// 保留 "meta-llama" 作为复合词元，方便模糊匹配。
	if strings.Contains(name, "meta-llama") {
		tokens = append(tokens, "meta-llama")
	}
	return tokens
}

// Provider extracts the vendor prefix before the first /, or returns "".
// Provider 提取第一个 / 之前的厂商前缀，无前缀时返回 ""。
func Provider(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		return strings.ToLower(trimmed[:idx])
	}
	return ""
}
