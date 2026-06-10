# terminal/budget 包

`terminal/budget` 负责把 `context/budget.TurnStatus` 格式化为终端统计行。

## 输出格式

```text
[Stats: elapsed=0.77s prompt↑=318 completion↓=37 total↑↓=355 <1% 128K]
```

`Renderer.WriteTurnStatus` 在 TTY 下会对各字段分段着色。

## API

| API | 说明 |
|-----|------|
| `FormatTurnStatusLine(status)` | 生成单行统计字符串 |

本包只做格式化，不写 stdout/stderr。`Renderer.WriteTurnStatus` 在 `internal/terminal` 中负责写入 `errOut`。
