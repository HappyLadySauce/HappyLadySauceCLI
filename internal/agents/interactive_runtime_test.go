package agents

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	contextsession "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/session"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/input"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/appdirs"
)

type stubAgentRunner struct {
	calls int
}

func (s *stubAgentRunner) Run(ctx context.Context, input []*schema.Message, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	s.calls++
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	if s.calls == 1 {
		gen.Send(&adk.AgentEvent{AgentName: "assistant", Err: errors.New("model failed")})
		gen.Close()
		return iter
	}
	gen.Send(&adk.AgentEvent{AgentName: "assistant", Action: adk.NewExitAction()})
	gen.Close()
	return iter
}

func TestInteractiveRuntimeRunContinuesAfterTurnError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HAPPLADYSAUCECLI_HOME", home)
	if err := appdirs.SetHomeDir(home); err != nil {
		t.Fatalf("SetHomeDir() error = %v", err)
	}
	t.Cleanup(func() { _ = appdirs.SetHomeDir("") })

	ctx := context.Background()
	session, err := contextsession.Open(ctx)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer session.Close()

	inputCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var out, errOut bytes.Buffer
	renderer := terminal.NewRenderer(&out, &errOut)
	runner := &stubAgentRunner{}
	runtime := &interactiveRuntime{
		runner:          runner,
		contextSession:  session,
		promptReader:    input.NewPromptReader(inputCtx, strings.NewReader("hello\nworld\n")),
		renderer:        renderer,
		maxModelContext: 128000,
	}

	if err := runtime.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if runner.calls != 2 {
		t.Fatalf("runner calls = %d, want 2", runner.calls)
	}
	if !strings.Contains(errOut.String(), "Error: model failed") {
		t.Fatalf("expected turn error on stderr, got %q", errOut.String())
	}
}
