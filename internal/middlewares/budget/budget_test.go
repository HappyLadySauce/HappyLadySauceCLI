package budget

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
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

	if _, err := NewBudgetMiddleware(0); err == nil {
		t.Fatal("NewBudgetMiddleware(0) error = nil, want error")
	}
	if _, err := NewBudgetMiddleware(128000); err != nil {
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

func TestBudgetMiddlewareAfterAgentWritesPostTurnStats(t *testing.T) {
	t.Parallel()

	writer := budget.NewBudgetWriter()
	session := usage.NewSessionContext()
	session.UpdateFromSnapshot(usage.UsageSnapshot{TotalTokens: 358, PromptTokens: 318, CompletionTokens: 40})
	ctx := usage.WithSessionContext(context.Background(), session)
	ctx = budget.WithBudgetWriter(ctx, writer)
	middleware := newTestBudgetMiddleware(t)
	writer.BeginTurn()
	writer.AddUsage(usage.UsageSnapshot{PromptTokens: 318, CompletionTokens: 40, TotalTokens: 358})

	_, err := middleware.AfterAgent(ctx, &adk.ChatModelAgentState{})
	if err != nil {
		t.Fatalf("AfterAgent() error = %v", err)
	}

	status := writer.ReadTurnStatus()
	if status.MaxContext != 180 {
		t.Fatalf("MaxContext = %d, want 180", status.MaxContext)
	}
	if status.ContextTokens != 358 {
		t.Fatalf("ContextTokens = %d, want provider session total 358", status.ContextTokens)
	}
	if status.PromptTokens != 318 {
		t.Fatalf("PromptTokens = %d, want last-hop prompt 318", status.PromptTokens)
	}
	if status.ElapsedMs < 0 {
		t.Fatalf("ElapsedMs = %d, want >= 0", status.ElapsedMs)
	}
}

func TestBudgetMiddlewareAfterAgentNilWriterNoops(t *testing.T) {
	t.Parallel()

	middleware := newTestBudgetMiddleware(t)
	_, err := middleware.AfterAgent(context.Background(), &adk.ChatModelAgentState{})
	if err != nil {
		t.Fatalf("AfterAgent() error = %v", err)
	}
}

func newTestBudgetMiddleware(t *testing.T) adk.ChatModelAgentMiddleware {
	t.Helper()
	middleware, err := NewBudgetMiddleware(180)
	if err != nil {
		t.Fatalf("NewBudgetMiddleware() error = %v", err)
	}
	return middleware
}
