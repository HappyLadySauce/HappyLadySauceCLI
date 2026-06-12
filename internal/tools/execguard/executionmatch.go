// Package execguard provides helpers for tool endpoints to reconcile execution targets with authorization.
// Package execguard 提供 tool endpoint 将执行目标与授权结果对齐的 helper。
package execguard

import (
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
