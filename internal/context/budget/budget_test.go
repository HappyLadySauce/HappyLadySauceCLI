package budget

import (
	"context"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

func newTestCalculator() *usage.Calculator {
	return usage.NewCalculator("gpt-4o", 1000)
}

func TestCountSplitsSystemAndConversation(t *testing.T) {
	t.Parallel()

	calc := newTestCalculator()
	messages := []*schema.Message{
		schema.SystemMessage("system policy"),
		schema.UserMessage("hello"),
		schema.AssistantMessage("hi", nil),
	}

	breakdown := calc.Count(usage.CountInput{Messages: messages})

	got := breakdown.Segs.System + breakdown.Segs.Conversation
	raw := usage.NewTokenEstimator("gpt-4o")
	want := raw.CountMessages(messages)
	if got != want {
		t.Fatalf("system + conversation = %d, CountMessages(full) = %d", got, want)
	}
}

func TestCountUsesFallbackInstruction(t *testing.T) {
	t.Parallel()

	calc := newTestCalculator()
	messages := []*schema.Message{schema.UserMessage("hello")}

	breakdown := calc.Count(usage.CountInput{
		Messages:    messages,
		Instruction: "fallback system",
	})

	raw := usage.NewTokenEstimator("gpt-4o")
	wantInst := raw.CountText("fallback system")
	if got := breakdown.Segs.System; got < wantInst {
		t.Fatalf("Segs.System = %d, want at least %d (instruction tokens)", got, wantInst)
	}
}

func TestCountReplyPriming(t *testing.T) {
	t.Parallel()

	calc := newTestCalculator()
	message := schema.SystemMessage("system only")

	breakdown := calc.Count(usage.CountInput{Messages: []*schema.Message{message}})

	raw := usage.NewTokenEstimator("gpt-4o")
	want := raw.CountMessage(message) + usage.ReplyPrimingTokens
	if got := breakdown.Segs.System; got != want {
		t.Fatalf("Segs.System = %d, want %d", got, want)
	}
}

func TestCountAllSegmentsAndPercent(t *testing.T) {
	t.Parallel()

	calc := newTestCalculator()
	breakdown := calc.Count(usage.CountInput{
		Messages:  []*schema.Message{schema.SystemMessage("system"), schema.UserMessage("hello")},
		ToolInfos: []*schema.ToolInfo{{Name: "lookup", Desc: "lookup data"}},
	})

	if breakdown.Segs.System <= 0 {
		t.Fatalf("Segs.System = %d, want > 0", breakdown.Segs.System)
	}
	if breakdown.Segs.Conversation <= 0 {
		t.Fatalf("Segs.Conversation = %d, want > 0", breakdown.Segs.Conversation)
	}
	if breakdown.Segs.Tools <= 0 {
		t.Fatalf("Segs.Tools = %d, want > 0", breakdown.Segs.Tools)
	}

	if got, want := breakdown.Total(), breakdown.Segs.Total(); got != want {
		t.Fatalf("Total() = %d, Segs.Total() = %d", got, want)
	}
	if got, want := breakdown.PercentUsed(), float64(breakdown.Total())/1000*100; got != want {
		t.Fatalf("PercentUsed() = %f, want %f", got, want)
	}
}

func TestCountClassifiesToolMessages(t *testing.T) {
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

	breakdown := calc.Count(usage.CountInput{Messages: messages, ToolInfos: toolInfos})

	if breakdown.Segs.Tools <= 0 {
		t.Fatalf("Segs.Tools = %d, want > 0", breakdown.Segs.Tools)
	}
}

func TestCountToolDefWithoutToolMessages(t *testing.T) {
	t.Parallel()

	calc := newTestCalculator()
	toolInfos := []*schema.ToolInfo{{Name: "lookup", Desc: "lookup data"}}
	breakdown := calc.Count(usage.CountInput{
		Messages:  []*schema.Message{schema.UserMessage("hello")},
		ToolInfos: toolInfos,
	})

	if breakdown.Segs.Tools <= 0 {
		t.Fatalf("Segs.Tools = %d, want > 0", breakdown.Segs.Tools)
	}
}


func TestBudgetWriterWriteReadClear(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	if got := writer.Read(); got != nil {
		t.Fatalf("initial Read() = %#v, want nil", got)
	}

	breakdown := &usage.Breakdown{
		MaxContext:     100,
		EstimatedTotal: 10,
		Segs:           usage.SegmentCounts{Conversation: 10},
	}
	writer.Write(breakdown)
	breakdown.Segs.Conversation = 99

	got := writer.Read()
	if got == nil {
		t.Fatal("Read() = nil, want breakdown")
	}
	if got.Segs.Conversation != 10 {
		t.Fatalf("Read().Segs.Conversation = %d, want defensive copy value 10", got.Segs.Conversation)
	}
	got.Segs.Conversation = 55
	if reread := writer.Read(); reread.Segs.Conversation != 10 {
		t.Fatalf("Read() returned non-defensive copy, got %d", reread.Segs.Conversation)
	}

	writer.Clear()
	if got := writer.Read(); got != nil {
		t.Fatalf("Read() after Clear() = %#v, want nil", got)
	}
}

func TestBudgetWriterConcurrent(t *testing.T) {
	writer := NewBudgetWriter()

	var wg sync.WaitGroup
	for i := 1; i <= 64; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			writer.Write(&usage.Breakdown{
				MaxContext:     1000,
				EstimatedTotal: i,
				Segs:           usage.SegmentCounts{Conversation: i},
			})
			_ = writer.Read()
		}()
	}
	wg.Wait()

	got := writer.Read()
	if got == nil {
		t.Fatal("Read() = nil after concurrent writes")
	}
	if got.EstimatedTotal <= 0 || got.Segs.Conversation <= 0 {
		t.Fatalf("Read() returned incomplete breakdown: %#v", got)
	}
}

func TestBudgetWriterContextRoundTrip(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	ctx := WithBudgetWriter(context.Background(), writer)
	if got := BudgetWriterFromContext(ctx); got != writer {
		t.Fatalf("BudgetWriterFromContext() = %#v, want original writer", got)
	}
	if got := BudgetWriterFromContext(context.Background()); got != nil {
		t.Fatalf("BudgetWriterFromContext(empty) = %#v, want nil", got)
	}
}

func TestBudgetWriterApplyUsage(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	writer.Write(&usage.Breakdown{
		MaxContext:     1000,
		EstimatedTotal: 200,
		Segs:           usage.SegmentCounts{Conversation: 200},
	})

	writer.ApplyUsage(usage.UsageSnapshot{
		PromptTokens: 250,
		Source:       usage.UsageSourceProvider,
	})

	got := writer.Read()
	if got.ActualPrompt != 250 || got.Total() != 250 || got.PercentUsed() != 25 {
		t.Fatalf("breakdown after usage = %#v, want actual usage merged", got)
	}
	if got.EstimatedTotal != 200 || got.Source != usage.UsageSourceProvider {
		t.Fatalf("estimated/source after usage = %d/%q, want 200/provider", got.EstimatedTotal, got.Source)
	}

	got.Segs.Conversation = 999
	if reread := writer.Read(); reread.Segs.Conversation != 250 {
		t.Fatalf("Read() leaked after usage, got %d", reread.Segs.Conversation)
	}
}

func TestBudgetWriterApplyUsageProportionalScaling(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	writer.Write(&usage.Breakdown{
		MaxContext:     128000,
		EstimatedTotal: 1500,
		Segs: usage.SegmentCounts{
			System:       500,
			Conversation: 800,
			Tools:        200,
		},
	})

	writer.ApplyUsage(usage.UsageSnapshot{
		PromptTokens:     1800,
		CompletionTokens: 30,
		Source:           usage.UsageSourceProvider,
	})

	got := writer.Read()
	if got.Segs.System != 600 || got.Segs.Conversation != 960 || got.Segs.Tools != 240 {
		t.Fatalf("Segs = %#v, want System=600 Conversation=960 Tools=240", got.Segs)
	}
	if got.ActualPrompt != 1800 || got.Total() != 1800 {
		t.Fatalf("ActualPrompt/Total = %d/%d, want 1800/1800", got.ActualPrompt, got.Total())
	}
	if got.ActualOutput != 30 {
		t.Fatalf("ActualOutput = %d, want 30", got.ActualOutput)
	}
	if got.Source != usage.UsageSourceProvider {
		t.Fatalf("Source = %q, want provider", got.Source)
	}
}

func TestBudgetWriterApplyUsageStoresCachedAndReasoning(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	writer.Write(&usage.Breakdown{
		MaxContext:     128000,
		EstimatedTotal: 100,
		Segs:           usage.SegmentCounts{Conversation: 100},
	})

	writer.ApplyUsage(usage.UsageSnapshot{
		PromptTokens:     120,
		CompletionTokens: 20,
		CachedTokens:     10,
		ReasoningTokens:  5,
		Source:           usage.UsageSourceProvider,
	})

	got := writer.Read()
	if got.CachedTokens != 10 || got.ReasoningTokens != 5 {
		t.Fatalf("cached/reasoning = %d/%d, want 10/5", got.CachedTokens, got.ReasoningTokens)
	}
}

func TestBudgetWriterFinalizeTurnUsesLastHopForContext(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	writer.BeginTurn()
	writer.AddUsage(usage.UsageSnapshot{PromptTokens: 400, CompletionTokens: 50})
	writer.AddUsage(usage.UsageSnapshot{PromptTokens: 561, CompletionTokens: 52})

	writer.FinalizeTurn(&usage.Breakdown{
		MaxContext:     128000,
		EstimatedTotal: 765,
		Segs: usage.SegmentCounts{
			System:       34,
			Conversation: 483,
			Tools:        248,
		},
		Source: usage.UsageSourceEstimated,
	})

	status := writer.ReadTurnStatus()
	if status.Stats.PromptTokens != 961 || status.Stats.CompletionTokens != 102 {
		t.Fatalf("stats = %#v, want accumulated prompt=961 completion=102", status.Stats)
	}
	if status.Budget == nil {
		t.Fatal("budget = nil")
	}
	if status.Budget.ActualPrompt != 561 || status.Budget.Total() != 561 {
		t.Fatalf("context total = %d/%d, want last-hop prompt 561", status.Budget.ActualPrompt, status.Budget.Total())
	}
	if status.Budget.Source != usage.UsageSourceProvider {
		t.Fatalf("Source = %q, want provider", status.Budget.Source)
	}
	if got := status.Budget.Segs.Total(); got != 561 {
		t.Fatalf("scaled segs total = %d, want 561", got)
	}
}

func TestBudgetWriterFinalizeTurnFallsBackToEstimateWithoutProvider(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	writer.BeginTurn()
	writer.FinalizeTurn(&usage.Breakdown{
		MaxContext:     128000,
		EstimatedTotal: 188,
		Segs: usage.SegmentCounts{
			System:       34,
			Conversation: 79,
			Tools:        75,
		},
	})

	status := writer.ReadTurnStatus()
	if status.Budget == nil {
		t.Fatal("budget = nil")
	}
	if status.Budget.Source != usage.UsageSourceEstimated {
		t.Fatalf("Source = %q, want estimated", status.Budget.Source)
	}
	if status.Budget.Total() != 188 || status.Budget.Segs.Total() != 188 {
		t.Fatalf("budget = %#v, want local estimate 188", status.Budget)
	}
}

func TestBudgetWriterNilReceivers(t *testing.T) {
	t.Parallel()

	var writer *BudgetWriter
	writer.Write(&usage.Breakdown{EstimatedTotal: 1})
	writer.Clear()
	if got := writer.Read(); got != nil {
		t.Fatalf("nil writer Read() = %#v, want nil", got)
	}
	if got := WithBudgetWriter(nil, NewBudgetWriter()); got != nil {
		t.Fatalf("WithBudgetWriter(nil, writer) = %#v, want nil", got)
	}
}
