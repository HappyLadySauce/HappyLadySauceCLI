# budget 包

`budget` 包在单轮 runner turn 内维护**线程安全的回合统计快照**，供终端在模型完成最终回复后输出一行状态。

## 终端输出

```text
[Stats: elapsed=0.83s prompt↑=318 completion↓=40 total↑↓=358 0.28% 128K]
```

| 字段 | 含义 | 数据来源 |
|------|------|----------|
| `elapsed` | 本轮耗时 | `BeginTurn` → `FinalizeTurn` 计时 |
| `prompt↑` | 最后一跳模型调用的输入 | provider 最后一跳 `prompt_tokens` |
| `completion↓` | 本回合模型生成量 | 各 hop `completion_tokens` **累加** |
| `total↑↓` | 会话上下文窗口占用 | `SessionContext.TotalTokens()`（provider 真值） |
| `0.28%` | `total↑↓` 占窗口比例（两位小数） | `total↑↓ ÷ MaxContext` |
| `128K` | 模型上下文窗口上限 | 配置 `MaxModelContext` |

## 单轮生命周期

```text
RunLoop: session（跨轮）+ WithBudgetWriter + WithTurnRecorder
  └── runner.Run(runCtx, history)
        ├── BeforeAgent              → writer.BeginTurn()
        ├── [每跳] UsageTrackingChatModel → session.Update + writer.AddUsage
        └── AfterAgent               → writer.FinalizeTurn(maxContext, session.Total)
  └── renderer.WriteTurnStatus(writer.ReadTurnStatus())
```

## API

| API | 说明 |
|-----|------|
| `NewBudgetWriter` | 创建空写入器 |
| `BeginTurn` / `AddUsage` / `FinalizeTurn` | 单轮计时、hop 用量、回合结束快照 |
| `ReadTurnStatus` | 返回 `TurnStats` |
| `WithBudgetWriter` / `BudgetWriterFromContext` | context 传递 writer |

## 依赖与集成

| 方向 | 说明 |
|------|------|
| 计量来源 | `usage.UsageTrackingChatModel` + `usage.TurnRecorder` |
| Session total | `usage.SessionContext` |
| Middleware | `internal/middlewares/budget/budget.go`（仅 BeforeAgent / AfterAgent） |
| RunLoop | `internal/agents/interactive.go` |
