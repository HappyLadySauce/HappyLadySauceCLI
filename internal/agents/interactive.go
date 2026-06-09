package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"k8s.io/klog/v2"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/input"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/prompts"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/config"
)

func RunLoop(ctx context.Context, cfg *config.Config) error {

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: cfg.Model.BaseURL,
		Model:   cfg.Model.Model,
		APIKey:  cfg.Model.APIKey,
	})
	if err != nil {
		return fmt.Errorf("new chat model: %w", err)
	}

	tools := tools.NewAgentTools()
	history := []*schema.Message{}
	promptReader := input.NewPromptReader(ctx, os.Stdin)
	renderer := terminal.NewRenderer(os.Stdout, os.Stderr)

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Model:       chatModel,
		Name:        "HappyLadySauce",
		Description: "A Agent for HAPPLADYSAUCECLI",
		Instruction: prompts.SystemPrompt,
		ToolsConfig: tools,
	})
	if err != nil {
		return fmt.Errorf("new chat model agent: %w", err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
	})

	for {
		renderer.Prompt()
		promptResult, ok := promptReader.Receive(ctx)
		if !ok {
			return nil
		}
		if promptResult.Error != nil {
			if errors.Is(promptResult.Error, context.Canceled) || errors.Is(promptResult.Error, context.DeadlineExceeded) {
				klog.Info("Agent loop stopped by context cancellation.")
				return nil
			}
			return fmt.Errorf("receive user input: %w", promptResult.Error)
		}

		prompt := strings.TrimSpace(promptResult.Text)
		if prompt == "" {
			continue
		}

		renderer.AfterUserInput()
		history = append(history, schema.UserMessage(prompt))

		iter := runner.Run(ctx, history)
		assistantMessage, exited, err := consumeAgentEvents(iter, renderer)
		if err != nil {
			return err
		}
		if assistantMessage != nil {
			history = append(history, assistantMessage)
		}
		if exited {
			return nil
		}
		renderer.FinishTurn()
	}
}

func consumeAgentEvents(iter *adk.AsyncIterator[*adk.AgentEvent], renderer *terminal.Renderer) (*schema.Message, bool, error) {
	var lastAssistant *schema.Message

	for {
		event, ok := iter.Next()
		if !ok {
			return lastAssistant, false, nil
		}
		if event.Err != nil {
			klog.Errorf("agent loop error: %v", event.Err)
			renderer.Error(event.Err)
			return nil, false, fmt.Errorf("agent loop error: %w", event.Err)
		}
		if event.Action != nil && event.Action.Exit {
			renderer.Exit()
			return lastAssistant, true, nil
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}

		msg, err := renderMessageOutput(event, renderer)
		if err != nil {
			return nil, false, fmt.Errorf("read agent message: %w", err)
		}
		if msg != nil && msg.Role == schema.Assistant {
			lastAssistant = msg
		}
	}
}

func renderMessageOutput(event *adk.AgentEvent, renderer *terminal.Renderer) (*schema.Message, error) {
	output := event.Output.MessageOutput
	if output.IsStreaming {
		return renderStreamingMessage(event, renderer)
	}

	msg := output.Message
	if msg == nil {
		return nil, nil
	}
	renderCompleteMessage(event, renderer, msg)
	return msg, nil
}

func renderStreamingMessage(event *adk.AgentEvent, renderer *terminal.Renderer) (*schema.Message, error) {
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
		renderer.StartThinkingAnimation(event.AgentName)
		defer renderer.StopThinkingAnimation()
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
				renderer.StopThinkingAnimation()
				renderer.ThinkingLabel(event.AgentName)
				renderedThinking = true
			}
			reasoningContent := chunk.ReasoningContent
			if firstThinkingChunk {
				reasoningContent = trimLeadingLineBreaks(reasoningContent)
				firstThinkingChunk = false
			}
			renderedContent, trailingLineBreaks := splitTrailingLineBreaks(reasoningContent)
			if renderedContent != "" {
				renderer.Token(pendingThinkingLineBreaks)
				renderer.Token(renderedContent)
				pendingThinkingLineBreaks = trailingLineBreaks
			} else {
				pendingThinkingLineBreaks += trailingLineBreaks
			}
		}
		if role == schema.Assistant && chunk.Content != "" {
			if !renderedAnswer {
				renderer.StopThinkingAnimation()
				if renderedThinking {
					renderer.FinishMessage()
				}
				renderer.AgentLabel(event.AgentName)
				renderedAnswer = true
			}
			content := chunk.Content
			if firstAnswerChunk {
				content = trimLeadingLineBreaks(content)
				firstAnswerChunk = false
			}
			renderer.Token(content)
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
		renderer.FinishMessage()
		return msg, nil
	}
	if renderedThinking {
		renderer.FinishMessage()
		return msg, nil
	}

	renderCompleteMessage(event, renderer, msg)
	return msg, nil
}

func renderCompleteMessage(event *adk.AgentEvent, renderer *terminal.Renderer, msg *schema.Message) {
	if msg == nil || (msg.Content == "" && msg.ReasoningContent == "") {
		return
	}
	switch msg.Role {
	case schema.Tool:
		toolName := msg.ToolName
		if toolName == "" && event.Output != nil && event.Output.MessageOutput != nil {
			toolName = event.Output.MessageOutput.ToolName
		}
		renderer.ToolMessage(toolName, msg.Content)
	case schema.Assistant, "":
		if msg.ReasoningContent != "" {
			renderer.ThinkingLabel(event.AgentName)
			renderer.Token(trimLineBreaks(msg.ReasoningContent))
			renderer.FinishMessage()
		}
		if msg.Content == "" {
			return
		}
		renderer.AgentLabel(event.AgentName)
		renderer.Token(trimLeadingLineBreaks(msg.Content))
		renderer.FinishMessage()
	default:
		renderer.AgentLabel(event.AgentName)
		renderer.Token(trimLeadingLineBreaks(msg.Content))
		renderer.FinishMessage()
	}
}

func trimLeadingLineBreaks(content string) string {
	return strings.TrimLeft(content, "\r\n")
}

func trimLineBreaks(content string) string {
	return strings.Trim(content, "\r\n")
}

func splitTrailingLineBreaks(content string) (body string, trailingLineBreaks string) {
	trimmed := strings.TrimRight(content, "\r\n")
	return trimmed, content[len(trimmed):]
}
