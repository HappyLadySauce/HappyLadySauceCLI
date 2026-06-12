package agents

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal"
)

func TestConsumeAgentEvents_StreamingAssistant(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(&adk.AgentEvent{
		AgentName: "assistant",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				IsStreaming: true,
				Role:        schema.Assistant,
				MessageStream: schema.StreamReaderFromArray([]*schema.Message{
					{Role: schema.Assistant, Content: "hello"},
					{Role: schema.Assistant, Content: " world"},
				}),
			},
		},
	})
	gen.Close()

	var out bytes.Buffer
	stream := terminal.NewRenderer(&out, &bytes.Buffer{})

	turnMessages, exited, err := ConsumeAgentEvents(context.Background(), iter, stream)
	if err != nil {
		t.Fatalf("ConsumeAgentEvents returned error: %v", err)
	}
	if exited {
		t.Fatal("expected exited=false")
	}
	if len(turnMessages) != 1 {
		t.Fatalf("turnMessages len = %d, want 1", len(turnMessages))
	}
	msg := turnMessages[0]
	if msg.Role != schema.Assistant || msg.Content != "hello world" {
		t.Fatalf("unexpected assistant message: %#v", msg)
	}
	if out.String() != "assistant> hello world\n" {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestConsumeAgentEvents_StreamingAssistantWithReasoning(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(&adk.AgentEvent{
		AgentName: "assistant",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				IsStreaming: true,
				Role:        schema.Assistant,
				MessageStream: schema.StreamReaderFromArray([]*schema.Message{
					{Role: schema.Assistant, ReasoningContent: "\n\nthink"},
					{Role: schema.Assistant, ReasoningContent: "\nmore\n\n"},
					{Role: schema.Assistant, Content: "\n\nanswer"},
				}),
			},
		},
	})
	gen.Close()

	var out bytes.Buffer
	stream := terminal.NewRenderer(&out, &bytes.Buffer{})

	turnMessages, exited, err := ConsumeAgentEvents(context.Background(), iter, stream)
	if err != nil {
		t.Fatalf("ConsumeAgentEvents returned error: %v", err)
	}
	if exited {
		t.Fatal("expected exited=false")
	}
	if len(turnMessages) != 1 {
		t.Fatalf("turnMessages len = %d, want 1", len(turnMessages))
	}
	msg := turnMessages[0]
	if msg.Role != schema.Assistant || msg.ReasoningContent != "think\nmore" || msg.Content != "answer" {
		t.Fatalf("unexpected assistant message: %#v", msg)
	}
	want := "assistant[thinking]> think\nmore\nassistant> answer\n"
	if out.String() != want {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestConsumeAgentEvents_NonStreamingAssistant(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(&adk.AgentEvent{
		AgentName: "assistant",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				Message: schema.AssistantMessage("done", nil),
			},
		},
	})
	gen.Close()

	var out bytes.Buffer
	stream := terminal.NewRenderer(&out, &bytes.Buffer{})

	turnMessages, exited, err := ConsumeAgentEvents(context.Background(), iter, stream)
	if err != nil {
		t.Fatalf("ConsumeAgentEvents returned error: %v", err)
	}
	if exited {
		t.Fatal("expected exited=false")
	}
	if len(turnMessages) != 1 || turnMessages[0].Content != "done" {
		t.Fatalf("unexpected turn messages: %#v", turnMessages)
	}
	if out.String() != "assistant> done\n" {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestConsumeAgentEvents_ToolMessageAppendedToHistory(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(&adk.AgentEvent{
		AgentName: "assistant",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				ToolName: "search",
				Message:  schema.ToolMessage("tool result", "call-1", schema.WithToolName("search")),
			},
		},
	})
	gen.Close()

	var out bytes.Buffer
	stream := terminal.NewRenderer(&out, &bytes.Buffer{})

	turnMessages, exited, err := ConsumeAgentEvents(context.Background(), iter, stream)
	if err != nil {
		t.Fatalf("ConsumeAgentEvents returned error: %v", err)
	}
	if exited {
		t.Fatal("expected exited=false")
	}
	if len(turnMessages) != 1 {
		t.Fatalf("turnMessages len = %d, want 1", len(turnMessages))
	}
	if turnMessages[0].Role != schema.Tool || turnMessages[0].Content != "tool result" {
		t.Fatalf("unexpected tool message: %#v", turnMessages[0])
	}
	if out.String() != "search> tool result\n" {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestConsumeAgentEvents_ReActTurnPreservesToolTrace(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(&adk.AgentEvent{
		AgentName: "assistant",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				Message: &schema.Message{
					Role:    schema.Assistant,
					Content: "\n\n",
					ToolCalls: []schema.ToolCall{{
						ID:   "call-1",
						Type: "function",
						Function: schema.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"city":"重庆","lang":"zh"}`,
						},
					}},
				},
			},
		},
	})
	gen.Send(&adk.AgentEvent{
		AgentName: "assistant",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				ToolName: "get_weather",
				Message:  schema.ToolMessage(`{"weather":"阴"}`, "call-1", schema.WithToolName("get_weather")),
			},
		},
	})
	gen.Send(&adk.AgentEvent{
		AgentName: "assistant",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				Message: schema.AssistantMessage("重庆今天阴天。", nil),
			},
		},
	})
	gen.Close()

	stream := terminal.NewRenderer(&bytes.Buffer{}, &bytes.Buffer{})
	turnMessages, exited, err := ConsumeAgentEvents(context.Background(), iter, stream)
	if err != nil {
		t.Fatalf("ConsumeAgentEvents returned error: %v", err)
	}
	if exited {
		t.Fatal("expected exited=false")
	}
	if len(turnMessages) != 3 {
		t.Fatalf("turnMessages len = %d, want 3", len(turnMessages))
	}
	if turnMessages[0].Role != schema.Assistant || len(turnMessages[0].ToolCalls) != 1 {
		t.Fatalf("first message = %#v, want assistant tool call", turnMessages[0])
	}
	if turnMessages[1].Role != schema.Tool {
		t.Fatalf("second message = %#v, want tool result", turnMessages[1])
	}
	if turnMessages[2].Role != schema.Assistant || turnMessages[2].Content != "重庆今天阴天。" {
		t.Fatalf("third message = %#v, want final assistant answer", turnMessages[2])
	}
}

func TestConsumeAgentEvents_Error(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(&adk.AgentEvent{AgentName: "assistant", Err: errors.New("boom")})
	gen.Close()

	var errOut bytes.Buffer
	stream := terminal.NewRenderer(&bytes.Buffer{}, &errOut)

	turnMessages, exited, err := ConsumeAgentEvents(context.Background(), iter, stream)
	if err == nil {
		t.Fatal("expected error")
	}
	if turnMessages != nil {
		t.Fatalf("expected no turn messages, got %#v", turnMessages)
	}
	if exited {
		t.Fatal("expected exited=false")
	}
	if !strings.Contains(err.Error(), "agent loop error: boom") {
		t.Fatalf("unexpected error: %v", err)
	}
	if errOut.String() != "Error: boom\n" {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}

func TestConsumeAgentEvents_Exit(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(&adk.AgentEvent{AgentName: "assistant", Action: adk.NewExitAction()})
	gen.Close()

	var out bytes.Buffer
	stream := terminal.NewRenderer(&out, &bytes.Buffer{})

	turnMessages, exited, err := ConsumeAgentEvents(context.Background(), iter, stream)
	if err != nil {
		t.Fatalf("ConsumeAgentEvents returned error: %v", err)
	}
	if turnMessages != nil {
		t.Fatalf("expected no turn messages, got %#v", turnMessages)
	}
	if !exited {
		t.Fatal("expected exited=true")
	}
	if out.String() != "Agent exited.\n" {
		t.Fatalf("unexpected output: %q", out.String())
	}
}
