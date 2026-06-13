# 内置文件工具集成安全审计

> **日期**: 2026-06-13 15:10  
> **范围**: `c95b1a3`（`Enhance Eino tools integration with built-in file operations and update documentation`）  
> **审查者**: Cursor Agent  
> **测试**: `make test` 全绿（30 packages）

---

## 总览

本次按 `review/prompt/REVIEW_PROMPT.md` 四阶段流程，对提交 `c95b1a3` 的 12 个文件（+1526/-25 行）做了独立审计。变更核心为：**5 个内置文件工具**（`file_read/list/edit/create/delete`）、**execution/files 执行层**、**runtime 共享 WorkspaceGuard 接线**、**文档与单测补齐**。

| 状态 | 数量 |
|------|------|
| 🔴 CRITICAL | 0 |
| 🟠 HIGH | 1 |
| 🟡 MEDIUM | 5 |
| 🟢 LOW | 3 |

**综合评分：约 4.0 / 5** — 授权链、scope 纪律、endpoint 二次校验与审计脱敏设计正确，测试覆盖较好；执行层缺少文件大小/行宽/超时边界，且存在 Lstat→Open/ReadFile 的 symlink TOCTOU 窗口，需在对外暴露前加固。

---

## 第一阶段：架构与调用链

```text
main (cmd/root.go)
  └─ app.run → agents.RunLoop
       └─ newInteractiveRuntime (interactive_setup.go)
            ├─ newRuntimeWorkspaceGuard(cfg.Security)     ← 共享 WorkspaceGuard
            ├─ tools.NewAgentTools(workspaceGuard)        ← 新增：注入 guard 创建 5 个文件工具
            ├─ tools.NewCapabilityRegistry / NewOperationBuilders
            ├─ middlewares.NewChatModelAgentMiddlewares
            │    └─ ExecutionSecurityMiddleware
            │         WrapInvokableToolCall → authorize → endpoint
            │           operationForTool
            │             ├─ filetools.OperationBuilder  ← 解析 path/脱敏摘要
            │             ├─ WorkspaceGuard.NormalizeResources
            │             ├─ ValidateNetworkResources
            │             └─ ValidateFileResources
            │           policy.Evaluate → approval → audit
            │           WithAuthorizedOperation(ctx)
            │           executionContext (仅 command.run 超时)
            │           endpoint (InferTool handler)
            └─ adk.NewChatModelAgent → REPL loop
```

**文件工具 endpoint 子链：**

```text
files.toolSet.{read|list|edit|create|delete}
  └─ execguard.RequireAuthorizedPath(ctx, guard, req.Path)   ← TOCTOU 二次校验
       └─ execfiles.Service.{ReadText|ListDirectory|EditText|CreateText|DeleteFile}
            └─ os.* I/O（Lstat / Open / ReadFile / Write / Remove）
```

**数据流向与错误传播：**

| 阶段 | 输入 | 输出 / 副作用 | 失败模式 |
|------|------|----------------|----------|
| OperationBuilder | RawJSON | OperationKind + path/file Resources + 脱敏 Summary | 空 path → Resources 为空 |
| NormalizeResources | path/file Resources | 规范化绝对路径 | 路径逃逸 → **hard-fail** |
| ValidateFileResources | Kind + Scopes + Resources | scope 三角一致 | 不匹配 → **hard-fail** |
| policy.Evaluate | OperationRequest | allow/review/deny | write/delete → review |
| RequireAuthorizedPath | ctx + actualPath | 规范化路径 | 与授权资源不一致 → **hard-fail** |
| Service I/O | 已授权路径 | 文件内容/元数据 | 工具错误 → **soft-fail**（middleware 包装 JSON） |
| ensureToolOutputWithinLimit | result JSON 大小 | 通过/拒绝 | 超限 → soft-fail |

---

## 第二阶段 / 第三阶段：跨层结论

| 检查项 | 结论 |
|--------|------|
| 中间件链顺序 | Security → Compact → Usage ✅ |
| 共享 WorkspaceGuard | runtime 注入 tools + middleware，生产路径一致 ✅ |
| scope 纪律 | Descriptor + ValidateFileResources + OperationBuilder 三角一致 ✅ |
| endpoint TOCTOU（路径） | RequireAuthorizedPath 二次校验 authorized resources ✅ |
| endpoint TOCTOU（inode/ symlink） | Lstat 校验后 Open/ReadFile **跟随 symlink**，存在竞态窗口 ⚠️ |
| 审计/审批脱敏 | write builder 仅 path/bytes/sha256，不含正文 ✅ |
| 文件操作超时 | executionContext **仅** command.run 套超时，file.* 无 deadline ⚠️ |
| 文件大小边界 | Read/Edit **无 max bytes**；Read 可扫完整文件计 totalLines ⚠️ |
| 输出预算 | middleware 事后检查 max_tool_output_bytes，endpoint 仍可能先分配大内存 ⚠️ |
| 测试 | service 单测、endpoint 授权、builder 脱敏、middleware file scope 不匹配 ✅ |
| middleware 端到端 | 缺少真实 `file_read` 工具经 WrapInvokable 全链路集成测 ⚠️ |

---

## 发现项

| # | 严重度 | 文件:行号 | 类别 | 问题 | 修复建议 |
|---|--------|-----------|------|------|----------|
| 1 | 🟠 HIGH | `internal/execution/files/service.go:369-377`、`236-243` | 安全 | `statRegularFile` 使用 `Lstat` 确认「普通文件」后，`os.Open` / `os.ReadFile` **会跟随 symlink**。在 authorize 与 I/O 之间的窄窗口内，若 workspace 内文件被替换为指向 workspace 外的 symlink，可能读取授权路径字符串对应位置之外的文件内容。 | 打开后二次校验 fd：`fstat(fd)` 与打开前 `Lstat` 比较 dev/inode；Unix 使用 `O_NOFOLLOW`；Windows 使用 `FILE_FLAG_OPEN_REPARSE_POINT` 并拒绝 reparse point。补充竞态单测（goroutine 轮换 symlink）。 |
| 2 | 🟡 MEDIUM | `internal/execution/files/service.go:133-178`、`226-261` | 稳定性/性能 | `ReadText` 为计算 `total_lines` **扫描整个文件**；`EditText` 用 `os.ReadFile` **无大小上限** 加载全文。workspace 内 GB 级文件可导致长时间阻塞或 OOM。 | 增加 `MaxFileBytes`（如 8–16 MiB，可配置）；Read 超限时返回明确错误；Edit 在 read 前 `Stat` 检查 size；大文件考虑流式 unique-match 或拒绝 edit。 |
| 3 | 🟡 MEDIUM | `internal/middlewares/security/security.go:630-635` | 稳定性 | `executionContext` 仅对 `command.run` 施加 `CommandTimeoutSeconds`；`file_read/list/edit` 等 I/O **无执行 deadline**，慢盘/大目录列举可阻塞 REPL 与 model 调用链。 | 为 `file.*` OperationKind 复用或单独配置 I/O 超时（如 30s），传入 Service 并在循环中检查 `ctx.Err()`（Read/List 已部分检查，Edit 仅入口检查一次）。 |
| 4 | 🟡 MEDIUM | `internal/execution/files/service.go:151-159` | 性能/稳定性 | `ReadText` 按行读取但**不限制单行字节数**；1000 行 × 超大单行可在 middleware 输出限制生效前耗尽内存。 | 增加 `MaxLineBytes`（如 64 KiB），超长行截断或 hard-fail；与 `max_tool_output_bytes` 联动估算。 |
| 5 | 🟡 MEDIUM | `internal/middlewares/security/security.go:144-149` + `service.go:171-177` | 性能 | middleware 在 endpoint **返回完整 JSON 后**才调用 `ensureToolOutputWithinLimit`；`file_read` 可先构建超大 `content` 字符串再被拒绝，内存峰值仍高。 | Service 层预预算（累计 bytes/lines）；或 middleware 对流式 tool 做增量 budget（已有 stream budget 模式可复用到 invokable 大结果）。 |
| 6 | 🟡 MEDIUM | `internal/tools/files/files.go:253-317` | 健壮性 | 全部 OperationBuilder 对 `json.Unmarshal` **丢弃错误**（`_ = json.Unmarshal(...)`）。畸形 JSON 导致空 path → authorize hard-fail，模型收到 invariant 错误而非可恢复的参数错误。 | 检查 unmarshal err；空 path 时设置明确 `SanitizedArgsSummary`；或在 builder 层返回 recoverable validation error（若 middleware 支持）。 |
| 7 | 🟢 LOW | `internal/middlewares/security/security_test.go` | 测试 | 已有通用 `file_tool` scope 不匹配与路径逃逸测试，但**缺少**注册真实 `file_read`/`file_edit` descriptor + builder + InferTool endpoint 的 middleware 集成测（authorize → approval mock → endpoint）。 | 增加 `TestWrapInvokableToolCallRunsFileReadWithinWorkspace` / 越界 path soft/hard-fail 用例，使用 `files.OperationBuilders()` 与真实 guard。 |
| 8 | 🟢 LOW | `internal/execution/files/service.go:391-402` | 安全/UX | `file_list` 返回 symlink 条目的 `path`，但后续 `file_read/edit` 对 symlink 拒绝（`Lstat` 非 regular）。模型可能反复失败。 | 文档/工具描述注明；或在 list 结果增加 `readable: false` 字段标识 symlink/目录。 |
| 9 | 🟢 LOW | `internal/tools/files/files.go:74-81` | 可维护性 | `NewTools` 每次创建独立 `execfiles.NewService()`，无状态虽正确，但与 execution 层「可注入 Service 便于测试/限流」模式不一致。 | 可选 `NewTools(guard, service)` 或 functional option，便于注入带 size limit 的 mock service。 |

---

## 已验证亮点

- **共享 guard 接线**：`interactive_setup.go` 同一 `workspaceGuard` 注入 `NewAgentTools` 与 middleware，避免 roots 分裂 ✅
- **endpoint 路径对齐**：五个 handler 均经 `execguard.RequireAuthorizedPath`，越权 path 在 endpoint 层 hard-fail ✅
- **scope / kind / resource 一致**：`file_list` 用 `ResourceKindPath`，读写删用 `ResourceKindFile`，与 `ValidateFileResources` 匹配 ✅
- **写入审计脱敏**：edit/create builder 摘要仅含 path、字节数、SHA-256，单测验证不含 `secret` 明文 ✅
- **原子写与权限保留**：`writeFileAtomically` + `EditText` 保留原 mode；`CreateText` 使用 `O_EXCL` 防覆盖 ✅
- **UTF-8 边界**：读/写/编辑均校验 UTF-8，拒绝二进制误操作 ✅
- **策略分级**：read/list allow；edit/create review+medium；delete review+high ✅
- **文档同步**：`architecture.md` §3.8 工具矩阵与约束与实现一致 ✅

---

## 修复优先级建议

1. **P0（对外启用 file 工具前）**：#1 symlink TOCTOU 加固（fd 二次校验 / O_NOFOLLOW）
2. **P1**：#2 文件大小上限、#3 file I/O 超时、#4 单行上限
3. **P2**：#5 输出预预算、#6 builder JSON 错误处理、#7–#9 LOW 项

---

审核修复处理完成后, 务必将review文档归档至review\archives目录
