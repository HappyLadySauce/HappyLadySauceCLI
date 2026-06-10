# usage 包

`usage` 包负责本地 tiktoken 估算与 provider 用量标准化。压缩（`compact`）与回合统计（`budget`）共享 `TokenEstimator`；**不做 token 分段分类**（Eino 无对应 API，本地拆分永远不准确）。

## 文件职责

| 文件 | 职责 |
|------|------|
| `estimate.go` | `TokenEstimator`、tiktoken 编码、消息/工具/文本计数、`EstimateVisiblePromptTokens` |
| `provider.go` | `UsageSnapshot`、`SnapshotFromMessage` |

## 核心 API

| API | 说明 |
|-----|------|
| `NewTokenEstimator(modelName)` | 创建本地估算器 |
| `CountMessages(messages)` | 估算消息 token（含 reply priming） |
| `CountModelToolContext(toolInfos, deferredToolInfos)` | 估算立即工具 schema + 延迟工具 name/desc |
| `EstimateVisiblePromptTokens(...)` | 消息 + 工具上下文总量，供 budget 无 provider 时兜底 |
| `SnapshotFromMessage(msg)` | 从 assistant 回复提取 provider 用量 |

### 工具 token 口径

- `ToolInfos` — 完整 JSON schema（`model.WithTools`）
- `DeferredToolInfos` — 仅 name + description（`model.WithDeferredTools`）

## Provider 用量

`UsageSnapshot` 字段：`PromptTokens`、`CompletionTokens`、`CachedTokens`、`ReasoningTokens`、`Source`（`provider` / `estimated`）。

budget 中间件在 `AfterModelRewriteState` 从最后一条 assistant 消息的 `ResponseMeta.Usage` 读取并累加。

## 本地估算约束

- 本地估算不是 provider billing 真相；有 provider 时 context 百分比以最后一跳 `prompt_tokens` 为准。
- OpenAI 新模型族优先 `o200k_base`，旧 GPT 与常见第三方用 `cl100k_base`。
- 未知模型回退 `cl100k_base`；tiktoken 加载失败时用字符粗估。

## 依赖方向

- 被 `internal/context/budget`、`internal/context/compact`、`internal/middlewares/budget` 引用。
