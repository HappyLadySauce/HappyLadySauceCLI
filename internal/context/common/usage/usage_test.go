package usage

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestCalculatorCountClassifiesMessages(t *testing.T) {
	t.Parallel()

	calc := NewCalculator("gpt-4o", 128000)
	breakdown := calc.Count(CountInput{
		Messages: []*schema.Message{
			schema.SystemMessage("you are helpful"),
			schema.UserMessage("hello"),
			schema.AssistantMessage("hi there", nil),
		},
	})

	if got := breakdown.Segs.System; got <= 0 {
		t.Fatalf("Segs.System = %d, want > 0", got)
	}
	if got := breakdown.Segs.Conversation; got <= 0 {
		t.Fatalf("Segs.Conversation = %d, want > 0", got)
	}
	if breakdown.Source != UsageSourceEstimated {
		t.Fatalf("Source = %q, want %q", breakdown.Source, UsageSourceEstimated)
	}
}

func TestCalculatorCountClassifiesToolMessages(t *testing.T) {
	t.Parallel()

	calc := NewCalculator("gpt-4o", 128000)
	breakdown := calc.Count(CountInput{
		Messages: []*schema.Message{
			schema.UserMessage("call tool"),
			{
				Role:     schema.Assistant,
				Content:  "",
				ToolCalls: []schema.ToolCall{{ID: "1", Type: "function", Function: schema.FunctionCall{Name: "weather", Arguments: `{"city":"bj"}`}}},
			},
			{ToolCallID: "1", Role: schema.Tool, ToolName: "weather", Content: "sunny"},
		},
	})

	if got := breakdown.Segs.Tools; got <= 0 {
		t.Fatalf("Segs.Tools = %d, want > 0", got)
	}
}

func TestCalculatorCountIncludesToolDefinitions(t *testing.T) {
	t.Parallel()

	calc := NewCalculator("gpt-4o", 128000)
	breakdown := calc.Count(CountInput{
		Messages:  []*schema.Message{schema.UserMessage("hi")},
		ToolInfos: []*schema.ToolInfo{{Name: "weather", Desc: "get weather", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{"city": {Type: schema.String}})}},
	})

	if got := breakdown.Segs.Tools; got <= 0 {
		t.Fatalf("Segs.Tools = %d, want > 0", got)
	}
}

func TestCalculatorCountIncludesInstruction(t *testing.T) {
	t.Parallel()

	calc := NewCalculator("gpt-4o", 128000)
	breakdown := calc.Count(CountInput{
		Messages:    []*schema.Message{schema.UserMessage("hi")},
		Instruction: "You are a helpful assistant with specific rules.",
	})

	if got := breakdown.Segs.System; got <= 0 {
		t.Fatalf("Segs.System = %d, want > 0 (instruction counted as system)", got)
	}
}

func TestCalculatorCountAddsReplyPriming(t *testing.T) {
	t.Parallel()

	calc := NewCalculator("gpt-4o", 128000)

	conversationOnly := calc.estimator.CountMessage(schema.UserMessage("hi"))

	breakdown := calc.Count(CountInput{
		Messages: []*schema.Message{schema.UserMessage("hi")},
	})

	if got, want := breakdown.EstimatedTotal, conversationOnly+ReplyPrimingTokens; got != want {
		t.Fatalf("EstimatedTotal = %d, want %d (conv %d + reply priming %d)", got, want, conversationOnly, ReplyPrimingTokens)
	}
}

func TestBreakdownApplyProviderProportionalScaling(t *testing.T) {
	t.Parallel()

	b := &Breakdown{
		Segs: SegmentCounts{
			System:       500,
			Conversation: 800,
			Tools:        200,
		},
		EstimatedTotal: 1500,
		MaxContext:     128000,
	}

	b.ApplyProvider(UsageSnapshot{
		PromptTokens: 1800,
		Source:       UsageSourceProvider,
	})

	if b.ActualPrompt != 1800 {
		t.Fatalf("ActualPrompt = %d, want 1800", b.ActualPrompt)
	}
	if b.Source != UsageSourceProvider {
		t.Fatalf("Source = %q, want %q", b.Source, UsageSourceProvider)
	}

	// System 500 × 1.2 = 600, Conversation 800 × 1.2 = 960, Tools 200 × 1.2 = 240
	if got := b.Segs.System; got != 600 {
		t.Fatalf("Segs.System = %d, want 600 after scaling", got)
	}
	if got := b.Segs.Conversation; got != 960 {
		t.Fatalf("Segs.Conversation = %d, want 960 after scaling", got)
	}
	if got := b.Segs.Tools; got != 240 {
		t.Fatalf("Segs.Tools = %d, want 240 after scaling", got)
	}
	if b.Total() != 1800 {
		t.Fatalf("Total() = %d, want 1800", b.Total())
	}
}

func TestBreakdownApplyProviderZeroEstimatedSkipsScaling(t *testing.T) {
	t.Parallel()

	b := &Breakdown{
		Segs:           SegmentCounts{Conversation: 100},
		EstimatedTotal: 0,
		MaxContext:     128000,
	}
	b.ApplyProvider(UsageSnapshot{PromptTokens: 500})

	if b.ActualPrompt != 500 {
		t.Fatalf("ActualPrompt = %d, want 500", b.ActualPrompt)
	}
	if got := b.Segs.Conversation; got != 100 {
		t.Fatalf("Segs.Conversation = %d, want 100 (not scaled)", got)
	}
}

func TestBreakdownTotalPrefersActual(t *testing.T) {
	t.Parallel()

	b := &Breakdown{
		EstimatedTotal: 1500,
		ActualPrompt:   1800,
		MaxContext:     128000,
	}
	if b.Total() != 1800 {
		t.Fatalf("Total() = %d, want 1800 (actual)", b.Total())
	}
}

func TestBreakdownTotalFallsBackToEstimated(t *testing.T) {
	t.Parallel()

	b := &Breakdown{
		EstimatedTotal: 1500,
		MaxContext:     128000,
	}
	if b.Total() != 1500 {
		t.Fatalf("Total() = %d, want 1500 (estimated)", b.Total())
	}
}

func TestBreakdownIsZero(t *testing.T) {
	t.Parallel()

	if !((*Breakdown)(nil).IsZero()) {
		t.Fatal("nil breakdown IsZero() = false, want true")
	}
	if !(&Breakdown{}).IsZero() {
		t.Fatal("empty breakdown IsZero() = false, want true")
	}
	if (&Breakdown{EstimatedTotal: 100}).IsZero() {
		t.Fatal("breakdown with EstimatedTotal IsZero() = true, want false")
	}
}

func TestBreakdownPercentUsed(t *testing.T) {
	t.Parallel()

	b := &Breakdown{
		EstimatedTotal: 50000,
		MaxContext:     128000,
	}
	if got, want := int(b.PercentUsed()), 39; got != want {
		t.Fatalf("PercentUsed() = %d%%, want %d%%", got, want)
	}
}

func TestSegmentCountsTotal(t *testing.T) {
	t.Parallel()

	segs := SegmentCounts{System: 100, Conversation: 200, Tools: 50}
	if got, want := segs.Total(), 350; got != want {
		t.Fatalf("Total() = %d, want %d", got, want)
	}
}

func TestSegmentCountsIsZero(t *testing.T) {
	t.Parallel()

	var zero SegmentCounts
	if !zero.IsZero() {
		t.Fatal("zero counts IsZero() = false, want true")
	}
	nonZero := SegmentCounts{Conversation: 10}
	if nonZero.IsZero() {
		t.Fatal("non-zero counts IsZero() = true, want false")
	}
}
