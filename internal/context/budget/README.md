# budget 包

`budget` 包在单轮 runner turn 内维护**线程安全的回合统计快照**，供终端在模型完成最终回复后输出一行状态。Eino 不提供 token 分段 API，本包只做总量统计，不做 conv/tools/sys 等本地分类。

## 终端输出

用户发消息后、模型最终回答结束时，`Renderer.WriteTurnStatus` 输出一行：

```text
[Stats: elapsed=0.77s prompt↑=318 completion↓=37 total↑↓=611 0.48% 128K]
```

| 字段 | 含义 | 数据来源 |
|------|------|----------|
| `elapsed` | 本轮耗时 | `BeginTurn` → `FinalizeTurn` 计时 |
| `prompt↑` | 最后一跳模型调用的输入上下文 | 各跳 provider `prompt_tokens` 的**最后一跳**（随历史增长，不多跳累加） |
| `completion↓` | 本回合模型生成量 | 各跳 `completion_tokens` **累加** |
| `total↑↓` | 会话上下文窗口占用总量 | 优先 `AfterAgent` 时 `state.Messages` 的本地 tiktoken 估算（反映压缩）；无估算时回退最后一跳 `prompt+completion` |
| `0.48%` | `total↑↓` 占窗口比例（两位小数） | `total↑↓ ÷ MaxContext` |
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
