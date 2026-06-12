// Package urlscope provides canonical URL comparison for allowlist checks.
// Package urlscope 提供用于白名单校验的 URL 规范化比较。
package urlscope

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// CanonicalURLForAllowlist normalizes a URL for descriptor allowlist comparison.
// CanonicalURLForAllowlist 规范化 URL 以供 descriptor 白名单比较。
func CanonicalURLForAllowlist(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("url scheme and host are required")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("url userinfo is not allowed")
	}

	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("url host is required")
	}

	port := parsed.Port()
	switch {
	case port == "":
	case scheme == "https" && port == "443":
	case scheme == "http" && port == "80":
	default:
		host = host + ":" + port
	}

	cleanPath := path.Clean(parsed.EscapedPath())
	if cleanPath == "." {
		cleanPath = "/"
	}
	if cleanPath != "/" {
		cleanPath = strings.TrimSuffix(cleanPath, "/")
		if cleanPath == "" {
			cleanPath = "/"
		}
	}

	canonical := &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   cleanPath,
	}
	if parsed.RawQuery != "" {
		canonical.RawQuery = parsed.RawQuery
	}
	return canonical.String(), nil
}

// Allowed reports whether resource matches one entry in allowed after canonicalization.
// Allowed 判断 resource 经规范化后是否与 allowed 中任一项匹配。
func Allowed(resource string, allowed []string) bool {
	normalizedResource, err := CanonicalURLForAllowlist(resource)
	if err != nil {
		return false
	}
	for _, candidate := range allowed {
		normalizedCandidate, err := CanonicalURLForAllowlist(candidate)
		if err != nil {
			continue
		}
		if normalizedResource == normalizedCandidate {
			return true
		}
	}
	return false
}
