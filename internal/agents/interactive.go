package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"k8s.io/klog/v2"

	ioChannel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/channel"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/prompts"
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
	input := ioChannel.NewContentChannel(ctx, os.Stdin)

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Model:       chatModel,
		Name:        "HAPPLADYSAUCECLI",
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
		fmt.Print("User>")
		promptResult, ok := ioChannel.Receive(ctx, input.ContentCh())
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

		history = append(history, schema.UserMessage(prompt))

		iter := runner.Run(ctx, history)
		exited, err := consumeAgentEvents(iter, &history)
		if err != nil {
			return err
		}
		if exited {
			return nil
		}
	}
}

func consumeAgentEvents(iter *adk.AsyncIterator[*adk.AgentEvent], history *[]*schema.Message) (bool, error) {
	for {
		event, ok := iter.Next()
		if !ok {
			return false, nil
		}
		if event.Err != nil {
			klog.Errorf("agent loop error: %v", event.Err)
			return false, fmt.Errorf("agent loop error: %w", event.Err)
		}
		if event.Action != nil && event.Action.Exit {
			fmt.Println("Agent 主动退出")
			return true, nil
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}

		msg, err := event.Output.MessageOutput.GetMessage()
		if err != nil {
			return false, fmt.Errorf("read agent message: %w", err)
		}
		if msg == nil {
			continue
		}

		if msg.Content != "" {
			label := event.AgentName
			if event.Output.MessageOutput.ToolName != "" {
				label = event.Output.MessageOutput.ToolName
			}
			fmt.Printf("[%s] %s\n", label, msg.Content)
		}

		*history = append(*history, msg)
	}
}
