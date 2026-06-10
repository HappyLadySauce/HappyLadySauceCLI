# usage 包

`usage` 包负责 **provider 用量标准化**、**ChatModel 层计量** 与 **跨轮 session total** 追踪。压缩触发与 Stats `total↑↓` 以 provider 真值为准；`TokenEstimator` 仅用于摘要辅助模型的 middle 段规模提示。

## 文件职责

| 文件 | 职责 |
|------|------|
| `provider.go` | `UsageSnapshot`、`SnapshotFromMessage` |
| `session.go` | `SessionContext`、跨轮 provider total、`WithSkipTracking` |
| `tracking_model.go` | `UsageTrackingChatModel` 装饰器（Generate/Stream 后更新 session） |
| `turn_recorder.go` | `TurnRecorder` 接口、单轮 hop 用量记录 |
| `estimate.go` | `TokenEstimator`（摘要 middle 段估算，不参与压缩触发） |

## 三层语义

| 字段 | 含义 | 更新时机 |
|------|------|----------|
| `prompt↑` | 本轮最后一跳模型调用输入 | 每 hop 后 `TurnRecorder.AddUsage` |
| `completion↓` | 本轮各 hop 生成量累加 | 每 hop 后 `TurnRecorder.AddUsage` |
| `total↑↓` | 会话上下文窗口占用（动态） | 每 hop 后 `SessionContext.UpdateFromSnapshot` |

`UpdateFromSnapshot` 优先 `TotalTokens`；为 0 时回退 `PromptTokens + CompletionTokens`。

## ChatModel 装饰器

```text
openai.NewChatModel → usage.NewTrackingChatModel(inner)
  ├── Agent ReAct 循环（Generate/Stream）
  └── Compactor 摘要（Generate + WithSkipTracking，不污染 session total）
```

`WithSkipTracking(ctx)` 用于压缩摘要等辅助调用。

## Context 传递

| API | 说明 |
|-----|------|
| `WithSessionContext` / `SessionFromContext` | RunLoop 级 session total |
| `WithTurnRecorder` / `TurnRecorderFromContext` | 单轮 `BudgetWriter` |
| `WithSkipTracking` / `SkipTracking` | 跳过计量 |

## Provider 不可用

`ResponseMeta.Usage == nil` 时 session total 不更新；压缩不触发；Stats `total↑↓` 为 0。

## 依赖方向

- 被 `internal/context/budget`、`internal/context/compact`、`internal/middlewares`、`internal/agents` 引用。
