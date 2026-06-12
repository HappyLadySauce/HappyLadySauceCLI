# Review: 能力网关安全中间件审计

> **日期**: 2026-06-12 17:14
> **审查范围**: `HEAD~2..HEAD` (c377bbd, a5a92f3)
> **风险等级**: HIGH
> **审查者**: Claude Code

---

## 变更概要

最近两个提交引入了基于 Eino ADK 的安全中间件体系：

| 提交 | 描述 |
|------|------|
| `a5a92f3` | Add capability gateway with security policy and interactive approval |
| `c377bbd` | Enhance documentation and workflow for code quality checks |

新增文件 17 个，修改文件 4 个，净增 ~1359 行。核心架构：

```
Tool Call → ExecutionSecurityMiddleware
              ├─ Registry.Get() → Descriptor
              ├─ Policy.Engine.Evaluate() → Decision
              └─ Decision.Review? → SessionGrants.IsAllowed() → Approver.Approve()
```

---

## 调用链全貌

```
main()
  └─ cmd/app/app.go:run()
       └─ agents/interactive.go:RunLoop()
            └─ interactive_setup.go:newInteractiveRuntime()
                 ├─ tools.NewAgentTools()
                 │    └─ weather.GetWeatherTool()           ← InferTool error 被丢弃
                 ├─ tools.NewCapabilityRegistry()
                 │    └─ capability.NewRegistry(weather.CapabilityDescriptor())
                 ├─ middlewares.NewChatModelAgentMiddlewares()
                 │    ├─ security.NewExecutionSecurityMiddleware()  ← 本次核心
                 │    │    ├─ capability.Registry
                 │    │    ├─ policy.Engine
                 │    │    ├─ policy.SessionGrants
                 │    │    └─ terminalApprover
                 │    ├─ compact.NewCompactMiddleware()
                 │    └─ usage.NewUsageMiddleware()
                 └─ adk.NewChatModelAgent()

每次 Tool Call 拦截流程:
  Eino ADK
    └─ ExecutionSecurityMiddleware.WrapInvokableToolCall()
         └─ authorize()
              ├─ descriptorForTool()
              │    └─ registry.Get("get_weather") → found
              ├─ policy.Evaluate(descriptor, registered=true)
              │    RiskLow + Allow → ActionAllow
              └─ switch ActionAllow → audit + return nil
         └─ endpoint(ctx, args) → getWeather() 执行
              ├─ http.DefaultClient.Do()          ← 无超时
              ├─ 未检查 StatusCode                 ← 
              └─ json.Unmarshal()
         └─ auditExecution()
              └─ descriptorForTool() 再查一次      ← 重复查询
```

---

## 发现的问题

### 1. HIGH: HTTP 客户端无超时

**文件**: `internal/tools/weather/weather.go:70`
**类别**: 安全/可用性

`http.DefaultClient` 的 `Timeout` 为 0（永不超时）。

**攻击场景**: 攻击者通过 prompt 注入使 agent 调用天气工具查询某个触发 API 挂起的城市，goroutine 永久阻塞，导致 agent 卡死。

**修复**:
```go
var weatherHTTPClient = &http.Client{Timeout: 10 * time.Second}

// 在 getWeather 中使用:
resp, err := weatherHTTPClient.Do(httpReq)
```

---

### 2. HIGH: HTTP 响应状态码未检查

**文件**: `internal/tools/weather/weather.go:70-79`
**类别**: 安全

HTTP 4xx/5xx 响应的 body 被静默解析为 `WeatherToolResult`，返回零值数据（City="", Temperature=0），agent 可能基于虚假数据决策。

**修复**:
```go
if resp.StatusCode < 200 || resp.StatusCode >= 300 {
    return nil, fmt.Errorf("weather API returned status %d", resp.StatusCode)
}
```

---

### 3. HIGH: InferTool 错误被静默丢弃

**文件**: `internal/tools/weather/weather.go:103`
**类别**: 稳定性

```go
tool, _ := utils.InferTool(...)  // error 被丢弃
```

若推断失败，tool 为 nil，后续 Eino 调用触发 nil pointer dereference → panic。

**修复**:
```go
t, err := utils.InferTool(...)
if err != nil {
    panic(fmt.Sprintf("get_weather tool inference failed: %v", err))
}
```

---

### 4. MEDIUM: TypeUnknown 在 validCapabilityType 中的语义矛盾

**文件**: `internal/capability/descriptor.go:134`
**类别**: 代码质量

`validCapabilityType` 返回 `true` 给 `TypeUnknown`，但 `Validate()` 在行 92 单独拒绝它。函数语义不一致 —— `TypeUnknown` 是安全占位类型，不应通过注册。

**修复**: 从 `validCapabilityType` 中移除 `TypeUnknown`。

---

### 5. MEDIUM: 全局审批锁阻塞所有并发工具调用

**文件**: `internal/middlewares/security/security.go:208-209`
**类别**: 性能/并发

```go
m.approvalMu.Lock()         // 全局互斥锁
// ... 等待用户输入 y/N（数秒到数分钟）
approval, err := m.approver.Approve(ctx, req)
```

当工具 A 等待审批时，不同工具 B 也被阻塞。应使用按 GrantKey 分片的细粒度锁。

---

### 6. MEDIUM: auditExecution 重复查询 descriptor

**文件**: `internal/middlewares/security/security.go:263-264`
**类别**: 可维护性

`authorize()` 已获取 descriptor（行 193），但未传递给 `auditExecution()`，导致重复 `registry.Get()` 调用。

---

### 7. MEDIUM: 审批提示写入 STDOUT 而非 STDERR

**文件**: `internal/terminal/renderer.go:203-207`
**类别**: 用户体验

审批提示应写入 `r.errOut`（类比 `Error()` 方法）。若用户重定向 STDOUT，审批提示不可见。

---

### 8. LOW: 初始化失败时 goroutine 可能泄露

**文件**: `internal/agents/interactive_setup.go:36-47`
**类别**: 资源管理

若 `NewCapabilityRegistry()` 失败，`promptReader` 的 goroutine 在输入 context 被取消前不会停止。

---

## 测试盲区

| 场景 | 覆盖 |
|------|------|
| nil tCtx 传入 toolName/toolCallID | ❌ 未测试 |
| approver 为 nil 的降级路径 | ❌ 未测试 |
| HTTP 错误状态码路径 | ❌ 未测试 |
| 不同工具的独立审批 | ❌ 未测试（只测了同一工具的序列化） |

---

## 总结

| 统计 | 值 |
|------|-----|
| HIGH | 3 |
| MEDIUM | 4 |
| LOW | 1 |
| 总发现 | 8 |
| 建议阻断 | #1, #2, #3 应在合并前修复 |
| 建议近期修复 | #4, #5, #6, #7 |
| 可延后 | #8 |
