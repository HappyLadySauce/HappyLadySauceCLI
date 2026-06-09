package common

import (
	"errors"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestEstimateBudgetSplitsSystemAndConversationWithoutDoubleCounting(t *testing.T) {
	t.Parallel()

	estimator := NewTokenEstimator("gpt-4o")
	messages := []*schema.Message{
		schema.SystemMessage("system policy"),
		schema.UserMessage("hello"),
		schema.AssistantMessage("hi", nil),
	}

	budget, err := EstimateBudget(BudgetInput{Messages: messages}, estimator, 1000)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	got := budget.Segments[SegmentSystem] + budget.Segments[SegmentConversation]
	want := estimator.CountMessages(messages)
	if got != want {
		t.Fatalf("system + conversation = %d, CountMessages(full) = %d", got, want)
	}
	if budget.Segments[SegmentSystem] != estimator.CountMessage(messages[0]) {
		t.Fatalf("system segment = %d, want %d", budget.Segments[SegmentSystem], estimator.CountMessage(messages[0]))
	}
}

func TestEstimateBudgetUsesFallbackInstructionWhenMessagesHaveNoSystem(t *testing.T) {
	t.Parallel()

	estimator := NewTokenEstimator("gpt-4o")
	messages := []*schema.Message{schema.UserMessage("hello")}

	budget, err := EstimateBudget(BudgetInput{
		Messages:            messages,
		FallbackInstruction: "fallback system",
	}, estimator, 1000)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	if got, want := budget.Segments[SegmentSystem], estimator.CountMessage(schema.SystemMessage("fallback system")); got != want {
		t.Fatalf("fallback system segment = %d, want %d", got, want)
	}
	if got, want := budget.TotalTokens, budget.Segments[SegmentSystem]+budget.Segments[SegmentConversation]; got != want {
		t.Fatalf("TotalTokens = %d, want segment sum %d", got, want)
	}
}

func TestEstimateBudgetAllocatesReplyPrimingToSystemWhenOnlySystemExists(t *testing.T) {
	t.Parallel()

	estimator := NewTokenEstimator("gpt-4o")
	message := schema.SystemMessage("system only")

	budget, err := EstimateBudget(BudgetInput{Messages: []*schema.Message{message}}, estimator, 1000)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	if got, want := budget.Segments[SegmentSystem], estimator.CountMessage(message)+replyPrimingTokens; got != want {
		t.Fatalf("system-only segment = %d, want %d", got, want)
	}
	if _, ok := budget.Segments[SegmentConversation]; ok {
		t.Fatalf("conversation segment exists for system-only input: %#v", budget.Segments)
	}
}

func TestEstimateBudgetAllSegmentsAndPercent(t *testing.T) {
	t.Parallel()

	estimator := NewTokenEstimator("gpt-4o")
	budget, err := EstimateBudget(BudgetInput{
		Messages:      []*schema.Message{schema.SystemMessage("system"), schema.UserMessage("hello")},
		ToolInfos:     []*schema.ToolInfo{{Name: "lookup", Desc: "lookup data"}},
		RulesText:     "rules",
		SkillsText:    "skills",
		MCPText:       "mcp",
		SubagentsText: "subagents",
	}, estimator, 1000)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	for _, segment := range []Segment{
		SegmentSystem,
		SegmentConversation,
		SegmentRules,
		SegmentSkills,
		SegmentMCP,
		SegmentSubagents,
	} {
		if budget.Segments[segment] <= 0 {
			t.Fatalf("segment %s = %d, want > 0 in %#v", segment, budget.Segments[segment], budget.Segments)
		}
	}

	sum := 0
	for _, tokens := range budget.Segments {
		sum += tokens
	}
	if budget.TotalTokens != sum {
		t.Fatalf("TotalTokens = %d, segment sum = %d", budget.TotalTokens, sum)
	}
	if got, want := budget.PercentFull, float64(budget.TotalTokens)/1000*100; got != want {
		t.Fatalf("PercentFull = %f, want %f", got, want)
	}
}

func TestEstimateBudgetCountsToolDefinitionsAsStaticSystemContext(t *testing.T) {
	t.Parallel()

	estimator := NewTokenEstimator("gpt-4o")
	messages := []*schema.Message{
		schema.SystemMessage("system"),
		schema.UserMessage("weather in Beijing"),
		schema.AssistantMessage("", []schema.ToolCall{
			{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "get_weather",
					Arguments: `{"city":"北京","lang":"zh"}`,
				},
			},
		}),
		schema.ToolMessage("sunny", "call_1", schema.WithToolName("get_weather")),
		schema.AssistantMessage("It is sunny.", nil),
	}
	toolInfos := []*schema.ToolInfo{{Name: "get_weather", Desc: "get weather"}}
	toolDefinitionTokens, err := estimator.CountTools(toolInfos)
	if err != nil {
		t.Fatalf("CountTools() error = %v", err)
	}

	budget, err := EstimateBudget(BudgetInput{Messages: messages, ToolInfos: toolInfos}, estimator, 1000)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	if budget.Segments[SegmentTools] <= 0 {
		t.Fatalf("tool interaction segment missing: %#v", budget.Segments)
	}

	systemMessages, conversationMessages, toolMessages := splitBudgetMessages(messages)
	wantSystemTokens := countMessageBodies(estimator, systemMessages) + toolDefinitionTokens
	if got := budget.Segments[SegmentSystem]; got != wantSystemTokens {
		t.Fatalf("system segment = %d, want system + tool definitions %d", got, wantSystemTokens)
	}
	wantMessageTokens := countMessageBodies(estimator, systemMessages) +
		countMessageBodies(estimator, conversationMessages) +
		countMessageBodies(estimator, toolMessages) +
		replyPrimingTokens +
		toolDefinitionTokens
	gotMessageTokens := budget.Segments[SegmentSystem] + budget.Segments[SegmentConversation] + budget.Segments[SegmentTools]
	if gotMessageTokens != wantMessageTokens {
		t.Fatalf("message segments = %d, want %d", gotMessageTokens, wantMessageTokens)
	}
}

func TestEstimateBudgetDoesNotShowToolSegmentWithoutToolMessages(t *testing.T) {
	t.Parallel()

	estimator := NewTokenEstimator("gpt-4o")
	toolInfos := []*schema.ToolInfo{{Name: "lookup", Desc: "lookup data"}}
	toolDefinitionTokens, err := estimator.CountTools(toolInfos)
	if err != nil {
		t.Fatalf("CountTools() error = %v", err)
	}
	budget, err := EstimateBudget(BudgetInput{
		Messages:  []*schema.Message{schema.UserMessage("hello")},
		ToolInfos: toolInfos,
	}, estimator, 1000)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	if _, ok := budget.Segments[SegmentTools]; ok {
		t.Fatalf("tool interaction segment exists without tool messages: %#v", budget.Segments)
	}
	if budget.Segments[SegmentSystem] < toolDefinitionTokens {
		t.Fatalf("system segment = %d, want at least tool definitions %d", budget.Segments[SegmentSystem], toolDefinitionTokens)
	}
}

func TestEstimateBudgetOmitsEmptyFutureSegments(t *testing.T) {
	t.Parallel()

	budget, err := EstimateBudget(BudgetInput{Messages: []*schema.Message{schema.UserMessage("hello")}}, NewTokenEstimator("gpt-4o"), 1000)
	if err != nil {
		t.Fatalf("EstimateBudget() error = %v", err)
	}

	for _, segment := range []Segment{SegmentRules, SegmentSkills, SegmentMCP, SegmentSubagents} {
		if _, ok := budget.Segments[segment]; ok {
			t.Fatalf("empty segment %s present in %#v", segment, budget.Segments)
		}
	}
}

func TestEstimateBudgetRequiresEstimator(t *testing.T) {
	t.Parallel()

	_, err := EstimateBudget(BudgetInput{}, nil, 1000)
	if err == nil {
		t.Fatal("EstimateBudget(nil estimator) error = nil, want error")
	}
}

func TestEstimateBudgetReturnsToolCountError(t *testing.T) {
	t.Parallel()

	badTool := &schema.ToolInfo{
		Name:  "bad",
		Extra: map[string]any{"bad": func() {}},
	}

	_, err := EstimateBudget(BudgetInput{ToolInfos: []*schema.ToolInfo{badTool}}, NewTokenEstimator("gpt-4o"), 1000)
	if err == nil {
		t.Fatal("EstimateBudget() error = nil, want tool marshal error")
	}
	if errors.Is(err, nil) {
		t.Fatalf("unexpected nil-like error: %v", err)
	}
}

func TestRecalculateBudgetTotals(t *testing.T) {
	t.Parallel()

	budget := &ContextBudget{
		MaxTokens: 100,
		Segments: map[Segment]int{
			SegmentConversation: 20,
			SegmentTools:        5,
			SegmentSystem:       -3,
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
