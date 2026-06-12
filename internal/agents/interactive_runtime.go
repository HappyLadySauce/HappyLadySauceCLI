package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"k8s.io/klog/v2"

	contextsession "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/session"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/logger"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/input"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal"
)

type interactiveRuntime struct {
	runner          *adk.Runner
	contextSession  *contextsession.Service
	promptReader    *input.PromptReader
	renderer        *terminal.Renderer
	history         []*schema.Message
	maxModelContext int
}

// Run consumes user prompts and executes one ChatModelAgent run per non-empty prompt.
// Run 消费用户 prompt，并为每条非空 prompt 执行一次 ChatModelAgent run。
func (r *interactiveRuntime) Run(ctx context.Context) error {
	if r == nil {
		return errors.New("interactive runtime is nil")
	}

	for {
		prompt, ok, err := r.receivePrompt(ctx)
		if !ok {
			return nil
		}
		if err != nil {
			return err
		}
		if prompt == "" {
			continue
		}

		exited, err := r.runPrompt(ctx, prompt)
		if err != nil {
			return err
		}
		if exited {
			return nil
		}
	}
}

// Close releases runtime-owned resources.
// Close 释放 runtime 持有的资源。
func (r *interactiveRuntime) Close() {
	if r == nil || r.contextSession == nil {
		return
	}
	if err := r.contextSession.Close(); err != nil {
		klog.Errorf("close context session: %v", err)
	}
}

func (r *interactiveRuntime) receivePrompt(ctx context.Context) (string, bool, error) {
	r.renderer.Prompt()
	promptResult, ok := r.promptReader.Receive(ctx)
	if !ok {
		return "", false, nil
	}
	if promptResult.Error != nil {
		if errors.Is(promptResult.Error, context.Canceled) || errors.Is(promptResult.Error, context.DeadlineExceeded) {
			klog.Info("Agent loop stopped by context cancellation.")
			return "", false, nil
		}
		return "", false, fmt.Errorf("receive user input: %w", promptResult.Error)
	}
	return strings.TrimSpace(promptResult.Text), true, nil
}

func (r *interactiveRuntime) runPrompt(ctx context.Context, prompt string) (bool, error) {
	r.renderer.AfterUserInput()
	userMessage := schema.UserMessage(prompt)
	r.history = append(r.history, userMessage)

	runCtx := r.contextSession.BeginTurn(ctx, prompt)
	logger.PhaseInfo(runCtx, 1, "user_turn_begin",
		"user_prompt_len", len(prompt),
		"history_messages", len(r.history))
	iter := r.runner.Run(runCtx, r.history)
	turnMessages, exited, err := ConsumeAgentEvents(runCtx, iter, r.renderer)
	if err != nil {
		modelCalls := r.contextSession.CurrentTurnCount()
		_, persistErr := r.contextSession.FinishTurn(runCtx, []*schema.Message{userMessage}, err)
		logger.PhaseInfo(runCtx, 1, "user_turn_end",
			"model_calls", modelCalls,
			"turn_messages", 1,
			"history_messages", len(r.history),
			"error", true)
		return false, errors.Join(err, persistErr)
	}

	conversationMessages := append([]*schema.Message{userMessage}, turnMessages...)
	modelCalls := r.contextSession.CurrentTurnCount()
	status, err := r.contextSession.FinishTurn(runCtx, conversationMessages, nil)
	if err != nil {
		return false, fmt.Errorf("save context conversation: %w", err)
	}
	if len(turnMessages) > 0 {
		r.history = append(r.history, turnMessages...)
	}

	logger.PhaseInfo(runCtx, 1, "user_turn_end",
		"model_calls", modelCalls,
		"turn_messages", len(conversationMessages),
		"history_messages", len(r.history),
		"prompt_agg", status.Prompt,
		"completion_agg", status.Completion,
		"total_agg", status.Total,
		"content", status.ContextTokens,
		"elapsed_ms", status.Elapsed.Milliseconds(),
		"error", false)
	r.renderer.WriteConversationStatus(status, r.maxModelContext)
	r.renderer.FinishTurn()
	return exited, nil
}
