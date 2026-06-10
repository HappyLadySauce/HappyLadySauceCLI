# usage 包

`usage` 包负责精细化 token 计算：本地 tiktoken 估算、provider 用量标准化，以及单次遍历中的上下文分段分类。

## 文件职责

| 文件 | 职责 |
|------|------|
| `estimate.go` | 本地 `TokenEstimator`、tiktoken 编码选择、消息/工具/文本计数 |
| `provider.go` | `UsageSnapshot`、从 Eino message 提取 provider 用量 |
| `usage.go` | `Segment`、`Breakdown`、`Calculator`、分类计数与 provider 比例缩放 |

## 核心 API

| API | 说明 |
|-----|------|
| `NewCalculator(modelName, maxContext)` | 创建带上下文窗口上限的计算器 |
| `Calculator.Count(CountInput)` | 单次遍历消息，同时估算 token 并分类到 `Segment` |
| `Calculator.Estimator()` | 获取底层 `TokenEstimator`，供 compactor 等模块共享 |
| `Breakdown.ApplyProvider(UsageSnapshot)` | 合并 provider 用量并按比例缩放各分段 |
| `SnapshotFromMessage(msg)` | 从 assistant 回复提取 provider 用量 |

## 分段（Segment）

与 ChatModelAgent `BeforeModelRewriteState` 的 `state` 输入对齐：

- `system` — `state.Messages` 中的 system 消息，以及 Run 级 `Instruction`
- `tools` — `state.ToolInfos` 完整 schema（`model.WithTools`）+ `state.DeferredToolInfos` 轻量定义（`model.WithDeferredTools`，仅 name+desc）+ `state.Messages` 中的工具轨迹（assistant `ToolCalls`、role=`tool` 结果）
- `conversation` — 其余 user/assistant 对话（不含 tool call）
- `rules` / `skills` / `mcp` / `subagents` — 预留

工具边界说明：ReAct 循环中，每次模型调用前 provider 同时收到 `ToolInfos`（`model.WithTools`）、`DeferredToolInfos`（`model.WithDeferredTools`）与消息里的 tool trace；三者合并计入 `tools`。立即工具按完整 JSON schema 计数；延迟工具仅计 name+description，避免把未加载 schema 高估进预算。

## Provider 合并

模型调用前使用本地估算（`Source = estimated`）。模型返回 usage 后，通过 `Breakdown.ApplyProvider` 或 `budget.BudgetWriter.ApplyUsage`：

1. 记录 `ActualPrompt`、`ActualOutput`、`CachedTokens`、`ReasoningTokens`
2. 以 `ActualPrompt / EstimatedTotal` 为比例缩放各分段
3. 将舍入余量分配到小数误差最大的分段，保证分段之和等于 actual prompt

## 约束

- 本地估算不是 provider billing 真相；provider 可用时以 actual prompt 为总量基准。
- 已知 OpenAI 新模型族优先使用 `o200k_base`，旧 GPT 和常见第三方模型使用 `cl100k_base` 近似。
- 未知模型默认回退 `cl100k_base`；只有 tiktoken 加载失败时才使用字符粗估。
