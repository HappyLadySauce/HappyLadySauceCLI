package budget

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
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

	if _, err := NewBudgetMiddleware(nil); err == nil {
		t.Fatal("NewBudgetMiddleware(nil) error = nil, want error")
	}
	// calculator with zero context is fine — validation is in budget layer
	calc := usage.NewCalculator("gpt-4o", 0)
	if _, err := NewBudgetMiddleware(calc); err != nil {
		t.Fatalf("NewBudgetMiddleware() error = %v", err)
	}
}

func TestBudgetMiddlewareNoWriterDoesNotModifyState(t *testing.T) {
	t.Parallel()

	middleware := newTestBudgetMiddleware(t)
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{schema.UserMessage("hello")}}

	_, got, err := middleware.BeforeModelRewriteState(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState() error = %v", err)
	}
	if got != state || got.Messages[0] != state.Messages[0] {
		t.Fatal("budget middleware should not modify state")
	}
}

func TestBudgetMiddlewareWritesBudget(t *testing.T) {
	t.Parallel()

	writer := contextbudget.NewBudgetWriter()
	ctx := contextbudget.WithBudgetWriter(context.Background(), writer)
	middleware := newTestBudgetMiddleware(t)
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			schema.SystemMessage("system"),
			schema.UserMessage("hello"),
		},
		ToolInfos: []*schema.ToolInfo{{Name: "lookup", Desc: "lookup data"}},
	}

	_, got, err := middleware.BeforeModelRewriteState(ctx, state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState() error = %v", err)
	}
	if got != state {
		t.Fatal("budget middleware should return original state")
	}
	budget := writer.Read()
	if budget == nil {
		t.Fatal("budget writer has nil snapshot")
	}
	if budget.Segs.System <= 0 ||
		budget.Segs.Conversation <= 0 {
		t.Fatalf("budget segments missing expected values: %#v", budget.Segs)
	}
	if budget.Segs.Tools <= 0 {
		t.Fatalf("Segs.Tools = %d, want > 0 for tool definitions", budget.Segs.Tools)
	}
}

func TestBudgetMiddlewareNilAndEmptyState(t *testing.T) {
	t.Parallel()

	writer := contextbudget.NewBudgetWriter()
	ctx := contextbudget.WithBudgetWriter(context.Background(), writer)
	middleware := newTestBudgetMiddleware(t)

	_, got, err := middleware.BeforeModelRewriteState(ctx, nil, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState(nil) error = %v", err)
	}
	if got != nil || writer.Read() != nil {
		t.Fatalf("nil state should no-op, state=%#v budget=%#v", got, writer.Read())
	}

	state := &adk.ChatModelAgentState{}
	_, got, err = middleware.BeforeModelRewriteState(ctx, state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState(empty) error = %v", err)
	}
	if got != state {
		t.Fatal("empty state should return original state")
	}
	if budget := writer.Read(); budget == nil || budget.TotalTokens != 0 {
		t.Fatalf("empty state budget = %#v, want zero snapshot", budget)
	}
}

func TestBudgetMiddlewareBadToolDoesNotBlockBudget(t *testing.T) {
	t.Parallel()

	writer := contextbudget.NewBudgetWriter()
	ctx := contextbudget.WithBudgetWriter(context.Background(), writer)
	middleware := newTestBudgetMiddleware(t)
	state := &adk.ChatModelAgentState{
		ToolInfos: []*schema.ToolInfo{{Name: "bad", Extra: map[string]any{"bad": func() {}}}},
	}

	_, got, err := middleware.BeforeModelRewriteState(ctx, state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState() error = %v", err)
	}
	if got != state {
		t.Fatal("budget middleware should return original state")
	}
	// Calculator swallows tool count errors — budget is written with estimated total 0.
	budget := writer.Read()
	if budget == nil {
		t.Fatal("budget should still be written when tool counting fails")
	}
}

func TestBudgetMiddlewareAfterContentSeesCompactedMessages(t *testing.T) {
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
	writer := contextbudget.NewBudgetWriter()
	ctx := contextbudget.WithBudgetWriter(context.Background(), writer)
	state := &adk.ChatModelAgentState{Messages: append([]*schema.Message{schema.SystemMessage("system")}, longConversation()...)}

	_, compacted, err := contentMiddleware.BeforeModelRewriteState(ctx, state, nil)
	if err != nil {
		t.Fatalf("content BeforeModelRewriteState() error = %v", err)
	}
	_, got, err := budgetMiddleware.BeforeModelRewriteState(ctx, compacted, nil)
	if err != nil {
		t.Fatalf("budget BeforeModelRewriteState() error = %v", err)
	}
	if got != compacted {
		t.Fatal("budget middleware should not modify compacted state")
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
	middleware, err := NewBudgetMiddleware(calc)
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
