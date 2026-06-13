# Command Sandbox 与文件校验安全审计

> **日期**: 2026-06-13 14:44  
> **范围**: `2bc9441`（`Enhance security middleware with file operation validation and command sandbox integration`）  
> **审查者**: Cursor Agent  
> **测试**: `make test` 全绿（30 packages）

---

## 总览

本次按 `review/prompt/REVIEW_PROMPT.md` 四阶段流程，对提交 `2bc9441` 的 20 个文件（+1321/-43 行）做了独立审计。变更核心为：**文件操作 scope/path 纪律**、**WSL2+bwrap 命令 sandbox 基础设施**、**中间件 authorize 链路集成 Probe**。

| 状态 | 数量 |
|------|------|
| 🔴 CRITICAL | 0 |
| 🟠 HIGH | 2 |
| 🟡 MEDIUM | 5 |
| 🟢 LOW | 4 |

**综合评分：约 3.8 / 5** — 文件校验与 fail-closed 探测方向正确，测试覆盖较好；但在 **command.run 真正落地前**，必须补齐 sandbox 执行强制与 Probe 超时，否则存在架构绕过与 REPL 阻塞风险。

---

## 第一阶段：架构与调用链

```text
main (cmd/root.go)
  └─ app.run → agents.RunLoop
       └─ newInteractiveRuntime (interactive_setup.go)
            ├─ tools.NewCapabilityRegistry / NewOperationBuilders / NewAgentTools
            ├─ newRuntimeWorkspaceGuard(cfg.Security)     ← 共享 WorkspaceGuard
            ├─ newRuntimeCommandSandbox(cfg.Security)     ← 共享 WSL2 Runner
            ├─ newTerminalApprover
            ├─ middlewares.NewChatModelAgentMiddlewares
            │    ├─ ExecutionSecurityMiddleware
            │    │    Wrap*ToolCall → authorize → endpoint
            │    │      operationForTool
            │    │        ├─ OperationBuilder 补充 OperationKind/Resources
            │    │        ├─ WorkspaceGuard.NormalizeResources
            │    │        ├─ ValidateNetworkResources
            │    │        └─ ValidateFileResources          ← 新增
            │    │      commandSandboxStatus → Probe()      ← 新增（仅 command.run）
            │    │      policy.Evaluate → approval
            │    │      WithAuthorizedOperation(ctx)
            │    │      executionContext (command.run 超时)
            │    │      endpoint → audit
            │    ├─ CompactMiddleware
            │    └─ UsageMiddleware
            └─ adk.NewChatModelAgent → REPL loop
```

**Layer 5（新增）命令 sandbox：**

```text
sandbox.NewRunner(Config)
  └─ WSL2Runner
       Probe(ctx)  → wsl.exe … sh -lc "command -v bwrap"
       Run(ctx, Request)
         ├─ Probe(ctx) 再次
         ├─ resolveWorkDir → windowsPathToWSL
         ├─ bwrapArgs (--unshare-net --clearenv --ro-bind workspace …)
         └─ osExecutor.Execute → exec.CommandContext(wsl.exe, …)
```

**数据流向与错误传播：**

| 阶段 | 输入 | 输出 / 副作用 | 失败模式 |
|------|------|----------------|----------|
| NormalizeResources | path/file Resources | 规范化绝对路径 | 路径逃逸 → **hard-fail** |
| ValidateFileResources | OperationKind + Scopes + Resources | scope 纪律校验 | 不匹配 → **hard-fail** |
| commandSandbox.Probe | ctx（无独立超时） | Status.Available | 不可用 → **soft-fail** policy_denied |
| policy.Evaluate | OperationRequest | allow/review/deny | deny → soft-fail |
| endpoint | ctx + authorized operation | tool result | 工具错误 → soft-fail |
| sandbox.Run（未来 endpoint） | Request.Command/Args/Env | bounded stdout/stderr | 尚未接入 middleware ctx |

---

## 第二阶段 / 第三阶段：跨层结论

| 检查项 | 结论 |
|--------|------|
| 中间件链顺序 | Security → Compact → Usage ✅ |
| 文件 scope 纪律 | `ValidateFileResources` + `Descriptor.Validate` 双重约束 ✅ |
| 路径 TOCTOU | `execguard.RequireAuthorizedPath` 供 endpoint 二次校验 ✅ |
| command.run 策略 | policy engine 强制 Review（`engine.go:79-81`）✅ |
| sandbox 不可用 | middleware 在 policy 前 deny，endpoint 不被调用 ✅ |
| sandbox 实际执行 | **Runner 仅 Probe，未注入 ctx，endpoint 无法被强制调用 sandbox.Run** ⚠️ |
| Probe 可靠性 | **无 Probe 专用超时；Run 内重复 Probe；无缓存** ⚠️ |
| FailClosed 配置 | options 层强制 true，但 **WSL2Runner 运行时未读取该字段** ⚠️ |
| 测试 | sandbox fake-executor、file/network 校验、sandbox deny 单测 ✅ |
| 当前生产工具 | 仅 weather（network）；command.run / file.* 工具尚未注册 ✅（风险未触发） |

---

## 发现项

| # | 严重度 | 文件:行号 | 类别 | 问题 | 修复建议 |
|---|--------|-----------|------|------|----------|
| 1 | 🟠 HIGH | `internal/middlewares/security/security.go:138-142`、`324-335` | 安全/架构 | 中间件只对 `command.run` 做 **Probe**，未将 `CommandSandbox` 注入 `ctx`，也未提供统一 execution gateway。未来 `command.run` endpoint 可直接 `exec.Command("cmd.exe")` 绕过 WSL2+bwrap，Probe 通过≠实际隔离。 | 新增 `security.WithCommandSandbox(ctx, runner)`（或 `internal/execution` 包统一入口），endpoint **必须**经该 API 执行；在 middleware 层禁止裸 `exec` 的静态/集成测试。 |
| 2 | 🟠 HIGH | `internal/execution/sandbox/wsl2.go:58-76`、`internal/middlewares/security/security.go:399-406` | 稳定性/安全 | `Probe()` 通过 `wsl.exe` 同步探测，**无独立 deadline**；authorize 使用的 `ctx` 通常无超时。WSL 未安装、distro 启动慢或 `wsl.exe` 挂起时，会阻塞整个 tool 调用链（含审批 UI 之前）。 | 为 Probe 增加固定短超时（如 3–5s）：`context.WithTimeout(ctx, probeTimeout)`；失败返回 `Available=false` 并记录 `sandbox_reason=probe_timeout`。 |
| 3 | 🟡 MEDIUM | `internal/execution/sandbox/wsl2.go:81-86` | 性能 | `Run()` 在每次执行前再次调用 `Probe()`，而 middleware authorize 已对同一 `command.run` 调用过一次 Probe，**双倍 wsl.exe 开销**。 | 将 Probe 结果短期缓存（TTL 或 per-runner mutex + timestamp）；或 Run 接受 `skipProbe bool` / 由调用方传入已验证 Status。 |
| 4 | 🟡 MEDIUM | `internal/security/denial.go:50-60`、`internal/middlewares/security/security.go:325-334` | 可观测性 | sandbox 不可用时使用 `decision_reason=command_sandbox_unavailable` 写审计，但返回模型的 JSON payload 仅含 `reason=policy_denied`，**无法区分** sandbox 故障与其他策略拒绝。 | 扩展 structured denial：如 `command_sandbox_unavailable`，或在 `FormatFailure` 中附带 `decision_reason` 字段（需更新 `toolresult` schema 与单测）。 |
| 5 | 🟡 MEDIUM | `internal/execution/sandbox/sandbox.go:29-34`、`wsl2.go:42-44` | 配置/可维护性 | `Config.FailClosed` 在 `NormalizeConfig` 与 options.Validate 中强制为 true，但 **WSL2Runner 从未读取**；实际 fail-closed 逻辑硬编码在中间件。配置项对运行时行为无影响，易误导运维。 | 删除 runner 层冗余字段，或在 `Probe/Run` 不可用路径显式检查 `cfg.FailClosed` 并与 middleware 行为对齐；文档注明「fail-closed 由 middleware 强制执行」。 |
| 6 | 🟡 MEDIUM | `internal/tools/execguard/executionmatch.go:50-62` | 安全/设计 | `MatchAuthorizedPath` 仅做 **精确路径匹配**，不支持目录前缀。未来 `file.list` 若授权目录 `/proj` 而 endpoint 列出 `/proj/sub`，会被误拒；若 builder 只授权子路径而 endpoint 读父目录，则可能漏检。 | 为 list/read 类操作增加 `MatchAuthorizedPathPrefix` 或按 OperationKind 分支；单测覆盖目录与子路径场景。 |
| 7 | 🟡 MEDIUM | `internal/middlewares/security/security_test.go` | 测试 | 已有 `ValidateFileResources` 单元测试与路径逃逸测试，但 **缺少 middleware 级**「file scope 与 OperationKind 不匹配 → hard-fail」集成测试（类似 network 越界测试）。 | 增加 `TestWrapInvokableToolCallRejectsFileScopeMismatch`：descriptor 含 `file:write`、builder 产出 `file.read` → 期望 hard-fail 且 endpoint 未调用。 |
| 8 | 🟢 LOW | `pkg/options/security.go:165`、`internal/execution/sandbox/wsl2.go:126-133` | 安全 | `WSLDistribution` 仅 `TrimSpace`，未校验 distribution 名称字符集；异常值可能干扰 `wsl.exe -d` 参数解析。 | 增加 `^[A-Za-z0-9._-]+$` 校验（或 WSL 官方命名约束）；非法值在 `Validate()` 拒绝。 |
| 9 | 🟢 LOW | `internal/execution/sandbox/wsl2.go:149-151` | 安全/设计 | bwrap 只读绑定 `/bin`、`/usr`、`/lib`、`/etc` 等系统路径；sandbox 内进程仍可读取 WSL 系统文件（如 `/etc/passwd`）。属 bubblewrap 常见权衡，但应在 architecture 文档明确「非完全空文件系统」。 | 文档 §9.4 补充系统路径 exposure 说明；若需更强隔离，评估 `--ro-bind` 最小化或自定义 rootfs。 |
| 10 | 🟢 LOW | `internal/agents/interactive_setup.go:60-79`、`internal/middlewares/middleware.go:48-55` | 可维护性 | runtime 与 middleware factory **各创建一套** WorkspaceGuard + CommandSandbox，再通过 config 注入共享实例。当前正确，但 factory fallback 路径（测试未注入时）会创建第二套实例，与 runtime roots 可能不一致。 | 测试/文档强调生产路径必须注入共享实例；factory fallback 仅用于单测或加注释 `// test-only fallback`。 |
| 11 | 🟢 LOW | `internal/execution/sandbox/wsl2.go:176-182` | 安全 | 允许的环境变量值无长度上限，极端值可能撑大 `wsl.exe` 命令行。 | 对 `--setenv` value 设合理 max len（如 4KiB），超限拒绝或截断并审计。 |

---

## 已验证亮点

- **文件 scope 纪律**：`ValidateFileResources` 强制 `file.*` OperationKind ↔ `file:*` scope ↔ path/file resource 三角一致 ✅
- **Descriptor 注册**：`validFileScope` 拒绝 `file:rename` 等非法 scope ✅
- **路径规范化链**：authorize 前 `NormalizeResources`，endpoint 可用 `execguard.RequireAuthorizedPath` 二次校验 ✅
- **command.run fail-closed**：sandbox 不可用时 soft-fail，endpoint 不被调用（`TestWrapInvokableToolCallDeniesCommandWhenSandboxUnavailable`）✅
- **WSL2 sandbox 单测**：fake executor 验证 bwrap 参数、`--unshare-net`、env allowlist、超时与输出截断 ✅
- **配置/env 绑定**：`HAPPLADYSAUCECLI_SECURITY_COMMAND_SANDBOX_*` 完整注册 ✅
- **command.run 超时**：`executionContext` 仅对 `OperationCommandRun` 套 `WithTimeout` ✅

---

## 修复优先级建议

1. **P0（落地 command.run 前必须）**：#1 sandbox 执行强制路径、#2 Probe 超时  
2. **P1**：#3 重复 Probe、#4 structured denial reason、#7 middleware file scope 集成测试  
3. **P2**：#5 FailClosed 死配置、#6 目录路径匹配、#8–#11 LOW 项  

---

审核修复处理完成后, 务必将review文档归档至review\archives目录
