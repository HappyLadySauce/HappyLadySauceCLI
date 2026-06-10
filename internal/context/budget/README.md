# budget 包

`budget` 包负责把一次模型调用前实际可见的 ctx 拆成稳定分段，并通过 `BudgetWriter` 在 runner turn 内传递最新快照。

## 分段口径

| Segment | 状态标签 | 含义 |
|---------|----------|------|
| `SegmentSystem` | `sys` | 静态模型上下文，包括 Eino 注入的 system / Instruction 和注册工具 schema |
| `SegmentConversation` | `conv` | 普通 user / assistant 对话消息 |
| `SegmentTools` | `tools` | 实际 tool call / tool result 消息 |
| `SegmentRules` / `SegmentSkills` / `SegmentMCP` / `SegmentSubagents` | `rules` / `skills` / `mcp` / `sub` | 后续扩展预留分段 |

注册工具 schema 会进入模型 prompt，但不是一次工具调用，因此归入 `sys`。只有发生真实工具调用或工具结果进入消息历史时，才出现 `tools`。

## API

| API | 说明 |
|-----|------|
| `EstimateBudget(input, estimator, maxContextTokens)` | 估算分段 token 快照 |
| `ContextBudget` | 分段结果，包含窗口大小、总 token、分段 map 和百分比 |
| `BudgetWriter` | 单轮 runner 的线程安全快照槽位 |
| `WithBudgetWriter` / `BudgetWriterFromContext` | 通过 context 在 RunLoop 与 middleware 之间传递 writer |

## 依赖方向

- `budget` 依赖 `common/usage` 的 `TokenEstimator`。
- `budget` 不依赖 terminal、agents 或 middlewares。
- middleware 只负责写入快照，terminal 只负责展示快照。
