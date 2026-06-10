# usage 包

`usage` 包负责精细化 token 计算：本地 tiktoken 估算、provider 用量标准化，以及**单次遍历**中的上下文三分段分类。`budget` 与 `compact` 均依赖本包，共享同一套 `TokenEstimator`。

## 文件职责

| 文件 | 职责 |
|------|------|
| `estimate.go` | `TokenEstimator`、tiktoken 编码选择、消息/工具/文本计数、`CountModelToolContext` |
| `provider.go` | `UsageSnapshot`、`UsageSourceProvider` / `UsageSourceEstimated`、`SnapshotFromMessage` |
| `usage.go` | `Segment`、`SegmentCounts`、`Breakdown`、`Calculator`、`ScaleSegmentCounts` |

## 三分段（Segment）

与 ChatModelAgent `BeforeModelRewriteState` 的 `state` 输入对齐，当前仅建模三个可见分段：

| Segment | `SegmentCounts` 字段 | 计入内容 |
|---------|---------------------|----------|
| `system` | `System` | `state.Messages` 中 role=system；Run 级 `Instruction`（middleware 注入） |
| `conversation` | `Conversation` | 其余 user/assistant 消息（不含 `ToolCalls`、非 role=tool）；含 `reasoning_content` |
| `tools` | `Tools` | `state.ToolInfos` 完整 JSON schema；`state.DeferredToolInfos` 轻量定义（仅 name+description）；消息中的 assistant `ToolCalls` 与 role=`tool` 结果 |

工具边界说明：ReAct 循环中，每次模型调用前 provider 同时收到 `ToolInfos`（`model.WithTools`）、`DeferredToolInfos`（`model.WithDeferredTools`）与消息里的 tool trace；三者合并计入 `tools`。立即工具按完整 schema 计数；延迟工具仅计 name+description，避免未加载 schema 被高估。

## 核心 API

| API | 说明 |
|-----|------|
| `NewTokenEstimator(modelName)` | 创建本地估算器（compactor 与 calculator 共享） |
| `NewCalculator(modelName, maxContext)` | 创建带窗口上限的分类计算器 |
| `Calculator.Count(CountInput)` | 单次遍历：估算 token 并填入 `SegmentCounts` |
| `Calculator.Estimator()` | 返回底层 `TokenEstimator` |
| `Breakdown.ApplyProvider(UsageSnapshot)` | 以 provider prompt 为总量，按比例缩放各分段 |
| `ScaleSegmentCounts` | 比例缩放 + 舍入余量吸收到最大分段 |
| `SnapshotFromMessage(msg)` | 从 assistant 回复的 `ResponseMeta.Usage` 提取用量 |

`CountInput` 字段：`Messages`、`ToolInfos`、`DeferredToolInfos`、`Instruction`。

`Breakdown` 关键字段：`Segs`、`EstimatedTotal`（缩放前）、`ActualPrompt` / `ActualOutput`（provider）、`Source`、`MaxContext`。`Total()` 优先返回 `ActualPrompt`，否则 `EstimatedTotal`。`PercentUsed()` 用于 Context 行百分比。

## Provider 与本地估算

| 阶段 | 行为 |
|------|------|
| 模型调用前 | `Calculator.Count` → `Source = estimated`，分段为纯本地 tiktoken |
| 模型返回后 | `Breakdown.ApplyProvider` 或 `budget.BudgetWriter.AddUsage` + `FinalizeTurn` |

`ApplyProvider` 步骤：

1. 写入 `ActualPrompt`、`ActualOutput`、`CachedTokens`、`ReasoningTokens`、`Source`
2. `scale = PromptTokens / EstimatedTotal`，对各分段四舍五入
3. 舍入余量分配到当前值最大的分段，保证分段之和等于 actual prompt

**budget 中间件策略**：Stats 行累加各跳 provider 用量；Context 行在 `FinalizeTurn` 时仅用**最后一跳** `prompt_tokens` 作为缩放目标（见 `budget` 包 README）。

## 本地估算约束

- 本地估算不是 provider billing 真相；provider 可用时以 actual prompt 为总量基准。
- OpenAI 新模型族优先 `o200k_base`，旧 GPT 与常见第三方模型用 `cl100k_base` 近似。
- 未知模型回退 `cl100k_base`；tiktoken 加载失败时用字符粗估（`fallbackCharsPerToken = 4`）。
- 每条消息附加 OpenAI chat framing 开销（`tokensPerMessage` 等）；整批消息另加 `ReplyPrimingTokens`（计入最大分段）。

## 依赖方向

- 被 `internal/context/budget`、`internal/context/compact`、`internal/middlewares/budget` 引用。
- 不依赖 terminal、agents 或 middlewares。
