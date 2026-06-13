// Package execguard provides helpers for tool endpoints to reconcile execution targets with authorization.
// Package execguard 提供 tool endpoint 将执行目标与授权结果对齐的 helper。
package execguard

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/utils/urlscope"
)

// MatchAuthorizedURL reports whether resolvedURL matches an authorized url resource.
// MatchAuthorizedURL 判断 resolvedURL 是否与已授权 url 资源匹配。
func MatchAuthorizedURL(operation securitycore.OperationRequest, resolvedURL string) bool {
	for _, resource := range operation.Resources {
		if resource.Kind != securitycore.ResourceKindURL {
			continue
		}
		if urlscope.Allowed(resolvedURL, []string{resource.Value}) {
			return true
		}
	}
	return false
}

// RequireAuthorizedPath normalizes actualPath and verifies it matches the authorized operation on ctx.
// RequireAuthorizedPath 规范化 actualPath，并校验其与 ctx 中已授权 operation 一致。
func RequireAuthorizedPath(ctx context.Context, guard *securitycore.WorkspaceGuard, actualPath string) (string, error) {
	if guard == nil {
		return "", fmt.Errorf("workspace guard is required")
	}
	operation, ok := securitycore.AuthorizedOperationFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("authorized operation is required")
	}
	normalized, err := guard.NormalizePath(actualPath)
	if err != nil {
		return "", fmt.Errorf("normalize execution path: %w", err)
	}
	if !MatchAuthorizedExecutionPath(operation, normalized) {
		return "", fmt.Errorf("execution path is outside authorized resources: %s", normalized)
	}
	return normalized, nil
}

// MatchAuthorizedPath reports whether resolvedPath exactly matches an authorized path/file resource.
// MatchAuthorizedPath 判断 resolvedPath 是否与已授权 path/file 资源精确匹配。
func MatchAuthorizedPath(operation securitycore.OperationRequest, resolvedPath string) bool {
	resolvedPath = cleanPathForCompare(resolvedPath)
	for _, resource := range operation.Resources {
		if resource.Kind != securitycore.ResourceKindPath && resource.Kind != securitycore.ResourceKindFile {
			continue
		}
		if samePath(cleanPathForCompare(resource.Value), resolvedPath) {
			return true
		}
	}
	return false
}

// MatchAuthorizedExecutionPath reports whether resolvedPath is valid for the authorized operation.
// MatchAuthorizedExecutionPath 判断 resolvedPath 是否符合已授权 operation 的路径语义。
func MatchAuthorizedExecutionPath(operation securitycore.OperationRequest, resolvedPath string) bool {
	if MatchAuthorizedPath(operation, resolvedPath) {
		return true
	}
	if operation.OperationKind != securitycore.OperationFileList {
		return false
	}
	return MatchAuthorizedPathPrefix(operation, resolvedPath)
}

// MatchAuthorizedPathPrefix reports whether resolvedPath is inside an authorized path resource.
// MatchAuthorizedPathPrefix 判断 resolvedPath 是否位于已授权 path resource 内。
func MatchAuthorizedPathPrefix(operation securitycore.OperationRequest, resolvedPath string) bool {
	resolvedPath = cleanPathForCompare(resolvedPath)
	for _, resource := range operation.Resources {
		if resource.Kind != securitycore.ResourceKindPath {
			continue
		}
		if pathWithin(cleanPathForCompare(resource.Value), resolvedPath) {
			return true
		}
	}
	return false
}

func cleanPathForCompare(value string) string {
	return filepath.Clean(strings.TrimSpace(value))
}

func samePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func pathWithin(root, child string) bool {
	if samePath(root, child) {
		return true
	}
	rel, err := filepath.Rel(root, child)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}
