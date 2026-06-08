package tokens

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestTokenCounterFallsBackForUnknownModel(t *testing.T) {
	counter, err := NewTokenCounter("unknown-local-model", "")
	if err != nil {
		t.Fatalf("NewTokenCounter() error = %v", err)
	}

	tokens, err := counter.CountMessages([]*schema.Message{
		schema.UserMessage("hello world"),
		schema.ToolMessage("tool output", "tool-call-id"),
		schema.AssistantMessage("", nil),
	}, nil)
	if err != nil {
		t.Fatalf("CountMessages() error = %v", err)
	}
	if tokens <= 0 {
		t.Fatalf("CountMessages() = %d, want positive token count", tokens)
	}
}

func TestTokenCounterReturnsErrorWhenUninitialized(t *testing.T) {
	var counter TokenCounter

	_, err := counter.CountMessages([]*schema.Message{schema.UserMessage("hello")}, nil)
	if err == nil {
		t.Fatal("CountMessages() error = nil, want non-nil")
	}
}
