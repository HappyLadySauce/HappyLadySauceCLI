# Security Audit Report: Security Middleware Implementation

**Audit Date**: 2026-06-12  
**Scope**: HEAD~4..HEAD (4 commits: capability registry, security middleware, approval handling, security options)  
**Files Audited**: 38 files, 2138 insertions, 175 deletions  
**Test Status**: All tests passing (30 packages)

---

## Architecture Overview

### 调用链图

```
main() → app.Run() → RunLoop()
  → newInteractiveRuntime()
    → tools.NewCapabilityRegistry()       // 注册 capability descriptor
    → tools.NewAgentTools()                // 创建 Eino tools
    → middlewares.NewChatModelAgentMiddlewares()
      → compact.NewCompactor()             // 上下文压缩
      → security.NewExecutionSecurityMiddleware()  // ★ 安全中间件
        → policy.NewEngine()               // 策略评估引擎
        → policy.NewSessionGrants()        // 会话级授权缓存
        → security.NewWorkspaceGuard()     // 路径沙箱
        + Approver (terminal)              // 交互式审批
      → compact.NewCompactMiddleware()
      → usage.NewUsageMiddleware()
    → adk.NewChatModelAgent(Handlers: [security, compact, usage])

Tool Call Flow:
  agent.Execute() → tool call triggered
    → [securityMiddleware.WrapInvokableToolCall]
      → m.authorize(ctx, tCtx, argsSummary)
        → m.operationForTool()          // 构建 OperationRequest
          → m.descriptorForTool()       // Registry 查找
          → builder(ctx, op, args)      // 工具专属 builder
          → SanitizeText()              // 脱敏参数
          → m.normalizeOperationResources()  // 路径规范化
        → m.policy.Evaluate(operation)  // 策略决策
        → switch decision.Action:
           Allow    → 直接放行
           Deny     → 返回错误
           Review   → m.grants.IsAllowed() 检查缓存
                     → m.lockApproval()     双检锁
                     → m.approver.Approve() 交互审批
                     → m.grants.Allow()     缓存授权
      → m.auditDecision()                // 记录决策审计
      → endpoint(ctx, args)              // 执行原始 tool call
      → m.auditExecution()               // 记录执行审计
```

### 数据流与错误传播
- 策略决策路径: `OperationRequest` → `Engine.Evaluate()` → `PolicyDecision` → 授权/拒绝/审批
- 错误传播: 所有安全层错误均返回到 agent loop，阻止原始 tool 执行
- 审计日志: 决策和执行均通过 `logger.Info/Error` 记录为结构化 klog

---

## Findings Summary

| # | Severity | File:Line | Category | Issue | Recommendation |
|---|----------|-----------|----------|-------|----------------|
| 1 | MEDIUM | `internal/middlewares/security/security.go:142-148` | Concurrency | Stream audit flag `audited` not synchronized | Protect with `atomic.Bool` or `sync.Mutex` |
| 2 | MEDIUM | `internal/middlewares/security/security.go:71,278-283` | Resource | `approvalLocks` sync.Map grows unbounded (mutex leak) | Periodically clean up unused mutex entries |
| 3 | MEDIUM | `internal/security/workspace.go:68-73` | Security | Symlink TOCTOU bypass for non-existent files | Resolve parent directory symlinks iteratively |
| 4 | MEDIUM | `internal/security/sanitizer.go:14` | Security | Bearer token regex misses `Bearer:token` (colon) | Add colon as valid separator |
| 5 | LOW | `internal/security/sanitizer.go:138-146` | Correctness | `truncate()` may split multi-byte UTF-8 characters | Use `[]rune` or `utf8.ValidString` aware slicing |
| 6 | LOW | `internal/security/operation.go:74-86` | Correctness | `GrantKey()` unescaped `\|` separator causes collisions | Escape `\|` in key components |
| 7 | LOW | `internal/middlewares/security/security.go:57,101` | Config | `CommandTimeoutSeconds`/`MaxToolOutputBytes` accepted but not enforced | Enforce limits in Wrap* methods or document as future-only |
| 8 | LOW | `internal/security/workspace.go:84` | Portability | `EqualFold` may mismatch on case-sensitive filesystems | Use `filepath.Match` or filesystem-aware comparison |

---

## Detailed Analysis

### Finding 1: Stream Audit Race Condition

**File**: `internal/middlewares/security/security.go:142-148` (and `:199-205` for Enhanced variant)

**Problem**: The `audited` boolean variable is closed over in the stream wrapper closures but accessed from multiple callbacks (`WithOnEOF`, `WithErrWrapper`) without synchronization. If multiple goroutines consume from the stream concurrently, they may race on reading/writing `audited`.

**Attack Scenario**: If the Eino framework uses concurrent goroutines for stream processing, `doAudit` may be called twice (or zero times) due to a data race on `audited`. This leads to duplicate or missing audit records for streamed tool calls.

**Fix**:
```go
var audited atomic.Bool
doAudit := func(streamErr error) {
    if audited.CompareAndSwap(false, true) {
        m.auditExecution(ctx, operation, start, streamErr)
    }
}
```

---

### Finding 2: Unbounded approvalLocks Map Growth

**File**: `internal/middlewares/security/security.go:71` (field), `:278-283` (lockApproval)

**Problem**: `lockApproval()` uses `sync.Map.LoadOrStore` to create a per-grant-key mutex, but these mutexes are never removed from the map. Each unique tool + resource combination creates a new entry. In long-running sessions with many different tool calls, this causes unbounded memory growth.

**Attack Scenario**: In a long-running interactive session (hours/days), an attacker could trigger many distinct tool calls with varying resource paths, causing the `approvalLocks` map to grow indefinitely. This is a slow memory leak.

**Fix**:
```go
func (m *ExecutionSecurityMiddleware) lockApproval(grantKey string) func() {
    mu := &sync.Mutex{}
    mu.Lock()
    actual, loaded := m.approvalLocks.LoadOrStore(grantKey, mu)
    if loaded {
        mu.Unlock() // Discard our new mutex
        actualMu := actual.(*sync.Mutex)
        actualMu.Lock()
        return actualMu.Unlock
    }
    return func() {
        mu.Unlock()
        // Clean up the entry after use — next call will create a fresh one
        m.approvalLocks.Delete(grantKey)
    }
}
```

---

### Finding 3: Symlink TOCTOU for Non-Existent Files in WorkspaceGuard

**File**: `internal/security/workspace.go:68-73`

**Problem**: `canonicalPath()` calls `filepath.EvalSymlinks()` to resolve symlinks, but when the final path component doesn't exist (`os.IsNotExist`), it falls through to purely lexical path resolution. If a parent directory in the path is a symlink to outside the workspace, the symlink is never resolved, and the path incorrectly passes validation.

**Attack Scenario**:
1. User creates a symlink inside workspace: `ln -s /etc /workspace/evil`
2. Tool attempts to read `/workspace/evil/passwd` (a non-existent file in current context, but consider write path)
3. `EvalSymlinks` tries to resolve the full path → fails (file doesn't exist yet) → `IsNotExist` → falls through
4. Lexical check: `filepath.Rel("/workspace", "/workspace/evil/passwd")` = `evil/passwd` → passes
5. The file operation goes through, writing to `/etc/passwd` via the symlink

**Fix**:
```go
func canonicalPath(path string) (string, error) {
    path = strings.TrimSpace(path)
    if path == "" {
        return "", fmt.Errorf("path is required")
    }
    if !filepath.IsAbs(path) {
        cwd, err := os.Getwd()
        if err != nil {
            return "", fmt.Errorf("get working directory: %w", err)
        }
        path = filepath.Join(cwd, path)
    }
    cleaned := filepath.Clean(path)
    // Resolve symlinks component-by-component so parent symlinks are caught
    // even when the final component doesn't exist.
    resolved, err := resolveSymlinksUpToLastExisting(cleaned)
    if err != nil {
        return "", err
    }
    absolute, err := filepath.Abs(resolved)
    if err != nil {
        return "", fmt.Errorf("resolve absolute path: %w", err)
    }
    return filepath.Clean(absolute), nil
}

// resolveSymlinksUpToLastExisting resolves symlinks for the longest existing prefix,
// then appends the non-existing suffix.
func resolveSymlinksUpToLastExisting(path string) (string, error) {
    evaluated, err := filepath.EvalSymlinks(path)
    if err == nil {
        return evaluated, nil
    }
    if !os.IsNotExist(err) {
        return "", fmt.Errorf("resolve symlinks: %w", err)
    }
    // Walk up until we find an existing parent we can resolve
    parent := filepath.Dir(path)
    if parent == path {
        return path, nil // reached root
    }
    resolvedParent, err := resolveSymlinksUpToLastExisting(parent)
    if err != nil {
        return "", err
    }
    return filepath.Join(resolvedParent, filepath.Base(path)), nil
}
```

---

### Finding 4: Bearer Token Regex Missing Colon Variant

**File**: `internal/security/sanitizer.go:14`

**Problem**: The regex `(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+` only matches Bearer followed by **whitespace**. It does not match `bearer:token123` (colon), `authorization:bearer:token` (colon), or `Bearer:token` (colon).

**Attack Scenario**: An LLM model call may produce arguments like `{"Authorization": "Bearer:secret123"}`. The sanitizer won't redact this, and it gets logged to diagnostic files or displayed in approval prompts.

**Fix**:
```go
// Change the pattern to match both colon and whitespace separators
regexp.MustCompile(`(?i)(bearer[\s:]+)[A-Za-z0-9._~+/=-]+`),
```

---

### Finding 5: UTF-8 Truncation in SummarizeArguments

**File**: `internal/security/sanitizer.go:138-146`

**Problem**: `truncate()` slices Go strings at byte boundaries, which can split multi-byte UTF-8 characters in the middle, producing invalid UTF-8 output.

**Fix**:
```go
func truncate(value string, maxLen int) string {
    if len(value) <= maxLen {
        return value
    }
    if maxLen <= 3 {
        return value[:maxLen]
    }
    // Walk backwards from the cut point to find a valid UTF-8 boundary
    cut := maxLen - 3
    for cut > 0 && cut < len(value) {
        if utf8.RuneStart(value[cut]) {
            break
        }
        cut--
    }
    return value[:cut] + "..."
}
```

---

### Finding 6: GrantKey Collision from Unescaped Separator

**File**: `internal/security/operation.go:74-86`

**Problem**: `GrantKey()` joins components with `|` without escaping. If a tool name, source, or resource value contains `|`, two different operations could produce the same key, causing unintended session-grant sharing.

**Fix**: Escape `|` and `=` in all components before joining:
```go
func escapeGrantKeyComponent(s string) string {
    return strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), "|", "\\|")
}
```

---

### Finding 7: CommandTimeout/MaxToolOutput Accepted but Not Enforced

**File**: `internal/middlewares/security/security.go:57,101`

**Problem**: `Config.CommandTimeoutSeconds` and `Config.MaxToolOutputBytes` are validated and stored but never used in any `Wrap*` method. The code comments mark them as "reserved for future", but they appear as working configuration options to users.

**Fix**: Either enforce the limits in `WrapInvokableToolCall`:
```go
func (m *ExecutionSecurityMiddleware) WrapInvokableToolCall(...) {
    return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
        operation, err := m.authorize(ctx, tCtx, ...)
        if err != nil {
            return "", err
        }
        // Enforce timeout
        if m.commandTimeoutSeconds > 0 {
            var cancel context.CancelFunc
            ctx, cancel = context.WithTimeout(ctx, time.Duration(m.commandTimeoutSeconds)*time.Second)
            defer cancel()
        }
        start := time.Now()
        result, err := endpoint(ctx, argumentsInJSON, opts...)
        m.auditExecution(ctx, operation, start, err)
        return result, err
    }, nil
}
```

Or reveal these as explicitly disabled until tool support lands:
```go
fs.IntVar(&o.CommandTimeoutSeconds, "security-command-timeout-seconds", o.CommandTimeoutSeconds,
    "Default timeout reserved for future command tools (not yet enforced)")
```

---

### Finding 8: EqualFold Path Comparison on Case-Sensitive Filesystems

**File**: `internal/security/workspace.go:84`

**Problem**: `strings.EqualFold(path, root)` uses case-insensitive comparison. On Linux/macOS (case-sensitive filesystems), two paths that differ only in case are different directories, but `EqualFold` would consider them equal. While the subsequent `filepath.Rel` check would catch most cases, an exact-match bypass is theoretically possible if the root and path differ only in case.

**Fix**: On Unix, skip `EqualFold` and rely solely on `filepath.Rel`:
```go
func pathWithinRoot(path, root string) bool {
    path = filepath.Clean(path)
    root = filepath.Clean(root)
    // On case-sensitive filesystems, only exact match counts
    if path == root {
        return true
    }
    // On Windows, paths are case-insensitive
    if runtime.GOOS == "windows" && strings.EqualFold(path, root) {
        return true
    }
    rel, err := filepath.Rel(root, path)
    if err != nil {
        return false
    }
    return rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}
```
Note: this requires adding `"runtime"` to imports.

---

## Cross-Layer Analysis

### Middleware Chain Order
`[securityMiddleware, compactMiddleware, usageMiddleware]` — Security is outermost (correct). It intercepts tool calls before compaction or usage tracking can observe them.

### Data Consistency
- **Capability Registry**: Thread-safe via `sync.RWMutex`. Read by `authorize()`, populated at startup. No write-after-read races since registry is immutable after initialization.
- **SessionGrants**: `sync.RWMutex` protected. The double-checked locking pattern in `authorize()` (lines 235-244) correctly handles the TOCTOU between `IsAllowed` and `Allow`.

### Failure Modes
- **WorkspaceGuard creation fails** → middleware creation fails → agent startup fails → process exits. Acceptable.
- **Policy engine**: Always succeeds (pure function, no I/O). No failure mode.
- **Approver fails/blocked** → approval denied, tool call rejected. Secure default.
- **Sanitizer fails**: `mustMarshal` returns empty byte slice on marshal error. Graceful degradation.
- **Compactor fails** → compaction skipped, original messages unchanged. Graceful degradation.

---

## Test Coverage Assessment

| Package | Coverage | Notes |
|---------|----------|-------|
| `internal/security/` | Good | Sanitizer, operation, workspace all tested |
| `internal/security/policy/` | Good | All decision branches + grant scoping tested |
| `internal/middlewares/security/` | Excellent | 11 test cases covering deny, allow, review, session cache, concurrent approval, path escape |
| `pkg/options/` | Adequate | Validation paths covered |

**Missing tests**:
- No test for `audited` flag race in stream wrappers (would need race detector)
- No test for symlink bypass in WorkspaceGuard when path doesn't exist
- No test for `approvalLocks` memory growth over many distinct tool calls

---

审核修复处理完成后, 务必将review文档归档至review\archives目录
