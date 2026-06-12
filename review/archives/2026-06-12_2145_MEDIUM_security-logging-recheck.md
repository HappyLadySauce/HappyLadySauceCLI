# Re-check: 安全层 + 单链路日志（对照 2130 报告）

> **日期**: 2026-06-12 21:45  
> **基准**: `review/archives/2026-06-12_2130_MEDIUM_security-logging-observability-audit.md`  
> **审查者**: Cursor Agent  
> **测试**: `make test` 全绿（30 packages）  
> **修复完成**: 2026-06-12 — N1（README phase 表）、N2（流式 Close 审计）已落地

---

## 修复记录（2026-06-12）

| 项 | 修复 |
|----|------|
| N1 | `README.md` 诊断日志 phase 表与示例对齐 `AGENTS.md`（`user_prompt_len`、V=1 tool `agent_event`、`capability_*`、`session_close`） |
| N2 | `stream_proxy.go` 新增 `proxyStreamReaderWithFinalize`；`WrapStreamable*` / `WrapEnhancedStreamable*` 在 Close 未消费时仍触发 `capability_call` 审计；单测覆盖 |

---

## 总览

相对 2130 报告，**13 项中 10 项已落地**，安全层与诊断日志已达到可交付状态。剩余问题以 **文档漂移** 和 **流式审计边界** 为主，无 CRITICAL 级阻塞。

| 状态 | 数量 |
|------|------|
| ✅ 已修复 | 10 |
| ⚠️ 部分修复 / 设计保留 | 1 |
| ❌ 仍开放 | 2 |

---

## 2130 发现项对照

| # | 原严重度 | 标题 | 状态 | 证据 |
|---|----------|------|------|------|
| 1 | HIGH | `ApprovalDefault` 未接线 | ✅ | `middleware.go:52` 注入 `policy.Config{ApprovalDefault: ...}`；`engine.go:108-113` `review()` 读取；`policy_test.go:123-134` |
| 2 | HIGH | weather 出站零审批 | ✅ | `engine.go:99-101` `network.*` / `network:` scope → Review；`policy_test.go:89-109` |
| 3 | HIGH | `Scopes` 无 enforcement | ✅ | `security.go:432-444` `validateOperationScopes` + URL allowlist；`security_test.go` network scope 拒绝用例 |
| 4 | MEDIUM | WorkspaceGuard 对当前工具无效 | ⚠️ 保留 | 仍仅 `path`/`file` kind 生效；flag help 已改为准确描述（`security.go:106`） |
| 5 | MEDIUM | 全局 CommandTimeout | ✅ | `security.go:517-521` 仅 `command.run` 套超时 |
| 6 | MEDIUM | Session grant 不含参数 | ✅ | `operation.go:128-132` network/command/high-risk 含 `args_sha`；`operation_test.go:65-88` |
| 7 | MEDIUM | `capability_call` 字段不足 | ✅ | `auditExecution` 含 `decision`/`decision_reason`/`approval_scope`/`output_bytes` |
| 8 | MEDIUM | JSONL 双链路与 dead code | ✅ | `conversationlog/` 已删除；`AGENTS.md`/`CLAUDE.md` 改为单链路 + SQLite |
| 9 | MEDIUM | 无 `session_close` | ✅ | `session/service.go:135-137`；实测 log L15 |
| 10 | MEDIUM | V=1 缺 tool 过程态 | ✅ | `agent_events.go:155-159` tool `agent_event` 降为 V=1 |
| 11 | LOW | 脱敏误伤/漏网 | ✅ | `sanitizer.go:112-119` 精确匹配 + `_suffix`；`architecture.md:252` 已说明 |
| 12 | LOW | architecture.md 格式 | ✅ | §13.2 末尾 stray fence 已去除 |
| 13 | LOW | Registry 一致性 | ✅ | `tools_test.go:52-77` AgentTools ↔ Registry ↔ Builder 三角校验 |

---

## 实测日志复验（`.HAPPLADYSAUCECLI/logs/happyladysaucecli.log`）

| 检查项 | 结果 |
|--------|------|
| `session_open` / `session_close` | ✅ L2 / L15 |
| 单文件诊断 log（无 JSONL） | ✅ 仅 `happyladysaucecli.log` |
| 工具调用审计链 | ⚠️ 本轮 turn 2 **未触发** `get_weather`（`tool_calls=[]`），故无 `capability_*` 行；属模型行为，非日志缺失 |
| Trace 字段 | ✅ `session_id` / `conversation_id` / `user_turn_seq` / `model_call` 齐全 |

> 要验证 weather 审批链，需再次提问并确认模型实际发起 tool call；通过后应出现 `decision_reason=network_operation` 的 `capability_policy` 行。

---

## 新发现（2130 未覆盖或修复后仍存）

| # | 严重度 | 文件 | 类别 | 问题 | 修复建议 |
|---|--------|------|------|------|----------|
| N1 | 🟡 MEDIUM | `README.md:177-193` | 文档 | phase 表与示例 **与实现不一致**：① 写 V=1 含 `user_prompt` 明文，代码仅 `user_prompt_len`；② 写 `agent_event` 为 V=2，工具事件已在 V=1；③ 示例含 `message_roster` 等不存在字段；④ 缺少 `capability_policy`/`capability_call`/`session_close`。 | 对齐 `AGENTS.md` 的 phase 表；示例改用真实 log 字段（SHA、无 prompt 正文）。 |
| N2 | 🟡 MEDIUM | `internal/middlewares/security/security.go:149-197` | 可观测性 | 流式工具：仅 `Close()` 而不消费时，`stream_opened` 已写但 **`capability_call` 完成审计可能缺失**（`TestWrapStreamableToolCallCanBeClosedWithoutConsumption` 未断言 audit）。 | `WithOnClose` 或 wrapper `Close()` hook 触发 `doAudit`；测试断言 audit 一次。 |
| N3 | 🟢 LOW | `pkg/options/security.go:84-87` | 配置 | `ApprovalDefault` 校验仅允许 `"review"`，engine 虽支持非 review 时 fail-closed deny（`approval_default_unsupported`），**用户无法配置其他合法模式**。 | 若长期仅 review：删除 flag 或在 help 注明「仅 review，其他值在 engine 层 deny」；若计划扩展：放宽 Validate 并文档化 deny 语义。 |
| N4 | 🟢 LOW | `internal/middlewares/security/security.go:432-442` | 安全 | Scope enforcement **仅 `network:`**；未来 `file:` scope 尚无对称校验（仍依赖 path kind + WorkspaceGuard）。 | 文件工具落地时补 `file:` scope 与 roots 交叉校验。 |
| N5 | 🟢 LOW | `internal/agents/approver.go:19` | 性能 | `terminalApprover.mu` 仍为全局锁，与 middleware 按 GrantKey 分锁叠加后，不同 capability 审批仍串行。 | 低优先级；多 tool 并行时再优化。 |

---

## 更新后评分

| 维度 | 2130 | 复验 |
|------|------|------|
| 策略与审批 | ★★★★☆ | ★★★★★ |
| 实际防护面（当前工具集） | ★★☆☆☆ | ★★★★☆ |
| 单链路可观测性 | ★★★☆☆ | ★★★★☆ |
| 文档与代码一致 | ★★☆☆☆ | ★★★☆☆（AGENTS 已对齐，README 仍漂移） |
| 测试覆盖 | ★★★★☆ | ★★★★☆ |

**综合：约 4.0 / 5** — 2130 报告中的 P0/P1 已基本完成；建议下一批只处理 **N1（README）** 与 **N2（流式 audit 边界）**。

---

## 建议后续动作

1. 修正 `README.md` 诊断日志章节（N1）。
2. 补流式 Close 审计 + 测试（N2）。
3. 手动跑一轮「重庆天气」并在 V=1 下确认出现 `capability_policy` + `capability_call`（含 `network_operation`）。
4. 本复验报告修复完成后，与 2130 报告一并保留在 `review/archives/`。

---

审核修复处理完成后, 务必将 review 文档归档至 `review\archives` 目录
