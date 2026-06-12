# Review: 操作级安全 / 持久化脱敏 / 审批粒度审计

> **日期**: 2026-06-12 19:21  
> **审查范围**: 当前工作区未提交变更（23 文件，+532 / -138）  
> **风险等级**: HIGH  
> **审查者**: Cursor Agent  

---

## 变更概要

本次变更在已有 capability 网关之上，引入 **OperationRequest 操作级策略输入**、**会话审批粒度（once / session）**、**上下文持久化脱敏** 与 **SecurityOptions 配置面**。核心新增/重构：

| 模块 | 变更 |
|------|------|
| `internal/security/` | 新增 `operation.go`、`sanitizer.go`、`workspace.go` |
| `internal/middlewares/security/` | 授权链从 `Descriptor` 升级为 `OperationRequest`；审计只记录 args SHA |
| `internal/security/policy/` | `Evaluate(OperationRequest)`；`SessionGrants` 按 `operation.GrantKey()` |
| `internal/agents/approver.go` | 支持 `y=once` / `s=session`；展示 operation/resources/args |
| `internal/context/tracker/` | 持久化前按 `security.persist_content` 脱敏或丢弃正文 |
| `pkg/options/security.go` | 新增 workspace / timeout / max output / persist 配置 |

---

## 第一阶段：架构与调用链

```
main()
  └─ cmd/app/app.go:run()
       ├─ config.Init(Home, Model, Security)     ← Security 首次注入全局配置
       └─ agents.RunLoop()
            └─ interactive_setup.go:newInteractiveRuntime()
                 ├─ tools.NewCapabilityRegistry()
                 ├─ tools.NewOperationBuilders()   ← 按 tool 名注册 OperationBuilder
                 └─ middlewares.NewChatModelAgentMiddlewares()
                      └─ [security → compact → usage]   ← 顺序未变

每次 Tool Call:
  Eino ADK
    └─ ExecutionSecurityMiddleware.Wrap*ToolCall()
         └─ authorize(ctx, tCtx, argsSummary)
              ├─ operationForTool()
              │    ├─ registry.Get(toolName) → Descriptor
              │    ├─ OperationBuilder(toolName)  ← 补充 OperationKind / Resources
              │    └─ SanitizeText(argsSummary)
              ├─ policy.Evaluate(operation)
              └─ ActionReview 分支:
                   ├─ grants.IsAllowed(operation)     ← GrantKey 含 operationKind + resources
                   ├─ lockApproval(grantKey)          ← 按 key 细粒度锁
                   ├─ approver.Approve()              ← once / session
                   └─ session 时 grants.Allow(operation)
         └─ endpoint() → 工具实现（如 weather HTTP）
         └─ auditExecution() → 日志仅含 args_summary_sha

回合结束持久化:
  session.Service.FinishTurn()
    └─ tracker.SetMessages()
         └─ messageRecordFromSchema()
              └─ securityPersistContentMode()
                   ├─ sanitized → SanitizeText + SanitizeJSON(RawJSON)
                   └─ metadata_only → 清空 content/reasoning/raw_json
    └─ contextstore → SQLite context_messages
```

**数据流**：工具参数 JSON → `SummarizeArguments`/`SanitizeText` → 审批 UI / 审计 SHA；schema.Message → tracker 脱敏 → SQLite。  
**错误传播**：策略 deny / 用户拒绝 → `authorize` 返回 error，endpoint 不执行；持久化失败在 `FinishTurn` 向上返回。

---

## 第四阶段：发现汇总

| # | 严重度 | 文件:行号 | 类别 | 问题 | 修复建议 |
|---|--------|-----------|------|------|----------|
| 1 | 🟠 HIGH | `pkg/options/security.go:28-33` | 安全/配置 | `WorkspaceRoots`、`CommandTimeoutSeconds`、`MaxToolOutputBytes`、`ApprovalDefault` 已通过 CLI/ENV 暴露并校验，但运行时 **除 `persist_content` 外均未接入**；`WorkspaceGuard` 已实现却未被 `interactive_setup` 或 middleware 引用。用户配置 workspace 根目录后会产生虚假安全感。 | 在 `newInteractiveRuntime` 中 `security.NewWorkspaceGuard(cfg.Security.WorkspaceRoots)` 并注入 future file tools / OperationBuilder；或在未接线前从 flags 中移除并标注 `reserved`。 |
| 2 | 🟠 HIGH | `internal/security/policy/engine.go:63-78` | 安全 | `Evaluate` 仅读取 `request.Capability`（descriptor 静态元数据），**忽略 `OperationRequest.Risk` 与 `OperationKind`**。未来 OperationBuilder 若根据参数提升风险（如 `command.run` + 危险路径），策略引擎不会触发 review/deny。 | 决策矩阵改为优先使用 `request.Risk`（非空时），并可选按 `OperationKind` 增加规则；单测覆盖 builder 提升 risk 的场景。 |
| 3 | 🟡 MEDIUM | `internal/security/sanitizer.go:58-80` | 安全 | 脱敏依赖 **键名启发式**（`isSensitiveKey`）与 **正则**（`SanitizeText`）。非敏感键名下的秘密（如 `{"city":"sk-live-..."}` 或自定义字段）会写入 SQLite `content`/`raw_json` 及审批提示 `args=`。 | 对 persistence 路径默认 `metadata_only` 或增加 `PersistContentRedactedOnly`；对 string 值增加 entropy/前缀检测（`sk-`, `ghp_` 等）；审批 UI 只显示 SHA 摘要而非完整 args。 |
| 4 | 🟡 MEDIUM | `internal/context/tracker/tracker.go:239` | 稳定性 | `json.Marshal(msg)` 错误被 `_` 丢弃；序列化失败时 `RawJSON` 为空但 `Content`/`Reasoning` 仍可能含未完整脱敏的正文，造成 **列与 RawJSON 不一致**。 | 检查 `err`；失败时清空 `RawJSON` 并对列字段再次 `SanitizeText`，或 clone msg 后写 sanitized 字段再 Marshal。 |
| 5 | 🟡 MEDIUM | `internal/middlewares/security/security.go:125-141` | 可观测性 | Streamable 工具审计依赖 consumer 读到 EOF；若 stream 创建后未消费/未 Close，`auditExecution` **永不触发**，安全事件缺失。 | 在 `defer result.Close()` 包装层增加 fallback audit；或 `WithOnClose` 钩子；单测覆盖「未 Recv 即丢弃 stream」路径。 |
| 6 | 🟡 MEDIUM | `internal/context/tracker/tracker.go:21` | 并发 | 包级可变 `persistContentModeOverride` 供测试注入，**无 mutex**；若后续新增并行测试修改该变量，可能与 `securityPersistContentMode()` 读者产生 data race。 | 改为 `sync/atomic.Value` 或 `testing` 专用 hook（如 `Tracker` 可选 `persistMode` 字段）。 |
| 7 | 🟡 MEDIUM | `internal/agents/approver.go:34-35` | 性能/UX | `terminalApprover.mu` 为 **全局互斥**，不同 capability 的并发审批也被串行化（middleware 已按 `GrantKey` 分锁，但 approver 再次全局锁）。多 tool 并行 review 时吞吐下降。 | 审批锁下沉到 middleware 已有 `lockApproval`；approver 仅负责 I/O，或按 session 使用独立 prompt channel。 |
| 8 | 🟢 LOW | `internal/security/operation.go:73-84` | 安全/设计 | `GrantKey` 中 `Resources` 顺序依赖 append 顺序，未排序；descriptor 与 builder 若顺序不同，同一逻辑操作可能产生 **不同 session grant**，导致重复审批。 | 对 `Resources` 按 `Kind+Value` 排序后再拼接 GrantKey（与旧 `descriptor.GrantKey` 的 sortedCopy 一致）。 |
| 9 | 🟢 LOW | `internal/security/sanitizer.go:86-94` | 质量 | `summarizeValue` 遍历 map 顺序随机，审批提示 `args=` 内容非确定性，不利于测试与日志比对。 | 对 map keys 排序后再构造 summary。 |
| 10 | 🟢 LOW | `internal/tools/tools.go:37-40` | 可维护性 | `NewOperationBuilders` 仅注册 `get_weather`；新增工具若忘记注册 builder，将回落到 `native.tool` + descriptor resources，**操作语义变粗**。 | 在 `NewAgentTools` 或 registry 测试中 assert builders 与 registry 工具名集合一致。 |

---

## 第二阶段要点（逐函数）

### `ExecutionSecurityMiddleware.authorize`
- **输入**：nil `tCtx` → 空 toolName；nil approver + review → deny；session grant 命中 → 跳过审批。
- **安全**：✅ once/session 分离（修复旧版“任意 y 即 session grant”）；✅ 审计不记录明文 args。
- **副作用**：session grants 写内存 map；审批锁 per GrantKey。

### `operationForTool` / `OperationBuilder`
- **信任边界**：builder 为内部注册函数，可覆写 `Resources`/`OperationKind`/`Risk`；policy 尚未消费动态 risk（见 #2）。

### `messageRecordFromSchema`
- **metadata_only**：正文全空，保留 role/tool 元数据 — ✅ 测试覆盖。
- **sanitized**：Content/Reasoning 与 RawJSON 双路径脱敏 — 对非 JSON 秘密仍有漏网（见 #3）。

### `SanitizeText` / `SummarizeArguments`
- Bearer、api_key=、PRIVATE KEY 块 — ✅ 单测覆盖。
- 边界：空字符串、非 JSON 参数 truncate 240 — 行为合理。

### `WorkspaceGuard.NormalizePath`
- 路径穿越、symlink 逃逸 — ✅ 单测覆盖；**尚未进入生产路径**（见 #1）。

### `terminalApprover.Approve`
- `y/yes` → once；`s/session` → session；默认 deny — ✅ 单测覆盖。
- 审批输出走 `errOut` — ✅（旧 review #7 已修复）。

---

## 第三阶段：跨层分析

| 维度 | 结论 |
|------|------|
| **中间件顺序** | `security → compact → usage` 合理；安全拦截在 compaction 之前，tool 未执行前即 deny。 |
| **数据一致性** | 策略层用 `OperationRequest`，grants 用 `GrantKey()`，审批 UI 用同一 operation — 一致。`policy.Risk` 与 `operation.Risk` 可能不一致（#2）。 |
| **失败模式** | 策略/审批失败 → tool 不执行 ✅。weather HTTP 已有 10s timeout + 状态码检查 + 1MiB body limit ✅。持久化脱敏失败无 fallback（Marshal 静默，#4）。 |
| **Session 审批语义** | 由“一次 y 永久 session”改为显式 `s=session` — **安全增强** ✅；`TestWrapInvokableToolCallApprovalDefaultsToOneOperation` 验证 once 默认。 |

---

## 测试与验证

- 全量 `make test`：**通过**（含 security / tracker / middleware 新增用例）。
- 仍缺覆盖：stream 未消费时审计（#5）、`policy.Evaluate` 动态 risk（#2）、Security 配置接线后集成测试（#1）。

---

## 总结

| 统计 | 值 |
|------|-----|
| CRITICAL | 0 |
| HIGH | 2 |
| MEDIUM | 5 |
| LOW | 3 |
| **合计** | **10** |

**建议合并前处理**：#1（配置与实现一致）、#2（策略消费 operation 级 risk）。  
**建议近期处理**：#3–#7。  
**可跟进**：#8–#10。

本次变更整体方向正确：操作级 grant key、once/session 审批分离、持久化脱敏与审计 SHA 均为实质性安全改进；主要风险在于 **已暴露但未生效的安全配置** 与 **策略引擎仍只看静态 descriptor**。
