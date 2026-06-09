package middlewares

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	contextcommon "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/compact"
)

func TestNewBudgetMiddlewareValidation(t *testing.T) {
	t.Parallel()

	if _, err := NewBudgetMiddleware(nil, 1000); err == nil {
		t.Fatal("NewBudgetMiddleware(nil) error = nil, want error")
	}
	if _, err := NewBudgetMiddleware(contextcommon.NewTokenEstimator("gpt-4o"), 0); err == nil {
		t.Fatal("NewBudgetMiddleware(max=0) error = nil, want error")
	}
}

func TestBudgetMiddlewareNoWriterDoesNotModifyState(t *testing.T) {
	t.Parallel()

	middleware := newTestBudgetMiddleware(t, 1000)
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

	writer := contextcommon.NewBudgetWriter()
	ctx := contextcommon.WithBudgetWriter(context.Background(), writer)
	middleware := newTestBudgetMiddleware(t, 1000)
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
	if budget.Segments[contextcommon.SegmentSystem] <= 0 ||
		budget.Segments[contextcommon.SegmentConversation] <= 0 {
		t.Fatalf("budget segments missing expected values: %#v", budget.Segments)
	}
	if _, ok := budget.Segments[contextcommon.SegmentTools]; ok {
		t.Fatalf("tool interaction segment exists without tool messages: %#v", budget.Segments)
	}
}

func TestBudgetMiddlewareNilAndEmptyState(t *testing.T) {
	t.Parallel()

	writer := contextcommon.NewBudgetWriter()
	ctx := contextcommon.WithBudgetWriter(context.Background(), writer)
	middleware := newTestBudgetMiddleware(t, 1000)

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

func TestBudgetMiddlewareSwallowsEstimateError(t *testing.T) {
	t.Parallel()

	writer := contextcommon.NewBudgetWriter()
	ctx := contextcommon.WithBudgetWriter(context.Background(), writer)
	middleware := newTestBudgetMiddleware(t, 1000)
	state := &adk.ChatModelAgentState{
		ToolInfos: []*schema.ToolInfo{{Name: "bad", Extra: map[string]any{"bad": func() {}}}},
	}

	_, got, err := middleware.BeforeModelRewriteState(ctx, state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState() error = %v", err)
	}
	if got != state {
		t.Fatal("budget middleware should return original state on estimate error")
	}
	if budget := writer.Read(); budget != nil {
		t.Fatalf("budget should not be written on estimate error: %#v", budget)
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
	contentMiddleware, err := NewContentMiddleware(compactor)
	if err != nil {
		t.Fatalf("NewContentMiddleware() error = %v", err)
	}
	budgetMiddleware := newTestBudgetMiddleware(t, 180)
	writer := contextcommon.NewBudgetWriter()
	ctx := contextcommon.WithBudgetWriter(context.Background(), writer)
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
	estimator := contextcommon.NewTokenEstimator("unknown-local-model")
	_, originalConversation := splitMessagesForTest(state.Messages)
	if budget.Segments[contextcommon.SegmentConversation] >= estimator.CountMessages(originalConversation) {
		t.Fatalf("budget conversation segment = %d, want less than original conversation tokens", budget.Segments[contextcommon.SegmentConversation])
	}
}

func newTestBudgetMiddleware(t *testing.T, maxContextTokens int) adk.ChatModelAgentMiddleware {
	t.Helper()
	middleware, err := NewBudgetMiddleware(contextcommon.NewTokenEstimator("unknown-local-model"), maxContextTokens)
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
