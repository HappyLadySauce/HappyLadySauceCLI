# budget 包

`budget` 包在单轮 runner turn 内维护**线程安全的回合统计快照**，供终端在模型完成最终回复后输出一行状态。Eino 不提供 token 分段 API，本包只做总量统计，不做 conv/tools/sys 等本地分类。

## 终端输出

用户发消息后、模型最终回答结束时，`Renderer.WriteTurnStatus` 输出一行：

```text
[Stats: elapsed=0.77s prompt↑=318 completion↓=37 total↑↓=355 <1% 128K]
```

| 字段 | 含义 | 数据来源 |
|------|------|----------|
| `elapsed` | 本轮耗时 | `BeginTurn` → `FinalizeTurn` 计时 |
| `prompt↑` | 本轮 API prompt 消耗（多跳累加） | `AddUsage` 聚合各跳 provider `prompt_tokens` |
| `completion↓` | 本轮 API completion 消耗（多跳累加） | `AddUsage` 聚合各跳 provider `completion_tokens` |
| `total↑↓` | 本轮 API token 总消耗 | `prompt↑ + completion↓` |
| `<1%` | 当前会话占窗口比例 | **最后一跳** provider `prompt_tokens` ÷ `MaxContext`；无 provider 时回退本地 tiktoken 总量估算 |
| `128K` | 模型上下文窗口上限 | 配置中的 `MaxModelContext` |

终端会对各字段分段着色（括号灰、耗时青、prompt 绿、completion 紫、total 亮白、窗口占用黄）。

多跳场景：`prompt↑` 为各跳之和；`context` 百分比以最后一跳 prompt 为准（与 provider 实际送入模型的上下文一致）。

## 单轮生命周期

```text
RunLoop: WithBudgetWriter(ctx, writer)
  └── runner.Run(runCtx, history)
        ├── BeforeAgent          → writer.BeginTurn()
        ├── [每跳模型调用]
        │     └── AfterModelRewriteState → writer.AddUsage(SnapshotFromMessage)
        └── AfterAgent           → EstimateVisiblePromptTokens → writer.FinalizeTurn(maxContext, estimated)
  └── renderer.WriteTurnStatus(writer.ReadTurnStatus())
```

## API

| API | 说明 |
|-----|------|
| `NewBudgetWriter` | 创建空写入器 |
| `BeginTurn` / `AddUsage` / `FinalizeTurn` | 单轮计时、用量聚合、回合结束快照 |
| `ReadTurnStatus` | 返回 `TurnStats` |
| `TurnStats.PercentUsed` | 计算 `ContextTokens / MaxContext` 百分比 |
| `WithBudgetWriter` / `BudgetWriterFromContext` | 通过 context 传递 writer |

## 依赖与集成

| 方向 | 说明 |
|------|------|
| 依赖 | `internal/context/common/usage`（`UsageSnapshot`、`TokenEstimator`） |
| Middleware | `internal/middlewares/budget/budget.go` |
| Terminal | `internal/terminal/budget/context_status.go`：`FormatTurnStatusLine` |
| RunLoop | `internal/agents/interactive.go` |
