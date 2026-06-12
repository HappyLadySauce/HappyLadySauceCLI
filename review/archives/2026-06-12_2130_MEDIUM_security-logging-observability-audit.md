# Review: 安全层实现 + 单链路日志可观测性审计

> **日期**: 2026-06-12 21:30  
> **审查范围**: 当前 HEAD 工作区（安全中间件、策略引擎、脱敏/工作区、诊断日志、SQLite 持久化）  
> **风险等级**: MEDIUM（综合）  
> **审查者**: Cursor Agent  
> **产品决策（已确认）**: **不采用双链路日志**（诊断 `happyladysaucecli.log` + JSONL `logs/session/*.jsonl`）。会话明细以 **SQLite `context.sqlite`（脱敏/元数据模式）** + **结构化诊断日志** 为唯一观测面。

---

## 变更 / 现状概要

| 模块 | 现状 |
|------|------|
| `internal/middlewares/security/` | 四类 Tool Call 钩子；策略 + 审批 + 超时 + 输出上限 + 双阶段审计 |
| `internal/security/policy/` | 决策矩阵；`SessionGrants` + `GrantKey` |
| `internal/security/` | `OperationRequest`、脱敏、工作区 guard |
| `internal/capability/` | Descriptor / Registry；未注册 → Unknown + Review |
| `internal/logger/` | 单文件 klog 诊断；trace 字段注入 |
| `internal/logger/conversationlog/` | **已实现但未接线**（与产品决策冲突，应清理） |
| `internal/context/tracker/` + SQLite | 回合/消息持久化；`persist_content` 脱敏 |
| `docs/security/architecture.md` | 安全架构文档（与实现大体一致） |
| `AGENTS.md` / `CLAUDE.md` | 仍描述 JSONL 双通道（**文档漂移**） |

**测试状态**（审查时）: `go test ./internal/security/... ./internal/middlewares/security/... ./internal/agents/...` 全部通过。

**实测日志**（`make run V=2`，天气工具调用）: 诊断日志可完整还原 `user_turn → model_call → capability_policy → capability_call → agent_event → persistence` 链路；`logs/session/` 目录不存在（符合「未接线 JSONL」事实）。

---

## 架构与调用链（简图）

```
CLI 启动
  └─ config.Init(Security) → logger.ConfigureDefaultFile()
  └─ newInteractiveRuntime()
       ├─ tools.NewCapabilityRegistry()
       ├─ tools.NewOperationBuilders()
       └─ middlewares.NewChatModelAgentMiddlewares()
            └─ [ExecutionSecurity → Compact → Usage]

每次 Tool Call:
  Wrap*ToolCall
    └─ authorize()
         ├─ operationForTool() → registry + builder + SanitizeText + NormalizePath(path|file)
         ├─ policy.Evaluate()
         └─ Review → grants / lockApproval / approver
    └─ executionContext(全局 CommandTimeoutSeconds)
    └─ endpoint()
    └─ ensureToolOutputWithinLimit / stream budget
    └─ auditDecision(V=1) + auditExecution(V=1)

回合结束:
  session.FinishTurn() → tracker.SetMessages() → SQLite（sanitized | metadata_only）
  logger.Info user_turn_end(V=1)
```

**观测面（单链路）**:

| 层级 | 载体 | 内容 |
|------|------|------|
| 运行期 trace | `happyladysaucecli.log` | phase、token、策略决策 SHA、耗时；**不含** prompt/tool 正文 |
| 会话回放 | `context.sqlite` | 消息列（脱敏或 metadata_only） |
| ~~明细 JSONL~~ | ~~`logs/session/*.jsonl`~~ | **不采用；删除相关代码与文档** |

---

## 发现汇总

| # | 严重度 | 文件:行号 | 类别 | 问题 | 修复建议 |
|---|--------|-----------|------|------|----------|
| 1 | 🟠 HIGH | `pkg/options/security.go:30-31` + `internal/security/policy/engine.go:64-86` | 安全/配置 | `ApprovalDefault` 已通过 CLI/ENV 暴露并校验（仅允许 `"review"`），但 **策略引擎与中间件从未读取**。配置项对用户可见却无运行时效果，易形成虚假安全感。 | **方案 A**：删除 `approval_default` 配置项直至有多模式需求；**方案 B**：在 `Engine.Evaluate` 或 middleware 层读取并影响默认 Review 行为。同步更新 `docs/security/architecture.md` §2。 |
| 2 | 🟠 HIGH | `internal/tools/weather/weather.go:126-135` + `internal/security/policy/engine.go:64-86` | 安全 | `get_weather` 为 `RiskLow + DefaultPolicyAllow`，**出站 HTTP 零审批**。与 agent CLI「外部副作用需确认」的直觉不符；实测日志 `decision=allow reason=default_policy_allow` 证实自动放行。 | 将 weather 改为 `RiskMedium + DefaultPolicyReview`，或在 policy 中对 `OperationKind` 前缀 `network.*` / `Scopes` 含 `network:` 强制 `ActionReview`；单测覆盖。 |
| 3 | 🟠 HIGH | `internal/capability/descriptor.go:80-81` + 全库 | 安全 | `Descriptor.Scopes` 已定义（如 `network:weather`），**无任何 enforcement**。未来 MCP/多工具扩展时 scope 仅为文档字段。 | 在 `operationForTool` 或独立 `scope.Enforcer` 中校验：network scope 只允许 descriptor.Resources 内 URL；file scope 走 WorkspaceGuard。未覆盖 scope 的工具默认 Review。 |
| 4 | 🟡 MEDIUM | `internal/middlewares/security/security.go:387-403` | 安全 | `WorkspaceGuard` 仅在 `OperationResource.Kind ∈ {path,file}` 时生效。当前唯一工具 `get_weather` 使用 `Kind: url`，**工作区配置对用户完全无感**。 | 文件类工具落地前：在 CLI help 标注 workspace roots「仅对 path/file 资源生效」；文件工具 OperationBuilder 必须使用 `path`/`file` kind；补充集成测试。 |
| 5 | 🟡 MEDIUM | `internal/middlewares/security/security.go:467-471` | 安全/运维 | `CommandTimeoutSeconds` 通过 `executionContext()` 应用于 **所有** tool call，而非仅 `command.run`。与 flag 描述「reserved for future command tools」不一致；长任务/短任务无法分策略。 | 仅当 `operation.OperationKind == command.run`（或 builder 标记 `needs_timeout`）时套 `WithTimeout`；其他工具依赖各自 HTTP client timeout（weather 已有 10s）。 |
| 6 | 🟡 MEDIUM | `internal/security/operation.go:74-86` + `internal/security/policy/grants.go:24-31` | 安全/UX | `GrantKey` **不含参数摘要**。用户对 `get_weather` 选 `s=session` 后，所有城市查询均自动放行（session 级授权过粗）。 | 文档与审批 UI 明确提示；可选对 `RiskHigh` / `network.*` 将 args SHA 纳入 GrantKey；或 session grant 仅允许 once。 |
| 7 | 🟡 MEDIUM | `internal/middlewares/security/security.go:428-447` | 可观测性 | `capability_call` 成功日志缺少 `decision`、`approval_scope`、`output_bytes`；与 `capability_policy` 分离，**单看执行行无法还原审批上下文**。 | 在 `auditExecution` 传入并记录 `decision`、`approval_scope`、`output_bytes`（可从 `toolOutputSize` 或 stream budget 读取）；失败路径字段与成功路径对齐。 |
| 8 | 🟡 MEDIUM | `internal/logger/conversationlog/manager.go`（全文件）+ `AGENTS.md:120-128` + `CLAUDE.md:120-128` | 文档/死代码 | **双链路 JSONL 已决定不采用**，但 `conversationlog` 包仍存在，AGENTS/CLAUDE 仍描述 JSONL 会话明细。新成员会误以为 `logs/session/` 会有文件。 | 删除 `internal/logger/conversationlog/`（或移至 `_deprecated` 并加 build tag）；更新 AGENTS.md、CLAUDE.md：观测面 = 诊断 log + SQLite；删除 JSONL 表格行与 `OpenSession` 描述。 |
| 9 | 🟡 MEDIUM | `internal/context/session/service.go:60-63` | 可观测性 | 有 `session_open`，**无 `session_close` / graceful shutdown** 日志。进程被 Ctrl+C 中断时（如实测 `make: 错误 1`）缺少收尾记录。 | 在 `interactiveRuntime.Close()` / `contextSession.Close()` 写 `phase=session_close`；signal handler 路径同样记录。 |
| 10 | 🟡 MEDIUM | `internal/agents/interactive_runtime.go:90-93` + `internal/logger/logging.go:11-12` | 可观测性 | V=1 默认下看不到 `model_call_begin`、`compaction_check`、`agent_event`、`persistence`，**过程态排查依赖 V=2**。安全 audit 在 V=1 可见（✓），但 tool 输出大小仅出现在 V=2 的 `agent_event`。 | 将 `agent_event` 中 `tool_name`+`content_len` 降为 V=1；或把 `output_bytes` 并入 Finding #7 的 `capability_call`（V=1）。 |
| 11 | 🟢 LOW | `internal/security/sanitizer.go:112-118` | 安全 | 敏感 key 匹配子串 `token` 等，可能误伤 `token_count` 等字段；非 JSON 秘密（值在 innocuous key 下）可能漏脱敏进 SQLite。 | 键名改为边界匹配（`_token`/`token_`）；string 值复用 `SanitizeText` 前缀规则（部分已有）。 |
| 12 | 🟢 LOW | `docs/security/architecture.md:491-492` | 文档 | 第十三节末尾有多余 ` ``` ` 闭合符，渲染可能异常。 | 删除 stray fence。 |
| 13 | 🟢 LOW | `internal/tools/tools.go:31-33` | 安全/维护 | Registry 与 `NewAgentTools()` **无启动时一致性断言**；新增 tool 若忘记注册 descriptor，依赖 Unknown→Review（安全网存在但依赖人工）。 | 启动时对比 tools 名集合与 registry keys；测试 `TestRegistryMatchesAgentTools` 已有 builder 覆盖，可扩展为 descriptor 覆盖。 |

---

## 已修复项（相对 `review/archives/` 早期报告，无需重复处理）

以下问题在后续提交中**已解决**，本报告不再列为待办：

| 原报告项 | 当前状态 |
|----------|----------|
| Stream `audited` 竞态 | 已改用 `atomic.Bool`（`security.go:161-167`） |
| `GrantKey` 分隔符转义 | 已实现 `escapeGrantKeyComponent` |
| `truncate()` UTF-8 安全 | 已用 `validUTF8Prefix` |
| `CommandTimeout` / `MaxToolOutput` 未 enforcement | 已在 `executionContext` / `ensureToolOutputWithinLimit` / stream budget 生效 |
| `WorkspaceGuard` 未注入 middleware | 已在 `middleware.go:45-57` 注入 |
| `Evaluate` 忽略 `request.Risk` | 已优先使用 `request.Risk`（`engine.go:72-78`） |
| `OperationKind command.run` 强制 Review | 已实现（`engine.go:79-81`） |

---

## 单链路日志可观测性评测（V=2 实测）

### 已满足

- Trace 关联：`session_id` / `conversation_id` / `user_turn_seq` / `model_call` 贯穿整轮。
- 工具调用：`capability_policy` + `capability_call` 双行，含 `tool_call_id`、`args_summary_sha`、`operation_kind`、`resources`。
- 隐私：用户 prompt 仅 `user_prompt_len`；参数仅 SHA；tool 正文仅 `content_len`（V=2）。
- Token 聚合：`user_turn_end` 与终端 Stats 一致。

### 待改进（不含 JSONL）

1. **明细回放路径写清**：文档统一为「SQLite + 脱敏列」，避免与 JSONL 混淆（Finding #8）。
2. **安全审计字段补全**（Finding #7）。
3. **生命周期收尾**（Finding #9）。
4. **V=1 过程态**（Finding #10）。

---

## 安全层综合评分（审查时）

| 维度 | 评分 | 说明 |
|------|------|------|
| 架构与中间件拦截 | ★★★★☆ | 四层清晰；四钩子齐全 |
| 策略与审批 | ★★★★☆ | 未知工具 Review；Deny 真阻断；审批锁合理 |
| 实际防护面（当前仅 weather） | ★★☆☆☆ | 网络出站自动 Allow；Scopes/Workspace 未 enforcement |
| 单链路可观测性 | ★★★☆☆ | V=2 完整；V=1 偏结果导向；无 session_close |
| 文档与代码一致 | ★★☆☆☆ | JSONL 漂移；ApprovalDefault 漂移 |

---

## 建议修复顺序

1. **P0 — 文档与死代码**：Finding #8（明确单链路；删 conversationlog；改 AGENTS/CLAUDE）。
2. **P1 — 安全策略**：Finding #2、#3（网络 Review + Scopes enforcement）。
3. **P1 — 配置诚实性**：Finding #1（ApprovalDefault 接线或删除）。
4. **P2 — 可观测性**：Finding #7、#9、#10。
5. **P2 — 行为细化**：Finding #4、#5、#6。
6. **P3 —  polish**：Finding #11、#12、#13。

---

## 测试补充建议

| 场景 | 包 |
|------|-----|
| `network.*` OperationKind 强制 Review | `internal/security/policy` |
| Scopes URL allowlist 拒绝越权 URL | `internal/middlewares/security` |
| `capability_call` 含 output_bytes / decision 字段 | `internal/middlewares/security` |
| Registry 与 AgentTools 名称一致 | `internal/tools` |
| 删除 conversationlog 后全量 `make test` | 全局 |

---

审核修复处理完成后, 务必将 review 文档归档至 `review\archives` 目录
