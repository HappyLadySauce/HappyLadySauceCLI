package compact

import (
	stdcontext "context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/estimate"
	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
	contexttracker "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/tracker"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/prompts"
)

type fakeChatModel struct {
	response      *schema.Message
	err           error
	input         []*schema.Message
	generateCalls int
}

func (m *fakeChatModel) Generate(ctx stdcontext.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.generateCalls++
	m.input = input
	if m.err != nil {
		return nil, m.err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return m.response, nil
}

func (m *fakeChatModel) Stream(ctx stdcontext.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{}), nil
}

func TestCompactIfNeededDoesNothingBelowWatermark(t *testing.T) {
	model := &fakeChatModel{response: schema.AssistantMessage("summary", nil)}
	compactor := newTestCompactor(t, model, 1000, 100)
	messages := []*schema.Message{
		schema.UserMessage("short"),
		schema.AssistantMessage("ok", nil),
	}

	got, changed, err := compactor.CompactIfNeeded(stdcontext.Background(), messages)
	if err != nil {
		t.Fatalf("CompactIfNeeded() error = %v", err)
	}
	if changed {
		t.Fatal("CompactIfNeeded() changed below watermark")
	}
	if len(got) != len(messages) || model.generateCalls != 0 {
		t.Fatalf("unexpected compaction result: len=%d calls=%d", len(got), model.generateCalls)
	}
}

func TestCompactIfNeededSummarizesMiddleMessages(t *testing.T) {
	model := &fakeChatModel{response: schema.AssistantMessage("## Goal\nSummarized", nil)}
	compactor := newTestCompactor(t, model, 180, 20)
	messages := []*schema.Message{
		schema.SystemMessage("system"),
		schema.UserMessage("first user request"),
		schema.AssistantMessage("first answer", nil),
		schema.UserMessage(strings.Repeat("middle user ", 80)),
		schema.AssistantMessage(strings.Repeat("middle answer ", 80), nil),
		schema.UserMessage("latest question"),
		schema.AssistantMessage("latest answer", nil),
		schema.UserMessage("final question"),
	}

	got, changed, err := compactor.CompactIfNeeded(testCtxAtCompactionTrigger(180, 20), messages)
	if err != nil {
		t.Fatalf("CompactIfNeeded() error = %v", err)
	}
	if !changed {
		t.Fatal("CompactIfNeeded() did not compact")
	}
	if model.generateCalls != 1 {
		t.Fatalf("Generate calls = %d, want 1", model.generateCalls)
	}
	if got[0].Role != schema.System || got[0].Content != "system" || got[1].Content != "first user request" || got[len(got)-1].Content != "final question" {
		t.Fatalf("unexpected head/tail preservation: %#v", got)
	}

	var summary *schema.Message
	for _, msg := range got {
		if msg != nil && strings.Contains(msg.Content, "## Goal\nSummarized") {
			summary = msg
			break
		}
	}
	if summary == nil {
		t.Fatalf("summary message not found: %#v", got)
	}
	if summary.Role != schema.User || summary.ReasoningContent != "" || len(summary.ToolCalls) != 0 {
		t.Fatalf("summary message has unsafe fields: %#v", summary)
	}
	if !strings.Contains(summary.Content, prompts.ContextCompactionSummaryPrefix) {
		t.Fatalf("summary missing prefix: %q", summary.Content)
	}
	if len(model.input) != 2 || !strings.Contains(model.input[1].Content, "middle user") {
		t.Fatalf("summary model input did not include middle transcript: %#v", model.input)
	}
}

func TestCompactIfNeededCountsSystemMessagePressure(t *testing.T) {
	model := &fakeChatModel{response: schema.AssistantMessage("## Goal\nSystem message summary", nil)}
	compactor := newTestCompactor(t, model, 1000, 100)
	messages := []*schema.Message{
		schema.SystemMessage(strings.Repeat("system policy ", 600)),
		schema.UserMessage("head user"),
		schema.AssistantMessage("head assistant", nil),
		schema.UserMessage("middle user"),
		schema.AssistantMessage("middle assistant", nil),
		schema.UserMessage("latest user"),
		schema.AssistantMessage("latest assistant", nil),
		schema.UserMessage("final user"),
	}

	got, changed, err := compactor.CompactIfNeeded(testCtxAtCompactionTrigger(1000, 100), messages)
	if err != nil {
		t.Fatalf("CompactIfNeeded() error = %v", err)
	}
	if !changed {
		t.Fatal("CompactIfNeeded() did not compact despite session total above trigger")
	}
	if len(model.input) != 2 || strings.Contains(model.input[1].Content, "system policy") {
		t.Fatalf("summary input should summarize only middle history, got %#v", model.input)
	}
	// system messages are preserved through compaction
	found := false
	for _, msg := range got {
		if msg != nil && msg.Role == schema.System {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("compacted messages should preserve system messages: %#v", got)
	}
}

func TestCompactIfNeededDoesNotCutToolPairs(t *testing.T) {
	model := &fakeChatModel{response: schema.AssistantMessage("## Goal\nTool summary", nil)}
	compactor := newTestCompactor(t, model, 160, 20)
	messages := []*schema.Message{
		schema.UserMessage("head user"),
		schema.AssistantMessage("head assistant", nil),
		schema.UserMessage(strings.Repeat("middle ", 80)),
		schema.AssistantMessage("calling", []schema.ToolCall{
			{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "lookup",
					Arguments: `{"q":"x"}`,
				},
			},
		}),
		schema.ToolMessage("result", "call_1", schema.WithToolName("lookup")),
		schema.UserMessage("latest"),
		schema.AssistantMessage("answer", nil),
	}

	got, changed, err := compactor.CompactIfNeeded(testCtxAtCompactionTrigger(160, 20), messages)
	if err != nil {
		t.Fatalf("CompactIfNeeded() error = %v", err)
	}
	if !changed {
		t.Fatal("CompactIfNeeded() did not compact")
	}
	if !tailHasToolPair(got, "call_1") {
		t.Fatalf("compacted messages cut tool pair: %#v", got)
	}
}

func TestCompactIfNeededDoesNotLeaveOpenToolCallInHead(t *testing.T) {
	model := &fakeChatModel{response: schema.AssistantMessage("## Goal\nHead tool summary", nil)}
	compactor := newTestCompactor(t, model, 160, 20)
	messages := []*schema.Message{
		schema.UserMessage("head user"),
		schema.AssistantMessage("calling", []schema.ToolCall{
			{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "lookup",
					Arguments: `{"q":"x"}`,
				},
			},
		}),
		schema.ToolMessage("result", "call_1", schema.WithToolName("lookup")),
		schema.UserMessage(strings.Repeat("middle ", 80)),
		schema.AssistantMessage(strings.Repeat("middle detail ", 80), nil),
		schema.AssistantMessage(strings.Repeat("middle answer ", 80), nil),
		schema.UserMessage("latest"),
		schema.AssistantMessage("answer", nil),
	}

	got, changed, err := compactor.CompactIfNeeded(testCtxAtCompactionTrigger(160, 20), messages)
	if err != nil {
		t.Fatalf("CompactIfNeeded() error = %v", err)
	}
	if !changed {
		t.Fatal("CompactIfNeeded() did not compact")
	}
	if !headHasToolPair(got, "call_1") {
		t.Fatalf("compacted messages left open tool call in head: %#v", got)
	}
}

func TestCompactIfNeededReturnsUnchangedWhenToolPairCannotBeCutSafely(t *testing.T) {
	model := &fakeChatModel{response: schema.AssistantMessage("summary", nil)}
	compactor := newTestCompactor(t, model, 120, 20)
	messages := []*schema.Message{
		schema.UserMessage("head user"),
		schema.AssistantMessage("calling", []schema.ToolCall{
			{ID: "call_1", Type: "function", Function: schema.FunctionCall{Name: "lookup"}},
		}),
		schema.ToolMessage(strings.Repeat("result ", 80), "call_1", schema.WithToolName("lookup")),
		schema.UserMessage("latest"),
		schema.AssistantMessage("answer", nil),
		schema.UserMessage("final"),
	}

	got, changed, err := compactor.CompactIfNeeded(testCtxAtCompactionTrigger(120, 20), messages)
	if !errors.Is(err, ErrUnsafeBoundary) {
		t.Fatalf("CompactIfNeeded() error = %v, want %v", err, ErrUnsafeBoundary)
	}
	if changed || len(got) != len(messages) || model.generateCalls != 0 {
		t.Fatalf("expected unchanged, changed=%v len=%d calls=%d", changed, len(got), model.generateCalls)
	}
}

func TestCompactIfNeededReturnsErrorWithoutDroppingMessages(t *testing.T) {
	wantErr := errors.New("model down")
	model := &fakeChatModel{err: wantErr}
	compactor := newTestCompactor(t, model, 160, 20)
	messages := longConversation()

	got, changed, err := compactor.CompactIfNeeded(testCtxAtCompactionTrigger(160, 20), messages)
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("CompactIfNeeded() error = %v, want %v", err, wantErr)
	}
	if changed || len(got) != len(messages) {
		t.Fatalf("expected unchanged on error, changed=%v len=%d", changed, len(got))
	}
}

func TestTokenEstimatorCountsMiddleMessages(t *testing.T) {
	estimator := estimate.NewTokenEstimator("unknown-local-model")
	tokens := estimator.CountMessages([]*schema.Message{
		{
			Role:             schema.Assistant,
			Content:          "answer",
			ReasoningContent: "thinking",
			ToolCalls: []schema.ToolCall{
				{ID: "call_1", Type: "function", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"q":"x"}`}},
			},
		},
		schema.ToolMessage("result", "call_1", schema.WithToolName("lookup")),
	})
	if tokens <= 0 {
		t.Fatalf("middle message token count = %d, want > 0", tokens)
	}
}

func TestSummaryTokenLimitHasMinimumFloor(t *testing.T) {
	compactor := newTestCompactor(t, &fakeChatModel{response: schema.AssistantMessage("summary", nil)}, 1000, 200)
	// 200/4 = 50 → clamped to minimumSummaryTokens (512)
	if got := compactor.summaryTokenLimit(); got != minimumSummaryTokens {
		t.Fatalf("summaryTokenLimit() = %d, want %d", got, minimumSummaryTokens)
	}
}

func TestSummaryTokenLimitHitsDefaultCap(t *testing.T) {
	compactor := newTestCompactor(t, &fakeChatModel{response: schema.AssistantMessage("summary", nil)}, 128000, 32768)
	// 32768/4 = 8192 → clamped to defaultSummaryTokens (4096)
	if got := compactor.summaryTokenLimit(); got != defaultSummaryTokens {
		t.Fatalf("summaryTokenLimit() = %d, want %d", got, defaultSummaryTokens)
	}
}

func testCtxAtCompactionTrigger(maxContext, maxOutput int) stdcontext.Context {
	tracker := contexttracker.New()
	tracker.BeginConversation()
	trigger := (maxContext - maxOutput) * compactionTriggerPercent / 100
	tracker.AddTurn(&contextmodel.Turn{Total: trigger})
	return contexttracker.WithTracker(stdcontext.Background(), tracker)
}

func newTestCompactor(t *testing.T, chatModel model.BaseChatModel, maxContext, maxOutput int) *Compactor {
	t.Helper()
	compactor, err := NewCompactor(Config{
		Model:           chatModel,
		ModelName:       "unknown-local-model",
		MaxModelContext: maxContext,
		MaxOutputTokens: maxOutput,
	})
	if err != nil {
		t.Fatalf("NewCompactor() error = %v", err)
	}
	return compactor
}

func longConversation() []*schema.Message {
	return []*schema.Message{
		schema.UserMessage("head user"),
		schema.AssistantMessage("head assistant", nil),
		schema.UserMessage(strings.Repeat("middle user ", 80)),
		schema.AssistantMessage(strings.Repeat("middle assistant ", 80), nil),
		schema.UserMessage("latest user"),
		schema.AssistantMessage("latest assistant", nil),
		schema.UserMessage("final user"),
	}
}

func headHasToolPair(messages []*schema.Message, callID string) bool {
	assistantIndex := -1
	toolIndex := -1
	for i, msg := range messages {
		if msg == nil {
			continue
		}
		if strings.Contains(msg.Content, prompts.ContextCompactionSummaryPrefix) {
			break
		}
		if msg.Role == schema.Assistant {
			for _, call := range msg.ToolCalls {
				if call.ID == callID {
					assistantIndex = i
				}
			}
		}
		if msg.Role == schema.Tool && msg.ToolCallID == callID {
			toolIndex = i
		}
	}
	return assistantIndex >= 0 && toolIndex > assistantIndex
}

func tailHasToolPair(messages []*schema.Message, callID string) bool {
	assistantIndex := -1
	toolIndex := -1
	for i, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.Role == schema.Assistant {
			for _, call := range msg.ToolCalls {
				if call.ID == callID {
					assistantIndex = i
				}
			}
		}
		if msg.Role == schema.Tool && msg.ToolCallID == callID {
			toolIndex = i
		}
	}
	return assistantIndex >= 0 && toolIndex > assistantIndex
}
