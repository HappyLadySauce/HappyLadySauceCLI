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

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
	contextusage "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/input"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/prompts"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/config"
)

func RunLoop(ctx context.Context, cfg *config.Config) error {

	chatModel, err := openai.NewChatModel(ctx, newChatModelConfig(cfg))
	if err != nil {
		return fmt.Errorf("new chat model: %w", err)
	}

	handlers, err := middlewares.NewChatModelAgentMiddlewares(middlewares.ChatModelAgentMiddlewareConfig{
		Model:           chatModel,
		ModelName:       cfg.Model.Model,
		MaxModelContext: cfg.Model.MaxModelContext,
		MaxOutputTokens: cfg.Model.MaxOutputTokens,
	})
	if err != nil {
		return err
	}

	agentTools := tools.NewAgentTools()
	history := []*schema.Message{}
	promptReader := input.NewPromptReader(ctx, os.Stdin)
	renderer := terminal.NewRenderer(os.Stdout, os.Stderr)
	agentInstruction := prompts.SystemPrompt

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Model:       chatModel,
		Name:        "HappyLadySauce",
		Description: "A Agent for HAPPLADYSAUCECLI",
		Instruction: agentInstruction,
		ToolsConfig: agentTools,
		Handlers:    handlers,
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

		budgetWriter := contextbudget.NewBudgetWriter()
		runCtx := contextbudget.WithBudgetWriter(ctx, budgetWriter)
		iter := runner.Run(runCtx, history)
		assistantMessage, exited, err := ConsumeAgentEvents(iter, renderer)
		if err != nil {
			return err
		}
		if assistantMessage != nil {
			history = append(history, assistantMessage)
			if snapshot, ok := contextusage.SnapshotFromMessage(assistantMessage); ok {
				budgetWriter.ApplyUsage(snapshot)
			}
		}
		if exited {
			return nil
		}
		renderer.WriteContextStatus(budgetWriter.Read())
		renderer.FinishTurn()
	}
}

// newChatModelConfig builds the OpenAI-compatible chat model configuration.
// newChatModelConfig 构建 OpenAI-compatible chat model 配置。
func newChatModelConfig(cfg *config.Config) *openai.ChatModelConfig {
	maxOutputTokens := cfg.Model.MaxOutputTokens
	return &openai.ChatModelConfig{
		BaseURL:             cfg.Model.BaseURL,
		Model:               cfg.Model.Model,
		APIKey:              cfg.Model.APIKey,
		MaxCompletionTokens: &maxOutputTokens,
	}
}
