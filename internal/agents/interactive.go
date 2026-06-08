package agents

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

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

	msgs := []*schema.Message{}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Model:       chatModel,
		Name:        "HAPPLADYSAUCECLI",
		Description: "A CLI for HAPPLADYSAUCECLI",
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

	iter := runner.Run(ctx, msgs)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024), 1024*1024*10) // 10MB

	for {
		if !scanner.Scan() {
			break
		}

		iter.Next()
	}

	return nil
}
