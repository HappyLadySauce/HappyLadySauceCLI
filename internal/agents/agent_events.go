// Package agents bridges Eino ADK agent events to terminal-friendly stream output.
//
// Flow overview:
//  1. RunLoop calls ConsumeAgentEvents with the ADK iterator and a Renderer (AgentEventStream).
//  2. ConsumeAgentEvents walks each AgentEvent and delegates message rendering.
//  3. renderMessageOutput chooses streaming vs complete-message rendering.
//  4. Structured lifecycle signals go through EmitAgentEvent; token text goes through Write.
//  5. All assistant and tool messages from the turn are returned for conversation history; exit/error short-circuit the loop.
//
// Package agents 将 Eino ADK agent 事件桥接为终端友好的流式输出。
//
// 流程概览：
//  1. RunLoop 调用 ConsumeAgentEvents，传入 ADK 迭代器与 Renderer（AgentEventStream）。
//  2. ConsumeAgentEvents 遍历每个 AgentEvent 并委托消息渲染。
//  3. renderMessageOutput 在流式与完整消息渲染之间分流。
//  4. 结构化生命周期信号走 EmitAgentEvent，逐 token 文本走 Write。
//  5. 返回本轮全部 assistant 与 tool 消息供会话历史使用；exit/error 会提前结束循环。
package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/logger"
)

const (
	// AgentStreamEventThinkingStarted means model execution has started but no text chunk is available yet.
	// AgentStreamEventThinkingStarted 表示模型已开始执行，但还没有可输出的文本片段。
	AgentStreamEventThinkingStarted = "thinking_started"

	// AgentStreamEventThinkingStopped means a real output chunk arrived and any pending thinking animation should stop.
	// AgentStreamEventThinkingStopped 表示真实输出片段已到达，等待中的 thinking 动画应停止。
	AgentStreamEventThinkingStopped = "thinking_stopped"

	// AgentStreamEventThinkingContentStarted means following Write calls belong to reasoning/thinking content.
	// AgentStreamEventThinkingContentStarted 表示后续 Write 调用属于 reasoning/thinking 内容。
	AgentStreamEventThinkingContentStarted = "thinking_content_started"

	// AgentStreamEventAnswerContentStarted means following Write calls belong to final assistant answer content.
	// AgentStreamEventAnswerContentStarted 表示后续 Write 调用属于最终 assistant 回复内容。
	AgentStreamEventAnswerContentStarted = "answer_content_started"

	// AgentStreamEventMessageFinished means the current logical message has ended.
	// AgentStreamEventMessageFinished 表示当前逻辑消息已结束。
	AgentStreamEventMessageFinished = "message_finished"

	// AgentStreamEventToolMessage means a tool result should be emitted as a complete message.
	// AgentStreamEventToolMessage 表示应以完整消息输出工具结果。
	AgentStreamEventToolMessage = "tool_message"

	// AgentStreamEventError means agent execution produced an error.
	// AgentStreamEventError 表示 agent 执行产生错误。
	AgentStreamEventError = "error"

	// AgentStreamEventExit means the agent requested the interactive loop to exit.
	// AgentStreamEventExit 表示 agent 请求退出交互循环。
	AgentStreamEventExit = "exit"
)

// AgentEventStream receives agent stream events and raw text chunks.
// Write is reserved for raw streaming text chunks, while EmitAgentEvent carries structured control events.
// AgentEventStream 接收 agent 流事件和原始文本片段；Write 用于原始流式文本，EmitAgentEvent 用于结构化控制事件。
type AgentEventStream interface {
	io.Writer

	// EmitAgentEvent emits a structured stream event.
	// kind should be one of the AgentStreamEvent* constants in this package.
	// agentName identifies the event source, toolName is set only for tool events,
	// content carries complete non-token payloads such as tool output, and err is set only for error events.
	// EmitAgentEvent 输出结构化流事件；kind 应使用本包 AgentStreamEvent* 常量。
	// agentName 标识事件来源，toolName 仅用于工具事件，content 承载工具输出等完整内容，err 仅用于错误事件。
	EmitAgentEvent(kind string, agentName string, toolName string, content string, err error)
}

// ConsumeAgentEvents consumes an Eino ADK event iterator and streams output fragments.
//
// It processes events in order until the iterator is exhausted or a terminal condition occurs:
//   - event.Err: emits AgentStreamEventError, returns (nil, false, err).
//   - event.Action.Exit: emits AgentStreamEventExit, returns (lastAssistant, true, nil).
//   - message output: rendered via renderMessageOutput; assistant and tool messages are collected for history.
//
// Returns all assistant and tool messages produced during the turn (in event order), whether the agent
// requested exit, and any fatal consumption error.
//
// ConsumeAgentEvents 消费 Eino ADK 事件迭代器，并以流式片段输出。
//
// 按顺序处理事件，直到迭代器结束或遇到终止条件：
//   - event.Err：发出 AgentStreamEventError，返回 (nil, false, err)。
//   - event.Action.Exit：发出 AgentStreamEventExit，返回 (lastAssistant, true, nil)。
//   - 消息输出：经 renderMessageOutput 渲染；assistant 与 tool 消息会进入历史收集列表。
//
// 返回值依次为：供历史追加的本轮消息列表、agent 是否请求退出、致命消费错误。
func ConsumeAgentEvents(ctx context.Context, iter *adk.AsyncIterator[*adk.AgentEvent], stream AgentEventStream) ([]*schema.Message, bool, error) {
	var turnMessages []*schema.Message

	for {
		event, ok := iter.Next()
		if !ok {
			// Iterator exhausted; return accumulated turn messages for history.
			// 迭代器耗尽；返回已累积的本轮消息供历史记录。
			return turnMessages, false, nil
		}
		if event.Err != nil {
			logAgentEvent(ctx, AgentStreamEventError, event.AgentName, "", "", event.Err)
			stream.EmitAgentEvent(AgentStreamEventError, event.AgentName, "", "", event.Err)
			return nil, false, fmt.Errorf("agent loop error: %w", event.Err)
		}
		if event.Action != nil && event.Action.Exit {
			logAgentEvent(ctx, AgentStreamEventExit, event.AgentName, "", "", nil)
			stream.EmitAgentEvent(AgentStreamEventExit, event.AgentName, "", "", nil)
			return turnMessages, true, nil
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			// Non-message events (e.g. intermediate actions) are ignored.
			// 非消息事件（如中间 action）直接跳过。
			continue
		}

		msg, err := renderMessageOutput(ctx, event, stream)
		if err != nil {
			return nil, false, fmt.Errorf("read agent message: %w", err)
		}
		if msg != nil && msg.Role == schema.Tool {
			toolName := msg.ToolName
			if toolName == "" && event.Output.MessageOutput != nil {
				toolName = event.Output.MessageOutput.ToolName
			}
			logAgentEvent(ctx, AgentStreamEventToolMessage, event.AgentName, toolName, msg.Content, nil)
		}
		if shouldAppendToHistory(msg) {
			turnMessages = append(turnMessages, cloneMessageForHistory(msg))
		}
	}
}

func logAgentEvent(ctx context.Context, kind, agentName, toolName, content string, eventErr error) {
	kvs := []any{"phase", "agent_event", "kind", kind, "agent_name", agentName}
	if toolName != "" {
		kvs = append(kvs, "tool_name", toolName)
	}
	if content != "" {
		kvs = append(kvs, "content_len", len(content))
	}
	if eventErr != nil {
		logger.Error(ctx, eventErr, "Agent event failed", kvs...)
		return
	}
	if toolName != "" || content != "" {
		logger.Info(ctx, 1, "Agent event emitted", kvs...)
		return
	}
	logger.Info(ctx, 2, "Agent event emitted", kvs...)
}

// shouldAppendToHistory reports whether a message should persist across runner turns.
// shouldAppendToHistory 判断消息是否应跨 runner 回合持久化。
func shouldAppendToHistory(msg *schema.Message) bool {
	if msg == nil {
		return false
	}
	switch msg.Role {
	case schema.Assistant, schema.Tool:
		return true
	default:
		return false
	}
}

// cloneMessageForHistory returns a defensive copy safe for long-lived conversation history.
// cloneMessageForHistory 返回可长期保存在会话历史中的防御性副本。
func cloneMessageForHistory(msg *schema.Message) *schema.Message {
	if msg == nil {
		return nil
	}
	next := *msg
	if msg.ToolCalls != nil {
		next.ToolCalls = append([]schema.ToolCall(nil), msg.ToolCalls...)
	}
	if msg.Extra != nil {
		next.Extra = make(map[string]any, len(msg.Extra))
		for k, v := range msg.Extra {
			next.Extra[k] = v
		}
	}
	return &next
}

// renderMessageOutput dispatches a single AgentEvent message output to the correct renderer.
// Streaming variants use incremental chunk handling; non-streaming variants emit the full message at once.
//
// renderMessageOutput 将单个 AgentEvent 的消息输出分发到对应渲染器。
// 流式走增量 chunk 处理；非流式一次性输出完整消息。
func renderMessageOutput(ctx context.Context, event *adk.AgentEvent, stream AgentEventStream) (*schema.Message, error) {
	output := event.Output.MessageOutput
	if output.IsStreaming {
		return renderStreamingMessage(ctx, event, stream)
	}

	msg := output.Message
	if msg == nil {
		return nil, nil
	}
	renderCompleteMessage(event, stream, msg)
	return msg, nil
}

// renderStreamingMessage incrementally renders a streaming MessageVariant.
//
// Event sequence for assistant streams (typical):
//  1. thinking_started — spinner while waiting for first chunk (assistant role only).
//  2. thinking_content_started + Write(reasoning) — optional reasoning/thinking tokens.
//  3. message_finished — closes the reasoning block when answer begins (if reasoning was shown).
//  4. answer_content_started + Write(content) — final assistant answer tokens.
//  5. message_finished — closes the answer block.
//  6. thinking_stopped — deferred cleanup when the stream ends.
//
// Chunks are concatenated into one schema.Message for history. If no incremental content was
// rendered (empty stream edge case), it falls back to renderCompleteMessage.
//
// renderStreamingMessage 对流式 MessageVariant 做增量渲染。
//
// assistant 流式事件的典型顺序：
//  1. thinking_started — 等待首个 chunk 时显示 spinner（仅 assistant 角色）。
//  2. thinking_content_started + Write(reasoning) — 可选的 reasoning/thinking 文本。
//  3. message_finished — 开始输出 answer 时结束 reasoning 块（若已展示 reasoning）。
//  4. answer_content_started + Write(content) — 最终 assistant 回复文本。
//  5. message_finished — 结束 answer 块。
//  6. thinking_stopped — 流结束时通过 defer 清理 spinner。
//
// 各 chunk 会合并为一条 schema.Message 供历史记录；若无增量内容（空流边界情况），回退到 renderCompleteMessage。
func renderStreamingMessage(ctx context.Context, event *adk.AgentEvent, stream AgentEventStream) (*schema.Message, error) {
	output := event.Output.MessageOutput
	if output.MessageStream == nil {
		return nil, nil
	}

	var chunks []*schema.Message
	renderedThinking := false
	renderedAnswer := false
	firstThinkingChunk := true
	firstAnswerChunk := true
	pendingThinkingLineBreaks := ""
	animateThinking := output.Role == "" || output.Role == schema.Assistant
	if animateThinking {
		// Show spinner until first real chunk; defer ensures cleanup even on early return.
		// 在首个真实 chunk 到达前显示 spinner；defer 保证提前返回时也能清理。
		stream.EmitAgentEvent(AgentStreamEventThinkingStarted, event.AgentName, "", "", nil)
		defer stream.EmitAgentEvent(AgentStreamEventThinkingStopped, event.AgentName, "", "", nil)
	}
	defer output.MessageStream.Close()

	for {
		chunk, err := output.MessageStream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if chunk == nil {
			continue
		}

		chunks = append(chunks, chunk)
		role := chunk.Role
		if role == "" {
			role = output.Role
		}
		if role == schema.Assistant && chunk.ReasoningContent != "" {
			if !renderedThinking {
				// First reasoning token: stop spinner and open thinking content block.
				// 首个 reasoning token：停止 spinner 并打开 thinking 内容块。
				stream.EmitAgentEvent(AgentStreamEventThinkingStopped, event.AgentName, "", "", nil)
				stream.EmitAgentEvent(AgentStreamEventThinkingContentStarted, event.AgentName, "", "", nil)
				renderedThinking = true
			}
			reasoningContent := chunk.ReasoningContent
			if firstThinkingChunk {
				reasoningContent = trimLeadingLineBreaks(reasoningContent)
				firstThinkingChunk = false
			}
			renderedContent, trailingLineBreaks := splitTrailingLineBreaks(reasoningContent)
			if renderedContent != "" {
				_, _ = stream.Write([]byte(pendingThinkingLineBreaks))
				_, _ = stream.Write([]byte(renderedContent))
				pendingThinkingLineBreaks = trailingLineBreaks
			} else {
				// Chunk is only trailing breaks; hold them until the next printable body arrives.
				// chunk 仅含尾部换行；暂存到下一个可打印正文再输出。
				pendingThinkingLineBreaks += trailingLineBreaks
			}
		}
		if role == schema.Assistant && chunk.Content != "" {
			if !renderedAnswer {
				// Transition from reasoning (if any) to final answer block.
				// 从 reasoning（若有）切换到最终 answer 块。
				stream.EmitAgentEvent(AgentStreamEventThinkingStopped, event.AgentName, "", "", nil)
				if renderedThinking {
					stream.EmitAgentEvent(AgentStreamEventMessageFinished, event.AgentName, "", "", nil)
				}
				stream.EmitAgentEvent(AgentStreamEventAnswerContentStarted, event.AgentName, "", "", nil)
				renderedAnswer = true
			}
			content := chunk.Content
			if firstAnswerChunk {
				content = trimLeadingLineBreaks(content)
				firstAnswerChunk = false
			}
			_, _ = stream.Write([]byte(content))
		}
	}

	if len(chunks) == 0 {
		return nil, nil
	}

	msg, err := schema.ConcatMessages(chunks)
	if err != nil {
		return nil, err
	}
	if msg.Role == "" {
		msg.Role = output.Role
	}
	if msg.ToolName == "" {
		msg.ToolName = output.ToolName
	}
	msg.ReasoningContent = trimLineBreaks(msg.ReasoningContent)
	msg.Content = trimLeadingLineBreaks(msg.Content)

	if renderedAnswer {
		stream.EmitAgentEvent(AgentStreamEventMessageFinished, event.AgentName, "", "", nil)
		return msg, nil
	}
	if renderedThinking {
		stream.EmitAgentEvent(AgentStreamEventMessageFinished, event.AgentName, "", "", nil)
		return msg, nil
	}

	// Stream had chunks but no incremental render path matched (e.g. tool/non-assistant role).
	// 流中有 chunk 但未命中增量渲染路径（如 tool/非 assistant 角色）。
	renderCompleteMessage(event, stream, msg)
	return msg, nil
}

// renderCompleteMessage renders a fully materialized message in one pass.
//
// Role handling:
//   - Tool: emits AgentStreamEventToolMessage with tool name and content.
//   - Assistant (or empty role): reasoning block first, then answer block; each wrapped with content_started/finished events.
//   - Other roles: treated as answer content with standard start/finish events.
//
// renderCompleteMessage 一次性渲染已完整落地的消息。
//
// 按角色处理：
//   - Tool：发出 AgentStreamEventToolMessage，携带工具名与内容。
//   - Assistant（或空 role）：先 reasoning 块，再 answer 块；各块用 content_started/finished 事件包裹。
//   - 其他 role：按 answer 内容处理，使用标准 start/finish 事件。
func renderCompleteMessage(event *adk.AgentEvent, stream AgentEventStream, msg *schema.Message) {
	if msg == nil || (msg.Content == "" && msg.ReasoningContent == "") {
		return
	}
	switch msg.Role {
	case schema.Tool:
		toolName := msg.ToolName
		if toolName == "" && event.Output != nil && event.Output.MessageOutput != nil {
			toolName = event.Output.MessageOutput.ToolName
		}
		stream.EmitAgentEvent(AgentStreamEventToolMessage, event.AgentName, toolName, msg.Content, nil)
	case schema.Assistant, "":
		if msg.ReasoningContent != "" {
			stream.EmitAgentEvent(AgentStreamEventThinkingContentStarted, event.AgentName, "", "", nil)
			_, _ = stream.Write([]byte(trimLineBreaks(msg.ReasoningContent)))
			stream.EmitAgentEvent(AgentStreamEventMessageFinished, event.AgentName, "", "", nil)
		}
		if msg.Content == "" {
			return
		}
		stream.EmitAgentEvent(AgentStreamEventAnswerContentStarted, event.AgentName, "", "", nil)
		_, _ = stream.Write([]byte(trimLeadingLineBreaks(msg.Content)))
		stream.EmitAgentEvent(AgentStreamEventMessageFinished, event.AgentName, "", "", nil)
	default:
		stream.EmitAgentEvent(AgentStreamEventAnswerContentStarted, event.AgentName, "", "", nil)
		_, _ = stream.Write([]byte(trimLeadingLineBreaks(msg.Content)))
		stream.EmitAgentEvent(AgentStreamEventMessageFinished, event.AgentName, "", "", nil)
	}
}

// trimLeadingLineBreaks removes leading CR/LF from the first chunk of a stream segment.
// trimLeadingLineBreaks 去掉流式片段首个 chunk 的前导换行符。
func trimLeadingLineBreaks(content string) string {
	return strings.TrimLeft(content, "\r\n")
}

// trimLineBreaks removes leading and trailing CR/LF from complete message bodies.
// trimLineBreaks 去掉完整消息体首尾换行符。
func trimLineBreaks(content string) string {
	return strings.Trim(content, "\r\n")
}

// splitTrailingLineBreaks separates printable body from trailing CR/LF in a chunk.
// Trailing breaks are deferred so partial chunks do not flush premature line endings to the terminal.
//
// splitTrailingLineBreaks 将 chunk 的可打印正文与尾部 CR/LF 分离。
// 尾部换行延迟输出，避免不完整 chunk 过早向终端刷出换行。
func splitTrailingLineBreaks(content string) (body string, trailingLineBreaks string) {
	trimmed := strings.TrimRight(content, "\r\n")
	return trimmed, content[len(trimmed):]
}
