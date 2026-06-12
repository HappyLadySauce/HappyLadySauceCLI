package security

import (
	"fmt"

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
