# HappyLadySauceCLI 安全架构文档

## 一、概览

安全系统由 **五层架构** 组成，在每一次工具调用前执行策略评估、人工审批、资源校验、密钥脱敏、命令 sandbox 探测与审计日志记录。

```
┌──────────────────────────────────────────────────────────────┐
│  Layer 1: 配置层                                              │
│  pkg/options/security.go    SecurityOptions + CLI flags      │
│  pkg/config/config.go       全局 Config 聚合                 │
├──────────────────────────────────────────────────────────────┤
│  Layer 2: 领域类型层                                          │
│  internal/capability/       Descriptor, Registry, RiskLevel  │
│  internal/security/         OperationRequest, AuditRecord    │
├──────────────────────────────────────────────────────────────┤
│  Layer 3: 策略引擎层                                          │
│  internal/security/policy/  决策矩阵 + 会话授权缓存           │
├──────────────────────────────────────────────────────────────┤
│  Layer 4: 中间件层                                            │
│  internal/middlewares/security/  ExecutionSecurityMiddleware │
│  internal/agents/approver.go    终端人工审批                  │
├──────────────────────────────────────────────────────────────┤
│  Layer 5: 命令 sandbox 层                                      │
│  internal/execution/sandbox/  WSL2 sandbox runner             │
└──────────────────────────────────────────────────────────────┘
```

### 关键文件清单

| 文件 | 职责 |
|------|------|
| `pkg/options/security.go` | SecurityOptions 结构体、CLI flags、校验逻辑 |
| `internal/capability/descriptor.go` | Capability 类型、RiskLevel、DefaultPolicy 定义 |
| `internal/capability/registry.go` | 线程安全的 Capability 注册表 |
| `internal/security/operation.go` | OperationRequest、AuditRecord、GrantKey 生成 |
| `internal/security/sanitizer.go` | 正则 + JSON 结构化密钥脱敏引擎 |
| `internal/security/authcontext.go` | 已授权 OperationRequest 的 context 传递 |
| `internal/utils/urlscope/` | URL 白名单规范化与比较（跨模块复用） |
| `internal/tools/execguard/` | tool endpoint 与已授权 URL 资源对齐 helper |
| `internal/security/workspace.go` | 路径遍历与符号链接保护 |
| `internal/execution/files/` | 文件读取、列举、精确替换、创建、删除的执行服务 |
| `internal/tools/files/` | 文件工具 schema、descriptor、operation builder 与 endpoint guard 接线 |
| `internal/execution/sandbox/` | command.run 的 WSL2 sandbox runner 抽象与 fake-executor 可测实现 |
| `internal/security/policy/engine.go` | 策略决策矩阵 |
| `internal/security/policy/grants.go` | 会话级审批授权缓存 |
| `internal/middlewares/security/security.go` | 执行安全中间件（核心） |
| `internal/middlewares/middleware.go` | 中间件链组装工厂 |
| `internal/tools/toolresult/toolresult.go` | 工具执行错误 JSON payload 格式化 |
| `internal/agents/approver.go` | 终端人工审批实现 |

---

## 二、配置层

### 2.1 SecurityOptions 结构体

```go
type SecurityOptions struct {
    WorkspaceRoots        []string  // 允许的工作区根目录
    PersistContent        string    // 持久化模式："sanitized" | "metadata_only"
    CommandTimeoutSeconds int       // command.run 执行超时（秒）
    MaxToolOutputBytes    int       // 工具输出最大字节数
    CommandSandbox        CommandSandboxOptions
}

type CommandSandboxOptions struct {
    Backend         string   // "wsl2"
    FailClosed      bool     // sandbox 不可用时拒绝 command.run
    Network         string   // "deny"
    WSLDistribution string   // 可选 WSL2 distribution
    AllowedEnvKeys  []string // 可传入 sandbox 的环境变量名 allowlist
}
```

### 2.2 默认值

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `WorkspaceRoots` | `[当前工作目录]` | 文件操作的允许根目录 |
| `PersistContent` | `"sanitized"` | 持久化内容经过脱敏处理 |
| `CommandTimeoutSeconds` | `30` | `command.run` 默认 30 秒超时 |
| `MaxToolOutputBytes` | `1048576` (1 MiB) | 工具输出上限 |
| `CommandSandbox.Backend` | `"wsl2"` | command.run 只通过 WSL2 sandbox 后端执行 |
| `CommandSandbox.FailClosed` | `true` | sandbox 不可用时拒绝 command.run，不降级到裸机执行 |
| `CommandSandbox.Network` | `"deny"` | command sandbox 默认禁止网络访问 |
| `CommandSandbox.AllowedEnvKeys` | `PATH, HOME, LANG, LC_ALL, TERM` | 命令环境变量 allowlist |

### 2.3 CLI 参数

```
--security-workspace-roots           (string slice)
--security-persist-content           (string, "sanitized" 或 "metadata_only")
--security-command-timeout-seconds   (int)
--security-max-tool-output-bytes     (int)
--security-command-sandbox-backend              (string, 仅 wsl2)
--security-command-sandbox-fail-closed          (bool, 必须 true)
--security-command-sandbox-network              (string, 仅 deny)
--security-command-sandbox-wsl-distribution     (string, optional)
--security-command-sandbox-allowed-env-keys     (string slice)
```

### 2.4 环境变量

```
HAPPLADYSAUCECLI_SECURITY_WORKSPACE_ROOTS
HAPPLADYSAUCECLI_SECURITY_PERSIST_CONTENT
HAPPLADYSAUCECLI_SECURITY_COMMAND_TIMEOUT_SECONDS
HAPPLADYSAUCECLI_SECURITY_MAX_TOOL_OUTPUT_BYTES
HAPPLADYSAUCECLI_SECURITY_COMMAND_SANDBOX_BACKEND
HAPPLADYSAUCECLI_SECURITY_COMMAND_SANDBOX_FAIL_CLOSED
HAPPLADYSAUCECLI_SECURITY_COMMAND_SANDBOX_NETWORK
HAPPLADYSAUCECLI_SECURITY_COMMAND_SANDBOX_WSL_DISTRIBUTION
HAPPLADYSAUCECLI_SECURITY_COMMAND_SANDBOX_ALLOWED_ENV_KEYS
```

配置优先级：`CLI flags > 配置文件 > 环境变量 > 默认值`

---

## 三、Capability 描述符类型系统

### 3.1 Descriptor 结构体

```go
type Descriptor struct {
    Name          string          // 能力名称（工具名）
    Type          CapabilityType  // 来源类型
    Source        string          // 来源标识（"builtin" 或 MCP server 名称）
    Risk          RiskLevel       // 风险等级
    DefaultPolicy DefaultPolicy   // 默认策略
    Scopes        []string        // 资源范围（如 network:weather）
    Resources     []string        // 涉及资源路径
}
```

### 3.2 CapabilityType（能力来源）

| 值 | 含义 |
|----|------|
| `native_tool` | 内置 Go/Eino 工具 |
| `mcp_tool` | MCP Server 暴露的 tool |
| `mcp_resource` | MCP Server 暴露的 resource |
| `mcp_prompt` | MCP Server 暴露的 prompt |
| `skill` | 由 skill 派生的能力 |
| `unknown` | 未注册工具的安全占位类型 |

### 3.3 RiskLevel（风险等级）

| 值 | 含义 |
|----|------|
| `low` | 低风险：显式允许时可直接执行 |
| `medium` | 中风险：声明 Allow 则信任执行，声明 Review 则提示用户 |
| `high` | 高风险：除直接拒绝外总是需要用户确认 |

### 3.4 DefaultPolicy（默认策略）

| 值 | 含义 |
|----|------|
| `allow` | 低/中风险调用默认允许 |
| `review` | 默认需要用户确认 |
| `deny` | 默认拒绝调用 |

### 3.5 Registry（注册表）

线程安全的 `map[name]Descriptor`，支持并发读写。未注册工具通过 `UnknownDescriptor(name)` 返回 `TypeUnknown / RiskHigh / DefaultPolicyReview` 的占位描述符。

### 3.6 Descriptor.Validate 网络注册纪律

`Register()` 调用 `Descriptor.Validate()`，对 HTTP 出站能力强制以下约束，避免新工具疏漏导致绕过 `network.*` 策略分支：

| 规则 | 说明 |
|------|------|
| `network:` scope 非空后缀 | 如 `network:weather`，禁止裸 `network:` |
| scope ↔ Resources | 含 `network:` scope 时必须声明 `Resources` URL 白名单 |
| Resources ↔ scope | 含 `http(s)` Resources 时必须声明 `network:` scope |
| builder 集成测试 | `tools_test.go` 对含 network/HTTP 描述符的工具断言 builder 产出 `network.*` OperationKind 与非空 Resources |

### 3.7 Descriptor.Validate 文件注册纪律

`file:` scope 已从预留字段变成安全信号。Descriptor 只接受以下文件 scope：

| scope | 匹配 OperationKind |
|-------|-------------------|
| `file:read` | `file.read` |
| `file:list` | `file.list` |
| `file:write` | `file.write` |
| `file:delete` | `file.delete` |

文件工具的 `OperationBuilder` 必须产出匹配的 `file.*` OperationKind，并至少产出一个 `Kind == "path"` 或 `Kind == "file"` 的资源。中间件会先用 `WorkspaceGuard.NormalizeResources` 规范化这些资源，再执行 `ValidateFileResources`，scope 与 OperationKind 不匹配会 hard-fail。

### 3.8 Built-in file tools

当前内置文件工具只覆盖 workspace roots 下的文本文件与单层目录操作：

| 工具 | OperationKind | Scope | Risk / DefaultPolicy | 约束 |
|------|---------------|-------|----------------------|------|
| `file_read` | `file.read` | `file:read` | low / allow | UTF-8 文本；`start_line` 默认 1，`max_lines` 默认 200，最大 1000 |
| `file_list` | `file.list` | `file:list` | low / allow | 仅单层目录；`max_entries` 默认 200，最大 1000 |
| `file_edit` | `file.write` | `file:write` | medium / review | `old_text` 必须非空且唯一匹配；原子替换并保留权限 |
| `file_create` | `file.write` | `file:write` | medium / review | 只创建新 UTF-8 文本文件；不覆盖、不自动创建父目录 |
| `file_delete` | `file.delete` | `file:delete` | high / review | 只删除普通文件；不删除目录、不递归、不展开 glob |

写入类 builder 的 `SanitizedArgsSummary` 只能包含 path、字节数与 SHA-256，不包含 `content`、`old_text` 或 `new_text` 正文。文件正文可以作为 `file_read` 工具结果返回给模型，但不得进入 diagnostic log、approval summary 或审计元数据。

---

## 四、操作请求模型

### 4.1 OperationRequest

```go
type OperationRequest struct {
    ToolName             string              // 工具名称
    ToolCallID           string              // 调用 ID
    Capability           capability.Descriptor
    Registered           bool                // 是否已注册
    OperationKind        string              // 操作类型（如 "command.run"）
    Resources            []OperationResource // 涉及的资源
    Risk                 capability.RiskLevel
    SanitizedArgsSummary string              // 脱敏后的参数摘要
}
```

### 4.2 OperationKind 常量

| 常量 | 值 | 说明 |
|------|-----|------|
| `OperationNativeTool` | `native.tool` | 通用内置工具 |
| `OperationFileRead` | `file.read` | 文件读取操作 |
| `OperationFileList` | `file.list` | 文件列表操作 |
| `OperationFileWrite` | `file.write` | 文件写入操作 |
| `OperationFileDelete` | `file.delete` | 文件删除操作 |
| `OperationCommandRun` | `command.run` | 命令执行操作，必须通过 command sandbox 探测 |

### 4.3 OperationBuilder

每个工具可以注册一个 `OperationBuilder` 函数，在中间件拦截时被调用，用于从原始与脱敏工具参数中提取 `OperationKind`、`Resources` 等字段：

```go
type OperationBuildInput struct {
    RawJSON string // 原始 argumentsInJSON
    Summary string // SummarizeArguments(RawJSON)
}

type OperationBuilder func(ctx context.Context, request OperationRequest, input OperationBuildInput) OperationRequest
```

`RawJSON` 供 builder 解析完整参数（避免 240 字符摘要截断）；`Summary` 供审批提示与审计摘要使用。

### 4.4 GrantKey 与 SessionGrantKey

`GrantKey()` 从操作属性生成稳定的单次调用标识符，用于审计与审批提示展示。key 的组成部分包括 capability type、source、name、operation kind、risk 和排序后的资源列表。分隔符 `\`、`|`、`=` 会被转义以防止碰撞攻击。高风险、`command.run` 与 `network.*` 操作还会纳入脱敏参数摘要的 SHA。

`SessionGrantKey()` 用于 session 级授权缓存与审批锁，在 `GrantKey` 基础上省略 `args_sha`（network 操作亦然），使同工具、同 URL 资源的不同参数可共享 session 授权。详见 §6.1 与 §10.3。

---

## 五、策略引擎

### 5.1 决策矩阵（优先级从高到低）

| 优先级 | 条件 | 动作 | 原因 |
|--------|------|------|------|
| 1 | `Registered == false`（未知工具） | `ActionReview` | `descriptor_missing` |
| 2 | `DefaultPolicy == deny` | `ActionDeny` | `default_policy_deny` |
| 3 | `Risk == high` | `ActionReview` | `high_risk` |
| 4 | `OperationKind == "command.run"` | `ActionReview` | `command_run` |
| 5 | `network.*` / `network:` scope，且非（`SourceBuiltin` + `RiskLow` + `DefaultPolicyAllow` + resources 非空） | `ActionReview` | `network_operation` |
| 6 | `DefaultPolicy == review` | `ActionReview` | `default_policy_review` |
| 7 | 其他情况 | `ActionAllow` | `default_policy_allow` |

### 5.2 关键设计决策

**RiskMedium + DefaultPolicyAllow = ActionAllow**（非 network）：中等风险工具如果声明了 Allow 策略，则被信任可直接执行；中等风险工具如果声明了 Review 策略，则会提示用户确认。**中等风险 network 工具一律 Review**，即使声明 `DefaultPolicyAllow`。

### 5.3 接口

```go
type PolicyDecision struct {
    Action Action   // "allow" | "review" | "deny"
    Reason string   // 决策原因
}
```

---

## 六、会话授权

### 6.1 SessionGrants

```go
type SessionGrants struct {
    mu     sync.RWMutex
    grants map[string]struct{}  // key = SessionGrantKey()
}
```

- 线程安全的 `sync.RWMutex` 保护
- 用户选择 session 范围审批后，`SessionGrantKey()` 被存入
- 后续相同 `SessionGrantKey()` 的调用自动跳过审批
- 仅在当前进程生命周期内有效

### 6.2 审批范围

| 范围 | 常量 | 含义 |
|------|------|------|
| 无 | `none` | 无需审批或拒绝 |
| 一次性 | `once` | 仅当前操作有效，下次重新提示 |
| 会话级 | `session` | 当前进程内同 SessionGrantKey 操作自动放行 |

---

## 七、密钥脱敏引擎

### 7.1 正则模式匹配（SanitizeText）

| 模式 | 匹配内容 |
|------|----------|
| Bearer Token | `Bearer <token>` 格式的认证头 |
| Key=Value 密钥 | `api_key`, `auth_token`, `secret`, `password` 等键值对 |
| PEM 私钥 | `-----BEGIN ... PRIVATE KEY-----` 块 |
| 已知前缀 Token | `sk-*`, `ghp_*`, `github_pat_*`, `xox*`, `AKIA*` 等 |

### 7.2 结构化 JSON 脱敏（SanitizeJSON）

- 解析 JSON → 递归遍历 → 脱敏敏感 key → 重新序列化
- 敏感 key 匹配规则：精确匹配或以下划线后缀匹配 `api_key`, `token`, `secret`, `password`, `authorization` 等字段，避免误伤 `token_count` 等统计字段
- JSON 解析失败时回退到文本正则脱敏

### 7.3 参数摘要（SummarizeArguments）

- 最大 240 字符，超出截断并追加 `...`
- 敏感值替换为 `[REDACTED]`
- Map key 按字典序排序以保证确定性
- 数组显示为 `[items=N]`
- UTF-8 安全截断（不在多字节字符中间切断）

### 7.4 使用场景

1. **审批提示**：`SummarizeArguments` 生成脱敏后的参数摘要展示给用户
2. **审计日志**：`NewAuditRecord` 调用 `SanitizeText` + SHA-256 哈希
3. **持久化**：`context/tracker` 在持久化消息内容时调用脱敏

---

## 八、工作区保护

### 8.1 WorkspaceGuard

```go
type WorkspaceGuard struct {
    roots []string  // 规范化的绝对路径列表
}
```

### 8.2 NormalizePath 校验流程

1. `filepath.Abs(filepath.Clean(path))` — 规范化路径
2. `filepath.EvalSymlinks` — 递归解析符号链接（不存在的末尾组件可优雅降级）
3. `filepath.Rel(root, path)` — 检查路径是否在允许根目录内
4. 如果 `Rel` 返回 `".."` 或以 `..` 开头，或返回绝对路径 → **拒绝**（路径逃逸）
5. Windows 下额外做了大小写不敏感的路径相等比较

### 8.3 默认行为

未配置 `WorkspaceRoots` 时，使用当前工作目录作为唯一允许根目录。

---

## 九、执行安全中间件（核心）

### 9.1 中间件结构

```go
type ExecutionSecurityMiddleware struct {
    *adk.BaseChatModelAgentMiddleware
    registry              *capability.Registry
    policy                *policy.Engine
    grants                *policy.SessionGrants
    approver              Approver
    builders              map[string]OperationBuilder
    workspaceGuard        *WorkspaceGuard
    commandTimeoutSeconds int
    maxToolOutputBytes    int
    approvalLocksMu       sync.Mutex
    approvalLocks         map[string]*approvalLockEntry  // 按 SessionGrantKey 的审批锁
}
```

### 9.2 四个工具调用钩子

中间件实现了 Eino ADK 的全部四种工具调用拦截方法：

| 方法 | 保护场景 |
|------|----------|
| `WrapInvokableToolCall` | 标准非流式工具调用 |
| `WrapStreamableToolCall` | 标准流式工具调用 |
| `WrapEnhancedInvokableToolCall` | 增强非流式工具调用 |
| `WrapEnhancedStreamableToolCall` | 增强流式工具调用 |

### 9.3 每次工具调用的处理流程

```
authorize()                          ← 策略评估 + 审批
  ├── descriptorForTool()            ← 在 Capability Registry 中查找
  ├── operationForTool()             ← 构建 OperationRequest
  │   ├── OperationBuilder 补充信息
  │   ├── SanitizeText 脱敏参数
  │   ├── WorkspaceGuard.NormalizeResources 校验路径资源
  │   ├── security.ValidateNetworkResources 校验 network/url allowlist（见下）
  │   └── security.ValidateFileResources 校验 file scope/path 纪律
  ├── commandSandbox.Probe()         ← command.run 必须 sandbox available，否则 fail-closed
  ├── policy.Evaluate()              ← 策略矩阵决策
  │
  ├── ActionAllow → 直接放行
  ├── ActionDeny  → 直接拒绝
  └── ActionReview:
      ├── 检查 SessionGrants 缓存
      ├── 未授权 → lockApproval(grantKey) 串行化
      ├── 二次检查 grants（双重检查锁定）
      ├── approver.Approve() 展示审批提示
      └── 审批通过 + scope=session → grants.Allow()

WithAuthorizedOperation(ctx)         ← 将已授权 OperationRequest 注入 context
executionContext()                   ← 对 command.run 应用超时控制
  └── context.WithTimeout(ctx, timeout)

endpoint() 调用实际工具              ← 执行；endpoint 不得作用于与 AuthorizedOperation.Resources 不一致的目标

ensureToolOutputWithinLimit()        ← 输出大小检验

auditExecution() / auditStreamOpened()  ← 审计日志
```

**软失败矩阵：**

| 错误类型 | 处理 | 回传模型 | ReAct |
|---------|------|---------|-------|
| 用户审批拒绝 | soft-fail JSON，`reason=user_denied` | 是 | 继续 |
| 策略 Deny | soft-fail JSON，`reason=policy_denied` | 是 | 继续 |
| 路径/scope 校验失败 | hard-fail Go error | 否 | 中断 |
| 无 Approver / 审批 I/O 失败 | hard-fail Go error | 否 | 中断 |
| 工具执行/网络/参数/输出超限 | soft-fail JSON | 是 | 继续 |

用户/策略拒绝以 `status=denial_returned recovered=true` 审计；工具 endpoint 执行失败以 `status=tool_error_returned recovered=true` 审计后回传给模型，ReAct 循环可继续。

**ValidateNetworkResources 触发条件**（满足任一即校验 `url` 资源）：

- descriptor `Scopes` 含 `network:` 前缀
- `OperationKind` 以 `network.` 开头
- `Resources` 中存在 `Kind == "url"` 的项

存在 `url` 资源但 descriptor `Resources` 白名单为空时 hard-fail。URL 比较使用 `internal/utils/urlscope` 的 `CanonicalURLForAllowlist`（scheme/host 小写、剥离默认端口、规范化 path、拒绝 userinfo、剥离 fragment）。

**ValidateFileResources 触发条件**（满足任一即校验文件 scope/path 纪律）：

- descriptor `Scopes` 含受支持的 `file:` scope
- `OperationKind` 以 `file.` 开头
- `Resources` 中存在 `Kind == "path"` 或 `Kind == "file"` 的项

文件操作必须满足：`file.*` OperationKind 与精确 `file:` scope 匹配，且 builder 产出 path/file resource。存在 `file:` scope 但 OperationKind 不是 `file.*`，或存在 path/file resource 但 OperationKind 不是 `file.*`，都会 hard-fail。

**Endpoint TOCTOU 约束**：授权基于 builder 产出的 `OperationRequest`；endpoint 应通过 `security.AuthorizedOperationFromContext(ctx)` 读取已授权资源，并使用 `execguard.MatchAuthorizedURL` / `execguard.RequireAuthorizedPath` 验证实际执行目标与授权一致，不得从 raw JSON 解析出不同的 path/URL 后静默执行。内置文件工具 endpoint 与 security middleware 必须共享 runtime setup 创建的同一个 `WorkspaceGuard`。

### 9.4 Command sandbox

`command.run` 不只依赖审批。中间件会先探测 `internal/execution/sandbox.Runner`，只有 WSL2 sandbox backend 返回 available 时才继续进入 policy/approval/endpoint 链路。sandbox unavailable 时返回 `policy_denied` soft-fail payload，`decision_reason=command_sandbox_unavailable`，endpoint 不会被调用。

当前 backend 只支持 `wsl2`：宿主侧通过 `wsl.exe` 进入可选 distribution，Linux 侧要求 `bwrap` 可用；执行时使用 `bwrap --unshare-net --clearenv`，默认禁止网络，workspace roots 只读绑定，`/tmp` 可写。原生 Windows 裸机执行不是 fallback。

### 9.5 流式输出的特殊处理

对于流式工具调用，审计时机被推迟到 **流 EOF 消费时** 而非流建立时，以获得准确的执行耗时。使用 `toolOutputBudget` 累加器跟踪流式输出的累计字节数，超出限制时写入 error payload chunk 并结束流，而不是以 Go error 中断 ToolNode。

若 consumer 在未 Recv 的情况下直接 `Close()`，`proxyStreamReaderWithFinalize` 会在转发 goroutine 退出时触发 `capability_call` 完成审计（`doAudit` 通过 atomic 保证至多一次）。

### 9.6 审批锁机制

`approvalLocks` 提供按 `SessionGrantKey()` 的并发审批串行化：

- 多个 goroutine 对同一 `SessionGrantKey()` 的工具调用会阻塞在同一把 `sync.Mutex` 上
- 只有一个审批对话框展示给用户
- 不同工具可以并发审批
- 使用引用计数 (`refs`) 进行安全回收

---

## 十、人工审批

### 10.1 Approver 接口

```go
type Approver interface {
    Approve(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error)
}
```

### 10.2 终端实现

`terminalApprover` 是当前的唯一实现，通过 stderr 展示审批提示：

```
Approve capability <tool> (operation=<kind> risk=<level> reason=<reason> resources=<...> args_sha=<sha> args_len=<n>)? [y=once/s=session/N]:
```

用户响应：
- `y` / `yes` — 审批一次（scope = once）
- `s` / `session` — 会话级审批（scope = session）
- 其他 — 拒绝

审批过程使用 `sync.Mutex` 保证与主输入流的互斥。

### 10.3 Session 授权 key

用户选择 `s`（session）后，授权写入 [`SessionGrants`](../../internal/security/policy/grants.go)，查询时使用 `OperationRequest.SessionGrantKey()`（不是单次调用的完整 `GrantKey()`）。

| 操作类型 | SessionGrantKey 是否含 `args_sha` | 含义 |
|----------|-----------------------------------|------|
| `network.*`（如 `get_weather`） | 否 | 同工具 + 同 URL 资源在本进程内免审，不同 city/lang 参数共享 session 授权 |
| `command.run` / `RiskHigh` | 是 | 不同命令参数或高风险参数不自动继承 session 授权 |
| 其他 | 否 | 按工具 + operation_kind + resources 复用 |

并发审批锁 [`lockApproval`](../../internal/middlewares/security/security.go) 同样基于 `SessionGrantKey()`，避免同工具不同参数并发重复弹窗。

单次 `GrantKey()` 仍含 `args_sha`（network 操作亦然），用于审计与审批提示展示，但不参与 session 存储。

---

## 十一、中间件链组装

### 11.1 注册顺序

中间件链按以下顺序注册到 Eino ADK Agent：

```
[ExecutionSecurityMiddleware, CompactMiddleware, UsageMiddleware]
```

- **Security** 排第一：在工具调用前最先拦截，确保所有工具调用都经过安全校验
- **Compact** 排第二：在模型调用前执行上下文压缩
- **Usage** 排第三：跟踪 token 用量

### 11.2 组装工厂

```go
func NewChatModelAgentMiddlewares(cfg ChatModelAgentMiddlewareConfig) ([]adk.ChatModelAgentMiddleware, error)
```

该工厂函数创建完整的中间件链，包括：
1. 接收 runtime setup 创建的共享 `WorkspaceGuard` 与 `CommandSandbox`（测试路径可由 factory fallback 创建）
2. 创建 `ExecutionSecurityMiddleware`（包含 Policy Engine + Session Grants + Approver + command sandbox probe）
3. 创建 `CompactMiddleware`
4. 创建 `UsageMiddleware`

---

## 十二、完整请求生命周期

```
1. CLI 启动
   ├── 加载 settings.json 到 Viper
   ├── 绑定环境变量 (HAPPLADYSAUCECLI_SECURITY_*)
   ├── 解析 CLI flags (--security-*)
   ├── Viper 反序列化到 SecurityOptions
   └── Validate() 检查默认值与约束

2. Agent 初始化 (interactive_setup.go)
   ├── tools.NewCapabilityRegistry()   创建 Capability 注册表
   ├── tools.NewOperationBuilders()    创建 OperationBuilder 映射
   ├── security.NewWorkspaceGuard()     创建共享 workspace guard
   ├── tools.NewAgentTools(guard)       创建 weather + file tools
   ├── sandbox.NewRunner()              创建 command sandbox runner
   ├── newTerminalApprover()           创建终端审批器
   └── NewChatModelAgentMiddlewares()  组装中间件链

3. 工具调用发生（LLM 指示调用工具）
   ├── 中间件拦截
   │   ├── authorize()                    策略评估 + 审批
   │   │   ├── descriptorForTool()        注册表查找
   │   │   ├── operationForTool()         构建操作请求
   │   │   ├── policy.Evaluate()          决策矩阵
   │   │   ├── (ActionAllow)              放行
   │   │   ├── (ActionDeny)               拒绝
   │   │   └── (ActionReview)             人工审批
   │   ├── executionContext()             超时控制
   │   ├── endpoint()                     实际工具调用
   │   ├── ensureToolOutputWithinLimit()  输出大小检查
   │   └── auditExecution()               审计记录
   └── 结果返回给 LLM

4. 持久化 (context/tracker/tracker.go)
   ├── PersistContentSanitized    → SanitizeText + SanitizeJSON
   └── PersistContentMetadataOnly → 仅保存元数据，不保存消息正文
```

---

## 十三、审计日志

### 13.1 AuditRecord 结构

```go
type AuditRecord struct {
    ToolName       string  // 工具名
    ToolCallID     string  // 调用 ID
    OperationKind  string  // 操作类型
    Resources      string  // 涉及的资源（摘要）
    ArgsSummary    string  // 脱敏后的参数摘要
    ArgsSummarySHA string  // 参数摘要的 SHA-256 哈希
    Risk           string  // 风险等级
    Decision       string  // 策略决策（allow/review/deny）
    DecisionReason string  // 决策原因
    ApprovalScope  string  // 审批范围（none/once/session）
    Status         string  // 状态（allowed/approved/denied）
    ElapsedMS      int64   // 执行耗时（毫秒）
}
```

### 13.2 审计时间线

每次工具调用产生两条审计日志：

1. **决策时**（`phase=capability_policy`）：记录策略评估结果、是否触发了审批、sandbox backend/status/reason
2. **执行后**（`phase=capability_call`）：记录策略决策、审批范围、输出字节数、执行耗时、成功/失败状态、sandbox backend/status/reason

流式工具调用额外产生 `status=stream_opened` 的中间态日志。

`sandbox_reason` 会先经过 `SanitizeText`；日志不会记录 raw command、raw file content 或完整环境变量。

---

## 十四、持久化内容安全

会话内容持久化支持两种模式：

| 模式 | 行为 |
|------|------|
| `sanitized` | 消息内容经过 `SanitizeText`（文本）和 `SanitizeJSON`（JSON）脱敏后保存 |
| `metadata_only` | 仅保存消息元数据（role、timestamp 等），不保存正文内容 |

脱敏在 `context/tracker/tracker.go` 的 `messageRecordFromSchema()` 中执行。

---

## 十五、测试覆盖

安全系统在 7 个测试文件中覆盖了以下场景：

| 测试文件 | 覆盖场景 |
|----------|----------|
| `security_test.go` (middleware) | Allow/Deny/Review 策略、Session Grant 缓存、并发审批串行化、超大输出拒绝、路径逃逸拒绝、命令超时、未知工具审批、参数脱敏、审批锁回收、流 EOF 审计 |
| `policy_test.go` | 低风险放行、高风险强制审批、命令执行审批、未注册工具审批、操作级风险覆盖、Session Grant key 作用域 |
| `operation_test.go` | GrantKey 中的资源排序、分隔符转义 |
| `sanitizer_test.go` | Bearer token、嵌套 JSON 密钥、已知前缀 token、确定性 key 排序、UTF-8 安全截断 |
| `workspace_test.go` | 路径遍历、符号链接逃逸、不存在文件的符号链接父路径 |
| `security_test.go` (options) | 默认值应用、无效持久化模式、非法 command sandbox backend/network/env key 拒绝 |
| `approver_test.go` | Yes/once 审批、session 审批、默认拒绝、提示渲染 |
| `middleware_test.go` | 链注册顺序（security, compact, usage = 3 handlers）、无效工作区根目录拒绝 |
| `wsl2_test.go` | WSL2 sandbox probe、fake executor、非零退出码、超时与输出截断 |
