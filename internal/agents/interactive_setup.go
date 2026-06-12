package agents

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"

	contextsession "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/session"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/input"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/prompts"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/config"
)

func newInteractiveRuntime(ctx context.Context, cfg *config.Config, in io.Reader, out io.Writer, errOut io.Writer) (*interactiveRuntime, error) {
	if cfg == nil || cfg.Model == nil {
		return nil, errors.New("agent runtime config is incomplete")
	}

	chatModel, err := openai.NewChatModel(ctx, newChatModelConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("new chat model: %w", err)
	}

	contextSession, err := contextsession.Open(ctx)
	if err != nil {
		return nil, err
	}

	inputCtx, cancelInput := context.WithCancel(ctx)
	renderer := terminal.NewRenderer(out, errOut)

	committed := false
	defer func() {
		if !committed {
			cancelInput()
			_ = contextSession.Close()
		}
	}()

	capRegistry, err := tools.NewCapabilityRegistry()
	if err != nil {
		return nil, fmt.Errorf("new capability registry: %w", err)
	}

	promptReader := input.NewPromptReader(inputCtx, in)
	agentTools, err := tools.NewAgentTools()
	if err != nil {
		return nil, fmt.Errorf("new agent tools: %w", err)
	}

	handlers, err := middlewares.NewChatModelAgentMiddlewares(middlewares.ChatModelAgentMiddlewareConfig{
		Model:              chatModel,
		ModelName:          cfg.Model.Model,
		MaxModelContext:    cfg.Model.MaxModelContext,
		MaxOutputTokens:    cfg.Model.MaxOutputTokens,
		CapabilityRegistry: capRegistry,
		OperationBuilders:  tools.NewOperationBuilders(),
		Approver:           newTerminalApprover(promptReader, renderer),
		Security:           cfg.Security,
	})
	if err != nil {
		return nil, err
	}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Model:       chatModel,
		Name:        "HappyLadySauce",
		Description: "A Agent for HAPPLADYSAUCECLI",
		Instruction: prompts.SystemPrompt,
		ToolsConfig: agentTools,
		Handlers:    handlers,
	})
	if err != nil {
		return nil, fmt.Errorf("new chat model agent: %w", err)
	}

	committed = true
	return &interactiveRuntime{
		runner: adk.NewRunner(ctx, adk.RunnerConfig{
			Agent:           agent,
			EnableStreaming: true,
		}),
		contextSession:  contextSession,
		promptReader:    promptReader,
		renderer:        renderer,
		maxModelContext: cfg.Model.MaxModelContext,
	}, nil
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
