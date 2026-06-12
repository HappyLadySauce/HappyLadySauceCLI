# HappyLadySauceCLI

基于 [Eino ADK](https://github.com/cloudwego/eino) 的交互式 AI Agent 命令行工具。连接任意 OpenAI 兼容的聊天模型，在终端中流式输出回复，并自动维护对话历史与上下文压缩。

## 特性

- **交互式 REPL** — 终端内持续对话，支持流式输出与思考过程展示
- **OpenAI 兼容** — 支持 OpenAI、LM Studio、Ollama 等兼容端点
- **自动上下文压缩** — 当 prompt 接近预算上限时，以语义摘要压缩中间历史，保留首尾关键消息
- **上下文预算状态行** — 每轮对话后显示本次 token 用量与上下文窗口占用
- **Provider 用量回写** — 从模型响应元数据读取实际 prompt / completion token，校准预算显示
- **模型元数据探测** — 启动时自动查询 provider 的上下文长度，未显式配置时自动采用
- **多行输入** — 支持 `\` 续行与 `"""` 多行块
- **内置工具** — 天气查询（uapis.cn API）

## 环境要求

- Go **1.26.1+**
- `make`（Windows 可用 [GnuWin32 make](http://gnuwin32.sourceforge.net/packages/make.htm) 或 Git Bash 自带 make）

## 快速开始

```bash
# 1. 克隆并进入项目
git clone https://github.com/HappyLadySauce/HappyLadySauceCLI.git
cd HappyLadySauceCLI

# 2. 配置环境变量
copy .env.example .env        # Windows
# cp .env.example .env        # macOS / Linux
# 编辑 .env，填写 API Key、Base URL、模型名

# 3. 运行
make run
```

构建二进制：

```bash
make build
# 输出: bin/HAPPLADYSAUCECLI.exe (Windows) 或 bin/HAPPLADYSAUCECLI (Unix)
```

启用诊断日志详细级别：

```bash
make run V=1   # 输出 klog.V(1) 因果主链日志
make run V=2   # 输出 klog.V(1) 和 klog.V(2) 详细决策日志
```

程序内建默认 home 为 `~/.HAPPLADYSAUCECLI`；仓库 `settings.json` 通过 `${HAPPLADYSAUCECLI_HOME}` 注入，Makefile 默认导出 `HAPPLADYSAUCECLI_HOME=.HAPPLADYSAUCECLI`，因此开发运行会把数据和诊断日志放到当前工作目录下。

## 配置

配置优先级（高 → 低）：

1. **命令行参数**
2. **环境变量**
3. **配置文件**

### 环境变量

| 变量 | 必填 | 说明 | 默认值 |
|------|------|------|--------|
| `HAPPLADYSAUCECLI_API_KEY` | 视 provider 而定 | API 密钥 | — |
| `HAPPLADYSAUCECLI_BASE_URL` | 是 | 模型 API 基址 | — |
| `HAPPLADYSAUCECLI_MODEL` | 是 | 模型名称 | — |
| `HAPPLADYSAUCECLI_HOME` | 否 | 程序 home 目录，保存 context 数据库和诊断日志；相对路径基于启动工作目录解析 | `~/.HAPPLADYSAUCECLI` |
| `HAPPLADYSAUCECLI_MAX_OUTPUT_TOKENS` | 否 | 单次最大输出 token | `32000` |
| `HAPPLADYSAUCECLI_MAX_MODEL_CONTEXT` | 否 | 模型上下文窗口 | `128000`（可被 provider 元数据覆盖） |

Makefile 会通过 `-include .env` 自动加载 `.env` 并导出变量。

### 配置文件

支持 JSON / YAML / YML / TOML，搜索顺序：

1. `--config <path>` 显式指定
2. 当前目录 `settings.{json,yaml,yml,toml}`
3. `~/HAPPLADYSAUCECLI/settings.*`

配置文件支持 `${ENV_VAR}` 展开，示例见仓库根目录 `settings.json`：

```json
{
    "home": "${HAPPLADYSAUCECLI_HOME}",
    "model": {
        "HAPPLADYSAUCECLI_API_KEY": "${HAPPLADYSAUCECLI_API_KEY}",
        "HAPPLADYSAUCECLI_BASE_URL": "${HAPPLADYSAUCECLI_BASE_URL}",
        "HAPPLADYSAUCECLI_MODEL": "${HAPPLADYSAUCECLI_MODEL}",
        "HAPPLADYSAUCECLI_MAX_OUTPUT_TOKENS": 32000,
        "HAPPLADYSAUCECLI_MAX_MODEL_CONTEXT": 128000
    }
}
```

### 命令行参数

```bash
HAPPLADYSAUCECLI \
  --url https://api.openai.com/v1 \
  --model gpt-4o \
  --apikey sk-... \
  --max-output-tokens 32000 \
  --max-model-context 128000
```

查看完整帮助：

```bash
make run -- --help
```

### 常见 Provider 示例

**OpenAI**

```env
HAPPLADYSAUCECLI_BASE_URL=https://api.openai.com/v1
HAPPLADYSAUCECLI_MODEL=gpt-4o
HAPPLADYSAUCECLI_API_KEY=sk-...
```

**Ollama**（本地，host:port 会自动补全为 `http://`）

```env
HAPPLADYSAUCECLI_BASE_URL=localhost:11434/v1
HAPPLADYSAUCECLI_MODEL=llama3
```

**LM Studio**（OpenAI 兼容端点）

```env
HAPPLADYSAUCECLI_BASE_URL=http://localhost:1234/v1
HAPPLADYSAUCECLI_MODEL=<loaded-model-id>
```

## 使用

启动后进入交互循环，输入 prompt 并回车发送。空行会被忽略。

### 多行输入

| 方式 | 说明 |
|------|------|
| `\` 续行 | 行末加 `\` 表示内容延续到下一行 |
| `"""` 块 | 单独一行输入 `"""` 开启多行块，再次输入 `"""` 结束 |

### 上下文状态行

每轮助手回复结束后，终端会输出一行紧凑的上下文预算信息，例如：

```
[Stats: elapsed=1.30s prompt↑=548 completion↓=86 total↑↓=634 content=634 0.50%(128K)]
```

- **prompt↑ / completion↓** — 来自 provider 响应的本轮输入与输出 token 用量（若 provider 返回）
- **total↑↓** — 当前用户交互内所有 model turn 的 provider total 聚合值
- **content** — ChatModel 层记录的最新 provider 上下文窗口占用，百分比也基于该值计算
- 无 provider 用量时，`content` 保持为 0，压缩不会基于本地估算触发

### 诊断日志

klog 诊断日志默认写入：

```text
<home>/logs/happyladysaucecli.log
```

终端只保留交互对话、工具输出和 `[Stats: ...]` 状态行；详细诊断写入日志文件。使用 `make run V=1` 打开因果主链，`make run V=2` 额外打开 model call 输入、compaction 检查、工具事件与持久化细节。

日志行统一带 `phase=` 字段，并尽量附带 `conversation_id` 与 `model_call`，便于 grep 串联一次用户交互：

| phase | 级别 | 含义 |
|-------|------|------|
| `session_open` | V=1 | CLI 启动，context session 创建 |
| `user_turn_begin` | V=1 | 收到用户输入，含截断后的 `user_prompt` |
| `model_call_end` | V=1 | 单次 model call 完成，含 token 与 `tool_calls` |
| `user_turn_end` | V=1 | 用户回合结束汇总（含 `turn_messages` / `history_messages`） |
| `model_call_begin` | V=2 | 每次 model call 前的输入消息 roster |
| `compaction_check` | V=2 | 压缩判定或未触发原因 |
| `agent_event` | V=2 | 工具结果、错误、退出等 agent 事件 |
| `persistence` | V=2 | context 快照写入 SQLite |
| `compaction_done` | Info | 实际发生压缩时 |

示例（天气查询回合，V=2）：

```text
phase=user_turn_begin conversation_id=conversation_abc user_turn_seq=2 user_prompt="重庆天气怎么样？" history_messages=3
phase=model_call_begin conversation_id=conversation_abc model_call=1 input_messages=4 message_roster="roles=user:1,assistant:1 ..."
phase=model_call_end conversation_id=conversation_abc model_call=1 turn_seq=1 prompt=341 completion=95 total=436 tool_calls=[get_weather]
phase=agent_event conversation_id=conversation_abc kind=tool_message tool_name=get_weather content_len=156
phase=user_turn_end conversation_id=conversation_abc model_calls=2 turn_messages=4 history_messages=7 content=617 elapsed_ms=2820
```

### 上下文压缩

当 provider session total 超过安全预算的 80%（`maxModelContext - maxOutputTokens`）时，中间对话会被自动摘要压缩，策略为：

- 保留头部 2 条 + 尾部 4 条非系统消息
- 中间部分生成结构化摘要（目标、约束、进展、决策、相关文件、下一步）
- 摘要失败时静默跳过，不中断对话

详细设计见 [`pkg/context/compact/README.md`](pkg/context/compact/README.md)。

## 开发

```bash
make help          # 查看所有 make 目标
make test          # 运行全部测试
make test-v        # 详细测试输出
make test-cover    # 测试覆盖率
make fmt           # go fmt
make vet           # go vet
make lint          # golangci-lint（未安装则跳过）
make check         # fmt + vet + lint + tidy（提交前建议）
make verify        # 只读质量门禁（vet + lint）
make clean         # 清理 bin/ 与覆盖率文件
```

运行单个测试：

```bash
go test ./internal/context/... -run TestCompactIfNeeded -v
```

## 项目结构

```
cmd/
  root.go              # 入口，信号处理
  app/                 # Cobra 命令与配置加载
internal/
  agents/              # REPL 主循环与事件消费
  context/             # 上下文压缩、token 估算、预算
  input/               # 多行 prompt 读取
  middlewares/         # Eino 中间件（压缩、预算、用量）
  models/metadata/     # Provider 模型元数据探测
  prompts/             # 系统提示与压缩摘要模板
  terminal/            # ANSI 渲染与状态行
  tools/               # Agent 工具注册
pkg/
  options/             # 配置选项与 Viper 绑定
  config/              # 运行时配置单例
```

## 架构概览

```
用户输入 → history → runner.Run → ConsumeAgentEvents → 终端渲染
                          ↑
              BeforeModelRewriteState 中间件
                    ├── 上下文压缩
                    ├── 预算追踪
                    └── 用量采集
```

- 基于 Eino ADK `ChatModelAgent`，启用流式输出
- 中间件在每次模型调用前重写 `state.Messages`，不修改原始 state
- 工具调用结果会渲染但不写入 assistant history（仅保留最后一条 assistant 消息）

## 文档

| 文档 | 内容 |
|------|------|
| [`pkg/context/compact/README.md`](pkg/context/compact/README.md) | 压缩机制与包边界 |
| [`internal/terminal/budget/README.md`](internal/terminal/budget/README.md) | 终端统计行格式 |
| [`CLAUDE.md`](CLAUDE.md) | 面向 AI 辅助开发的仓库指南 |

## 技术栈

- [Eino ADK](https://github.com/cloudwego/eino) — Agent 框架
- [eino-ext OpenAI](https://github.com/cloudwego/eino-ext) — OpenAI 兼容模型组件
- [Cobra](https://github.com/spf13/cobra) + [Viper](https://github.com/spf13/viper) — CLI 与配置
- [tiktoken-go](https://github.com/pkoukk/tiktoken-go) — Token 估算
