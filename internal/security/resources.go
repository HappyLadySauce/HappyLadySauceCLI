package security

import (
	"fmt"
	"strings"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/utils/urlscope"
)

// NormalizeResources returns resources with local path-like values canonicalized by the guard.
// NormalizeResources 返回已由 guard 规范化本地路径类 value 的资源列表。
func (g *WorkspaceGuard) NormalizeResources(resources []OperationResource) ([]OperationResource, error) {
	if len(resources) == 0 {
		return nil, nil
	}
	next := make([]OperationResource, 0, len(resources))
	for _, resource := range resources {
		switch resource.Kind {
		case ResourceKindPath, ResourceKindFile:
			normalized, err := g.NormalizePath(resource.Value)
			if err != nil {
				return nil, fmt.Errorf("normalize operation resource: %w", err)
			}
			resource.Value = normalized
		}
		next = append(next, resource)
	}
	return next, nil
}

// ValidateNetworkResources verifies URL resources against the capability descriptor allowlist.
// ValidateNetworkResources 根据 capability descriptor 白名单校验 URL 资源。
func ValidateNetworkResources(operation OperationRequest) error {
	if !operation.RequiresNetworkResourceValidation() {
		return nil
	}

	for _, resource := range operation.Resources {
		if resource.Kind != ResourceKindURL {
			continue
		}
		if len(operation.Capability.Resources) == 0 {
			return fmt.Errorf("network resource requires descriptor resources allowlist")
		}
		if !urlscope.Allowed(resource.Value, operation.Capability.Resources) {
			return fmt.Errorf("network resource is outside descriptor resources: %s", resource.Value)
		}
	}
	return nil
}

// ValidateFileResources verifies file operation scopes and normalized path resources.
// ValidateFileResources 校验文件操作 scope 与规范化后的 path/file 资源。
func ValidateFileResources(operation OperationRequest) error {
	if !operation.RequiresFileResourceValidation() {
		return nil
	}

	fileScopeCount := countFileScopes(operation.Capability.Scopes)
	hasFileOperationKind := strings.HasPrefix(operation.OperationKind, "file.")
	hasFileResource := operation.HasResourceKind(ResourceKindPath) || operation.HasResourceKind(ResourceKindFile)

	if fileScopeCount > 0 && !hasFileOperationKind {
		return fmt.Errorf("file scope requires file operation kind: %s", operation.OperationKind)
	}
	if hasFileResource && !hasFileOperationKind {
		return fmt.Errorf("file resource requires file operation kind: %s", operation.OperationKind)
	}
	if !hasFileOperationKind {
		return nil
	}
	requiredScope := operation.RequiredFileScope()
	if requiredScope == "" {
		return fmt.Errorf("unsupported file operation kind: %s", operation.OperationKind)
	}
	if !hasScope(operation.Capability.Scopes, requiredScope) {
		return fmt.Errorf("file operation %s requires scope %s", operation.OperationKind, requiredScope)
	}
	if !hasFileResource {
		return fmt.Errorf("file operation %s requires path or file resource", operation.OperationKind)
	}
	return nil
}

func countFileScopes(scopes []string) int {
	count := 0
	for _, scope := range scopes {
		if IsSupportedFileScope(scope) {
			count++
		}
	}
	return count
}

func hasScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if strings.TrimSpace(scope) == want {
			return true
		}
	}
	return false
}
