# usage 包

`usage` 包负责本地 token 估算，是上下文压缩和预算监视的底层计数模块。

## 职责

- 根据运行时模型名选择 tiktoken encoding。
- 估算 `schema.Message`、tool schema、纯文本和多模态 JSON 片段的 token。
- 暴露 OpenAI chat framing 口径中的 `ReplyPrimingTokens`，供上层分段预算复用。

## API

| API | 说明 |
|-----|------|
| `NewTokenEstimator(modelName)` | 创建模型名感知的估算器 |
| `CountMessages(messages)` | 估算完整消息列表，包含 reply priming |
| `CountMessage(message)` | 估算单条消息，包含 role、content、reasoning、tool call、多模态字段 |
| `CountTools(tools)` | 估算注册工具 schema JSON 成本 |
| `CountInstruction(text)` | 估算静态 instruction 文本 |
| `CountText(text)` | 估算任意文本 |

## 约束

- 这是本地估算，不是 provider billing 真相。
- 已知 OpenAI 新模型族优先使用 `o200k_base`，旧 GPT 和常见第三方模型使用 `cl100k_base` 近似。
- 未知模型默认回退 `cl100k_base`；只有 tiktoken 加载失败时才使用字符粗估。
