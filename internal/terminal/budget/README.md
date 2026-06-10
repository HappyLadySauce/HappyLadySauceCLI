# terminal/budget 包

`terminal/budget` 负责把 `context/budget.TurnStatus` 格式化为终端统计行。

## 输出格式

```text
[ Stats: elapsed=960ms prompt↑=1340 completion↓=31 context <1% 128K ]
```

## API

| API | 说明 |
|-----|------|
| `FormatTurnStatusLine(status)` | 生成单行统计字符串 |

本包只做格式化，不写 stdout/stderr。`Renderer.WriteTurnStatus` 在 `internal/terminal` 中负责写入 `errOut`。
