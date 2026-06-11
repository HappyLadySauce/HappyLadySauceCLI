# Eino ChatModelAgent 工作机制指南

本文档详细说明 [CloudWeGo Eino ADK](https://github.com/cloudwego/eino) 中 `ChatModelAgent` 的工作方式：消息发送处理机制、工具调用流程、事件结构、以及在本项目中的实际用法。

适用版本：Eino ADK v0.9.4（`github.com/cloudwego/eino v0.9.4`）。

相关文档：

- [Eino 文档索引](./README.md)
- [Eino 组件生态指南](./component-ecosystem-guide.md) — `schema` / `model` / `callbacks` / `tool` / `adk` 分层、消息与 Token 用量全链路
- [ChatModelAgent 中间件实践](./middleware-guide.md)

---

## 1. 概述

`ChatModelAgent` 是 Eino ADK 的核心 agent 实现，封装了一个完整的 **ReAct（Reasoning + Acting）循环**：

```
用户输入 → Runner.Run() → [模型调用 → 工具执行 → 模型调用 → ...] → 最终回复
```

每次 `Run()` 调用会启动一个 agent 回合，内部可能包含多次模型调用与工具执行，直到模型产出不带 tool calls 的最终回复，或触发 return-directly 工具，或达到最大迭代次数。

本项目在 `internal/agents/interactive.go` 中使用它来驱动整个交互式 REPL 循环。

---

## 2. 构造与配置

### 2.1 创建 Agent

```go
agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
    Model:       chatModel,        // model.BaseChatModel — 底层聊天模型
    Name:        "HappyLadySauce", // agent 名称，用于事件标识
    Description: "A Agent for HAPPLADYSAUCECLI",
    Instruction: agentInstruction, // 系统指令（System Prompt）
    ToolsConfig: agentTools,       // 工具配置
    Handlers:    handlers,         // 中间件链
})
```

**关键字段说明：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `Model` | `model.BaseChatModel` | 底层聊天模型接口，本项目使用 `openai.ChatModel` |
| `Name` | `string` | Agent 标识名，会出现在每个 `AgentEvent.AgentName` 中 |
| `Instruction` | `string` | 系统指令文本，框架在每次模型调用时自动作为 SystemMessage 注入 |
| `ToolsConfig` | `adk.ToolsConfig` | 工具配置，包含工具列表和工具相关中间件 |
| `Handlers` | `[]adk.ChatModelAgentMiddleware` | 中间件链（推荐方式）；旧版 `Middlewares` 已标记 Deprecated |
| `MaxIterations` | `int` | 最大 ReAct 迭代次数，超过返回 `ErrExceedMaxIterations` |
| `ModelRetryConfig` | — | 内置模型调用重试配置 |
| `ModelFailoverConfig` | — | 内置模型故障转移配置 |

### 2.2 创建 Runner

Agent 不能直接调用，需要通过 `Runner` 包装：

```go
runner := adk.NewRunner(ctx, adk.RunnerConfig{
    Agent:           agent,
    EnableStreaming: true, // 启用流式输出
})
```

`Runner` 负责：
- 管理单次 `Run()` 的生命周期
- 将 `EnableStreaming` 传递给底层模型
- 生成 `AsyncIterator[*AgentEvent]` 事件流

### 2.3 调用方式

```go
iter := runner.Run(runCtx, history)
// iter 是 *adk.AsyncIterator[*adk.AgentEvent]
```

- `runCtx`：上下文，可通过 `adk.SetRunLocalValue` 传递本次 Run 级数据
- `history`：`[]*schema.Message`，当前的完整对话历史（包含 system、user、assistant、tool 消息）
- 返回值：异步事件迭代器，由调用方逐条消费

---

## 3. ReAct 循环内部机制

### 3.1 循环流程图

```
runner.Run(ctx, messages)
  │
  ├─ BeforeAgent 钩子（修改 Instruction / Tools 等 Run 级配置）
  │
  └─ ReAct 循环（最多 MaxIterations 次）：
       │
       ├─ 1. 构建模型输入
       │      ├─ Instruction → SystemMessage
       │      ├─ state.Messages（对话历史）
       │      └─ state.ToolInfos + state.DeferredToolInfos → 工具 schema
       │
       ├─ 2. BeforeModelRewriteState 钩子
       │      └─ 可修改 state.Messages / state.ToolInfos / state.DeferredToolInfos
       │
       ├─ 3. 模型调用（可能含重试 / 故障转移）
       │      └─ WrapModel 钩子可包装模型调用
       │
       ├─ 4. AfterModelRewriteState 钩子
       │      └─ 读取模型返回的最新消息
       │
       ├─ 5. 检查模型输出
       │      ├─ 无 ToolCalls → 产出最终 Assistant 事件，循环结束
       │      ├─ 有 ToolCalls → 产出 Assistant 事件（含 tool_calls）
       │      └─ 错误 → 产出 Error 事件
       │
       ├─ 6. 执行工具（每条 tool_call 逐一执行）
       │      ├─ Wrap*ToolCall 钩子可包装工具调用
       │      ├─ Tool.InvokableRun / StreamableRun
       │      └─ 产出 Tool 消息事件
       │
       └─ 7. 工具结果追加到 state.Messages，回到步骤 1
       │
       └─ 超迭代 → ErrExceedMaxIterations
  │
  └─ AfterAgent 钩子（仅成功结束时）
```

### 3.2 模型输入构建（框架内部）

每次模型调用前，框架自动将以下内容组装为 `[]*schema.Message` 发送给模型：

1. **Instruction → SystemMessage**：`config.Instruction` 被包装为一条 `role: system` 消息，放在消息列表最前面
2. **state.Messages**：对话历史，包含 user、assistant、tool 消息
3. **state.ToolInfos**：当前轮次模型可见的工具 schema 列表
4. **state.DeferredToolInfos**：延迟检索的工具 schema（通过 ToolSearchTool 动态加载）

这意味着：**Instruction 不占用 `state.Messages` 的槽位**，而是在框架层自动注入。这也是 `Compactor.CompactIfNeeded` 不需要单独计算 instruction token 的原因——Eino 的 `defaultGenModelInput` 已将其注入为一条 SystemMessage，`CountMessages` 遍历时天然计入。

### 3.3 流式与非流式

`RunnerConfig.EnableStreaming` 决定模型调用方式：

- **流式**（`EnableStreaming: true`）：模型通过 `Stream()` 返回 `*schema.StreamReader[*schema.Message]`，每个 chunk 作为独立的 `AgentEvent` 发出，`MessageVariant.IsStreaming = true`
- **非流式**（`EnableStreaming: false`）：模型通过 `Generate()` 一次性返回完整 `*schema.Message`，作为单个 `AgentEvent` 发出，`MessageVariant.IsStreaming = false`

---

## 4. 事件系统

### 4.1 AgentEvent 结构

每次 `runner.Run()` 返回一个 `*adk.AsyncIterator[*adk.AgentEvent]`，其中的核心结构：

```go
type AgentEvent struct {
    AgentName string       // agent 名称（来自 ChatModelAgentConfig.Name）
    Err       error        // 错误（非 nil 时表示致命错误）
    Action    *AgentAction // 特殊动作（如 ExitAction）
    Output    *AgentOutput // 消息输出（模型回复或工具结果）
}

type AgentOutput struct {
    MessageOutput *MessageVariant // 消息变体
}

type MessageVariant struct {
    IsStreaming   bool                           // true = 流式分块，false = 完整消息
    Message       *schema.Message                // 非流式：完整消息
    MessageStream *schema.StreamReader[*schema.Message] // 流式：消息流 reader
    Role          schema.RoleType                // 角色
    ToolName      string                         // 工具名称（仅工具消息）
}
```

### 4.2 事件类型与消费

本项目中 `ConsumeAgentEvents`（[internal/agents/agent_events.go](internal/agents/agent_events.go)）负责消费事件流：

```
遍历事件迭代器：
  ├─ event.Err != nil        → 发出 "error" 事件，返回错误
  ├─ event.Action.Exit       → 发出 "exit" 事件，返回 (messages, true, nil)
  ├─ event.Output == nil     → 跳过（非消息事件）
  └─ event.Output.MessageOutput != nil
       ├─ IsStreaming == true  → renderStreamingMessage()
       └─ IsStreaming == false → renderCompleteMessage()
```

### 4.3 流式事件的典型序列

对于一次 **assistant 流式回复**，事件序列为：

```
1. AgentEvent{Output.MessageOutput.IsStreaming: true}
   → 内部 MessageStream 包含多个 chunk
   → renderStreamingMessage() 逐 chunk 渲染：

   thinking_started          ← spinner 开始（仅 assistant 角色）
   thinking_content_started  ← reasoning/thinking 文本开始
   Write("think...")         ← reasoning token 逐块输出
   message_finished          ← reasoning 段结束
   answer_content_started    ← 最终 answer 文本开始
   Write("answer...")        ← answer token 逐块输出
   message_finished          ← answer 段结束
   thinking_stopped          ← 清理 spinner
```

对于一次 **ReAct 工具调用轮次**，事件序列为：

```
1. AgentEvent{Output.MessageOutput.Message: AssistantMessage + ToolCalls}
   → renderCompleteMessage() 输出 assistant 的 tool_call 决定
   → 消息含 ToolCalls，追加到 turnMessages

2. AgentEvent{Output.MessageOutput.ToolName: "get_weather", Message: ToolMessage}
   → renderCompleteMessage() 输出工具执行结果
   → 消息 role=Tool，追加到 turnMessages

3. AgentEvent{Output.MessageOutput.Message: AssistantMessage (最终回复)}
   → 模型综合工具结果后的最终回答
```

### 4.4 历史消息收集规则

`ConsumeAgentEvents` 只将 **Assistant** 和 **Tool** 角色的消息加入返回值：

```go
func shouldAppendToHistory(msg *schema.Message) bool {
    switch msg.Role {
    case schema.Assistant, schema.Tool:
        return true
    default:
        return false
    }
}
```

System 和 User 消息由调用方（`RunLoop`）自行管理。User 消息在 `runner.Run()` 之前手动 `append` 到 history；System 消息由框架自动注入。

---

## 5. 工具调用机制

### 5.1 工具注册

工具通过 `adk.ToolsConfig` 注册：

```go
func NewAgentTools() adk.ToolsConfig {
    return adk.ToolsConfig{
        ToolsNodeConfig: compose.ToolsNodeConfig{
            Tools: []tool.BaseTool{
                weather.GetWeatherTool(),
            },
        },
    }
}
```

`ToolsNodeConfig` 来自 `github.com/cloudwego/eino/compose`，支持：
- `Tools`：`[]tool.BaseTool` — 基础工具列表
- `ToolCallMiddlewares`：工具调用中间件链

### 5.2 工具类型层次

Eino 的 `tool.BaseTool` 是一个 interface，不同能力对应不同子接口：

| 接口 | 方法 | 适用场景 |
|------|------|----------|
| `tool.InvokableTool` | `InvokableRun(ctx, args, opts...) (string, error)` | 同步工具（最常见） |
| `tool.StreamableTool` | `StreamableRun(ctx, args, opts...) (*StreamReader, error)` | 流式输出工具 |
| Enhanced 变体 | 携带更多上下文 | 需要访问 agent state 的工具 |

中间件的 `Wrap*ToolCall` 钩子**仅对实现了对应接口的工具生效**。

### 5.3 InferTool — 自动工具包装

本项目使用 `utils.InferTool` 从 Go 函数自动生成工具：

```go
func GetWeatherTool() tool.InvokableTool {
    tool, _ := utils.InferTool(
        "get_weather",                              // 工具名
        "天气工具, 获取指定城市的天气信息",            // 工具描述
        getWeather,                                 // 实现函数
    )
    return tool
}
```

`InferTool` 自动完成：
- 从函数签名提取参数与返回值类型
- 通过 `jsonschema` struct tag 生成 JSON Schema（供模型识别参数格式）
- 包装为 `tool.InvokableTool` 接口

参数结构体示例：

```go
type WeatherToolParams struct {
    City string `json:"city" jsonschema:"description=城市名称, 示例: 北京, required"`
    Lang string `json:"lang" jsonschema:"description=语言: zh/en, optional"`
}
```

### 5.4 工具调用流程（框架内部）

当模型返回包含 `ToolCalls` 的消息时：

1. 框架遍历每条 `ToolCall`
2. 根据 `ToolCall.Function.Name` 匹配注册的工具
3. **未匹配**：返回错误 tool result（"tool not found"）
4. **匹配成功**：调用 `Tool.InvokableRun(ctx, args)` 或对应的流式方法
5. 包装工具结果为 `role: tool` 消息，附带 `ToolName` 和 `ToolCallID`
6. 工具结果追加到 `state.Messages`，进入下一轮模型调用

工具调用包装链（由外到内）：

```
eventSenderToolWrapper
  → ToolsConfig.ToolCallMiddlewares
  → AgentMiddleware.WrapToolCall（旧）
  → ChatModelAgentMiddleware.Wrap*ToolCall（新，洋葱模型）
  → callbackInjectedToolCall
  → Tool.InvokableRun / StreamableRun
```

### 5.5 工具上下文

中间件中的 `Wrap*ToolCall` 接收 `*adk.ToolContext`，提供：

| 字段 | 说明 |
|------|------|
| `Name` | 工具名称 |
| `CallID` | 本次调用的唯一 ID（对应 `ToolCall.ID`） |

---

## 6. 消息内容详解

### 6.1 schema.Message 结构

```go
type Message struct {
    Role             RoleType      // system / user / assistant / tool
    Content          string        // 消息正文
    ReasoningContent string        // thinking/reasoning 内容（部分模型支持）
    ToolCalls        []ToolCall    // 工具调用列表（assistant 角色）
    ToolCallID       string        // 工具调用 ID（tool 角色）
    ToolName         string        // 工具名称（tool 角色）
    ResponseMeta     *ResponseMeta // 模型响应元数据（含 token 用量）
    Extra            map[string]any // 扩展字段
}

type ResponseMeta struct {
    Usage *TokenUsage // provider 返回的 token 用量
}

type TokenUsage struct {
    PromptTokens     int
    CompletionTokens int
    // ... 其他用量字段
}

type ToolCall struct {
    ID       string       // 调用唯一 ID
    Type     string       // "function"
    Function FunctionCall // 函数名与参数
}

type FunctionCall struct {
    Name      string // 工具名
    Arguments string // JSON 序列化的参数
}
```

### 6.2 流式 Chunk 消息

流式模式下，`MessageStream.Recv()` 返回的每条 chunk 也是 `*schema.Message`，但字段按语义分段：

- **Reasoning chunk**：`Role: Assistant`，`ReasoningContent` 非空，`Content` 为空
- **Answer chunk**：`Role: Assistant`，`Content` 非空，`ReasoningContent` 为空
- **Control chunk**：可能 `Role` 为空（此时继承 `MessageVariant.Role`）

`renderStreamingMessage` 的责任是：
1. 区分 reasoning chunk 和 answer chunk
2. 在两者切换时发出对应的生命周期事件（`thinking_content_started` → `message_finished` → `answer_content_started`）
3. 处理首 chunk 和尾 chunk 的换行符（前导换行去掉，尾部换行暂存）
4. 流结束后用 `schema.ConcatMessages(chunks)` 合并为一条完整消息

### 6.3 消息在 history 中的角色

对话历史中各种消息的排列规则：

```
[SystemMessage, UserMessage, AssistantMessage, ToolMessage, AssistantMessage, ...]
```

每次 `runner.Run()` 后：
- `RunLoop` 将返回的 `turnMessages`（含 Assistant + Tool）追加到 history
- 下一轮的 User 消息由 `RunLoop` 读取用户输入后追加
- System 消息由框架在模型调用时自动注入（不存入 history）

### 6.4 消息与 ResponseMeta

每次模型调用（Generate / Stream）的响应中包含 `ResponseMeta.Usage`：

```go
// 示例：从 assistant 消息中读取 provider 用量
if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
    promptTokens := msg.ResponseMeta.Usage.PromptTokens
    completionTokens := msg.ResponseMeta.Usage.CompletionTokens
}
```

**注意**：`schema.Message` 没有顶层 `Usage` 字段；`ChatModelAgentState` 也没有 `Usage` / `Message`（单数）字段。用量在 `state.Messages` 各条 assistant 消息的 `ResponseMeta.Usage` 上。

Token 用量在 Eino 各层（`schema` / `callbacks` / ChatModel 装饰器 / ADK middleware）的读取方式、调用路径与统计盲区，见 [组件生态指南 §9](./component-ecosystem-guide.md#9-token-用量各层读取方式对比)。

本项目的 `budgetMiddleware.AfterModelRewriteState` 通过读取 `lastAssistantMessage(state.Messages)` 的 `ResponseMeta.Usage` 收集 provider 用量。

---

## 7. state.Messages 的生命周期

### 7.1 消息的注入者

| 消息角色 | 注入者 | 时机 |
|----------|--------|------|
| System | 框架（`defaultGenModelInput`） | 每次模型调用前自动注入 |
| User | `RunLoop` | 用户输入后 `append(history, schema.UserMessage(prompt))` |
| Assistant（含 ToolCalls） | Agent ReAct 循环 | 模型返回后，通过 `AgentEvent` 输出 |
| Tool | Agent ReAct 循环 | 工具执行后，通过 `AgentEvent` 输出 |

### 7.2 中间件可操作的内容

在 `BeforeModelRewriteState` 中，中间件可修改以下状态字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `state.Messages` | `[]*schema.Message` | 发送给模型的完整消息列表（含 system） |
| `state.ToolInfos` | `[]*schema.ToolInfo` | 立即传给模型的工具 schema |
| `state.DeferredToolInfos` | `[]*schema.ToolInfo` | 延迟检索的工具（ToolSearchTool 使用） |

**重要约束**：修改 state 时必须拷贝后返回新指针，不能直接修改入参。

---

## 8. 本项目中的完整数据流

### 8.1 从用户输入到终端输出的全链路

```
1. promptReader.Receive(ctx)
   → 读取用户输入文本

2. history = append(history, schema.UserMessage(prompt))
   → 用户消息追加到对话历史

3. budgetWriter.BeginTurn()
   → budgetMiddleware.BeforeAgent 启动计时与用量聚合

4. iter := runner.Run(runCtx, history)
   → 触发 ChatModelAgent ReAct 循环

5. 每次模型调用前：
   ├─ contentMiddleware.BeforeModelRewriteState
   │    └─ compactor.CompactIfNeeded(ctx, state.Messages)
   │         ├─ 读取 SessionContext.TotalTokens → 超 80% 水位线?
   │         ├─ selectBoundary() → [head, middle, tail]
   │         ├─ 辅助模型 generateSummary(middle)
   │         └─ assembleCompactedMessages → 新 Messages
   └─ 修改后的 Messages 传给模型

6. 模型返回后：
   ├─ budgetMiddleware.AfterModelRewriteState
   │    └─ 从最后一条 AssistantMessage 读取 ResponseMeta.Usage
   └─ 如有 ToolCalls → 框架执行工具 → 回到步骤 5

7. ConsumeAgentEvents(iter, renderer)
   ├─ 逐条消费 AgentEvent
   ├─ 流式 chunk → renderStreamingMessage → renderer.Write(content)
   ├─ 完整消息 → renderCompleteMessage → renderer.EmitAgentEvent(...)
   └─ 收集 Assistant + Tool 消息到 turnMessages

8. history = append(history, turnMessages...)
   → 本轮回复追加到对话历史

9. usageMiddleware.WrapModel
   └─ 每次 Generate/Stream 完成后追加 Turn 到 ConversationRecorder

10. sessionContext.FinishConversation + renderer.WriteConversationStatus
    → 落 SQLite，并输出单行统计（如 "[Stats: elapsed=0.77s prompt↑=318 completion↓=37 content↑↓=318 0.25%(128K)]"；prompt↑/completion↓/content↑↓ 来自本次 Conversation 聚合，TTY 下分段着色）
```

### 8.2 中间件执行顺序

本项目注册了两个中间件，按洋葱模型执行：

```go
// 注册顺序：[contentMiddleware, usageMiddleware]
//                          ↑ 外层        ↑ 内层（更靠近模型）

Handlers: []adk.ChatModelAgentMiddleware{contentMiddleware, usageMiddleware}
```

对应的执行顺序：

```
BeforeAgent:
  contentMiddleware.BeforeAgent (no-op，继承 Base)
  usageMiddleware.BeforeAgent (no-op，继承 Base)

模型调用:
  contentMiddleware.BeforeModelRewriteState (上下文压缩)
  usageMiddleware.BeforeModelRewriteState (no-op)
  → usageMiddleware.WrapModel → 模型调用 → 记录 Turn
  usageMiddleware.AfterModelRewriteState (no-op)
  contentMiddleware.AfterModelRewriteState (no-op)

AfterAgent（成功结束时）:
  contentMiddleware.AfterAgent (no-op)
  usageMiddleware.AfterAgent (no-op)
```

---

## 9. 自定义工具接入示例

以本项目的天气工具为例，完整接入步骤：

### 步骤 1：定义参数和返回结构体

```go
type WeatherToolParams struct {
    City string `json:"city" jsonschema:"description=城市名称, required"`
    Lang string `json:"lang" jsonschema:"description=语言: zh/en, optional"`
}

type WeatherToolResult struct {
    Province    string `json:"province"`
    City        string `json:"city"`
    Weather     string `json:"weather"`
    Temperature int    `json:"temperature"`
    // ...
}
```

### 步骤 2：实现函数

```go
func getWeather(ctx context.Context, req *WeatherToolParams) (*WeatherToolResult, error) {
    // 校验参数
    // 调用 API
    // 返回结果
}
```

### 步骤 3：包装为 InvokableTool

```go
func GetWeatherTool() tool.InvokableTool {
    t, _ := utils.InferTool("get_weather", "天气工具, 获取指定城市的天气信息", getWeather)
    return t
}
```

### 步骤 4：注册到 AgentTools

```go
func NewAgentTools() adk.ToolsConfig {
    return adk.ToolsConfig{
        ToolsNodeConfig: compose.ToolsNodeConfig{
            Tools: []tool.BaseTool{
                weather.GetWeatherTool(),
            },
        },
    }
}
```

### 步骤 5：关联到 Agent

```go
agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
    // ...
    ToolsConfig: tools.NewAgentTools(),
})
```

注册后，工具的 JSON Schema 会自动包含在每次模型调用的 tool list 中，模型可以自主决定何时调用。

---

## 10. 上下文压缩机制

当 `SessionContext.TotalTokens()` 超过安全预算的 80%（`(maxModelContext - maxOutputTokens) * 80%`）时，触发压缩：

### 10.1 压缩策略

```
原始消息：[SYS, msg1, msg2, msg3, msg4, msg5, msg6, msg7, msg8, msg9]
                      │                                            │
                head (2条)                                     tail (4条)
                      ├────────── middle（被摘要） ──────────┤

压缩后：  [SYS, msg1, msg2, SUMMARY_USER_MSG, msg7, msg8, msg9]
```

- **Head**：保留对话开头的 2 条非 system 消息
- **Tail**：保留对话末尾的 4 条消息（从后往前数，避免拆散 tool call/result 对）
- **Middle**：调用辅助模型生成结构化摘要

### 10.2 摘要格式

辅助模型生成六段式摘要：

- **Goal**：对话目标
- **Constraints**：约束条件
- **Progress**：已完成进展
- **Decisions**：已做决策
- **Relevant Files**：相关文件
- **Next Steps**：后续步骤

摘要以 `[CONTEXT COMPACTION - REFERENCE ONLY]` 为前缀，以 UserMessage 形式插入（匿名，确保后续压缩轮次也能识别并保留）。

### 10.3 失败处理

摘要失败时**静默跳过**，返回原始消息不变。压缩器不会中断对话。

---

## 11. 关键类型速查

### 11.1 核心 interface

```go
// 聊天模型接口
type BaseChatModel interface {
    Generate(ctx, input []*schema.Message, opts ...Option) (*schema.Message, error)
    Stream(ctx, input []*schema.Message, opts ...Option) (*schema.StreamReader[*schema.Message], error)
}

// 工具接口
type InvokableTool interface {
    Info(ctx) (*schema.ToolInfo, error)
    InvokableRun(ctx, argumentsInJSON string, opts ...Option) (string, error)
}

// 中间件接口
type ChatModelAgentMiddleware interface {
    BeforeAgent(ctx, *ChatModelAgentContext) (context.Context, *ChatModelAgentContext, error)
    AfterAgent(ctx, *ChatModelAgentState) (context.Context, error)
    BeforeModelRewriteState(ctx, *ChatModelAgentState, *ModelContext) (context.Context, *ChatModelAgentState, error)
    AfterModelRewriteState(ctx, *ChatModelAgentState, *ModelContext) (context.Context, *ChatModelAgentState, error)
    WrapModel(ctx, model.BaseModel[*schema.Message], *ModelContext) (model.BaseModel[*schema.Message], error)
    WrapInvokableToolCall(ctx, InvokableToolCallEndpoint, *ToolContext) (InvokableToolCallEndpoint, error)
    WrapStreamableToolCall(ctx, StreamableToolCallEndpoint, *ToolContext) (StreamableToolCallEndpoint, error)
    // ... Enhanced 变体
}
```

### 11.2 本项目使用的 Eino 公共类型

| 包路径 | 关键类型 |
|--------|----------|
| `github.com/cloudwego/eino/adk` | `ChatModelAgent`, `ChatModelAgentConfig`, `AgentEvent`, `AgentOutput`, `MessageVariant`, `ChatModelAgentMiddleware`, `ChatModelAgentState`, `ModelContext`, `ToolContext`, `Runner`, `RunnerConfig`, `AsyncIterator`, `BaseChatModelAgentMiddleware` |
| `github.com/cloudwego/eino/schema` | `Message`, `ToolCall`, `FunctionCall`, `ToolInfo`, `StreamReader`, `RoleType`, `ResponseMeta`, `TokenUsage` |
| `github.com/cloudwego/eino/components/model` | `BaseChatModel`（即 `BaseModel[*schema.Message]`）, `Option`, `WithMaxTokens` |
| `github.com/cloudwego/eino/components/tool` | `BaseTool`, `InvokableTool`, `Option` |
| `github.com/cloudwego/eino/components/tool/utils` | `InferTool` |
| `github.com/cloudwego/eino/compose` | `ToolsNodeConfig` |

---

## 12. 调试技巧

### 12.1 观察事件流

在 `ConsumeAgentEvents` 循环中加入 `klog` 打印每个事件的类型和内容，可观测 ReAct 循环的实际步数。

### 12.2 查看 state.Messages 变化

在 `contentMiddleware.BeforeModelRewriteState` 中加入日志，对比压缩前后的消息数量与 `SessionContext.TotalTokens()`。

### 12.3 模拟 Agent 事件

使用 `adk.NewAsyncIteratorPair[*adk.AgentEvent]()` 创建测试用的事件迭代器，注入 mock 事件来验证 `ConsumeAgentEvents` 的行为（详见 `agent_events_test.go`）。

### 12.4 中间件单元测试

不需要真实 LLM 或完整 Agent，直接构造 `*adk.ChatModelAgentState` 并调用中间件钩子即可（详见 `content_test.go` 和 `budget_test.go`）。

---

## 13. 反模式

| 反模式 | 原因 | 正确做法 |
|--------|------|----------|
| 直接修改 `state.Messages` 切片 | 共享底层数组，可能污染其他 handler | 拷贝后返回新指针 |
| 在 `WrapModel` 中改 input messages | 不持久化，破坏 prompt cache | 使用 `BeforeModelRewriteState` |
| 在 `AfterAgent` 做错误恢复 | 失败路径不调用 `AfterAgent` | Runner 层或事件流中处理 |
| `state.Messages` 里手动加 SystemMessage | 与框架注入冲突 | 通过 `Instruction` 字段传入 |
| 忘记 clone ToolCalls 就存入 history | 共享底层 slice 导致数据竞争 | 使用 `cloneMessageForHistory` |

---

更多 API 细节见 [Eino ADK 官方仓库](https://github.com/cloudwego/eino/tree/main/adk)。
