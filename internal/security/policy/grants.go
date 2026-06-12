package policy

import (
	"sync"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
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

// Allow records a session-scoped approval for descriptor.
// Allow 记录 descriptor 的会话级授权。
func (g *SessionGrants) Allow(descriptor capability.Descriptor) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.grants[descriptor.GrantKey()] = struct{}{}
}

// IsAllowed reports whether descriptor has a session-scoped approval.
// IsAllowed 判断 descriptor 是否已有会话级授权。
func (g *SessionGrants) IsAllowed(descriptor capability.Descriptor) bool {
	if g == nil {
		return false
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.grants[descriptor.GrantKey()]
	return ok
}
