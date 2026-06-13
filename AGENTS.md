# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Build/Run/Test

```bash
make build          # Build binary to bin/HAPPLADYSAUCECLI.exe
make run            # Run with go run (needs HAPPLADYSAUCECLI_BASE_URL + HAPPLADYSAUCECLI_MODEL env vars)
make run V=1        # Run with klog.V(1) phase trace logs written to <home>/logs/
make run V=2        # Run with klog.V(1) and klog.V(2) diagnostics (model_call_begin, compaction_check, agent_event)
make test           # Run all tests
make test-v         # Run all tests verbose
make test-cover     # Run tests with coverage
go test ./internal/context/... -run TestCompactIfNeeded -v   # Run a single test
make fmt            # go fmt ./...
make vet            # go vet ./...
make lint           # golangci-lint run (skipped if not installed)
make check          # fmt + vet + lint + tidy (use before commit)
make verify         # Read-only quality gate (vet + lint, no fmt)
make clean          # Remove bin/, coverage files
```

Requires `HAPPLADYSAUCECLI_BASE_URL` and `HAPPLADYSAUCECLI_MODEL` environment variables (or a `settings.json` config file). Copy `.env.example` → `.env` and fill in values.

## Editing Workflow

**After every code change, run quality checks through make targets — never use raw `go build`/`go fmt`/`go test` directly.**

```bash
make check && make test    # Full cycle: fmt + vet + lint + tidy, then run all tests
```

`make check` runs `fmt → vet → lint → tidy` in one pass. If any step fails, fix the issues before proceeding.

For a read-only check that does not mutate files:

```bash
make verify               # vet + lint only (no fmt, no tidy)
```

To run a single failing test with verbose output:

```bash
go test ./internal/context/... -run TestCompactIfNeeded -v
```

`make lint` auto-installs `golangci-lint` via `go install` if not already in PATH — no manual setup needed.

## Architecture

This is an **interactive AI agent CLI** written in Go, built on the [Eino ADK](https://github.com/cloudwego/eino) framework. It connects to any OpenAI-compatible chat model, streams responses to the terminal, and maintains conversation history with automatic context compaction.

### Entry & Startup Flow

1. `cmd/root.go` — `main()`, sets up signal-aware context
2. `cmd/app/app.go` — Cobra command, Viper config binding, option validation → `run()`
3. `internal/agents/interactive.go` — `RunLoop()`: creates chat model, Eino agent, then the REPL loop

### Configuration Loading (priority order)

- **CLI flags** (`--home`, `--url`, `--model`, `--apikey`, `--max-output-tokens`, `--max-model-context`)
- **Environment variables** (`HAPPLADYSAUCECLI_HOME`, `HAPPLADYSAUCECLI_BASE_URL`, `HAPPLADYSAUCECLI_MODEL`, etc.)
- **Config file** (`--config <path>`) → current directory `settings.{json,yaml,yml,toml}` → `~/HAPPLADYSAUCECLI/settings.*`

Config files support `${ENV_VAR}` expansion. The `HAPPLADYSAUCECLI_MODEL` env var is bound to `model.HAPPLADYSAUCECLI_MODEL` (not the top-level `model` key) to prevent flattening the nested config block.

### Interactive Loop (`internal/agents/interactive.go`)

```
read prompt → append to history → runner.Run(ctx, history) → ConsumeAgentEvents(iter, renderer) → append assistant reply → repeat
```

- User input uses `internal/input/` with `\` line continuation and `"""` multiline blocks.
- The `PromptReader` runs a goroutine producer with a channel-based consumer to decouple I/O from the agent loop.

### Event Streaming Pipeline (`internal/agents/agent_events.go`)

`ConsumeAgentEvents` iterates Eino ADK `AgentEvent` stream and delegates to the `AgentEventStream` interface (implemented by `terminal.Renderer`). Events follow a lifecycle:

- **Streaming assistant**: `thinking_started` → `thinking_content_started` + `Write(reasoning)` → `message_finished` → `answer_content_started` + `Write(content)` → `message_finished` → `thinking_stopped`
- **Non-streaming assistant**: single `renderCompleteMessage` with reasoning/answer blocks
- **Tool messages**: `tool_message` — rendered but NOT appended to assistant history (only the last `schema.Assistant` message is returned)
- **Error/Exit**: short-circuit the loop

Key detail: trailing newlines in streaming reasoning chunks are deferred (`pendingThinkingLineBreaks`) to avoid premature line breaks before the next printable content arrives.

### Context Compaction (`internal/context/`)

Hermes-style semantic summarization that triggers when provider session total exceeds 80% of the safe budget (`maxModelContext - maxOutputTokens`). v1 does not enforce a post-compaction target ratio.

- **`compact.go`** — `Compactor.CompactIfNeeded()`: checks token pressure via the latest provider context total from `tracker.TotalTokens()`, calls `selectBoundary()`, estimates only the middle segment for the summary prompt, generates summary via the auxiliary model, and assembles `[system, head, summary, tail]`
- **`boundary.go`** — Splits non-system conversation context into head (2 messages) + middle (summarized) + tail (4 messages). Walks `tailStart` backward to avoid breaking tool call/result pairs. Existing system messages are preserved and prepended after compaction.
- **`assemble.go`** — Assembles the compacted message list with cloned messages (to avoid mutating originals)
- **`usage.go`** — provider usage normalization, ChatModel-layer tracking, session total, and `TokenEstimator` for middle-summary sizing

On summarization failure, compaction is silently skipped (original messages returned unchanged).

### Compact Middleware (`internal/middlewares/compact/compact.go`)

A `ChatModelAgentMiddleware` that hooks `BeforeModelRewriteState`. Before every model call, it runs `Compactor.CompactIfNeeded()` on `state.Messages`. If changed, a copy of the state is returned with compacted messages — the original state is never mutated. Compaction errors are logged and swallowed (state passes through unchanged).

### Terminal Stats

The post-turn stats line separates token meanings:

```text
[Stats: elapsed=1.30s prompt↑=548 completion↓=86 total↑↓=634 content=634 0.50%(128K)]
```

`total↑↓` is the aggregate provider total across all model turns in the current user interaction. `content` is the latest provider context total from `tracker.TotalTokens()` and is the value used for context-window percentage and compaction pressure.

### Logging System (`internal/logger/`)

The project uses a single diagnostic log plus sanitized SQLite context persistence:

| Channel | Path | Format | Content |
|---------|------|--------|---------|
| Diagnostic log | `<home>/logs/happyladysaucecli.log` | `[Info]`/`[Error]` prefix + text `key=value` | Lightweight phase tracking with trace correlation |
| Context replay | `<home>/context.sqlite` | SQLite | Sanitized or metadata-only conversation snapshots |

**Trace**: Correlation IDs (`session_id`, `conversation_id`, `user_turn_seq`, `model_call`) are propagated via `logger.AttachTurn()` / `logger.FromContext()`. `logger.Info()` and `logger.Error()` auto-inject trace fields into structured klog entries.

**Structured diagnostic API**: `logger.Info(ctx, v, msg, kvs...)` for verbosity-gated structured lines and `logger.Error(ctx, err, msg, kvs...)` for error lines. Callers pass `phase` as a structured field. V=1 covers `session_open`, `session_close`, `user_turn_begin`, `model_call_end`, `capability_policy`, `capability_call`, tool `agent_event`, and `user_turn_end`; V=2 adds `model_call_begin`, `compaction_check`, non-tool `agent_event`, and `persistence`.

**klog setup**: `logger.ConfigureDefaultFile()` redirects klog output during app startup (called from `cmd/app/app.go`). The built-in default home is `~/.HAPPLADYSAUCECLI`; the repo `settings.json` uses `${HAPPLADYSAUCECLI_HOME}` and Makefile defaults `HAPPLADYSAUCECLI_HOME=.HAPPLADYSAUCECLI` for local development.

**Context persistence**: conversation replay data is stored through `internal/context/session` into SQLite after sanitization. The project does not write per-session JSONL detail logs.

### Terminal Rendering (`internal/terminal/renderer.go`)

Thread-safe renderer with ANSI color support (disabled for non-terminal writers). Features:
- Spinner animation for thinking state (goroutine with ticker, cleanly stopped via channel)
- Color-coded output: user green, agent cyan, thinking yellow, tool magenta, error red
- `EmitAgentEvent` accepts string constants (not imported types) to avoid circular dependency with `internal/agents`

### System Prompts

- `internal/prompts.SystemPrompt` — main agent instruction
- `pkg/context/compact.summarySystemPrompt` — instructs the auxiliary model to produce structured summaries with sections: Goal, Constraints, Progress, Decisions, Relevant Files, Next Steps
- `pkg/context/compact.summaryPrefix` — marks compacted content as `[CONTEXT COMPACTION - REFERENCE ONLY]`
- `pkg/context/compact.renderMessagesForSummary()` — renders messages as stable text transcript for the summarizer

### Execution Security & Tool Soft-Fail (`internal/middlewares/security/`)

- `ExecutionSecurityMiddleware` wraps tool invocations: policy evaluation → user approval → audit → endpoint
- Recoverable failures (user/policy denial, tool execution errors) return JSON payloads to the model with `nil` Go error so the ReAct loop continues; invariant violations (path/scope, missing approver) still hard-fail
- Malformed tool JSON and empty required paths are recoverable `invalid_arguments` payloads and must not call endpoints
- File operations must use matching `file:*` scopes plus normalized path/file resources, and endpoints must re-check actual paths with `execguard.RequireAuthorizedPath`
- File operations use `security.file_operation_timeout_seconds`, `security.file_max_bytes`, `security.file_max_line_bytes`, and `security.max_tool_output_bytes`; file read/edit/delete reject symlink/reparse targets and `file_list` reports `entries`, `returned_entries`, `truncated`, and per-entry `readable`
- `command.run` requires an available WSL2 sandbox runner; sandbox probe uses a short timeout/cache, unavailable sandbox returns `command_sandbox_unavailable`, and authorized endpoints must execute through `commandsandbox.RunFromContext(ctx, request)` rather than native Windows execution
- Denial sentinels and reasons live in `internal/security/denial.go`; JSON formatting in `internal/tools/toolresult/`
- Full error-handling matrix: `docs/security/architecture.md` §9

### Tools (`internal/tools/`)

- `tools.go` — central factory for built-in Eino tools, capability descriptors, and operation builders; runtime setup injects the shared `WorkspaceGuard` and `execfiles.Service`
- `weather/weather.go` — Calls uapis.cn weather API. Built with `utils.InferTool` which auto-generates JSON Schema from Go structs. Validates city (required) and lang (zh/en).
- `files/files.go` — Built-in guarded file tools: `file_read`, `file_list`, `file_edit`, `file_create`, `file_delete`. Endpoints must use the shared `WorkspaceGuard` and `execguard.RequireAuthorizedPath`; write/delete tools require review and audit only path/size/hash metadata, never raw file content.

## Code Conventions

- **Bilingual comments**: Every exported function, type, and constant has both English and Chinese doc comments. This is deliberate — follow the pattern.
- **Packages**: `internal/` for app-internal code, `pkg/` for shared library code usable by other projects.
- **Error handling**: Uses `errors.Join` for multi-field validation, `fmt.Errorf` with `%w` for wrapping.
- **Testing**: Standard `testing` package. Test helpers use `t.Helper()`. Viper tests call `viper.Reset()` + `t.Cleanup(viper.Reset)`. Mock chat models implement `model.BaseChatModel` interface (both `Generate` + `Stream`).
- **Configuration**: Mapstructure tags on option structs (e.g., `mapstructure:"HAPPLADYSAUCECLI_API_KEY"`).
