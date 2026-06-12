package policy

import (
	"sync"

	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
)

// SessionGrants stores approvals that are valid only for the current process session.
// SessionGrants 存储仅在当前进程会话内有效的授权。
type SessionGrants struct {
	mu     sync.RWMutex
	grants map[string]struct{}
}

// NewSessionGrants creates an empty session grant store.
// NewSessionGrants 创建空的会话授权存储。
func NewSessionGrants() *SessionGrants {
	return &SessionGrants{grants: map[string]struct{}{}}
}

// Allow records a session-scoped approval for operation.
// Allow 记录 operation 的会话级授权。
func (g *SessionGrants) Allow(operation securitycore.OperationRequest) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.grants[operation.SessionGrantKey()] = struct{}{}
}

// IsAllowed reports whether operation has a session-scoped approval.
// IsAllowed 判断 operation 是否已有会话级授权。
func (g *SessionGrants) IsAllowed(operation securitycore.OperationRequest) bool {
	if g == nil {
		return false
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.grants[operation.SessionGrantKey()]
	return ok
}
