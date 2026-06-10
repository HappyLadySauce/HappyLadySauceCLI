package budget

import (
	"context"
	"sync"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

func TestBudgetWriterWriteReadClear(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	if got := writer.Read(); got != nil {
		t.Fatalf("initial Read() = %#v, want nil", got)
	}

	budget := &ContextBudget{
		MaxTokens:   100,
		TotalTokens: 10,
		Segs:        usage.SegmentCounts{Conversation: 10},
		PercentFull: 10,
	}
	writer.Write(budget)
	budget.Segs.Conversation = 99

	got := writer.Read()
	if got == nil {
		t.Fatal("Read() = nil, want budget")
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
			writer.Write(&ContextBudget{
				MaxTokens:   1000,
				TotalTokens: i,
				Segs:        usage.SegmentCounts{Conversation: i},
				PercentFull: float64(i) / 10,
			})
			_ = writer.Read()
		}()
	}
	wg.Wait()

	got := writer.Read()
	if got == nil {
		t.Fatal("Read() = nil after concurrent writes")
	}
	if got.TotalTokens <= 0 || got.Segs.Conversation <= 0 {
		t.Fatalf("Read() returned incomplete budget: %#v", got)
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
	writer.Write(&ContextBudget{
		MaxTokens:            1000,
		TotalTokens:          200,
		EstimatedTotalTokens: 200,
		Segs:                 usage.SegmentCounts{Conversation: 200},
		PercentFull:          20,
	})

	writer.ApplyUsage(usage.UsageSnapshot{
		PromptTokens: 250,
		Source:       usage.UsageSourceProvider,
	})

	got := writer.Read()
	if got.ActualPromptTokens != 250 || got.TotalTokens != 250 || got.PercentFull != 25 {
		t.Fatalf("budget after usage = %#v, want actual usage merged", got)
	}
	if got.EstimatedTotalTokens != 200 || got.UsageSource != usage.UsageSourceProvider {
		t.Fatalf("estimated/source after usage = %d/%q, want 200/provider", got.EstimatedTotalTokens, got.UsageSource)
	}

	got.Segs.Conversation = 999
	if reread := writer.Read(); reread.Segs.Conversation != 250 {
		t.Fatalf("Read() leaked after usage, got %d", reread.Segs.Conversation)
	}
}

func TestBudgetWriterApplyUsageProportionalScaling(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	writer.Write(&ContextBudget{
		MaxTokens:            128000,
		TotalTokens:          1500,
		EstimatedTotalTokens: 1500,
		Segs: usage.SegmentCounts{
			System:       500,
			Conversation: 800,
			Tools:        200,
		},
		PercentFull: 1.17,
	})

	writer.ApplyUsage(usage.UsageSnapshot{
		PromptTokens:    1800,
		CompletionTokens: 30,
		Source:           usage.UsageSourceProvider,
	})

	got := writer.Read()
	if got.Segs.System != 600 || got.Segs.Conversation != 960 || got.Segs.Tools != 240 {
		t.Fatalf("Segs = %#v, want System=600 Conversation=960 Tools=240", got.Segs)
	}
	if got.ActualPromptTokens != 1800 || got.TotalTokens != 1800 {
		t.Fatalf("ActualPrompt/Total = %d/%d, want 1800/1800", got.ActualPromptTokens, got.TotalTokens)
	}
	if got.ActualCompletionTokens != 30 {
		t.Fatalf("ActualCompletionTokens = %d, want 30", got.ActualCompletionTokens)
	}
	if got.UsageSource != usage.UsageSourceProvider {
		t.Fatalf("UsageSource = %q, want provider", got.UsageSource)
	}
}

func TestBudgetWriterApplyUsageStoresCachedAndReasoning(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	writer.Write(&ContextBudget{
		MaxTokens:            128000,
		TotalTokens:          100,
		EstimatedTotalTokens: 100,
		Segs:                 usage.SegmentCounts{Conversation: 100},
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

func TestBudgetWriterNilReceivers(t *testing.T) {
	t.Parallel()

	var writer *BudgetWriter
	writer.Write(&ContextBudget{TotalTokens: 1})
	writer.Clear()
	if got := writer.Read(); got != nil {
		t.Fatalf("nil writer Read() = %#v, want nil", got)
	}
	if got := WithBudgetWriter(nil, NewBudgetWriter()); got != nil {
		t.Fatalf("WithBudgetWriter(nil, writer) = %#v, want nil", got)
	}
}
