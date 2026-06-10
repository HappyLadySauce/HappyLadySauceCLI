package budget

import (
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

func newTestCalculator() *usage.Calculator {
	return usage.NewCalculator("gpt-4o", 1000)
}

func TestEstimateBudgetSplitsSystemAndConversation(t *testing.T) {
	t.Parallel()

	calc := newTestCalculator()
	messages := []*schema.Message{
		schema.SystemMessage("system policy"),
		schema.UserMessage("hello"),
		schema.AssistantMessage("hi", nil),
	}

	budget, err := EstimateBudget(BudgetInput{Messages: messages}, calc)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	got := budget.Segs.System + budget.Segs.Conversation
	raw := usage.NewTokenEstimator("gpt-4o")
	want := raw.CountMessages(messages)
	if got != want {
		t.Fatalf("system + conversation = %d, CountMessages(full) = %d", got, want)
	}
}

func TestEstimateBudgetUsesFallbackInstruction(t *testing.T) {
	t.Parallel()

	calc := newTestCalculator()
	messages := []*schema.Message{schema.UserMessage("hello")}

	budget, err := EstimateBudget(BudgetInput{
		Messages:            messages,
		FallbackInstruction: "fallback system",
	}, calc)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	raw := usage.NewTokenEstimator("gpt-4o")
	wantInst := raw.CountText("fallback system")
	if got := budget.Segs.System; got < wantInst {
		t.Fatalf("Segs.System = %d, want at least %d (instruction tokens)", got, wantInst)
	}
}

func TestEstimateBudgetReplyPriming(t *testing.T) {
	t.Parallel()

	calc := newTestCalculator()
	message := schema.SystemMessage("system only")

	budget, err := EstimateBudget(BudgetInput{Messages: []*schema.Message{message}}, calc)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	raw := usage.NewTokenEstimator("gpt-4o")
	want := raw.CountMessage(message) + usage.ReplyPrimingTokens
	if got := budget.Segs.System; got != want {
		t.Fatalf("Segs.System = %d, want %d", got, want)
	}
}

func TestEstimateBudgetAllSegmentsAndPercent(t *testing.T) {
	t.Parallel()

	calc := newTestCalculator()
	budget, err := EstimateBudget(BudgetInput{
		Messages:  []*schema.Message{schema.SystemMessage("system"), schema.UserMessage("hello")},
		ToolInfos: []*schema.ToolInfo{{Name: "lookup", Desc: "lookup data"}},
	}, calc)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	if budget.Segs.System <= 0 {
		t.Fatalf("Segs.System = %d, want > 0", budget.Segs.System)
	}
	if budget.Segs.Conversation <= 0 {
		t.Fatalf("Segs.Conversation = %d, want > 0", budget.Segs.Conversation)
	}
	if budget.Segs.Tools <= 0 {
		t.Fatalf("Segs.Tools = %d, want > 0", budget.Segs.Tools)
	}

	got := budget.Segs.Total()
	if budget.TotalTokens != got {
		t.Fatalf("TotalTokens = %d, Segs.Total() = %d", budget.TotalTokens, got)
	}
	if got, want := budget.PercentFull, float64(budget.TotalTokens)/1000*100; got != want {
		t.Fatalf("PercentFull = %f, want %f", got, want)
	}
}

func TestEstimateBudgetClassifiesToolMessages(t *testing.T) {
	t.Parallel()

	calc := newTestCalculator()
	messages := []*schema.Message{
		schema.SystemMessage("system"),
		schema.UserMessage("weather in Beijing"),
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"北京","lang":"zh"}`,
					},
				},
			},
		},
		schema.ToolMessage("sunny", "call_1", schema.WithToolName("get_weather")),
		schema.AssistantMessage("It is sunny.", nil),
	}
	toolInfos := []*schema.ToolInfo{{Name: "get_weather", Desc: "get weather"}}

	budget, err := EstimateBudget(BudgetInput{Messages: messages, ToolInfos: toolInfos}, calc)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	if budget.Segs.Tools <= 0 {
		t.Fatalf("Segs.Tools = %d, want > 0", budget.Segs.Tools)
	}
}

func TestEstimateBudgetToolDefWithoutToolMessages(t *testing.T) {
	t.Parallel()

	calc := newTestCalculator()
	toolInfos := []*schema.ToolInfo{{Name: "lookup", Desc: "lookup data"}}
	budget, err := EstimateBudget(BudgetInput{
		Messages:  []*schema.Message{schema.UserMessage("hello")},
		ToolInfos: toolInfos,
	}, calc)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	if budget.Segs.Tools <= 0 {
		t.Fatalf("Segs.Tools = %d, want > 0", budget.Segs.Tools)
	}
}

func TestEstimateBudgetRequiresCalculator(t *testing.T) {
	t.Parallel()

	_, err := EstimateBudget(BudgetInput{}, nil)
	if err == nil {
		t.Fatal("EstimateBudget(nil calculator) error = nil, want error")
	}
}

func TestRecalculateBudgetTotals(t *testing.T) {
	t.Parallel()

	budget := &ContextBudget{
		MaxTokens: 100,
		Segs: usage.SegmentCounts{
			Conversation: 20,
			Tools:        5,
			System:       -3,
		},
	}

	RecalculateBudgetTotals(budget)

	if got, want := budget.TotalTokens, 25; got != want {
		t.Fatalf("TotalTokens = %d, want %d", got, want)
	}
	if got, want := budget.PercentFull, 25.0; got != want {
		t.Fatalf("PercentFull = %f, want %f", got, want)
	}
}
