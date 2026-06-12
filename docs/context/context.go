我看完了当前实现和几份资料。结论：本项目现在的 `pkg/context/compact` 是一个合理的 v1，但还不是生产级上下文管理。

当前实现核心在 [compact.go](D:/Code/happyladysauce/HappyLadySauceCLI/pkg/context/compact/compact.go:89)：用 provider 上一次返回的 `TotalTokens()` 做 80% 水位触发，然后固定保留 `head=2`、`tail=4`，把 middle 交给模型生成摘要。边界保护做得不错，尤其是 [boundary.go](D:/Code/happyladysauce/HappyLadySauceCLI/pkg/context/compact/boundary.go:18) 会避免把 tool call/result 拆开；摘要 prompt 也已经要求保留决策、错误、文件、命令、下一步等信息，见 [prompts.go](D:/Code/happyladysauce/HappyLadySauceCLI/pkg/context/compact/prompts.go:22)。

生产级做法通常不是“摘要一下历史”这么简单，而是几层一起工作：

1. **稳定前缀 + 追加式上下文**  
   Manus 特别强调围绕 KV cache/prompt cache 设计：系统提示、工具定义、序列化顺序要稳定，历史尽量 append-only。OpenAI prompt caching 文档也建议把静态/重复内容放前面，动态内容放后面，并监控 cached tokens。

2. **短期记忆、长期记忆、外部状态分层**  
   LangGraph 把 short-term memory 当作 thread-scoped state，把 long-term memory 放到跨 session store。生产 agent 不会只依赖消息列表，还会把文件、artifact、notes、retrieved docs 作为可恢复的外部上下文。

3. **压缩要可恢复，不只是丢弃**  
   Manus 的思路是大内容可以移出窗口，但保留 URL、文件路径、文档路径等引用。失败的工具调用和错误也应保留或摘要进状态，因为它们能阻止 agent 重复失败路径。

4. **压缩要有预算目标和结果校验**  
   Anthropic 提到 compaction、structured note-taking、sub-agent isolation 三类长任务技术。OpenAI compaction 文档也把压缩后的窗口视为下一轮 canonical context。也就是说，压缩完成后应该重新估算 token，确保真的落到目标预算，而不是只假设摘要会变短。

5. **running summary 比反复总结整段更稳**  
   LangMem/LangGraph 的 summarization 有 `running_summary` 概念，会记录已总结的消息，后续只合并新增部分。本项目现在把旧摘要作为 user message 继续参与下一轮压缩，能工作，但缺少显式 summary state、message id、版本和去重边界。

我建议第一阶段先不要上复杂 RAG 或长期记忆，先把当前 compaction 做扎实：

- 从固定 `head/tail` 改成 **token budget 驱动**：保留最近消息直到占用目标预算，例如压缩后控制在 safe prompt budget 的 50%-60%。
- 增加 **post-compaction token estimate**：压缩后如果仍超过目标，进入更强压缩或返回明确错误日志。
- 引入 **running summary 结构**：区分“已有摘要”和“新 middle”，避免反复摘要旧摘要导致漂移。
- 增强摘要 schema：保留 `Goal / Constraints / Progress / Decisions / Files / Commands / Errors / Next Steps`，其中 `Errors` 单独列出来。
- 为 tool result 做轻量策略：老的长 tool output 可摘要，但保留 tool 名、参数、结果摘要、错误和可恢复引用。

参考资料：  
[Manus 上下文工程文章](https://manus.im/zh-cn/blog/Context-Engineering-for-AI-Agents-Lessons-from-Building-Manus)、[Anthropic Effective context engineering](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)、[OpenAI Compaction docs](https://developers.openai.com/api/docs/guides/compaction)、[OpenAI Prompt caching docs](https://developers.openai.com/api/docs/guides/prompt-caching)、[LangGraph Memory docs](https://docs.langchain.com/oss/python/langgraph/add-memory)。知乎链接我这边直接打开没有正文内容，只能看到搜索摘要，所以没有把它作为主要依据。

我的建议是先做“预算驱动 compaction + running summary + 压缩后校验”这一版。这个方向不大改架构，但能明显提升可靠性。你确认后我再给具体改造方案。