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
		Segments:    map[Segment]int{SegmentConversation: 10},
		PercentFull: 10,
	}
	writer.Write(budget)
	budget.Segments[SegmentConversation] = 99

	got := writer.Read()
	if got == nil {
		t.Fatal("Read() = nil, want budget")
	}
	if got.Segments[SegmentConversation] != 10 {
		t.Fatalf("Read() segment = %d, want defensive copy value 10", got.Segments[SegmentConversation])
	}
	got.Segments[SegmentConversation] = 55
	if reread := writer.Read(); reread.Segments[SegmentConversation] != 10 {
		t.Fatalf("Read() leaked mutable map, got %d", reread.Segments[SegmentConversation])
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
				Segments:    map[Segment]int{SegmentConversation: i},
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
	if got.TotalTokens <= 0 || got.Segments[SegmentConversation] <= 0 {
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
		Segments:             map[Segment]int{SegmentConversation: 200},
		PercentFull:          20,
	})

	writer.ApplyUsage(usage.UsageSnapshot{
		PromptTokens:     250,
		CompletionTokens: 30,
		Source:           usage.UsageSourceProvider,
	})

	got := writer.Read()
	if got.ActualPromptTokens != 250 || got.ActualCompletionTokens != 30 || got.TotalTokens != 250 || got.PercentFull != 25 {
		t.Fatalf("budget after usage = %#v, want actual usage merged", got)
	}
	if got.EstimatedTotalTokens != 200 || got.UsageSource != usage.UsageSourceProvider {
		t.Fatalf("estimated/source after usage = %d/%q, want 200/provider", got.EstimatedTotalTokens, got.UsageSource)
	}

	got.Segments[SegmentConversation] = 999
	if reread := writer.Read(); reread.Segments[SegmentConversation] != 200 {
		t.Fatalf("Read() leaked mutable map after usage, got %d", reread.Segments[SegmentConversation])
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
