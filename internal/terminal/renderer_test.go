package terminal

import (
	"bytes"
	"errors"
	"testing"
)

func TestRenderer_WritesInteractiveOutput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	renderer := NewRenderer(&out, &errOut)

	renderer.Prompt()
	renderer.AfterUserInput()
	renderer.AgentLabel("agent")
	renderer.Token("hello")
	renderer.Token(" world")
	renderer.FinishMessage()
	renderer.ThinkingLabel("agent")
	renderer.Token("thinking")
	renderer.FinishMessage()
	renderer.ToolMessage("search", "done")
	renderer.Exit()
	renderer.FinishTurn()
	renderer.Error(errors.New("boom"))

	wantOut := "User> \nagent> hello world\nagent[thinking]> thinking\nsearch> done\nAgent exited.\n\n"
	if out.String() != wantOut {
		t.Fatalf("unexpected stdout: %q", out.String())
	}
	if errOut.String() != "Error: boom\n" {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}
