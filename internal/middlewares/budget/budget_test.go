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

	if _, err := NewBudgetMiddleware(nil, ""); err == nil {
		t.Fatal("NewBudgetMiddleware(nil) error = nil, want error")
	}
	calc := usage.NewCalculator("gpt-4o", 0)
	if _, err := NewBudgetMiddleware(calc, ""); err != nil {
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
	if status := writer.ReadTurnStatus(); status.Stats.ElapsedMs != 0 {
		t.Fatalf("ElapsedMs = %d before turn ends, want 0", status.Stats.ElapsedMs)
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
	if status := writer.ReadTurnStatus(); status.Stats.PromptTokens != 100 || status.Stats.CompletionTokens != 10 {
		t.Fatalf("turn stats = %#v, want prompt=100 completion=10", status.Stats)
	}
}

func TestBudgetMiddlewareAfterAgentWritesPostTurnBudget(t *testing.T) {
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
	if status.Budget == nil {
		t.Fatal("budget writer has nil snapshot")
	}
	if status.Budget.Segs.System <= 0 || status.Budget.Segs.Conversation <= 0 {
		t.Fatalf("budget segments missing expected values: %#v", status.Budget.Segs)
	}
	if status.Budget.Segs.Tools <= 0 {
		t.Fatalf("Segs.Tools = %d, want > 0 for tool definitions", status.Budget.Segs.Tools)
	}
	if status.Stats.ElapsedMs < 0 {
		t.Fatalf("ElapsedMs = %d, want >= 0", status.Stats.ElapsedMs)
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
	if writer.Read() != nil {
		t.Fatalf("nil state should not write budget, got %#v", writer.Read())
	}
}

func TestBudgetMiddlewareAfterAgentBadToolStillWritesBudget(t *testing.T) {
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
	if writer.Read() == nil {
		t.Fatal("budget should still be written when tool counting fails")
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

	budget := writer.Read()
	if budget == nil {
		t.Fatal("budget writer has nil snapshot")
	}
	estimator := usage.NewTokenEstimator("unknown-local-model")
	_, originalConversation := splitMessagesForTest(state.Messages)
	if budget.Segs.Conversation >= estimator.CountMessages(originalConversation) {
		t.Fatalf("budget conversation segment = %d, want less than original conversation tokens", budget.Segs.Conversation)
	}
}

func newTestBudgetMiddleware(t *testing.T) adk.ChatModelAgentMiddleware {
	t.Helper()
	calc := usage.NewCalculator("unknown-local-model", 180)
	middleware, err := NewBudgetMiddleware(calc, "test instruction")
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
