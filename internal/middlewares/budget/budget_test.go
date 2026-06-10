package budget

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/compact"
	contentmiddleware "github.com/HappyLadySauce/HappyLadySauceCLI/internal/middlewares/content"
)

type fakeChatModel struct {
	response *schema.Message
	err      error
}

func (m *fakeChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *fakeChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{}), nil
}

func TestNewBudgetMiddlewareValidation(t *testing.T) {
	t.Parallel()

	estimator := usage.NewTokenEstimator("gpt-4o")
	if _, err := NewBudgetMiddleware(0, estimator); err == nil {
		t.Fatal("NewBudgetMiddleware(0) error = nil, want error")
	}
	if _, err := NewBudgetMiddleware(128000, nil); err == nil {
		t.Fatal("NewBudgetMiddleware(nil estimator) error = nil, want error")
	}
	if _, err := NewBudgetMiddleware(128000, estimator); err != nil {
		t.Fatalf("NewBudgetMiddleware() error = %v", err)
	}
}

func TestBudgetMiddlewareBeforeAgentStartsTurn(t *testing.T) {
	t.Parallel()

	writer := budget.NewBudgetWriter()
	ctx := budget.WithBudgetWriter(context.Background(), writer)
	middleware := newTestBudgetMiddleware(t)

	ctx, _, err := middleware.BeforeAgent(ctx, &adk.ChatModelAgentContext{})
	if err != nil {
		t.Fatalf("BeforeAgent() error = %v", err)
	}
	if status := writer.ReadTurnStatus(); status.ElapsedMs != 0 {
		t.Fatalf("ElapsedMs = %d before turn ends, want 0", status.ElapsedMs)
	}
}

func TestBudgetMiddlewareAfterModelRewriteStateAccumulatesUsage(t *testing.T) {
	t.Parallel()

	writer := budget.NewBudgetWriter()
	ctx := budget.WithBudgetWriter(context.Background(), writer)
	middleware := newTestBudgetMiddleware(t)
	writer.BeginTurn()

	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			schema.UserMessage("hello"),
			{
				Role:    schema.Assistant,
				Content: "hi",
				ResponseMeta: &schema.ResponseMeta{
					Usage: &schema.TokenUsage{PromptTokens: 100, CompletionTokens: 10},
				},
			},
		},
	}

	_, got, err := middleware.AfterModelRewriteState(ctx, state, nil)
	if err != nil {
		t.Fatalf("AfterModelRewriteState() error = %v", err)
	}
	if got != state {
		t.Fatal("budget middleware should not modify state")
	}
	if status := writer.ReadTurnStatus(); status.CompletionTokens != 10 {
		t.Fatalf("turn stats = %#v, want completion=10", status)
	}
	writer.FinalizeTurn(128000, 0)
	if status := writer.ReadTurnStatus(); status.PromptTokens != 100 {
		t.Fatalf("PromptTokens = %d after FinalizeTurn, want last-hop prompt 100", status.PromptTokens)
	}
}

func TestBudgetMiddlewareAfterAgentWritesPostTurnStats(t *testing.T) {
	t.Parallel()

	writer := budget.NewBudgetWriter()
	ctx := budget.WithBudgetWriter(context.Background(), writer)
	middleware := newTestBudgetMiddleware(t)
	writer.BeginTurn()

	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			schema.SystemMessage("system"),
			schema.UserMessage("hello"),
			schema.AssistantMessage("done", nil),
		},
		ToolInfos: []*schema.ToolInfo{{Name: "lookup", Desc: "lookup data"}},
	}

	ctx, err := middleware.AfterAgent(ctx, state)
	if err != nil {
		t.Fatalf("AfterAgent() error = %v", err)
	}
	if ctx == nil {
		t.Fatal("AfterAgent() ctx = nil")
	}

	status := writer.ReadTurnStatus()
	if status.MaxContext != 180 {
		t.Fatalf("MaxContext = %d, want 180", status.MaxContext)
	}
	if status.ContextTokens <= 0 {
		t.Fatalf("ContextTokens = %d, want > 0 from local estimate", status.ContextTokens)
	}
	if status.ElapsedMs < 0 {
		t.Fatalf("ElapsedMs = %d, want >= 0", status.ElapsedMs)
	}
}

func TestBudgetMiddlewareAfterAgentNilStateNoops(t *testing.T) {
	t.Parallel()

	writer := budget.NewBudgetWriter()
	ctx := budget.WithBudgetWriter(context.Background(), writer)
	middleware := newTestBudgetMiddleware(t)

	_, err := middleware.AfterAgent(ctx, nil)
	if err != nil {
		t.Fatalf("AfterAgent(nil) error = %v", err)
	}
	if got := writer.ReadTurnStatus(); got.ContextTokens != 0 {
		t.Fatalf("nil state should not write stats, got %#v", got)
	}
}

func TestBudgetMiddlewareAfterAgentBadToolStillWritesStats(t *testing.T) {
	t.Parallel()

	writer := budget.NewBudgetWriter()
	ctx := budget.WithBudgetWriter(context.Background(), writer)
	middleware := newTestBudgetMiddleware(t)
	writer.BeginTurn()

	state := &adk.ChatModelAgentState{
		Messages:  []*schema.Message{schema.UserMessage("hello"), schema.AssistantMessage("ok", nil)},
		ToolInfos: []*schema.ToolInfo{{Name: "bad", Extra: map[string]any{"bad": func() {}}}},
	}

	_, err := middleware.AfterAgent(ctx, state)
	if err != nil {
		t.Fatalf("AfterAgent() error = %v", err)
	}
	if got := writer.ReadTurnStatus(); got.ContextTokens <= 0 {
		t.Fatal("stats should still be written when tool counting fails")
	}
}

func TestBudgetMiddlewareAfterAgentUsesCompactedMessages(t *testing.T) {
	model := &fakeChatModel{response: schema.AssistantMessage("## Goal\nsummary", nil)}
	compactor, err := compact.NewCompactor(compact.Config{
		Model:           model,
		ModelName:       "unknown-local-model",
		MaxModelContext: 180,
		MaxOutputTokens: 20,
	})
	if err != nil {
		t.Fatalf("NewCompactor() error = %v", err)
	}
	contentMiddleware, err := contentmiddleware.NewContentMiddleware(compactor)
	if err != nil {
		t.Fatalf("NewContentMiddleware() error = %v", err)
	}
	budgetMiddleware := newTestBudgetMiddleware(t)
	writer := budget.NewBudgetWriter()
	ctx := budget.WithBudgetWriter(context.Background(), writer)
	state := &adk.ChatModelAgentState{Messages: append([]*schema.Message{schema.SystemMessage("system")}, longConversation()...)}

	_, compacted, err := contentMiddleware.BeforeModelRewriteState(ctx, state, nil)
	if err != nil {
		t.Fatalf("content BeforeModelRewriteState() error = %v", err)
	}
	compacted.Messages = append(compacted.Messages, schema.AssistantMessage("final answer", nil))
	writer.BeginTurn()

	_, err = budgetMiddleware.AfterAgent(ctx, compacted)
	if err != nil {
		t.Fatalf("AfterAgent() error = %v", err)
	}

	status := writer.ReadTurnStatus()
	if status.ContextTokens <= 0 {
		t.Fatal("stats writer has zero context tokens")
	}
	estimator := usage.NewTokenEstimator("unknown-local-model")
	_, originalConversation := splitMessagesForTest(state.Messages)
	originalTokens := estimator.CountMessages(originalConversation)
	if status.ContextTokens >= originalTokens {
		t.Fatalf("context tokens = %d, want less than original conversation tokens %d", status.ContextTokens, originalTokens)
	}
}

func newTestBudgetMiddleware(t *testing.T) adk.ChatModelAgentMiddleware {
	t.Helper()
	estimator := usage.NewTokenEstimator("unknown-local-model")
	middleware, err := NewBudgetMiddleware(180, estimator)
	if err != nil {
		t.Fatalf("NewBudgetMiddleware() error = %v", err)
	}
	return middleware
}

func splitMessagesForTest(messages []*schema.Message) ([]*schema.Message, []*schema.Message) {
	systemMessages := make([]*schema.Message, 0, 1)
	conversationMessages := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.Role == schema.System {
			systemMessages = append(systemMessages, msg)
			continue
		}
		conversationMessages = append(conversationMessages, msg)
	}
	return systemMessages, conversationMessages
}

func longConversation() []*schema.Message {
	return []*schema.Message{
		schema.UserMessage("head user"),
		schema.AssistantMessage("head answer", nil),
		schema.UserMessage(strings.Repeat("middle ", 80)),
		schema.AssistantMessage(strings.Repeat("middle answer ", 80), nil),
		schema.UserMessage("latest"),
		schema.AssistantMessage("answer", nil),
		schema.UserMessage("final"),
	}
}
