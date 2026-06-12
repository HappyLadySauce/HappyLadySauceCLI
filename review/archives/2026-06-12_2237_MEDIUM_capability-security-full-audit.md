# Capability 安全中间件全面审计

> **日期**: 2026-06-12 22:37  
> **范围**: `a5a92f3..dcd1845`（Capability Gateway → 拒绝/工具错误 soft-fail → 日志/流式审计 → 低风险 network 放行）  
> **审查者**: Cursor Agent  
> **测试**: `make test` 全绿（30 packages）

---

## 总览

本次按 `review/prompt/REVIEW_PROMPT.md` 四阶段流程，对 capability 安全栈自 `a5a92f3` 起的全部变更做了独立复验。当前**无 CRITICAL/HIGH 阻塞项**；主要剩余风险为文档漂移、新工具注册纪律，以及若干 LOW 级 UX/配置诚实性问题。

| 状态 | 数量 |
|------|------|
| 🔴 CRITICAL | 0 |
| 🟠 HIGH | 0 |
| 🟡 MEDIUM | 4 |
| 🟢 LOW | 6 |

**综合评分：约 4.2 / 5** — 可交付；建议优先修正 architecture.md 中 GrantKey 文档漂移，并在注册新 HTTP 工具时强制 `network.*` + `network:` scope。

---

## 第一阶段：架构与调用链

```text
main (cmd/root.go)
  └─ app.run → agents.RunLoop
       └─ newInteractiveRuntime (interactive_setup.go)
            ├─ tools.NewCapabilityRegistry / NewOperationBuilders / NewAgentTools
            ├─ newTerminalApprover
            ├─ middlewares.NewChatModelAgentMiddlewares
            │    ├─ ExecutionSecurityMiddleware   ← Wrap* + authorize + audit
            │    ├─ CompactMiddleware             ← BeforeModelRewriteState
            │    └─ UsageMiddleware               ← model_call_end / token
            ├─ adk.NewChatModelAgent(Handlers: [security, compact, usage])
            └─ interactiveRuntime.Run REPL
                 read prompt → runner.Run(history)
                   → ConsumeAgentEvents → terminal.Renderer
                 context/session: BeginTurn / FinishTurn → SQLite (sanitized)
```

**数据流向：**

| 阶段 | 输入 | 输出 / 副作用 |
|------|------|----------------|
| Descriptor | `capability.Registry` | Risk、DefaultPolicy、Scopes、Resources |
| OperationBuilder | 脱敏 args summary | OperationKind、Resources（如 `network.weather` + url） |
| Policy Engine | `OperationRequest` | allow / review / deny |
| Authorize | SessionGrants + Approver | grant 或 recoverable denial |
| Endpoint | ctx（command 才有 timeout） | tool result 或 JSON error payload |
| Audit | `logger.Info/Error` | `capability_policy` / `capability_call` → klog 双文件 |

**错误传播矩阵（与 README 一致）：**

| 类别 | 示例 | 行为 |
|------|------|------|
| Recoverable | 用户拒绝、策略 Deny、工具执行失败、输出超限 | JSON payload + `nil` error → ReAct 继续 |
| Invariant | 路径逃逸、network URL 越界、无 Approver、审批 I/O 失败 | Go error → 回合 hard-fail |
| LLM 失败 | API 错误 | 终端报错，REPL 不退出 |

---

## 第二阶段 / 第三阶段：跨层结论

| 检查项 | 结论 |
|--------|------|
| 中间件链顺序 | Security → Compact → Usage；工具拦截与 model 前后钩子职责分离 ✅ |
| Registry 三角一致 | `tools_test.go` 校验 registry ↔ builders ↔ AgentTools ✅ |
| 未知工具 | `Registered=false` → `descriptor_missing` → Review ✅ |
| 低风险 network | `RiskLow + DefaultPolicyAllow + network.*` → Allow（`dcd1845` 有意决策）✅ |
| **中风险 network** | `RiskMedium + Allow + network.*` → **Review**（`policy_test.go:111-130`）✅ |
| network URL allowlist | `validateOperationScopes` + `resourceURLAllowed`；越界 hard-fail ✅ |
| 拒绝 soft-fail | `denial.go` + `finishAuthorize` + `toolresult.FormatFailure` ✅ |
| 流式审计 | EOF / Close 均触发 `capability_call`（`stream_proxy.go`）✅ |
| SessionGrantKey | network 不含 args_sha；session 级授权可跨参数复用（单测覆盖）✅ |
| WorkspaceGuard | 仅 path/file kind；当前无文件工具 ⚠️ |
| 持久化脱敏 | SanitizeText + SanitizeJSON；metadata_only 可跳过正文 ✅ |

---

## 发现项

| # | 严重度 | 文件:行号 | 类别 | 问题 | 修复建议 |
|---|--------|-----------|------|------|----------|
| 1 | 🟡 MEDIUM | `docs/security/architecture.md:220-226` | 文档 | §6.1 仍写 session 存储 key 为 `GrantKey()`，实现已改为 [`SessionGrantKey()`](internal/security/operation.go:93-108)（network 不含 args_sha）。 | 更新 §6.1/§6.2，区分 `GrantKey`（审计/单次，含 args_sha）与 `SessionGrantKey`（session 存储）。 |
| 2 | 🟡 MEDIUM | `docs/security/architecture.md:378-382` | 文档 | §9.5 写审批锁按 `GrantKey` 串行，代码为 [`lockApproval(operation.SessionGrantKey())`](internal/middlewares/security/security.go:340)。 | 更正为 `SessionGrantKey`，与 §10.3 及 `operation_test.go` 对齐。 |
| 3 | 🟡 MEDIUM | `AGENTS.md` / `CLAUDE.md` | 文档 | Architecture / Tools 节未描述 **拒绝与工具错误 soft-fail** 矩阵（[`README.md:270-277`](README.md:270-277) 已有）。新贡献者易误以为审批拒绝会 hard-fail 整轮。 | 在 Tools 或 Middleware 节增加 3–5 行摘要，链到 `docs/security/architecture.md` §9。 |
| 4 | 🟡 MEDIUM | `internal/security/policy/engine.go:106-109` | 安全/纪律 | **`RiskMedium + DefaultPolicyAllow` 的非 network 工具会直接 Allow**（跳过 review 分支后的默认路径）。HTTP 工具若未声明 `network.*` OperationKind 与 `network:` scope，将绕过 network 专项审查。当前 `get_weather` 配置正确；风险在未来新工具注册疏漏。 | 在 `capability.Descriptor.Validate` 或 registry 测试中增加规则：含 `network:` scope 或 builder 产出 url 资源时，OperationKind 必须以 `network.` 开头；或为 medium+allow 增加显式 `RequiresReview` 标记。 |
| 5 | 🟢 LOW | `internal/security/policy/engine.go:77-81` | 文档 | 注释写「RiskMedium + DefaultPolicyAllow → ActionAllow」，未说明 **network 分支在前**会先落入 Review。易误导维护者。 | 注释改为：「非 network/command/high 的 RiskMedium+Allow 才 Allow；network.* 仅 RiskLow+Allow 豁免。」 |
| 6 | 🟢 LOW | `pkg/options/security.go:84-87` | 配置 | `security.approval_default` 暴露为 CLI/ENV，但 `Validate()` 仅允许 `"review"`；其他值在 engine 层 fail-closed deny，用户无法配置合法替代模式。 | flag help 注明「当前仅 review 生效」，或移除此配置项直至有多模式需求。 |
| 7 | 🟢 LOW | `internal/agents/approver.go:19-35` | 性能/UX | `terminalApprover.mu` 为**全局锁**；与 middleware 按 `SessionGrantKey` 分锁叠加后，不同 capability 审批仍串行。 | 低优先级：per-session channel 或文档说明单终端 REPL 设计约束。 |
| 8 | 🟢 LOW | `internal/middlewares/security/security.go:469-480` | 安全 | Scope enforcement **仅 `network:` URL**；未来 `file:` scope 尚无对称 roots 交叉校验（仍依赖 path kind + WorkspaceGuard）。 | 文件工具落地时补 `file:` scope 校验及单测。 |
| 9 | 🟢 LOW | `internal/tools/weather/weather.go:25` | 可观测性 | 天气 HTTP 硬编码 `10s` 超时，未复用 `security.command_timeout_seconds`（该配置仅作用于 `command.run`）。 | 可接受；若需统一，增加 `NetworkTimeoutSeconds` 或由 builder 注入 deadline。 |
| 10 | 🟢 LOW | `internal/middlewares/security/stream_proxy.go:17-38` | 并发 | 流式 tool 每调用启动转发 goroutine；`finalize`/审计在 goroutine 退出时**异步**触发。 | 可接受；V=1 日志顺序可能与终端 tool 输出略错位。 |
| 11 | 🟢 LOW | `internal/security/sanitizer.go:62-85` | 安全 | 脱敏对 string 值走 `SanitizeText`（含 `sk-`/`ghp_` 等前缀），但对**无关键词、高熵秘密**（如 `{ "note": "hunter2" }`）可能仍写入 SQLite。 | 默认高敏环境用 `metadata_only`；或增加 entropy/heuristic 值级检测。 |

---

## 与 2230 草稿报告的差异说明

| 2230 原项 | 本次结论 |
|-----------|----------|
| #1「RiskMedium network 免审批」 | **不成立**。`TestEngineReviewsNetworkOperationsWhenNotLowRiskAllow` 已证明 medium network → Review。应关注 **非 network 路径的 medium+allow 自动放行**（本次 #4）。 |
| #2–#4 文档漂移 | **仍成立**（本次 #1–#3） |
| #5–#9 LOW 项 | **仍成立**（本次 #6–#10） |

---

## 已验证亮点

- **拒绝 soft-fail**：`agent_events_test` 三轮 ReAct 通过 ✅
- **工具错误 soft-fail**：endpoint/超限 → `tool_error_returned` ✅
- **REPL 容错**：`interactive_runtime.go:50-54` LLM 错误不 exit ✅
- **HTTP 出站**：weather 10s timeout + 1MiB body cap + 状态码检查 ✅
- **network 越界**：`TestWrapInvokableToolCallRejectsNetworkResourceOutsideScope` hard-fail ✅
- **日志权限**：`logs/` 目录 `0700`、日志文件 `0600` ✅
- **klog 双文件**：info / error 分离 ✅

---

## 建议后续动作

1. 修正 `architecture.md` GrantKey / SessionGrantKey 漂移（#1、#2）。
2. 同步 AGENTS.md / CLAUDE.md soft-fail 说明（#3）。
3. 为新工具增加 registry 纪律检查或文档 checklist（#4）。
4. 手动回归：`make run V=1` → 重庆天气（无审批）→ 故意拒绝审批与 tool 参数错误，确认 `[tool denied]` / `[tool error]` 与 `capability_call` 日志。

---

## 评分

| 维度 | 评分 |
|------|------|
| 策略与审批 | ★★★★☆ |
| 实际防护面 | ★★★★☆ |
| 单链路可观测性 | ★★★★★ |
| 文档与代码一致 | ★★★★☆（GrantKey 漂移待修） |
| 测试覆盖 | ★★★★☆ |

---

审核修复处理完成后, 务必将 review 文档归档至 `review\archives` 目录

---

## 修复记录（2026-06-12）

| # | 状态 | 说明 |
|---|------|------|
| 1 | ✅ | `architecture.md` §4.4 / §6.1 / §6.2：`GrantKey` vs `SessionGrantKey` |
| 2 | ✅ | `architecture.md` §9.5 + 结构体注释：审批锁改为 `SessionGrantKey()` |
| 3 | ✅ | `AGENTS.md` / `CLAUDE.md`：Execution Security & soft-fail 摘要 |
| 4 | ✅ | `Descriptor.Validate` 网络注册纪律 + `tools_test` builder 断言 + §3.6 文档 |
| 5 | ✅ | `engine.go` 注释已区分 medium network vs 非 network |
| 6 | ✅ | 删除无效 `security.approval_default` 配置 |
| 7–11 | ⏭️ | LOW 遗留（审批全局锁、file: scope、weather 超时、流式 goroutine、脱敏熵） |

验证：`make check && make test` 全绿。
