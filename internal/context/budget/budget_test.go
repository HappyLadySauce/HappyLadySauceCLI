package budget

import (
	"context"
	"sync"
	"testing"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

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

func TestBudgetWriterUsesLastHopPromptNotAccumulated(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	writer.BeginTurn()
	writer.AddUsage(usage.UsageSnapshot{PromptTokens: 400, CompletionTokens: 50})
	writer.AddUsage(usage.UsageSnapshot{PromptTokens: 561, CompletionTokens: 52, TotalTokens: 613})

	writer.FinalizeTurn(128000, 613)

	status := writer.ReadTurnStatus()
	if status.PromptTokens != 561 {
		t.Fatalf("PromptTokens = %d, want last-hop prompt 561", status.PromptTokens)
	}
	if status.CompletionTokens != 102 {
		t.Fatalf("CompletionTokens = %d, want accumulated completion 102", status.CompletionTokens)
	}
	if status.ContextTokens != 613 {
		t.Fatalf("ContextTokens = %d, want provider session total 613", status.ContextTokens)
	}
	if status.TotalTokens() != 613 {
		t.Fatalf("TotalTokens() = %d, want provider session total 613", status.TotalTokens())
	}
	if got, want := status.PercentUsed(), float64(613)/128000*100; got != want {
		t.Fatalf("PercentUsed() = %f, want %f", got, want)
	}
}

func TestBudgetWriterFinalizeTurnUsesProviderSessionTotal(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	writer.BeginTurn()
	writer.AddUsage(usage.UsageSnapshot{PromptTokens: 547, CompletionTokens: 58, TotalTokens: 605})

	writer.FinalizeTurn(128000, 605)

	status := writer.ReadTurnStatus()
	if status.PromptTokens != 547 {
		t.Fatalf("PromptTokens = %d, want 547", status.PromptTokens)
	}
	if status.ContextTokens != 605 {
		t.Fatalf("ContextTokens = %d, want provider session total 605", status.ContextTokens)
	}
}

func TestBudgetWriterFinalizeTurnWithoutProviderSessionTotal(t *testing.T) {
	t.Parallel()

	writer := NewBudgetWriter()
	writer.BeginTurn()
	writer.FinalizeTurn(128000, 0)

	status := writer.ReadTurnStatus()
	if status.ContextTokens != 0 {
		t.Fatalf("ContextTokens = %d, want 0 without provider session total", status.ContextTokens)
	}
	if status.PromptTokens != 0 {
		t.Fatalf("PromptTokens = %d, want 0 without provider usage", status.PromptTokens)
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
			writer.BeginTurn()
			writer.AddUsage(usage.UsageSnapshot{PromptTokens: i, CompletionTokens: i, TotalTokens: i * 2})
			writer.FinalizeTurn(1000, i*2)
			_ = writer.ReadTurnStatus()
		}()
	}
	wg.Wait()

	status := writer.ReadTurnStatus()
	if status.ContextTokens <= 0 || status.MaxContext != 1000 {
		t.Fatalf("ReadTurnStatus() returned incomplete stats: %#v", status)
	}
}

func TestBudgetWriterNilReceivers(t *testing.T) {
	t.Parallel()

	var writer *BudgetWriter
	writer.BeginTurn()
	writer.AddUsage(usage.UsageSnapshot{PromptTokens: 1})
	writer.FinalizeTurn(1000, 1)
	if got := writer.ReadTurnStatus(); got.ContextTokens != 0 {
		t.Fatalf("nil writer ReadTurnStatus() = %#v, want zero value", got)
	}
	if got := WithBudgetWriter(nil, NewBudgetWriter()); got != nil {
		t.Fatalf("WithBudgetWriter(nil, writer) = %#v, want nil", got)
	}
}

func TestTurnStatsIsZero(t *testing.T) {
	t.Parallel()

	if got := (TurnStats{}).IsZero(); !got {
		t.Fatal("empty IsZero() = false, want true")
	}
	stats := TurnStats{ContextTokens: 1}
	if got := stats.IsZero(); got {
		t.Fatal("non-empty IsZero() = true, want false")
	}
}

func TestTurnStatsTotalTokensReturnsContextOccupancy(t *testing.T) {
	t.Parallel()

	stats := TurnStats{PromptTokens: 547, CompletionTokens: 58, ContextTokens: 605}
	if got, want := stats.TotalTokens(), 605; got != want {
		t.Fatalf("TotalTokens() = %d, want session context %d", got, want)
	}
}

func TestTurnStatsPercentUsed(t *testing.T) {
	t.Parallel()

	if got := (TurnStats{}).PercentUsed(); got != 0 {
		t.Fatalf("empty PercentUsed() = %f, want 0", got)
	}
	stats := TurnStats{ContextTokens: 500, MaxContext: 1000}
	if got, want := stats.PercentUsed(), 50.0; got != want {
		t.Fatalf("PercentUsed() = %f, want %f", got, want)
	}
}
