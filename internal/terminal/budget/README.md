# budget 包

`terminal/budget` 包只负责把 `context/common/budget.ContextBudget` 格式化成终端状态行。

## 职责

- 按固定优先级选择 token 最高的前三个非零分段。
- 生成紧凑状态行，例如 `[context <1% 128K | sys 94 | conv 9]`。
- 保持纯格式化逻辑，不直接写 stdout/stderr。

## 边界

- `Renderer.WriteContextStatus` 留在 `internal/terminal` 根包，负责把格式化结果写入 `errOut`。
- 本包不依赖 renderer，避免跨目录扩展同一个 Go package。
