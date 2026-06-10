# Eino 组件生态指南

本文档描述 [CloudWeGo Eino](https://github.com/cloudwego/eino) 在 **ChatModel + Agent** 场景下的完整组件生态：包分层、消息数据结构、调用链路、扩展点，以及 **Token 用量** 在各层的产生与读取方式。

适用版本：Eino **v0.9.4**（`github.com/cloudwego/eino v0.9.4`）。实现细节以该版本源码为准；升级大版本时请对照官方 changelog。

配套文档：

- [ChatModelAgent 工作机制](./chatmodelagent-guide.md)
- [ChatModelAgent 中间件实践](./middleware-guide.md)

---

## 1. 生态分层总览

Eino 不是单一「Agent SDK」，而是由多层可组合组件构成的栈。从下到上：

```
┌─────────────────────────────────────────────────────────────────┐
│  应用层（本项目）                                                 │
│  RunLoop → Runner.Run → ConsumeAgentEvents → history 管理        │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│  adk（Agent Development Kit）                                   │
│  ChatModelAgent · Runner · ChatModelAgentMiddleware · 事件流      │
└────────────────────────────┬────────────────────────────────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
┌───────▼───────┐  ┌─────────▼─────────┐  ┌──────▼──────┐
│ compose       │  │ components/tool   │  │ callbacks   │
│ Graph ·       │  │ BaseTool ·        │  │ OnStart/    │
│ ToolsNode     │  │ InvokableTool ·   │  │ OnEnd ·     │
│               │  │ InferTool         │  │ 流式回调     │
└───────┬───────┘  └─────────┬─────────┘  └──────┬──────┘
        │                    │                    │
        └────────────────────┼────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│  components/model                                                │
│  BaseChatModel · Option · CallbackInput/Output · TokenUsage       │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│  schema                                                          │
│  Message · RoleType · ToolCall · ResponseMeta · TokenUsage ·     │
│  StreamReader · ToolInfo · ConcatMessages                        │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│  eino-ext（可选实现层）                                           │
│  openai.ChatModel · embedding · 各厂商 ACL                       │
└─────────────────────────────────────────────────────────────────┘
```

**核心原则**：越靠近底层，越接近「一次 HTTP/API 调用」的边界；越靠近 `adk`，越接近「多轮 ReAct、状态持久化、事件流」的编排边界。

---

## 2. 包与类型速查

### 2.1 核心包

| 包路径 | 职责 | 典型入口类型 |
|--------|------|--------------|
| `github.com/cloudwego/eino/schema` | 消息、工具 schema、流、Token 结构 | `Message`, `ToolInfo`, `TokenUsage`, `StreamReader` |
| `github.com/cloudwego/eino/components/model` | 聊天模型抽象与调用选项 | `BaseChatModel`, `Option`, `CallbackInput`, `CallbackOutput` |
| `github.com/cloudwego/eino/callbacks` | 横切回调（计时、日志、计量） | `HandlerBuilder`, `RunInfo` |
| `github.com/cloudwego/eino/components/tool` | 工具抽象 | `BaseTool`, `InvokableTool`, `StreamableTool` |
| `github.com/cloudwego/eino/components/tool/utils` | 从 Go 函数生成工具 | `InferTool` |
| `github.com/cloudwego/eino/compose` | 图编排、ToolsNode | `Graph`, `ToolsNodeConfig` |
| `github.com/cloudwego/eino/adk` | ChatModelAgent、Runner、Middleware | `ChatModelAgent`, `Runner`, `ChatModelAgentMiddleware` |
| `github.com/cloudwego/eino-ext/components/model/openai` | OpenAI 兼容 ChatModel 实现 | `openai.ChatModel`, `openai.ChatModelConfig` |

### 2.2 两条消息类型轨道

Eino v0.9 同时支持两种消息泛型参数 `M`：

| 轨道 | 消息类型 | 模型接口 | Agent |
|------|----------|----------|-------|
| **经典 Chat**（本项目） | `*schema.Message` | `model.BaseChatModel` | `adk.ChatModelAgent` |
| **Agentic** | `*schema.AgenticMessage` | `model.AgenticModel` | `adk.TypedChatModelAgent[*schema.AgenticMessage]` |

泛型别名关系：

```go
type ChatModelAgentState     = TypedChatModelAgentState[*schema.Message]
type ChatModelAgentMiddleware = TypedChatModelAgentMiddleware[*schema.Message]
type ModelContext            = TypedModelContext[*schema.Message]
```

Agentic 轨道的用量字段为 `AgenticResponseMeta.TokenUsage`（非 `Usage`）。本文以下默认以 **`*schema.Message` 经典轨道** 为主。

---

## 3. schema 层：消息与 Token 的载体

### 3.1 Message 结构

`schema.Message` 是模型 **输入与输出** 的统一数据结构：

```go
type Message struct {
    Role             RoleType       // system / user / assistant / tool
    Content          string         // 文本正文
    ReasoningContent string         // thinking / reasoning（部分模型）
    ToolCalls        []ToolCall     // assistant 发出的工具调用
    ToolCallID       string         // tool 消息关联的 call ID
    ToolName         string         // tool 消息的工具名
    ResponseMeta     *ResponseMeta  // 模型响应元数据（含 Token 用量）
    Extra            map[string]any // 扩展（如 message ID）
    // ... 多模态字段 UserInputMultiContent / AssistantGenMultiContent
}
```

辅助构造函数：

```go
schema.SystemMessage(content)
schema.UserMessage(content)
schema.AssistantMessage(content, toolCalls)
schema.ToolMessage(content, toolCallID, opts...)
```

### 3.2 角色与典型排列

```
[System, User, Assistant, Tool, Assistant, Tool, Assistant, ...]
```

| 角色 | 常见来源 | 是否进入应用层 history |
|------|----------|------------------------|
| `system` | 框架将 `Instruction` 注入；或 history 中显式 system | 视应用而定；本项目 system 可由压缩保留 |
| `user` | 用户输入 | 由 RunLoop 管理 |
| `assistant` | 模型输出（含 tool_calls） | 是 |
| `tool` | 工具执行结果 | 是 |

### 3.3 ResponseMeta 与 TokenUsage

Provider 返回的用量挂在 **消息元数据** 上，而非 `Message` 顶层：

```go
type ResponseMeta struct {
    FinishReason string
    Usage        *TokenUsage   // ← 经典轨道的用量入口
    LogProbs     *LogProbs
}

type TokenUsage struct {
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
    PromptTokenDetails     PromptTokenDetails     // CachedTokens 等
    CompletionTokensDetails CompletionTokensDetails // ReasoningTokens 等
}
```

读取示例：

```go
if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
    u := msg.ResponseMeta.Usage
    _ = u.PromptTokens
    _ = u.CompletionTokens
}
```

**注意**：`schema.Message` **没有** `Usage` 顶层字段；不存在 `msg.Usage`，正确路径是 `msg.ResponseMeta.Usage`。

### 3.4 流式 Chunk 与合并

`Stream()` 返回 `*schema.StreamReader[*schema.Message]`。每个 chunk 也是 `*schema.Message`，但字段按语义分段：

- Reasoning chunk：`ReasoningContent` 非空，`Content` 可能为空
- Answer chunk：`Content` 非空
- 末尾 usage chunk：可能 **无正文**，仅携带 `ResponseMeta.Usage`（取决于实现）

流结束后用 `schema.ConcatMessages(chunks)` 合并为一条完整消息；合并逻辑会取各 chunk 中 **最大的** `PromptTokens` / `CompletionTokens` 等（见 `schema/message.go` 中 `ConcatMessages` 实现）。

`StreamReader` **只能读一次**；多消费者需 `Copy` 后再读。

---

## 4. components/model 层：ChatModel 抽象

### 4.1 BaseChatModel 接口

```go
type BaseChatModel interface {
    Generate(ctx context.Context, input []*schema.Message, opts ...Option) (*schema.Message, error)
    Stream(ctx context.Context, input []*schema.Message, opts ...Option) (*schema.StreamReader[*schema.Message], error)
}
```

等价于 `BaseModel[*schema.Message]`。所有厂商实现（OpenAI 兼容、Gemini 等）最终都要满足该接口。

### 4.2 调用选项 Option

每次 `Generate` / `Stream` 可通过 `model.Option` 覆盖行为。常用项（`model.Options`）：

| Option | 作用 |
|--------|------|
| `WithTemperature` | 采样温度 |
| `WithMaxTokens` | 最大生成 token |
| `WithModel` | 覆盖模型名 |
| `WithTools` | 本次请求的工具 schema 列表 |
| `WithDeferredTools` | 延迟加载工具（server-side tool search） |
| `WithToolChoice` | 强制/禁止工具调用 |
| `WithStop` | 停止词 |

ADK 在每次模型调用前，将 `state.ToolInfos` / `state.DeferredToolInfos` 转为 `model.WithTools` / `model.WithDeferredTools` 追加到 opts（state 优先于调用方 opts）。

### 4.3 ToolCallingChatModel

```go
type ToolCallingChatModel interface {
    BaseChatModel
    WithTools(tools []*schema.ToolInfo) (ToolCallingChatModel, error) // 返回新实例，不 mutate
}
```

已废弃的 `ChatModel.BindTools` 会原地修改实例，并发不安全。ADK 框架通过 **每次调用的 `model.WithTools` option** 绑定工具，而非依赖 `BindTools`。

### 4.4 实现层：eino-ext openai

本项目使用 `github.com/cloudwego/eino-ext/components/model/openai`：

```go
chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
    BaseURL:             cfg.BaseURL,
    Model:               cfg.Model,
    APIKey:              cfg.APIKey,
    MaxCompletionTokens: &maxOutput,
})
```

实现职责：

1. 将 `[]*schema.Message` 转为 OpenAI API 请求
2. 解析响应为 `*schema.Message`，填充 `ResponseMeta.Usage`
3. 触发 `callbacks.OnStart` / `callbacks.OnEnd`（或流式 `OnEndWithStreamOutput`）

非流式结束时同时填充 callback 与 message：

```go
callbacks.OnEnd(ctx, &model.CallbackOutput{
    Message:    outMsg,
    TokenUsage: toModelCallbackUsage(outMsg.ResponseMeta), // 与 ResponseMeta 同源
})
```

---

## 5. callbacks 层：横切观测

### 5.1 模型 Callback 载荷

```go
// components/model/callback_extra.go
type CallbackInput struct {
    Messages   []*schema.Message
    Tools      []*schema.ToolInfo
    ToolChoice *schema.ToolChoice
    Config     *Config
    Extra      map[string]any
}

type CallbackOutput struct {
    Message    *schema.Message
    Config     *Config
    TokenUsage *TokenUsage   // 顶层字段，便于 OnEnd 直接读取
    Extra      map[string]any
}
```

`model.TokenUsage` 与 `schema.TokenUsage` 字段对应，属于 callback 包的平行类型。

### 5.2 注册全局 Handler

```go
handler := callbacks.NewHandlerBuilder().
    OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
        mi := model.ConvCallbackInput(input)
        if mi != nil {
            _ = len(mi.Messages)
        }
        return ctx
    }).
    OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
        mo := model.ConvCallbackOutput(output)
        if mo != nil && mo.TokenUsage != nil {
            _ = mo.TokenUsage.TotalTokens
        }
        return ctx
    }).
    Build()
```

适用场景：**不经过 ADK**、直接调用 `BaseChatModel`，或需要全局拦截所有模型组件的观测/计量。

### 5.3 CallbackOutput 与 ResponseMeta 的关系

| 字段 | 层级 | 是否进入 state.Messages | 典型内容 |
|------|------|-------------------------|----------|
| `CallbackOutput.TokenUsage` | callback 顶层 | 否 | 与 ResponseMeta 同源的快照 |
| `Message.ResponseMeta.Usage` | 消息元数据 | 是（append 到 state 后） | provider 原始用量 |

OpenAI 实现中二者由同一次 API `usage` 字段派生。流式最后一帧可能出现 `Message == nil` 且 `TokenUsage != nil` 的纯用量包。

---

## 6. compose + tool 层：工具执行

### 6.1 工具接口层次

```go
type BaseTool interface { /* 标记接口 */ }

type InvokableTool interface {
    Info(ctx context.Context) (*schema.ToolInfo, error)
    InvokableRun(ctx context.Context, argumentsInJSON string, opts ...Option) (string, error)
}

type StreamableTool interface { /* StreamableRun → StreamReader[string] */ }
// EnhancedInvokableTool / EnhancedStreamableTool — 返回 *schema.ToolResult
```

`utils.InferTool` 从 Go 函数 + struct tag 自动生成 `InvokableTool` 与 JSON Schema。

### 6.2 ADK 工具配置

```go
adk.ToolsConfig{
    ToolsNodeConfig: compose.ToolsNodeConfig{
        Tools:               []tool.BaseTool{...},
        ToolCallMiddlewares: []compose.ToolMiddleware{...},
    },
}
```

工具调用链（由外到内）见 [middleware-guide.md §2.3](./middleware-guide.md#23-框架执行顺序)。

工具结果包装为 `role: tool` 的 `schema.Message`，带 `ToolCallID` 与 `ToolName`，追加到 `state.Messages` 后触发下一轮模型调用。

---

## 7. adk 层：Agent、State、Context

### 7.1 构造与运行

```go
agent, _ := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
    Model:       chatModel,      // model.BaseChatModel
    Instruction: systemPrompt,
    ToolsConfig: toolsConfig,
    Handlers:    []adk.ChatModelAgentMiddleware{...},
})

runner := adk.NewRunner(ctx, adk.RunnerConfig{
    Agent:           agent,
    EnableStreaming: true,
})

iter := runner.Run(ctx, history) // *adk.AsyncIterator[*adk.AgentEvent]
```

### 7.2 三种 Context / State 类型（勿混淆）

| 类型 | 生命周期 | 字段摘要 | 典型用途 |
|------|----------|----------|----------|
| `ChatModelAgentContext` | 整次 `Run()`，`BeforeAgent` 可改 | `Instruction`, `Tools`, `ReturnDirectly`, `ToolSearchTool` | 动态 system、增删可执行工具 |
| `ChatModelAgentState` | 每次模型调用前后，`Before/AfterModelRewriteState` 可改 | `Messages`, `ToolInfos`, `DeferredToolInfos` | 裁剪历史、动态 tool schema |
| `ModelContext` | 单次模型调用，`WrapModel` 只读 | `Tools`（Deprecated）, `ModelRetryConfig`, `ModelFailoverConfig` | 包装模型、读重试/故障转移配置 |

**重要**：`ChatModelAgentState` **没有** `Usage`、`Message`（单数）字段。用量不在 state 顶层，而在 `state.Messages` 里各条 assistant 消息的 `ResponseMeta.Usage` 上。

内部持久化结构 `adk.State`（`typedState`）比公开的 `ChatModelAgentState` 更多字段（迭代计数、ReturnDirectly 标志、checkpoint 等），中间件应只操作 `ChatModelAgentState` API。

### 7.3 ChatModelAgentMiddleware 全接口

```go
type ChatModelAgentMiddleware interface {
    BeforeAgent(ctx, *ChatModelAgentContext) (ctx, *ChatModelAgentContext, error)
    AfterAgent(ctx, *ChatModelAgentState) (ctx, error)

    BeforeModelRewriteState(ctx, *ChatModelAgentState, *ModelContext) (ctx, *ChatModelAgentState, error)
    AfterModelRewriteState(ctx, *ChatModelAgentState, *ModelContext) (ctx, *ChatModelAgentState, error)

    WrapModel(ctx, model.BaseChatModel, *ModelContext) (model.BaseChatModel, error)

    WrapInvokableToolCall(ctx, InvokableToolCallEndpoint, *ToolContext) (InvokableToolCallEndpoint, error)
    WrapStreamableToolCall(ctx, StreamableToolCallEndpoint, *ToolContext) (StreamableToolCallEndpoint, error)
    WrapEnhancedInvokableToolCall(...)
    WrapEnhancedStreamableToolCall(...)
}
```

嵌入 `*adk.BaseChatModelAgentMiddleware` 即可获得全部 no-op 默认实现。

### 7.4 旧版 AgentMiddleware（Deprecated）

`ChatModelAgentConfig.Middlewares []AgentMiddleware` 仍可用，但仅适合：

- `AdditionalInstruction` / `AdditionalTools` 静态追加
- `BeforeChatModel` / `AfterChatModel` 简单闭包（**不能**返回修改后的 context）

新逻辑应写在 `Handlers []ChatModelAgentMiddleware`。

### 7.5 Run 级上下文存储

```go
adk.SetRunLocalValue(ctx, key, value)  // 当前 Run 有效，支持 interrupt/resume
adk.GetRunLocalValue(ctx, key)
adk.TypedSendEvent(ctx, event)         // 向事件流注入自定义 AgentEvent
```

自定义类型需 `schema.RegisterName[T]()` 以支持 checkpoint gob 序列化。

### 7.6 内置框架包装器（用户不可直接 new，但需知晓顺序）

模型调用链上的内置层（详见 [middleware-guide.md](./middleware-guide.md)）：

- `failoverModelWrapper` / `retryModelWrapper`
- `eventSenderModelWrapper`（默认发 AgentEvent）
- `callbackInjectionModelWrapper`
- 用户 `WrapModel` 装饰器

---

## 8. 端到端数据流：一次模型 Hop

以下描述 ReAct 循环内 **单次** `Generate` / `Stream` 的完整路径：

```
1. 从 typedState 读出 Messages / ToolInfos / DeferredToolInfos
        ↓
2. 组装 ChatModelAgentState
        ↓
3. BeforeModelRewriteState（用户 Handlers，可改 Messages / ToolInfos）
        ↓
4. 持久化 state → typedState
        ↓
5. 追加 model.WithTools / WithDeferredTools 到 opts
        ↓
6. WrapModel 链 → 内置 retry/failover/event/callback 包装
        ↓
7. chatModel.Generate/Stream(state.Messages, opts...)
        │   ├─ callbacks.OnStart(CallbackInput{Messages, Tools, ...})
        │   ├─ HTTP/API 调用
        │   └─ callbacks.OnEnd(CallbackOutput{Message, TokenUsage})
        ↓
8. 返回 *schema.Message（含 ResponseMeta.Usage）
        ↓
9. append 到 state.Messages
        ↓
10. AfterModelRewriteState（用户 Handlers，可读最后一条 assistant）
        ↓
11. 持久化 state → typedState
        ↓
12. 若无 ToolCalls → 结束循环；否则执行工具 → 回到步骤 1
```

**Instruction 注入点**：在步骤 5 之前，框架 `genModelInput` 将 `Instruction` 转为 `SystemMessage`  prepend 到发给模型的消息列表。该 system 消息 **不在** `state.Messages` 槽位中单独维护，但会出现在实际 API 请求里。

---

## 9. Token 用量：各层读取方式对比

### 9.1 四种扩展点

| 层级 | 机制 | 读取用量方式 | 覆盖范围 |
|------|------|--------------|----------|
| **schema** | 读返回值 | `msg.ResponseMeta.Usage` | 持有 `*schema.Message` 的任意代码 |
| **callbacks** | 全局 `OnEnd` | `model.ConvCallbackOutput(out).TokenUsage` 或 `.Message.ResponseMeta.Usage` | 所有启用 callback 的 model 调用 |
| **ChatModel 装饰器** | 包装 `BaseChatModel` | `Generate`/`Stream` 返回后读 `ResponseMeta` | 所有经过该实例的调用（含旁路直调） |
| **ADK Middleware** | `AfterModelRewriteState` | `lastAssistantMessage(state.Messages).ResponseMeta.Usage` | 仅 Agent ReAct 主循环内的 hop |

### 9.2 调用路径与统计盲区

同一 `BaseChatModel` 实例可能被多处持有：

```
chatModel（裸实例或装饰实例）
  ├─ ChatModelAgent ReAct 循环     → 经过 ADK 包装链 + AfterModelRewriteState
  ├─ 应用代码直接 model.Generate   → 仅 callbacks / 装饰器可统计
  └─ Compactor 等辅助逻辑直调       → 同上；不经过 Agent middleware
```

因此：

- **Agent middleware 统计**无法自动覆盖「绕过 Agent 的直调」。
- **ChatModel 装饰器**或 **全局 callbacks** 才能统一计量所有 API 调用。
- **压缩摘要**等辅助模型调用若与主 Agent 共用裸 `chatModel`，在仅使用 `AfterModelRewriteState` 时会被漏计。

### 9.3 流式 Token 读取注意点

1. 中间 chunk 通常 **没有** 完整 `Usage`。
2. 以 **合并后的最终 message** 或 **callback 最后一帧** 为准。
3. `schema.ConcatMessages` 对 usage 取 max，而非累加（避免重复计数）。

### 9.4 Provider 不可用时的回退

部分本地模型 / 兼容端点不返回 `usage`，此时 `ResponseMeta.Usage == nil`。应用层常见回退：

- 本地 `TokenEstimator`（tiktoken / 字符估算）对 `state.Messages` + tools + instruction 估算
- 在 UI 标注 `source: estimated` 与 `source: provider` 区分

---

## 10. 扩展机制选型指南

```
需求                                    推荐层级
────────────────────────────────────────────────────────────
改对话历史 / 压缩上下文                  ADK BeforeModelRewriteState
改模型可见 tool schema（不动可执行集）    ADK BeforeModelRewriteState → ToolInfos
改 system / 可执行工具（整次 Run）       ADK BeforeAgent → ChatModelAgentContext
包装单次模型调用（计时、限流）            ADK WrapModel 或 ChatModel 装饰器
统计所有 model API 调用（含旁路）        ChatModel 装饰器 或 callbacks OnEnd
拦截工具执行                             ADK Wrap*ToolCall
消费流式 UI 事件                          Runner → AgentEvent 迭代器
全局日志 / 追踪                          callbacks.HandlerBuilder
```

### 10.1 WrapModel vs ChatModel 装饰器 vs AfterModelRewriteState

| 维度 | `WrapModel` | 构造时装饰 `BaseChatModel` | `AfterModelRewriteState` |
|------|-------------|---------------------------|--------------------------|
| 注册位置 | Agent Handlers | `NewChatModel` 之后包一层 | Agent Handlers |
| 覆盖旁路直调 | 否（仅 Agent 内） | **是**（持有同一实例即可） | 否 |
| 改 input messages | 不推荐（不持久化） | 可调 input，但同样不写入 state | **推荐**（`BeforeModelRewriteState`） |
| 读 output usage | Generate/Stream 返回时 | 同上 | 从 `state.Messages` 刮取 |
| 与 prompt cache | WrapModel 改 input 会破坏 | 同左 | 改 state 是正确位置 |

### 10.2 反模式（生态层）

| 反模式 | 原因 |
|--------|------|
| 假设 `ChatModelAgentState` 有 `Usage` / `Message` | 该类型仅有 `Messages` 切片 |
| 使用 `msg.Usage` | 应使用 `msg.ResponseMeta.Usage` |
| 在 `WrapModel` 中持久化改 messages | 改动不写入 typedState |
| 用 `ChatModel.BindTools` 共享实例 | 并发不安全；用 `model.WithTools` option |
| 仅在 `AfterModelRewriteState` 统计且 Compactor 直调裸 model | 辅助调用漏计 |
| 流式中途 chunk 累加 Usage | 应等 ConcatMessages 或最终 callback 帧 |

---

## 11. 事件流与消息的关系

`Runner.Run` 产出 `*adk.AgentEvent`，与 `state.Messages` **并行**存在：

```go
type AgentEvent struct {
    AgentName string
    Err       error
    Action    *AgentAction
    Output    *AgentOutput
}

type MessageVariant struct {
    IsStreaming   bool
    Message       *schema.Message                // 非流式
    MessageStream *schema.StreamReader[*schema.Message] // 流式
    Role          schema.RoleType
    ToolName      string
}
```

- **事件流**：面向 UI / 日志的增量消费；流式时 `MessageStream` 逐 chunk 读取。
- **state.Messages**：面向下一轮模型输入的对话状态；模型 hop 结束后 append assistant/tool 消息。

`MessageVariant` 中的 message **可能**含 `ResponseMeta.Usage`（非流式完整消息）；流式场景下 usage 通常在流结束后才完整。应用层若要做回合统计，更可靠的是 **模型返回合并后** 或 **callback OnEnd** 读取，而非依赖每个 UI 事件 chunk。

---

## 12. Eino 官方内置 Middleware 参考

`github.com/cloudwego/eino/adk/middlewares/` 提供可复用的官方中间件（可直接作 Handlers 注册或作实现参考）：

| 包 | 功能 |
|----|------|
| `summarization` | 上下文摘要压缩 |
| `reduction` | 工具结果裁剪 / token 减负 |
| `filesystem` | 文件系统工具集成 |
| `skill` | Skill 加载 |
| `patchtoolcalls` | 修复 tool call 格式 |
| `dynamictool/toolsearch` | 动态工具检索 |

均属 `TypedChatModelAgentMiddleware[M]` 实现，遵循与本文相同的 state / 钩子契约。

---

## 13. 与本项目的关系（非本文重点）

本项目组合方式：

```
openai.NewChatModel
  → adk.NewChatModelAgent（Handlers: content + budget middleware）
  → adk.NewRunner（EnableStreaming: true）
  → RunLoop 管理 history + BudgetWriter（context 传递）
```

- **压缩**：`contentMiddleware` → `BeforeModelRewriteState` → `Compactor.CompactIfNeeded`（**直调** `model.Generate`）
- **用量**：`budgetMiddleware` → `AfterModelRewriteState` 读 `ResponseMeta.Usage`；`AfterAgent` 本地估算回退

架构讨论结论：若需覆盖 Compactor 等旁路调用，应将计量下沉至 **ChatModel 装饰器** 或 **callbacks**，Agent 层保留压缩与回合展示。详见各 `internal/` 包实现。

---

## 14. 调试清单

| 目标 | 手段 |
|------|------|
| 确认单次 hop 发了哪些 messages | `BeforeModelRewriteState` 打日志 `len(state.Messages)` + 末条 role |
| 确认 provider 是否返回 usage | 打印 `lastAssistantMessage(...).ResponseMeta` |
| 区分流式 / 非流式 | `RunnerConfig.EnableStreaming` + 事件 `IsStreaming` |
| 验证 callback 链路 | 注册 `OnEndFn` 打印 `mo.TokenUsage` |
| 验证旁路直调是否被统计 | 对 Compactor 调用的 model 打 wrapper 日志 |

---

## 15. 相关链接

- [Eino 主仓库](https://github.com/cloudwego/eino)
- [Eino ADK 源码](https://github.com/cloudwego/eino/tree/main/adk)
- [Eino-Ext OpenAI ChatModel](https://github.com/cloudwego/eino-ext/tree/main/components/model/openai)
- [本项目 ChatModelAgent 指南](./chatmodelagent-guide.md)
- [本项目 Middleware 指南](./middleware-guide.md)
