# 上下文配置规范

本文档定义 HappyLadySauceCLI v1 上下文体系的用户可见配置。上下文压缩策略默认启用，内部阈值和边界策略由代码维护，不暴露给普通用户调参。

相关文档：[总览](./README.md) · [压缩](./compression.md) · [记忆](./memory.md) · [会话](./sessions.md)

---

## 1. 设计原则

| 原则 | 说明 |
|------|------|
| 用户只配置必要信息 | 只暴露模型、数据目录、memory 开关等用户能理解并会修改的配置 |
| 压缩策略内部化 | 阈值、tail/head 保护、字符估算、冷却策略等属于实现细节 |
| 先稳定默认实现 | v1 不提供 `context.engine`、插件式压缩引擎、辅助摘要模型配置 |
| 配置失败要明确 | 模型配置错误直接失败；上下文内部策略失败只记录英文 warning 并保留原消息 |

---

## 2. 配置文件位置

与项目现有配置机制一致：

| 优先级 | 路径 |
|--------|------|
| 1 | `--config` / `-f` 显式指定 |
| 2 | 当前工作目录 `settings.{json,yaml,yml,toml}` |
| 3 | `~/.HAPPLADYSAUCECLI/settings.{json,yaml,yml,toml}` |
| 4 | CLI flags + 环境变量 |

---

## 3. v1 Schema

```json
{
  "model": {
    "HAPPLADYSAUCECLI_API_KEY": "",
    "HAPPLADYSAUCECLI_BASE_URL": "",
    "HAPPLADYSAUCECLI_MODEL": "",
    "HAPPLADYSAUCECLI_MAX_OUTPUT_TOKENS": 32000,
    "HAPPLADYSAUCECLI_MAX_MODEL_CONTEXT": 128000
  },
  "data_dir": null,
  "memory": {
    "enabled": true
  }
}
```

`null` 表示使用内置默认。

---

## 4. model

已有配置，上下文体系只依赖这两个窗口参数：

| 键 | 类型 | 默认 | 说明 |
|----|------|------|------|
| `HAPPLADYSAUCECLI_MAX_MODEL_CONTEXT` | int | `128000` | 主模型 context window，是压缩水位计算基准 |
| `HAPPLADYSAUCECLI_MAX_OUTPUT_TOKENS` | int | `32000` | 单次回复最大 token，必须小于 context window |

其它字段仍由模型初始化使用：

| 键 | 类型 | 说明 |
|----|------|------|
| `HAPPLADYSAUCECLI_MODEL` | string | 主 Agent 模型名 |
| `HAPPLADYSAUCECLI_BASE_URL` | string | OpenAI-compatible API 地址 |
| `HAPPLADYSAUCECLI_API_KEY` | string | API 密钥 |

约束：`MAX_MODEL_CONTEXT > MAX_OUTPUT_TOKENS`。

---

## 5. data_dir

| 键 | 类型 | 默认 | 说明 |
|----|------|------|------|
| `data_dir` | string/null | `~/.HAPPLADYSAUCECLI` | memory、session DB、skills 等本地数据根目录 |

派生路径：

| 资源 | 默认路径 |
|------|----------|
| memory 文件 | `{data_dir}/memories/` |
| skills | `{data_dir}/skills/` |
| session DB | `{data_dir}/state.db` |

v1 不单独暴露 `memory.dir` 或 `sessions.db_path`。需要迁移数据目录时，修改 `data_dir` 即可。

---

## 6. memory

| 键 | 类型 | 默认 | 说明 |
|----|------|------|------|
| `memory.enabled` | bool | `true` | 是否在会话启动时加载持久 memory 快照 |

`MEMORY.md` / `USER.md` 的字符上限、redact 策略、文件锁策略均为内部实现细节，不进入用户配置。

---

## 7. 不再暴露的配置

以下项目不进入 v1 用户配置：

| 配置 | 原因 |
|------|------|
| `context.engine` | 当前没有多引擎/插件需求，默认 `internal/context.Compactor` 即可 |
| `compression.threshold` | 内部水位策略，用户不应理解 token 压缩阈值 |
| `compression.target_ratio` | 边界选择算法细节 |
| `compression.protect_last_n` / `protect_first_n` | 消息保护策略细节，应由实现保证 |
| `compression.hygiene_threshold` | v1 不做独立 PreRun Hygiene 配置 |
| `compression.chars_per_token` | 粗估兜底实现细节 |
| `compression.prune_tool_result_min_chars` | 剪枝算法细节 |
| `compression.summary_failure_cooldown_seconds` | anti-thrashing 内部策略 |
| `auxiliary.compression.*` | v1 复用主模型摘要，后续确有成本需求再抽象 `Summarizer` |
| `prompt_caching.*` | v1 暂不实现 provider-specific prompt caching |

---

## 8. 环境变量映射

v1 只绑定现有模型环境变量：

| 配置键 | 环境变量 |
|--------|----------|
| `model.HAPPLADYSAUCECLI_API_KEY` | `HAPPLADYSAUCECLI_API_KEY` |
| `model.HAPPLADYSAUCECLI_BASE_URL` | `HAPPLADYSAUCECLI_BASE_URL` |
| `model.HAPPLADYSAUCECLI_MODEL` | `HAPPLADYSAUCECLI_MODEL` |
| `model.HAPPLADYSAUCECLI_MAX_OUTPUT_TOKENS` | `HAPPLADYSAUCECLI_MAX_OUTPUT_TOKENS` |
| `model.HAPPLADYSAUCECLI_MAX_MODEL_CONTEXT` | `HAPPLADYSAUCECLI_MAX_MODEL_CONTEXT` |

`data_dir` 和 `memory.enabled` 可先只支持配置文件；是否增加环境变量以后再根据实际使用需求决定。

---

## 9. Go 配置类型

v1 建议扩展为：

```go
type Config struct {
    Model   *options.ModelOptions `mapstructure:"model"`
    DataDir string                `mapstructure:"data_dir"`
    Memory  *options.MemoryOptions `mapstructure:"memory"`
}

type MemoryOptions struct {
    Enabled bool `mapstructure:"enabled"`
}
```

压缩配置不进入 `pkg/options`。压缩器通过 `ModelOptions` 和内部默认策略初始化。

---

## 10. 后续扩展门槛

只有出现明确需求时才扩展配置：

- 多 provider 成本差异明显，再增加摘要模型配置。
- 需要接入非默认压缩策略，再考虑引擎接口或策略选项。
- 真实支持 Anthropic cache control，再增加 prompt caching 配置。

新增配置前必须先确认：普通用户是否会理解并主动修改；否则保持 internal default。
