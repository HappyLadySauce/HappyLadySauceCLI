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
	if err != nil {
		return fmt.Errorf("new runner: %w", err)
	}

	iter := runner.Run(ctx, history)

	for {
		fmt.Println("User>")
		promptResult, ok := ioChannel.Receive(ctx, input.ContentCh())
		if !ok {
			return nil
		}
		if promptResult.Error != nil {
			if errors.Is(promptResult.Error, context.Canceled) || errors.Is(promptResult.Error, context.DeadlineExceeded) {
				fmt.Fprintln(os.Stderr, "Agent loop stopped by context cancellation.")
				return nil
			}
			return fmt.Errorf("receive user input: %w", promptResult.Error)
		}

		prompt := strings.TrimSpace(promptResult.Text)
		if prompt == "" {
			continue
		}

		

		iter.Next()
	}

	return nil
}
