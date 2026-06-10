# compact 包

Hermes 风格的语义上下文压缩：当 prompt token 接近模型窗口上限时，将对话历史切分为 **head / middle / tail**，对 middle 段调用辅助模型生成结构化摘要，再组装为 `[system, head, summary, tail]` 供后续模型调用使用。

上层集成：`internal/middlewares/content/content.go` 在每次 `BeforeModelRewriteState` 钩子中调用 `Compactor.CompactIfNeeded()`。

---

## 压缩流程

```mermaid
flowchart TD
    A[CompactIfNeeded] --> B{messages 为空?}
    B -->|是| Z1[原样返回, changed=false]
    B -->|否| C[estimateTotalTokens]
    C --> D{total >= triggerTokens?}
    D -->|否| Z2[原样返回, changed=false]
    D -->|是| E[selectBoundary]
    E --> F{boundary.ok?}
    F -->|否| Z3[返回 ErrUnsafeBoundary]
    F -->|是| G[generateSummary]
    G --> H{摘要成功?}
    H -->|否| Z4[返回 error, 原 messages 不变]
    H -->|是| I[assembleCompactedMessages]
    I --> Z5[返回新 messages, changed=true]
```

### 阶段说明

| 阶段 | 职责 | 关键函数 |
|------|------|----------|
| 1. 压力检测 | 估算消息 + 工具 schema（含 `DeferredToolInfos`）token；Eino 注入的 Instruction 作为 SystemMessage 自然计入 | `estimateTotalTokens`, `triggerTokens`, `safePromptBudget` |
| 2. 边界切分 | 拆分并保留 system 消息，仅对非 system 上下文划分 head/middle/tail，保证 tool 配对完整 | `splitSystemAndContextMessages`, `selectBoundary` |
| 3. 中间段摘要 | 调用主模型生成六段式结构化摘要 | `generateSummary`, `summaryTokenLimit` |
| 4. 结果组装 | 不修改原消息，拼接 `[system, head, summary, tail]` | `assembleCompactedMessages` |

### 触发条件

```text
safePromptBudget = maxContextTokens - maxOutputTokens
triggerTokens    = safePromptBudget × 80%
```

当 `estimateTotalTokens(messages, toolInfos, deferredToolInfos) >= triggerTokens` 时进入压缩。Eino 的 `defaultGenModelInput` 会将 Instruction 以 `SystemMessage` 形式注入 `state.Messages` 头部，因此 `CountMessages` 已天然包含 Instruction，无需在 `Compactor` 中单独跟踪。

### 工具 token 口径

与 `usage.TokenEstimator.CountModelToolContext` 一致：

- `toolInfos` — 完整 JSON schema（`model.WithTools`）
- `deferredToolInfos` — 仅 name + description（`model.WithDeferredTools`）

压缩决策的 token 总量 = 消息 token + 上述工具上下文 token。

### 默认边界参数

| 常量 | 值 | 含义 |
|------|-----|------|
| `defaultHeadMessages` | 2 | 非 system 上下文中保留的头部消息数 |
| `defaultTailMessages` | 4 | 尾部至少保留的最近消息数 |
| `compactionTriggerPercent` | 80 | 触发压缩的 prompt 预算比例 |
| `defaultSummaryTokens` | 4096 | 摘要输出 token 上限（大模型场景） |
| `minimumSummaryTokens` | 512 | 摘要输出 token 下限（小 max output 场景） |

摘要输出上限由 `summaryTokenLimit()` 动态计算：`maxOutputTokens / 4`，并 clamp 到 `[512, 4096]`。

---

## 文件职责

| 文件 | 内容 |
|------|------|
| `compact.go` | `Compactor` 类型、入口 `CompactIfNeeded`、token 估算与摘要生成 |
| `boundary.go` | head/middle/tail 切分及 tool call/result 配对保护 |
| `assemble.go` | 压缩结果消息列表组装 |
| `compact_test.go` | 单元测试 |

---

## 函数调用关系

### 总览（以 `CompactIfNeeded` 为根）

```text
NewCompactor(cfg)
  └── usage.NewTokenEstimator(modelName)   # cfg.Estimator 为空时

CompactIfNeeded(ctx, messages, toolInfos, deferredToolInfos)
  ├── estimateTotalTokens(messages, toolInfos, deferredToolInfos)
  │     ├── TokenEstimator.CountMessages(messages)
  │     └── TokenEstimator.CountModelToolContext(toolInfos, deferredToolInfos)
  ├── triggerTokens()
  │     └── safePromptBudget()
  ├── splitSystemAndContextMessages(messages)     [boundary.go]
  ├── selectBoundary(contextMessages)             [boundary.go]
  │     ├── adjustHeadEndForToolPairs(...)
  │     ├── adjustTailStartForToolPairs(...)
  │     ├── cloneMessages(...) × 3
  │     └── hasCompleteToolPairs(head/tail)
  ├── generateSummary(ctx, middle, middleTokens)
  │     ├── summaryTokenLimit()
  │     ├── prompts.ContextCompactionSystemPrompt
  │     ├── prompts.ContextCompactionUserPrompt(...)
  │     └── model.Generate(...)
  └── assembleCompactedMessages(system, head, summary, tail)  [assemble.go]
```

### 调用关系图

```mermaid
flowchart LR
    subgraph compact.go
        NC[NewCompactor]
        CIN[CompactIfNeeded]
        ETT[estimateTotalTokens]
        TT[triggerTokens]
        SPB[safePromptBudget]
        STL[summaryTokenLimit]
        GS[generateSummary]
    end

    subgraph boundary.go
        SB[selectBoundary]
        WSM[withoutSystemMessages]
        AHE[adjustHeadEndForToolPairs]
        ATS[adjustTailStartForToolPairs]
        CM[cloneMessages]
        HCTP[hasCompleteToolPairs]
    end

    subgraph assemble.go
        ACM[assembleCompactedMessages]
    end

    subgraph external
        TE[usage.TokenEstimator]
        PR[prompts.*]
        MD[model.Generate]
    end

    NC --> TE
    CIN --> ETT --> TE
    CIN --> TT --> SPB
    CIN --> SB
    CIN --> GS --> STL
    GS --> PR
    GS --> MD
    CIN --> ACM

    SB --> WSM
    SB --> AHE
    SB --> ATS
    SB --> CM
    SB --> HCTP
```

---

## 各函数说明

### compact.go

| 函数 | 可见性 | 说明 |
|------|--------|------|
| `NewCompactor` | 导出 | 校验配置，创建 `Compactor`；可注入共享 `usage.TokenEstimator` |
| `CompactIfNeeded` | 导出 | **主入口**：检测压力 → 切分 → 摘要 → 组装 |
| `estimateTotalTokens` | 私有 | 消息 token + `CountModelToolContext(toolInfos, deferredToolInfos)` |
| `triggerTokens` | 私有 | 返回 80% 安全预算水位线 |
| `safePromptBudget` | 私有 | `maxContextTokens - maxOutputTokens` |
| `summaryTokenLimit` | 私有 | 摘要模型的 `MaxTokens` 上限 |
| `generateSummary` | 私有 | 构造压缩 prompt，调用模型，返回带前缀的 user 摘要消息 |

### boundary.go

| 函数 | 可见性 | 说明 |
|------|--------|------|
| `splitSystemAndContextMessages` | 包内 | 拆分 system messages 与可压缩上下文 |
| `selectBoundary` | 包内 | 在非 system 上下文中选择 head/middle/tail；失败时 `ok=false` |
| `adjustHeadEndForToolPairs` | 包内 | head 内未闭合的 tool_call 需把对应 result 纳入 head |
| `adjustTailStartForToolPairs` | 包内 | tail 内 tool_result 需把对应 assistant 纳入 tail |
| `hasCompleteToolPairs` | 包内 | 校验 segment 内 tool_call 与 tool_result 一一配对 |
| `cloneMessages` | 包内 | 浅拷贝消息，避免污染原历史 |

### assemble.go

| 函数 | 可见性 | 说明 |
|------|--------|------|
| `assembleCompactedMessages` | 包内 | 按顺序拼接 system、head、summary（可选）、tail |

---

## 边界切分算法（selectBoundary）

```text
messages (非 system context)
├── head   : messages[0 : headEnd)           默认 2 条
├── middle : messages[headEnd : tailStart)   待摘要
└── tail   : messages[tailStart : ]          默认 4 条
```

处理顺序：拆分 system → 长度校验 → 初始切点 → head 扩展（tool 配对）→ tail 收缩（tool 配对）→ 克隆与校验。任一环节失败 → `compactionBoundary{ok: false}` → `ErrUnsafeBoundary`。

---

## 摘要生成（generateSummary）

输入为 middle 段消息，构造 system + user 两条 prompt，调用 `model.Generate`。输出为 **user 角色**消息，前缀 `prompts.ContextCompactionSummaryPrefix`，以便后续压缩轮次不会被当作 system 过滤。

建议摘要结构：`Goal` / `Constraints` / `Progress` / `Decisions` / `Relevant Files` / `Next Steps`。

---

## 返回值与错误语义

```go
func (c *Compactor) CompactIfNeeded(
    ctx context.Context,
    messages []*schema.Message,
    toolInfos []*schema.ToolInfo,
    deferredToolInfos []*schema.ToolInfo,
) (next []*schema.Message, changed bool, err error)
```

| 场景 | `next` | `changed` | `err` |
|------|--------|-----------|-------|
| 未达触发水位 | 原 messages | `false` | `nil` |
| 压缩成功 | `[system, head, summary, tail]` | `true` | `nil` |
| 边界不安全 | 原 messages | `false` | `ErrUnsafeBoundary` |
| 摘要失败 | 原 messages | `false` | 非 nil error |
| token 估算失败 | 原 messages | `false` | 非 nil error |

Middleware 在 `err != nil` 时记录 warning 并透传原 state，不中断 agent 运行。

---

## 外部依赖

| 包 | 用途 |
|----|------|
| `internal/context/common/usage` | `TokenEstimator`、`CountModelToolContext` |
| `internal/prompts` | 压缩 system/user prompt 与摘要前缀 |
| `github.com/cloudwego/eino/components/model` | `Generate` 生成摘要 |
| `github.com/cloudwego/eino/schema` | 消息与工具类型 |

---

## 相关文档

- 设计总览：[`docs/context/compression.md`](../../../docs/context/compression.md)
- Middleware 集成：[`internal/middlewares/content/content.go`](../../middlewares/content/content.go)
- Token 分类与工具计数：[`internal/context/common/usage/README.md`](../common/usage/README.md)
- 终端 Context 行展示：[`internal/context/budget/README.md`](../budget/README.md)
- 创建 Compactor：[`internal/agents/interactive.go`](../../agents/interactive.go)
