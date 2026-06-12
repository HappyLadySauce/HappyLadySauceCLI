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

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/contextstore"
	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
	contexttracker "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/tracker"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/input"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/prompts"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools"
	"github.com/HappyLadySauce/HappyLadySauceCLI/migrations"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/config"
	storagesqlite "github.com/HappyLadySauce/HappyLadySauceCLI/pkg/storage/sqlite"
)

func RunLoop(ctx context.Context, cfg *config.Config) error {
	if cfg == nil || cfg.Model == nil {
		return errors.New("agent runtime config is incomplete")
	}

	chatModel, err := openai.NewChatModel(ctx, newChatModelConfig(cfg))
	if err != nil {
		return fmt.Errorf("new chat model: %w", err)
	}

	contextDB, err := storagesqlite.OpenDefault(ctx, "context.sqlite")
	if err != nil {
		return fmt.Errorf("open context database: %w", err)
	}
	defer func() {
		if closeErr := contextDB.Close(); closeErr != nil {
			klog.Errorf("close context database: %v", closeErr)
		}
	}()

	if err := migrations.Apply(ctx, contextDB); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	contextStore := contextstore.New(contextDB)
	sessionTracker := contexttracker.New()

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

		sessionTracker.BeginConversation()
		runCtx := contexttracker.WithTracker(ctx, sessionTracker)
		iter := runner.Run(runCtx, history)
		turnMessages, exited, err := ConsumeAgentEvents(iter, renderer)
		if err != nil {
			sessionTracker.SetMessages([]*schema.Message{userMessage})
			conversation := sessionTracker.FinishConversation(err)
			persistErr := persistContextSnapshots(ctx, contextStore, sessionTracker, conversation)
			return errors.Join(err, persistErr)
		}
		conversationMessages := append([]*schema.Message{userMessage}, turnMessages...)
		sessionTracker.SetMessages(conversationMessages)
		conversation := sessionTracker.FinishConversation(nil)
		if err := persistContextSnapshots(ctx, contextStore, sessionTracker, conversation); err != nil {
			return fmt.Errorf("save context conversation: %w", err)
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

// persistContextSnapshots stores the session aggregate before its child conversation.
// This preserves the foreign-key order while keeping persistence outside the tracker.
//
// persistContextSnapshots 先保存 session 聚合，再保存其子 conversation。
// 这样既保证外键顺序，也让持久化逻辑留在 tracker 外部。
func persistContextSnapshots(ctx context.Context, store *contextstore.Repository, tracker *contexttracker.Tracker, conversation *contextmodel.Conversation) error {
	if store == nil || tracker == nil {
		return nil
	}
	if err := store.SaveSession(ctx, tracker.Session()); err != nil {
		return err
	}
	if err := store.SaveConversation(ctx, conversation); err != nil {
		return err
	}
	return nil
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
