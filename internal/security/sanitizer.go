package security

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const redactedValue = "[REDACTED]"

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|auth[_-]?token|access[_-]?token|secret|password|passwd|pwd)\s*[:=]\s*)("[^"]+"|'[^']+'|[^\s,;}\]]+)`),
	regexp.MustCompile(`(?is)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\b(sk-[A-Za-z0-9_-]{12,}|ghp_[A-Za-z0-9_]{12,}|github_pat_[A-Za-z0-9_]{20,}|xox[baprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{12,})\b`),
}

// SanitizeText redacts common secrets from text before logs or persistence.
// SanitizeText 在写入日志或持久化前脱敏常见密钥。
func SanitizeText(value string) string {
	if value == "" {
		return ""
	}
	out := value
	out = sensitivePatterns[0].ReplaceAllString(out, `${1}`+redactedValue)
	out = sensitivePatterns[1].ReplaceAllString(out, `${1}`+redactedValue)
	out = sensitivePatterns[2].ReplaceAllString(out, redactedValue)
	out = sensitivePatterns[3].ReplaceAllString(out, redactedValue)
	return out
}

// SanitizeJSON redacts secrets in serialized JSON while preserving valid JSON when possible.
// SanitizeJSON 在尽量保持 JSON 有效的前提下脱敏已序列化 JSON 中的密钥。
func SanitizeJSON(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	var payload any
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return SanitizeText(value)
	}
	return string(mustMarshal(sanitizeValue(payload)))
}

// SummarizeArguments returns a sanitized compact argument summary for approval prompts.
// SummarizeArguments 返回用于审批提示的脱敏参数摘要。
func SummarizeArguments(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var payload any
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return truncate(SanitizeText(value), 240)
	}
	return truncate(summarizeValue(payload), 240)
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		next := make(map[string]any, len(typed))
		for _, key := range sortedKeys(typed) {
			value := typed[key]
			if isSensitiveKey(key) {
				next[key] = redactedValue
				continue
			}
			next[key] = sanitizeValue(value)
		}
		return next
	case []any:
		next := make([]any, 0, len(typed))
		for _, value := range typed {
			next = append(next, sanitizeValue(value))
		}
		return next
	case string:
		return SanitizeText(typed)
	default:
		return typed
	}
}

func summarizeValue(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		parts := make([]string, 0, len(typed))
		for _, key := range sortedKeys(typed) {
			value := typed[key]
			if isSensitiveKey(key) {
				parts = append(parts, key+"="+redactedValue)
				continue
			}
			parts = append(parts, key+"="+summarizeValue(value))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		return fmt.Sprintf("[items=%d]", len(typed))
	case string:
		return SanitizeText(typed)
	case nil:
		return "null"
	default:
		return fmt.Sprint(typed)
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	for _, marker := range []string{"api_key", "apikey", "token", "secret", "password", "passwd", "pwd", "authorization"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mustMarshal(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		return []byte(SanitizeText(""))
	}
	return data
}

func truncate(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	if maxLen <= 3 {
		return value[:maxLen]
	}
	return value[:maxLen-3] + "..."
}
