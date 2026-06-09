package agents

import (
	"bytes"
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
	renderer := terminal.NewRenderer(&out, &bytes.Buffer{})

	msg, exited, err := consumeAgentEvents(iter, renderer)
	if err != nil {
		t.Fatalf("consumeAgentEvents returned error: %v", err)
	}
	if exited {
		t.Fatal("expected exited=false")
	}
	if msg == nil || msg.Role != schema.Assistant || msg.Content != "hello world" {
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
	renderer := terminal.NewRenderer(&out, &bytes.Buffer{})

	msg, exited, err := consumeAgentEvents(iter, renderer)
	if err != nil {
		t.Fatalf("consumeAgentEvents returned error: %v", err)
	}
	if exited {
		t.Fatal("expected exited=false")
	}
	if msg == nil || msg.Role != schema.Assistant || msg.ReasoningContent != "think\nmore" || msg.Content != "answer" {
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
	renderer := terminal.NewRenderer(&out, &bytes.Buffer{})

	msg, exited, err := consumeAgentEvents(iter, renderer)
	if err != nil {
		t.Fatalf("consumeAgentEvents returned error: %v", err)
	}
	if exited {
		t.Fatal("expected exited=false")
	}
	if msg == nil || msg.Role != schema.Assistant || msg.Content != "done" {
		t.Fatalf("unexpected assistant message: %#v", msg)
	}
	if out.String() != "assistant> done\n" {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestConsumeAgentEvents_ToolMessageDoesNotBecomeAssistantHistory(t *testing.T) {
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
	renderer := terminal.NewRenderer(&out, &bytes.Buffer{})

	msg, exited, err := consumeAgentEvents(iter, renderer)
	if err != nil {
		t.Fatalf("consumeAgentEvents returned error: %v", err)
	}
	if exited {
		t.Fatal("expected exited=false")
	}
	if msg != nil {
		t.Fatalf("expected no assistant history message, got %#v", msg)
	}
	if out.String() != "search> tool result\n" {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestConsumeAgentEvents_Error(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(&adk.AgentEvent{AgentName: "assistant", Err: errors.New("boom")})
	gen.Close()

	var errOut bytes.Buffer
	renderer := terminal.NewRenderer(&bytes.Buffer{}, &errOut)

	msg, exited, err := consumeAgentEvents(iter, renderer)
	if err == nil {
		t.Fatal("expected error")
	}
	if msg != nil {
		t.Fatalf("expected no assistant message, got %#v", msg)
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
	renderer := terminal.NewRenderer(&out, &bytes.Buffer{})

	msg, exited, err := consumeAgentEvents(iter, renderer)
	if err != nil {
		t.Fatalf("consumeAgentEvents returned error: %v", err)
	}
	if msg != nil {
		t.Fatalf("expected no assistant message, got %#v", msg)
	}
	if !exited {
		t.Fatal("expected exited=true")
	}
	if out.String() != "Agent exited.\n" {
		t.Fatalf("unexpected output: %q", out.String())
	}
}
