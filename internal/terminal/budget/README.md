# terminal/budget 包

`terminal/budget` 负责把稳定的 `context/status.Status` DTO 格式化为终端统计行。

## 输出格式

```text
[Stats: elapsed=0.77s prompt↑=318 completion↓=37 total↑↓=634 content=318 0.25%(128K)]
```

`total↑↓` 是本次用户交互内所有 model turn 的 provider total 聚合值；`content` 是最新 provider total，表示当前上下文窗口占用，百分比只基于 `content / maxContext` 计算。

`Renderer.WriteConversationStatus` 在 TTY 下会对各字段分段着色。

## API

| API | 说明 |
|-----|------|
| `FormatConversationStatusLine(status, maxContext)` | 生成单行统计字符串 |

本包只做格式化，不写 stdout/stderr。`Renderer.WriteConversationStatus` 在 `internal/terminal` 中负责写入 `errOut`。
