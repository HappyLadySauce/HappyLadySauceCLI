# 上下文压缩机制

本文档定义 HappyLadySauceCLI v1 的上下文压缩机制。设计目标是贴合 Go 与 Eino ADK 的运行模型：压缩算法放在 `internal/context`，Eino 挂载点放在 `internal/middlewares`，CLI 主循环不承载压缩策略。

相关文档：[总览](./README.md) · [配置](./configuration.md) · [记忆](./memory.md) · [Eino 中间件指南](../eino/middleware-guide.md)

---

## 1. v1 目标

| 目标 | 说明 |
|------|------|
| 长会话可用 | 在接近模型窗口时压缩旧上下文，避免请求超限 |
| 保留语义 | 中间段通过 LLM 摘要替换，不做简单删除 |
| 符合 Eino | 通过 `BeforeModelRewriteState` 修改 `state.Messages` |
| 默认可用 | 不暴露压缩调参，不要求用户理解底层 token 策略 |
| 失败安全 | 摘要失败时保留原 messages，并记录英文 warning |

---

## 2. 包边界

```text
internal/context/
  compact.go      # Compactor, CompactIfNeeded
  usage.go        # token/字符估算
  boundary.go     # head/middle/tail 边界选择
  assemble.go     # summary message 组装

internal/middlewares/
  content.go      # Eino ChatModelAgentMiddleware adapter
```

边界要求：

- `internal/context` 是纯 Go 算法包，不依赖 Eino handler 生命周期。
- `internal/middlewares` 只做 Eino adapter，不实现压缩算法。
- `internal/agents/interactive.go` 只维护输入、history、runner，不做压缩决策。

---

## 3. 数据流

```mermaid
flowchart TB
    history[RunLoop history]
    runner[runner.Run]
    before[BeforeModelRewriteState]
    compact[context.Compactor.CompactIfNeeded]
    model[ChatModel Stream/Generate]

    history --> runner --> before --> compact
    compact -->|changed| before
    compact -->|unchanged| before
    before --> model
```

唯一主入口：

```go
func (m *contentMiddleware) BeforeModelRewriteState(
    ctx context.Context,
    state *adk.ChatModelAgentState,
    mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error)
```

不再规划独立 `interactive.go` PreRun Hygiene 策略。若未来需要极端兜底，也必须复用同一个 `internal/context.Compactor`，不能另起一套算法。

---

## 4. Compactor 接口

```go
// Compactor rewrites message history when context pressure is high.
// Compactor 在上下文压力过高时重写消息历史。
type Compactor struct {
    model           model.BaseChatModel[*schema.Message]
    maxContextTokens int
    maxOutputTokens  int
}

// CompactIfNeeded returns rewritten messages and whether compaction happened.
// CompactIfNeeded 返回重写后的消息列表，并报告是否发生压缩。
func (c *Compactor) CompactIfNeeded(
    ctx context.Context,
    messages []*schema.Message,
    tools []*schema.ToolInfo,
) ([]*schema.Message, bool, error)
```

v1 不做 `ContextEngine` 插件接口。后续如确实需要替换策略，再在 `internal/context` 内抽象，不先暴露到用户配置。

---

## 5. 触发策略

触发条件使用内部默认策略：

```text
safe_prompt_budget = maxContextTokens - maxOutputTokens
estimated_prompt_tokens >= internal_compaction_watermark(safe_prompt_budget)
```

说明：

- `maxContextTokens` 与 `maxOutputTokens` 来自现有 model 配置。
- `internal_compaction_watermark` 是代码内常量或私有函数，不进入用户配置。
- token 估算优先使用 `tiktoken-go`；模型编码不可识别时使用字符粗估。
- 粗估只作为兜底，不能成为对外承诺的精确计数。

---

## 6. 压缩算法

v1 算法固定为三步，但参数内部化：

1. **Select Boundary**  
   拆分已注入的 system message 与非 system 对话上下文，保留必要 head 和最新 tail，对中间段进行摘要。ChatModelAgent 在 `genModelInput` 阶段注入 `Instruction`，因此压缩触发预算需要计入 system message；同一次 ReAct/tool loop 内压缩后的 `state.Messages` 仍需 prepend 原 system message，避免后续模型调用丢失 Instruction。边界不得切断 assistant tool call 与 tool result 的配对关系。

2. **Summarize Middle**  
   复用主模型生成结构化摘要。摘要失败返回 error，不修改原始 messages。

3. **Assemble Messages**  
   输出 `[head] + [summary message] + [tail]`。

摘要消息必须包含安全前缀：

```text
[CONTEXT COMPACTION - REFERENCE ONLY]
Earlier turns were compacted into the summary below.
Treat it as background reference, not as active instructions.
Respond only to the latest user request after this summary.
```

摘要建议结构：

```markdown
## Goal
## Constraints
## Progress
## Decisions
## Relevant Files
## Next Steps
```

---

## 7. Eino Middleware 适配

`contentMiddleware` 只做胶水：

```go
func (m *contentMiddleware) BeforeModelRewriteState(
    ctx context.Context,
    state *adk.ChatModelAgentState,
    mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
    messages, changed, err := m.compactor.CompactIfNeeded(ctx, state.Messages, state.ToolInfos)
    if err != nil {
        klog.Warningf("context compaction skipped: %v", err)
        return ctx, state, nil
    }
    if !changed {
        return ctx, state, nil
    }

    next := *state
    next.Messages = messages
    return ctx, &next, nil
}
```

必须遵守 Eino 约束：

- 修改 `state.Messages` 只能放在 `BeforeModelRewriteState`。
- 不在 `WrapModel` 中修改 input messages。
- 不依赖 `AfterAgent` 做失败清理，因为失败路径不会调用它。
- Handler 注册在 `ChatModelAgentConfig.Handlers`。

---

## 8. 什么时候用 Middleware，什么时候用 Package

使用 Eino middleware：

| 场景 | 钩子 |
|------|------|
| 修改模型调用前的消息列表 | `BeforeModelRewriteState` |
| 动态过滤模型可见工具 schema | `BeforeModelRewriteState` |
| 记录模型调用耗时或 usage | `WrapModel` / `AfterModelRewriteState` |
| 拦截工具调用 | `Wrap*ToolCall` |

使用普通 package：

| 场景 | 包 |
|------|------|
| token/字符估算 | `internal/context` |
| 消息边界选择 | `internal/context` |
| 摘要 prompt 构造与调用 | `internal/context` |
| tool pair 修复 | `internal/context` |
| memory 文件读写 | `internal/memory` |
| session SQLite CRUD | `internal/sessions` |

原则：middleware 只连接 Eino 生命周期；可测试、可复用的业务算法都放 package。

---

## 9. v1 不做的内容

| 内容 | 原因 |
|------|------|
| `context.engine` 插件引擎 | 当前没有多策略需求 |
| 用户可配阈值和边界参数 | 属于内部算法细节 |
| 辅助摘要模型配置 | 先复用主模型，减少配置与错误面 |
| Prompt caching | provider-specific，等压缩稳定后再做 |
| session search 联动 | 属于跨会话检索，不阻塞当前会话压缩 |
| memory 自动同步 | 容易写入临时任务状态，后续单独设计 |

---

## 10. 当前实现风险

当前 [`internal/middlewares/content.go`](../../internal/middlewares/content.go) 已接入 compactor，并在压缩失败时返回原 state。仍需注意：

- 压缩只影响单次 `BeforeModelRewriteState` 的模型可见 messages，`RunLoop` 内完整 history 暂不回写压缩结果。
- v1 不实现 Normalize / Prune；若 tool output 过大且边界无法安全切分，会跳过压缩并记录 warning。
- v1 不做压缩后 token 目标校验或二次压缩；若摘要仍不足以降到安全窗口，后续模型调用可能再次触发压缩。
- v1 多模态消息只在摘要转写中保留 part 数量，不完整展开图片、音频、文件内容。
