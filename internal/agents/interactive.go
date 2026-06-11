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

	contextsqlite "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/store/sqlite"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/usage"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/input"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/prompts"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/config"
)

func RunLoop(ctx context.Context, cfg *config.Config) error {
	if cfg == nil || cfg.Model == nil {
		return errors.New("agent runtime config is incomplete")
	}

	rawChatModel, err := openai.NewChatModel(ctx, newChatModelConfig(cfg))
	if err != nil {
		return fmt.Errorf("new chat model: %w", err)
	}
	chatModel := rawChatModel

	contextStore, err := contextsqlite.OpenDefault(ctx)
	if err != nil {
		return fmt.Errorf("open context store: %w", err)
	}
	sessionContext := usage.NewSessionContext(usage.WithStore(contextStore))
	defer func() {
		if closeErr := sessionContext.Close(); closeErr != nil {
			klog.Errorf("close context store: %v", closeErr)
		}
	}()

	agentInstruction := prompts.SystemPrompt
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
		userMessage := schema.UserMessage(prompt)
		history = append(history, userMessage)

		conversationRecorder := sessionContext.BeginConversation()
		runCtx := usage.WithSessionContext(ctx, sessionContext)
		runCtx = usage.WithConversationRecorder(runCtx, conversationRecorder)
		iter := runner.Run(runCtx, history)
		turnMessages, exited, err := ConsumeAgentEvents(iter, renderer)
		if err != nil {
			conversationRecorder.SetMessages([]*schema.Message{userMessage})
			_, persistErr := sessionContext.FinishConversation(ctx, conversationRecorder, err)
			return errors.Join(err, persistErr)
		}
		conversationMessages := append([]*schema.Message{userMessage}, turnMessages...)
		conversationRecorder.SetMessages(conversationMessages)
		conversation, persistErr := sessionContext.FinishConversation(ctx, conversationRecorder, nil)
		if persistErr != nil {
			return fmt.Errorf("save context conversation: %w", persistErr)
		}
		if len(turnMessages) > 0 {
			history = append(history, turnMessages...)
		}
		renderer.WriteConversationStatus(conversation, cfg.Model.MaxModelContext)
		renderer.FinishTurn()
		if exited {
			return nil
		}
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
