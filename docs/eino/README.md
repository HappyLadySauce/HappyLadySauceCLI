# Eino 文档索引

本目录记录 [CloudWeGo Eino](https://github.com/cloudwego/eino) 生态在本项目中的用法与架构参考。适用版本：**Eino ADK v0.9.4**（`github.com/cloudwego/eino v0.9.4`）。

## 文档列表

| 文档 | 范围 |
|------|------|
| [component-ecosystem-guide.md](./component-ecosystem-guide.md) | **组件生态总览**：分层架构、`schema` / `model` / `callbacks` / `tool` / `compose` / `adk` 全链路、消息与 Token 用量在各层的传递方式、扩展点对比 |
| [chatmodelagent-guide.md](./chatmodelagent-guide.md) | **ChatModelAgent 工作机制**：ReAct 循环、事件流、工具调用、消息生命周期、本项目消费方式 |
| [middleware-guide.md](./middleware-guide.md) | **ChatModelAgentMiddleware 实践**：钩子职责、注册顺序、实现模板、常见场景与反模式 |

## 阅读顺序建议

1. 先读 **component-ecosystem-guide**，建立 Eino 分层与数据流的全局图。
2. 再读 **chatmodelagent-guide**，理解 Agent / Runner / 事件在应用层的具体行为。
3. 需要编写或调试中间件时，查阅 **middleware-guide**。

## 相关仓库

| 仓库 | 用途 |
|------|------|
| [cloudwego/eino](https://github.com/cloudwego/eino) | 核心：`schema`、`components`、`compose`、`callbacks`、`adk` |
| [cloudwego/eino-ext](https://github.com/cloudwego/eino-ext) | 扩展实现：OpenAI 兼容 ChatModel、Embedding 等 |
