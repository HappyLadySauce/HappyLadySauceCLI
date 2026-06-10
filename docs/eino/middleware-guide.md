# Eino ChatModelAgent 中间件使用指南

本文档说明如何在 [CloudWeGo Eino ADK](https://github.com/cloudwego/eino) 中为 `ChatModelAgent` 编写、注册与测试中间件（Handler）。

适用版本：Eino ADK v0.9.x 及相近版本。`ChatModelAgentConfig.Handlers` 为推荐方式；`Middlewares`（`AgentMiddleware` 结构体）已标记为 Deprecated。

相关文档：

- [Eino 文档索引](./README.md)
- [Eino 组件生态指南](./component-ecosystem-guide.md) — 扩展点对比、Token 用量各层读取、调用路径与统计盲区
- [ChatModelAgent 工作机制](./chatmodelagent-guide.md)

---

## 1. 概述

`ChatModelAgent` 通过 ReAct 循环运行：模型生成 →（可选）执行工具 → 再次调用模型，直到给出最终回答或触发 return-directly 工具。

中间件让你在循环的各阶段插入逻辑，而无需修改框架源码。Eino 提供两套扩展机制：

| 机制 | 配置字段 | 类型 | 适用场景 |
|------|----------|------|----------|
| **Handlers（推荐）** | `ChatModelAgentConfig.Handlers` | `[]adk.ChatModelAgentMiddleware` | 修改 state、传播 context、包装模型/工具调用 |
| **Middlewares（旧）** | `ChatModelAgentConfig.Middlewares` | `[]adk.AgentMiddleware` | 静态追加 instruction/tools、简单闭包钩子 |

类型别名：

```go
type ChatModelAgentMiddleware = TypedChatModelAgentMiddleware[*schema.Message]
type ChatModelAgentState     = TypedChatModelAgentState[*schema.Message]
type ModelContext            = TypedModelContext[*schema.Message]
```

**优先使用 Handlers**。仅在「固定加一段 system 文案 / 固定多挂几个 tool」等简单场景，才考虑旧的 `AgentMiddleware`。

### 为什么用接口而不是结构体？

`AgentMiddleware` 是闭包结构体，回调只能返回 `error`，无法返回修改后的 `context.Context`，也不支持在实现体上挂自定义方法。

`ChatModelAgentMiddleware` 是开放接口：

- 钩子返回 `(context.Context, ..., error)`，context 可沿调用链传播
- `WrapModel` / `Wrap*ToolCall` 支持装饰器链
- 配置集中在 struct 字段，便于测试与复用

---

## 2. 生命周期

```
BeforeAgent
  └─ loop（每次模型调用）
       BeforeModelRewriteState
       WrapModel → Model.Generate / Stream
       AfterModelRewriteState
       └─ 若有 tool calls
            Wrap*ToolCall → Tool.Run
AfterAgent（仅成功结束时）
```

### 2.1 钩子职责

| 钩子 | 调用时机 | 主要可操作对象 | 是否持久化到 state |
|------|----------|----------------|-------------------|
| `BeforeAgent` | 每次 `Run()` 开始前 | `ChatModelAgentContext`：`Instruction`、`Tools`、`ReturnDirectly`、`ToolSearchTool` | 整次 Run |
| `AfterAgent` | 成功结束 | `ChatModelAgentState`（通常只读） | — |
| `BeforeModelRewriteState` | **每次**调模型前 | `state.Messages`、`state.ToolInfos`、`state.DeferredToolInfos` | ✅ |
| `AfterModelRewriteState` | **每次**调模型后 | 同上（含模型最新回复） | ✅ |
| `WrapModel` | 每次模型调用 | 包装 `model.BaseModel` | ❌ 仅单次调用 |
| `WrapInvokableToolCall` | 同步普通工具 | 包装 `InvokableToolCallEndpoint` | ❌ |
| `WrapStreamableToolCall` | 流式普通工具 | 包装 `StreamableToolCallEndpoint` | ❌ |
| `WrapEnhancedInvokableToolCall` | 同步增强工具 | 包装 `EnhancedInvokableToolCallEndpoint` | ❌ |
| `WrapEnhancedStreamableToolCall` | 流式增强工具 | 包装 `EnhancedStreamableToolCallEndpoint` | ❌ |

`ToolContext` 在工具包装时提供 `Name`（工具名）和 `CallID`（本次调用 ID）。

### 2.2 重要约束

1. **改消息或改工具列表** → 使用 `BeforeModelRewriteState`。不要在 `WrapModel` 里改 input messages 或 `model.WithTools`（改动不持久化，且会破坏 prompt cache）。
2. **`AfterAgent` 不是 finally**：超迭代（`ErrExceedMaxIterations`）、context 取消、模型错误等失败路径**不会**调用。
3. **Fail-fast**：同阶段任一 handler 返回 `error`，后续 handler 不再执行，错误进入事件流。
4. **注册顺序**：Handlers 切片中**先注册的更靠外**（洋葱模型）。`[A, B, C]` → `A(B(C(model)))`。

### 2.3 框架执行顺序

#### 模型调用（由外到内）

```
1.  AgentMiddleware.BeforeChatModel
2.  ChatModelAgentMiddleware.BeforeModelRewriteState
3.  failoverModelWrapper（若配置 ModelFailoverConfig）
4.  retryModelWrapper（若配置 ModelRetryConfig）
5.  eventSenderModelWrapper（内置）
6.  ChatModelAgentMiddleware.WrapModel（用户；先注册 = 最外层）
7.  callbackInjectionModelWrapper
8.  failoverProxyModel / Model.Generate|Stream
9.  ChatModelAgentMiddleware.AfterModelRewriteState
10. AgentMiddleware.AfterChatModel
```

#### 工具调用（由外到内）

```
eventSenderToolWrapper
  → ToolsConfig.ToolCallMiddlewares
  → AgentMiddleware.WrapToolCall
  → ChatModelAgentMiddleware.Wrap*ToolCall（先注册 = 最外层）
  → callbackInjectedToolCall
  → Tool.InvokableRun / StreamableRun
```

Handlers 在 `Middlewares` **之后**处理，按注册顺序串联。

---

## 3. 注册方式

创建 Agent 时通过 `Handlers` 传入：

```go
agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
    Name:        "assistant",
    Description: "A helpful assistant",
    Model:       chatModel,
    Instruction: "You are a helpful assistant.",
    ToolsConfig: adk.ToolsConfig{
        ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools},
    },
    Handlers: []adk.ChatModelAgentMiddleware{
        loggingHandler,
        trimHandler,
    },
})
```

多个 handler 时注意顺序含义：

```go
Handlers: []adk.ChatModelAgentMiddleware{
    auditHandler,   // 外层：最后接触到模型输出
    metricsHandler, // 内层：更靠近真实模型调用
}
```

与旧版 `Middlewares` 可共存；新逻辑应写在 `Handlers` 中。

---

## 4. 实现模板

嵌入 `*adk.BaseChatModelAgentMiddleware` 即可获得全部方法的 no-op 默认实现，只覆盖需要的钩子：

```go
package mymiddleware

import (
    "context"
    "log"

    "github.com/cloudwego/eino/adk"
    "github.com/cloudwego/eino/components/model"
    "github.com/cloudwego/eino/schema"
)

type LoggingHandler struct {
    *adk.BaseChatModelAgentMiddleware
}

func NewLoggingHandler() adk.ChatModelAgentMiddleware {
    return &LoggingHandler{
        BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
    }
}

func (h *LoggingHandler) BeforeModelRewriteState(
    ctx context.Context,
    state *adk.ChatModelAgentState,
    mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
    log.Printf("model call with %d messages", len(state.Messages))
    return ctx, state, nil
}
```

修改 state 时建议拷贝后返回新指针，避免意外共享：

```go
func (h *TrimHandler) BeforeModelRewriteState(
    ctx context.Context,
    state *adk.ChatModelAgentState,
    mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
    if state == nil || len(state.Messages) == 0 {
        return ctx, state, nil
    }
    trimmed := trimMessages(state.Messages, h.maxMessages)
    if len(trimmed) == len(state.Messages) {
        return ctx, state, nil
    }
    next := *state
    next.Messages = trimmed
    return ctx, &next, nil
}
```

---

## 5. Context 与 Run 级状态

### 5.1 通过 context 传递数据

钩子可返回新的 `context.Context`，后续钩子与同次 Run 内的包装器均可读取：

```go
type statsKey struct{}

type Stats struct {
    PromptTokens int
    CallCount    int
}

func WithStats(ctx context.Context) (context.Context, *Stats) {
    s := &Stats{}
    return context.WithValue(ctx, statsKey{}, s), s
}

func StatsFrom(ctx context.Context) *Stats {
    s, _ := ctx.Value(statsKey{}).(*Stats)
    return s
}

func (h *MetricsHandler) BeforeAgent(
    ctx context.Context,
    runCtx *adk.ChatModelAgentContext,
) (context.Context, *adk.ChatModelAgentContext, error) {
    ctx, _ = WithStats(ctx)
    return ctx, runCtx, nil
}

func (h *MetricsHandler) AfterModelRewriteState(
    ctx context.Context,
    state *adk.ChatModelAgentState,
    mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
    if stats := StatsFrom(ctx); stats != nil {
        stats.CallCount++
        // 从 state.Messages 最后一条 assistant 消息读取 usage
    }
    return ctx, state, nil
}
```

调用 `runner.Run(ctx, messages)` 时，把带统计器的 ctx 传入即可在 Run 结束后读取。

### 5.2 Eino 内置 Run 级存储

在 handler 执行期间可使用：

- `adk.SetRunLocalValue(ctx, key, value)` — 当前 `Run()` 内有效，支持 interrupt/resume
- `adk.GetRunLocalValue(ctx, key)`

自定义类型需在 `init()` 中调用 `schema.RegisterName[T]()` 以支持 gob 序列化。

---

## 6. 常见场景

### 6.1 修改 system prompt 或工具集（整次 Run）

**钩子**：`BeforeAgent`

```go
func (h *DynamicPromptHandler) BeforeAgent(
    ctx context.Context,
    runCtx *adk.ChatModelAgentContext,
) (context.Context, *adk.ChatModelAgentContext, error) {
    next := *runCtx
    next.Instruction = runCtx.Instruction + "\n\n" + h.extraInstruction
    next.Tools = append(append([]tool.BaseTool(nil), runCtx.Tools...), h.extraTools...)
    return ctx, &next, nil
}
```

影响：工具定义与**实际可执行**工具同时改变，持续整次 Run。

### 6.2 动态过滤模型可见工具（按轮次）

**钩子**：`BeforeModelRewriteState`

改 `state.ToolInfos`（立即传给模型的工具 schema）和/或 `state.DeferredToolInfos`（延迟检索工具）。改动会写入 agent state，影响后续所有模型调用。

适合：根据对话进度隐藏危险工具、按意图缩减 tool schema 体积。

### 6.3 历史消息裁剪 / Token 预算

**钩子**：`BeforeModelRewriteState`

改 `state.Messages`。常见策略：

- 保留最新一条 user 消息
- 按条数或 token 预算从头部删除旧消息
- 将早期对话摘要为一条 system 消息

### 6.4 包装模型调用（计时、日志、自定义重试）

**钩子**：`WrapModel`

```go
func (h *TimingHandler) WrapModel(
    ctx context.Context,
    next model.BaseModel[*schema.Message],
    mc *adk.ModelContext,
) (model.BaseModel[*schema.Message], error) {
    if next == nil {
        return next, nil
    }
    return &timedModel{next: next, clock: h.clock}, nil
}

type timedModel struct {
    next  model.BaseModel[*schema.Message]
    clock func() time.Time
}

func (m *timedModel) Generate(
    ctx context.Context,
    input []*schema.Message,
    opts ...model.Option,
) (*schema.Message, error) {
    start := m.clock()
    msg, err := m.next.Generate(ctx, input, opts...)
    recordDuration(ctx, time.Since(start))
    return msg, err
}

func (m *timedModel) Stream(
    ctx context.Context,
    input []*schema.Message,
    opts ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
    start := m.clock()
    stream, err := m.next.Stream(ctx, input, opts...)
    recordDuration(ctx, time.Since(start))
    return stream, err
}
```

框架内置的 `ModelRetryConfig`、`ModelFailoverConfig` 应优先于手写重试逻辑。

### 6.5 工具调用拦截（审批、审计、限流）

**钩子**：`WrapInvokableToolCall`（或对应 Streamable / Enhanced 变体）

```go
func (h *ApprovalHandler) WrapInvokableToolCall(
    ctx context.Context,
    endpoint adk.InvokableToolCallEndpoint,
    tCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
    return func(ctx context.Context, args string, opts ...tool.Option) (string, error) {
        if err := h.approve(ctx, tCtx.Name, args); err != nil {
            return "", err
        }
        return endpoint(ctx, args, opts...)
    }, nil
}
```

每种 `Wrap*ToolCall` 仅对实现了对应接口的工具生效。

### 6.6 控制事件流内容

默认情况下，模型事件在**所有**用户 `WrapModel` 处理完之后发送（内容为修改后的消息）。

若需要事件携带**原始**模型输出，在 Handlers **最后**（最内层）追加：

```go
adk.NewEventSenderModelWrapper()
```

工具事件同理，使用 `adk.NewEventSenderToolWrapper()`，其位置决定事件反映哪一层 handler 的处理结果。框架检测到用户显式注册后会跳过内置 event sender，避免重复事件。

### 6.7 成功结束后的收尾

**钩子**：`AfterAgent`

适用于：落库、发送完成通知、写入 session。不要依赖它做错误恢复。

---

## 7. 工具列表：两种修改方式

| 方式 | 钩子 | 修改对象 | 影响范围 |
|------|------|----------|----------|
| 改可执行工具集 | `BeforeAgent` | `ChatModelAgentContext.Tools` | 整次 Run；schema 与执行一致 |
| 改模型可见 schema | `BeforeModelRewriteState` | `state.ToolInfos`、`state.DeferredToolInfos` | 当前及后续模型调用（推荐用于动态过滤） |

**不推荐**在 `WrapModel` 中通过 `model.WithTools` 改工具列表。

---

## 8. 测试

中间件钩子可单独单元测试，无需真实 LLM 或完整 Agent：

```go
func TestTrimHandlerKeepsLatestUser(t *testing.T) {
    h := NewTrimHandler(TrimConfig{MaxMessages: 2})

    state := &adk.ChatModelAgentState{
        Messages: []*schema.Message{
            schema.UserMessage("old"),
            schema.AssistantMessage("reply", nil),
            schema.UserMessage("latest"),
        },
    }

    _, out, err := h.BeforeModelRewriteState(context.Background(), state, nil)
    if err != nil {
        t.Fatal(err)
    }
    last := out.Messages[len(out.Messages)-1]
    if last.Content != "latest" {
        t.Fatalf("got %q, want latest user kept", last.Content)
    }
}
```

`WrapModel` 可 mock `model.BaseModel` 验证 `Generate` / `Stream` 包装行为。`Wrap*ToolCall` 可传入记录调用的 fake endpoint。

建议覆盖：

- 未超限时 state 不变
- 超限时保留关键消息（如最新 user）
- error 路径正确传播
- 多 handler 串联时的顺序假设

---

## 9. 反模式

| 反模式 | 原因 | 正确做法 |
|--------|------|----------|
| 在 `WrapModel` 中改 input messages | 不持久化，破坏 cache | `BeforeModelRewriteState` |
| 在 `WrapModel` 中 `model.WithTools` | 仅单次生效 | `BeforeModelRewriteState` 改 `ToolInfos` |
| 在 `AfterAgent` 做失败清理 | 失败路径不调用 | Runner 层或事件流 |
| handler 内 panic | 中断整次 Run | 返回 `error` |
| 未文档化的 handler 顺序依赖 | 洋葱模型难推理 | 明确注释顺序；审计放外层、计量放内层 |

---

## 10. 快速决策树

```
需要改 system prompt / 固定加 tool（整次 Run）？
  └─ BeforeAgent（或 AgentMiddleware.AdditionalInstruction / AdditionalTools）

需要每轮裁剪历史 / 动态过滤工具 schema？
  └─ BeforeModelRewriteState

需要记录 token、耗时、写审计日志？
  └─ 优先 ChatModel 装饰器或 callbacks OnEnd（覆盖旁路直调）；
     Agent 内可用 AfterModelRewriteState + WrapModel + context 传递
     （详见 [组件生态指南 §9–§10](./component-ecosystem-guide.md#9-token-用量各层读取方式对比)）

需要拦截或包装工具执行？
  └─ WrapInvokableToolCall / WrapEnhanced*

需要重试或切换备用模型？
  └─ ModelRetryConfig / ModelFailoverConfig，必要时再补 WrapModel

仅在成功结束时收尾？
  └─ AfterAgent
```

更多 API 细节见 [Eino ADK 官方仓库](https://github.com/cloudwego/eino/tree/main/adk)。
