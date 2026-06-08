package messages

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	tokenutils "github.com/HappyLadySauce/HappyLadySauceCLI/pkg/utils/tokens"
)

func TestTrimByMessageCountKeepsLatestMessagesWithoutProtectingSystem(t *testing.T) {
	trimmed := TrimByMessageCount([]*schema.Message{
		schema.SystemMessage("system"),
		schema.UserMessage("first"),
		schema.AssistantMessage("second", nil),
		schema.UserMessage("third"),
	}, 2)

	if got, want := messageContents(trimmed), "second\nthird\n"; got != want {
		t.Fatalf("TrimByMessageCount contents = %q, want %q", got, want)
	}
}

func TestTrimByTokenBudgetKeepsLatestUserWithoutProtectingSystem(t *testing.T) {
	counter, err := tokenutils.NewTokenCounter("unknown-local-model", "")
	if err != nil {
		t.Fatalf("NewTokenCounter() error = %v", err)
	}

	trimmed, err := TrimByTokenBudget([]*schema.Message{
		schema.SystemMessage("system can be removed " + strings.Repeat("s ", 200)),
		schema.UserMessage("old user " + strings.Repeat("x ", 200)),
		schema.AssistantMessage("old assistant "+strings.Repeat("y ", 200), nil),
		schema.UserMessage("latest user must remain"),
	}, nil, 64, counter)
	if err != nil {
		t.Fatalf("TrimByTokenBudget() error = %v", err)
	}

	contents := messageContents(trimmed)
	if !strings.Contains(contents, "latest user must remain") {
		t.Fatalf("trimmed messages missing latest user: %q", contents)
	}
	if strings.Contains(contents, "system can be removed") || strings.Contains(contents, "old user") || strings.Contains(contents, "old assistant") {
		t.Fatalf("trimmed messages still contain old history: %q", contents)
	}
}

func TestTrimByTokenBudgetErrorsWhenLatestUserStillExceedsBudget(t *testing.T) {
	counter, err := tokenutils.NewTokenCounter("unknown-local-model", "")
	if err != nil {
		t.Fatalf("NewTokenCounter() error = %v", err)
	}

	_, err = TrimByTokenBudget([]*schema.Message{
		schema.UserMessage("latest user must remain " + strings.Repeat("x ", 100)),
	}, nil, 1, counter)
	if err == nil {
		t.Fatal("TrimByTokenBudget() error = nil, want budget exceeded error")
	}
	if !strings.Contains(err.Error(), "message token budget exceeded after trimming") {
		t.Fatalf("TrimByTokenBudget() error = %q, want budget exceeded message", err)
	}
}

func TestLatestAssistantUsageReturnsLatestAssistantUsage(t *testing.T) {
	first := schema.AssistantMessage("first", nil)
	first.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{TotalTokens: 3}}
	latest := schema.AssistantMessage("latest", nil)
	latest.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{TotalTokens: 7}}

	usage := LatestAssistantUsage([]*schema.Message{
		schema.UserMessage("hello"),
		first,
		schema.AssistantMessage("missing usage", nil),
		latest,
	})
	if usage == nil || usage.TotalTokens != 7 {
		t.Fatalf("LatestAssistantUsage() = %#v, want total tokens 7", usage)
	}
}

func TestLatestAssistantUsageReturnsNilWhenUsageMissing(t *testing.T) {
	usage := LatestAssistantUsage([]*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("missing usage", nil),
	})
	if usage != nil {
		t.Fatalf("LatestAssistantUsage() = %#v, want nil", usage)
	}
}

func TestSameMessageSliceComparesByMessagePointer(t *testing.T) {
	first := schema.UserMessage("first")
	second := schema.AssistantMessage("second", nil)

	if !SameMessageSlice([]*schema.Message{first, second}, []*schema.Message{first, second}) {
		t.Fatal("SameMessageSlice() = false, want true")
	}
	if SameMessageSlice([]*schema.Message{first, second}, []*schema.Message{first, schema.AssistantMessage("second", nil)}) {
		t.Fatal("SameMessageSlice() = true for different message pointers, want false")
	}
}

func messageContents(messages []*schema.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		if msg != nil {
			b.WriteString(msg.Content)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
