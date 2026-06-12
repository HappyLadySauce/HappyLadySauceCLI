# Post-Hardening 安全与可观测性审计

> **日期**: 2026-06-12 22:30  
> **范围**: `a5a92f3..dcd1845`（Capability Gateway → 拒绝 soft-fail → 日志/流式审计 → 低风险 network 放行）  
> **审查者**: Cursor Agent  
> **测试**: `make test` 全绿（30 packages）

---

## 总览

相对 [2145 复验报告](archives/2026-06-12_2145_MEDIUM_security-logging-recheck.md)，**N1/N2 已落地**，拒绝 soft-fail、双文件 klog、流式 Close 审计、SessionGrantKey 宽粒度均已实现。当前无 CRITICAL 阻塞；主要剩余风险集中在 **策略矩阵对中风险 network 的放行**、**文档细节漂移** 与 **审批 UX 锁粒度**。

| 状态 | 数量 |
|------|------|
| 🔴 CRITICAL | 0 |
| 🟠 HIGH | 0 |
| 🟡 MEDIUM | 4 |
| 🟢 LOW | 5 |

**综合评分：约 4.2 / 5** — 可交付；建议下一批处理 M1（中风险 network 策略）与 M2（文档 GrantKey 漂移）。

---

## 第一阶段：架构与调用链

```text
main (cmd/root.go)
  └─ app.run → agents.RunLoop
       └─ newInteractiveRuntime
            ├─ tools.NewCapabilityRegistry / NewOperationBuilders / NewAgentTools
            ├─ middlewares.NewChatModelAgentMiddlewares
            │    ├─ ExecutionSecurityMiddleware  ← 工具 Wrap* + authorize + audit
            │    ├─ CompactMiddleware            ← BeforeModelRewriteState
            │    └─ UsageMiddleware              ← model_call_end / token 追踪
            ├─ adk.NewChatModelAgent + Runner
            └─ interactiveRuntime.Run REPL 循环
                 read prompt → runner.Run(history)
                   → ConsumeAgentEvents → terminal.Renderer
                 context/session: BeginTurn / FinishTurn → SQLite
```

**数据流要点：**

- 工具权限：`Descriptor`（registry）→ `OperationBuilder`（运行时 kind/resources）→ `policy.Engine.Evaluate` → `authorize`（session grant / Approver）→ endpoint。
- 错误传播：invariant 类（路径/scope/无 Approver）→ Go error → ToolNode 中断；可恢复类（tool error / user&policy denial）→ JSON payload + `nil` error → ReAct 继续。
- 日志：`logger.Info/Error` → klog 双文件（info/error），trace 字段由 `AttachTurn` 注入。

---

## 第二阶段 / 第三阶段：跨层结论

| 检查项 | 结论 |
|--------|------|
| 中间件链顺序 | security 负责 tool 包装，compact/usage 负责 model 前后；职责分离合理 ✅ |
| 策略 vs descriptor | `get_weather` 声明 `RiskLow+Allow`，engine 对低风险 network 放行（`dcd1845`）✅ |
| 未知工具 | `Registered=false` → `descriptor_missing` → Review ✅ |
| network URL allowlist | `validateOperationScopes` 校验 descriptor.Resources ✅ |
| 流式审计 | EOF + Close 均触发 `capability_call`（`stream_proxy.go`）✅ |
| WorkspaceGuard | 仅 `path`/`file` kind；当前无文件工具，配置对用户无感 ⚠️ |
| 持久化 | SQLite + sanitizer；无 JSONL 双写 ✅ |

---

## 发现项

| # | 严重度 | 文件:行号 | 类别 | 问题 | 修复建议 |
|---|--------|-----------|------|------|----------|
| 1 | 🟡 MEDIUM | `internal/security/policy/engine.go:100-104` | 安全 | **`RiskMedium + DefaultPolicyAllow` 的 network 工具免审批**。矩阵仅对 `RiskLow` 豁免 `network_operation`；未来若注册 `RiskMedium` 且声明 Allow 的 HTTP 工具（如通用 fetch），将零确认出站。 | 将豁免条件收紧为「仅 builtin + RiskLow + Allow + descriptor.Resources 非空」；或要求 medium network 一律 Review，单测覆盖 `RiskMedium`/`network.*` 组合。 |
| 2 | 🟡 MEDIUM | `docs/security/architecture.md:220-226` | 文档 | §6.1 写 session 存储 key 为 `GrantKey()`，实现已改为 [`SessionGrantKey()`](internal/security/operation.go:93-108)（network 不含 args_sha）。 | 更新 §6.1/§6.2 表格与 prose，区分 `GrantKey`（审计/单次）与 `SessionGrantKey`（session 存储）。 |
| 3 | 🟡 MEDIUM | `docs/security/architecture.md:378-382` | 文档 | §9.5 写审批锁按 `GrantKey` 串行，代码为 [`lockApproval(operation.SessionGrantKey())`](internal/middlewares/security/security.go:340)。 | 更正为 `SessionGrantKey`，并注明与 §10.3 一致。 |
| 4 | 🟡 MEDIUM | `AGENTS.md` / `CLAUDE.md` | 文档 | 架构段未描述 **拒绝/工具错误 soft-fail** 与错误处理矩阵（`README.md:270-277` 已有）。新贡献者易误以为审批拒绝仍 hard-fail。 | 在 AGENTS/CLAUDE 的 Architecture 或 Tools 节增加 3–5 行 soft-fail 摘要，链到 `docs/security/architecture.md` §9。 |
| 5 | 🟢 LOW | `pkg/options/security.go:84-87` | 配置 | `security.approval_default` 暴露为 CLI/ENV 配置，但 `Validate()` 仅允许 `"review"`；非 review 值在 engine 层 fail-closed deny，**用户无法配置其他合法模式**。 | 在 flag help 注明「当前仅 review 生效」；或删除该配置项直至有多模式需求。 |
| 6 | 🟢 LOW | `internal/agents/approver.go:19-35` | 性能/UX | `terminalApprover.mu` 为**全局锁**，与 middleware 按 `SessionGrantKey` 分锁叠加后，不同 capability 的审批仍串行，多 tool 并行时会排队。 | 低优先级：改为 per-session 或 channel 队列；或文档说明单终端 REPL 设计约束。 |
| 7 | 🟢 LOW | `internal/middlewares/security/security.go:469-480` | 安全 | Scope enforcement **仅 `network:`**；未来 `file:` scope 尚无对称 roots 交叉校验（仍依赖 path kind + WorkspaceGuard）。 | 文件工具落地时补 `file:` scope 与 workspace roots 校验及单测。 |
| 8 | 🟢 LOW | `internal/tools/weather/weather.go:25` | 可观测性 | 天气 HTTP 使用硬编码 `10s` 超时，**未复用** `security.command_timeout_seconds`（该配置仅作用于 `command.run`）。 | 可接受；若需统一，为 network 工具增加可选 `NetworkTimeoutSeconds` 或让 builder 注入 deadline。 |
| 9 | 🟢 LOW | `internal/middlewares/security/stream_proxy.go:17-38` | 并发 | 每个流式 tool 调用启动转发 goroutine；`finalize`/审计在 goroutine 退出时**异步**触发（Close 后数 ms）。 | 可接受；V=1 日志顺序可能与终端 tool 输出略错位。若需严格同步，可在测试中已用的 wait 模式文档化。 |

---

## 2130 / 2145 遗留项对照

| 原项 | 当前状态 |
|------|----------|
| 2145-N1 README phase 漂移 | ✅ 已修复 |
| 2145-N2 流式 Close 审计 | ✅ `stream_proxy.go` + 单测 |
| 2145-N3 ApprovalDefault 配置诚实性 | ⚠️ 仍开放（#5） |
| 2145-N4 file: scope | ⚠️ 仍开放（#7） |
| 2145-N5 审批全局锁 | ⚠️ 仍开放（#6） |
| 2130-#2 network 零审批 | ⚠️ **产品决策变更**：`dcd1845` 对 `RiskLow+Allow` 放行；见 #1 对 medium 的连带风险 |

---

## 已验证亮点

- **拒绝 soft-fail**：`denial.go` + `finishAuthorize` + `toolresult.FormatFailure`；`agent_events_test` 三轮 ReAct ✅
- **工具错误 soft-fail**：endpoint/超限 → JSON + `tool_error_returned` ✅
- **REPL 容错**：`interactive_runtime.go:50-54` LLM/回合错误不 exit ✅
- **Registry 三角校验**：`tools_test.go` registry ↔ builders ↔ AgentTools ✅
- **HTTP 出站**：weather 10s timeout + 1MiB body cap + 状态码检查 ✅
- **脱敏**：`sanitizer.go` 精确 key + 持久化/审计路径 ✅

---

## 建议后续动作

1. 决策 #1：中风险 network 是否一律 Review（推荐）或收紧 exempt 条件。
2. 修正 architecture.md GrantKey / SessionGrantKey 漂移（#2、#3）。
3. 同步 AGENTS.md / CLAUDE.md soft-fail 说明（#4）。
4. 手动回归：`make run V=1` → 重庆天气（应无审批）→ 故意触发 tool 参数错误与用户拒绝，确认 `[tool error]` / `[tool denied]` 与日志 `capability_call`。

---

## 更新后评分

| 维度 | 2145 复验 | 本次 |
|------|-----------|------|
| 策略与审批 | ★★★★★ | ★★★★☆（低风险 network 放行是有意权衡） |
| 实际防护面 | ★★★★☆ | ★★★★☆ |
| 单链路可观测性 | ★★★★☆ | ★★★★★ |
| 文档与代码一致 | ★★★☆☆ | ★★★★☆（architecture 少量 GrantKey 漂移） |
| 测试覆盖 | ★★★★☆ | ★★★★☆ |

---

审核修复处理完成后, 务必将 review 文档归档至 `review\archives` 目录

---

## 修复记录（2026-06-12）

| # | 状态 | 说明 |
|---|------|------|
| 1 | ✅ | `engine.go`：network 免审收紧为 `SourceBuiltin + RiskLow + DefaultPolicyAllow + resources 非空`；新增 `TestEngineReviewsLowRiskNetworkWithoutResources` |
| 2 | ✅ | `architecture.md` §4.4 / §6.1 / §6.2：区分 `GrantKey` 与 `SessionGrantKey` |
| 3 | ✅ | `architecture.md` §9.5：审批锁改为 `SessionGrantKey()` |
| 4 | ✅ | `AGENTS.md` / `CLAUDE.md`：补充 Execution Security & soft-fail 摘要 |
| 5 | ✅ | 删除无效 `security.approval_default` 配置（options / config env / policy.Config / middleware 注入） |
| — | ✅ | 去重：`user_denied` / `policy_denied` 常量统一至 `internal/security/denial.go`，`toolresult` 引用 `IsStructuredDenialReason` |
| 6–9 | ⏭️ | 未处理（审批全局锁、file: scope、weather 超时、流式 goroutine 时序）— 仍为 LOW 遗留 |

验证：`make check && make test` 全绿（30 packages）。
