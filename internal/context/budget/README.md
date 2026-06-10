# budget 包

`budget` 包在单轮 runner turn 内维护**线程安全的上下文预算快照**，供终端在模型完成最终回复后展示两行状态。Token 分类与本地估算由 `internal/context/common/usage` 负责；本包只负责聚合 provider 用量、按策略缩放分段，并通过 `context.Context` 在 RunLoop 与 middleware 之间传递。

## 终端两行状态

用户发消息后、模型最终回答结束时，`Renderer.WriteTurnStatus` 输出：

```text
[ Stats: elapsed=1234ms prompt↑=936 completion↓=42 ]
[context 1% 128K | conv 312 | tools 132 | sys 107]
```

| 行 | 含义 | 数据来源 |
|----|------|----------|
| **Stats** | 本轮 API 消耗（ReAct 多跳时累加） | `TurnStats`：`AddUsage` 聚合各跳 provider `prompt_tokens` / `completion_tokens` |
| **Context** | 当前会话占上下文窗口的比例与分段 | `usage.Breakdown`：`FinalizeTurn` 用**最后一跳** provider `prompt_tokens` 作为总量，本地分类分段按比例缩放 |

多跳场景下 Stats 的 `prompt↑` 为各跳之和，Context 总量为最后一跳 prompt（与 provider 实际送入模型的上下文一致）。无 provider 用量时 `Source = estimated`，Context 回退纯本地 tiktoken 估算。

## 分段口径

与 `usage.SegmentCounts` 对齐，终端固定展示 `conv | tools | sys`：

| 字段 | 标签 | 含义 |
|------|------|------|
| `Segs.Conversation` | `conv` | user / assistant 对话（不含 tool call / tool result） |
| `Segs.Tools` | `tools` | `ToolInfos` 完整 schema、`DeferredToolInfos` 轻量定义（name+desc）、消息中的工具轨迹 |
| `Segs.System` | `sys` | `state.Messages` 中的 system 消息 + Run 级 `Instruction` |

注册工具 schema 计入 `tools`（每次模型调用都会随 prompt 发送）。只有真实 tool call / tool result 进入历史后，`tools` 段才会随轨迹增长。

## 单轮生命周期

```text
RunLoop: WithBudgetWriter(ctx, writer)
  └── runner.Run(runCtx, history)
        ├── BeforeAgent          → writer.BeginTurn()
        ├── [每跳模型调用]
        │     └── AfterModelRewriteState → writer.AddUsage(SnapshotFromMessage)
        └── AfterAgent           → calculator.Count(...) → writer.FinalizeTurn(breakdown)
  └── renderer.WriteTurnStatus(writer.ReadTurnStatus())
```

- `BeginTurn`：重置耗时与累加用量，记录 `turnStart`。
- `AddUsage`：累加 Stats；若本跳有 `prompt_tokens`，更新 `lastHopPrompt`（供 Context 行总量）。
- `FinalizeTurn`：结束耗时统计；若有 `lastHopPrompt`，对本地 `Breakdown` 调用 `ApplyProvider` 按比例缩放分段；写入 `ActualOutput`（来自累加的 completion）。

## API

| API | 说明 |
|-----|------|
| `NewBudgetWriter` | 创建空写入器 |
| `BeginTurn` / `AddUsage` / `FinalizeTurn` | 单轮计时、用量聚合、回合结束快照（middleware 调用） |
| `ReadTurnStatus` | 返回 `TurnStats` + `*usage.Breakdown`（RunLoop 展示用） |
| `Write` / `Read` / `Clear` | 直接读写 breakdown 快照 |
| `ApplyUsage` | 将 provider 用量合并进已有 breakdown（比例缩放） |
| `WithBudgetWriter` / `BudgetWriterFromContext` | 通过 context 传递 writer |

## 依赖与集成

| 方向 | 说明 |
|------|------|
| 依赖 | `internal/context/common/usage`（`Breakdown`、`UsageSnapshot`） |
| 不依赖 | terminal、agents、middlewares（保持包边界） |
| Middleware | `internal/middlewares/budget/budget.go`：只写快照，不渲染 |
| Terminal | `internal/terminal/budget/context_status.go`：`FormatStatsLine`、`FormatContextStatusLine` |
| RunLoop | `internal/agents/interactive.go`：每轮创建 writer，回合结束后 `WriteTurnStatus` |

## 已知限制

- Context 行的 **conv / tools / sys 拆分**仍为本地比例估算，仅**总量**对齐 provider 最后一跳 prompt。
- 若 Eino 已将 `Instruction` 注入为 system 消息，本地分类可能对 system 重复计入（比例缩放后影响有限）。
- `reasoning_content` 计入 `conversation` 本地估算。
