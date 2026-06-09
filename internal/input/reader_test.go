package input

import (
	"context"
	"strings"
	"testing"
)

func TestPromptReader_SingleLine(t *testing.T) {
	results := readAllPrompts(t, "hello\n")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Text != "hello" {
		t.Fatalf("expected hello, got %q", results[0].Text)
	}
}

func TestPromptReader_BackslashContinuation(t *testing.T) {
	results := readAllPrompts(t, "line one\\\nline two\n")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	want := "line one\nline two"
	if results[0].Text != want {
		t.Fatalf("expected %q, got %q", want, results[0].Text)
	}
}

func TestPromptReader_MultilineBlock(t *testing.T) {
	results := readAllPrompts(t, "\"\"\"\nfirst\n\nsecond\n\"\"\"\n")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	want := "first\n\nsecond"
	if results[0].Text != want {
		t.Fatalf("expected %q, got %q", want, results[0].Text)
	}
}

func TestPromptReader_ReturnsEmptyLines(t *testing.T) {
	results := readAllPrompts(t, "\n\nhello\n\n")
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	wants := []string{"", "", "hello", ""}
	for i, want := range wants {
		if results[i].Text != want {
			t.Fatalf("result[%d].Text = %q, want %q", i, results[i].Text, want)
		}
	}
}

func TestPromptReader_LiteralBackslashAtEnd(t *testing.T) {
	results := readAllPrompts(t, "C:\\\\\n")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Text != `C:\` {
		t.Fatalf("expected C:\\, got %q", results[0].Text)
	}
}

func TestPromptReader_LiteralBackslashN(t *testing.T) {
	results := readAllPrompts(t, "print(\"\\n\")\n")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	want := `print("\n")`
	if results[0].Text != want {
		t.Fatalf("expected %q, got %q", want, results[0].Text)
	}
}

func TestPromptReader_ForwardSlashUnchanged(t *testing.T) {
	results := readAllPrompts(t, "https://example.com/a/b\n")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	want := "https://example.com/a/b"
	if results[0].Text != want {
		t.Fatalf("expected %q, got %q", want, results[0].Text)
	}
}

func TestPromptReader_MultipleMessages(t *testing.T) {
	results := readAllPrompts(t, "one\n\"\"\"\ntwo\nthree\n\"\"\"\nfour\n")
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Text != "one" || results[1].Text != "two\nthree" || results[2].Text != "four" {
		t.Fatalf("unexpected messages: %#v", results)
	}
}

func TestSplitLineContinuation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		line      string
		content   string
		continues bool
	}{
		{line: `foo\`, content: "foo", continues: true},
		{line: `foo\\`, content: `foo\`, continues: false},
		{line: `foo\\\`, content: `foo\`, continues: true},
		{line: `print("\n")`, content: `print("\n")`, continues: false},
		{line: `a/b\c`, content: `a/b\c`, continues: false},
	}

	for _, tc := range cases {
		content, continues := splitLineContinuation(tc.line)
		if content != tc.content || continues != tc.continues {
			t.Fatalf("splitLineContinuation(%q) = (%q, %v), want (%q, %v)",
				tc.line, content, continues, tc.content, tc.continues)
		}
	}
}

func readAllPrompts(t *testing.T, content string) []PromptResult {
	t.Helper()

	reader := NewPromptReader(context.Background(), strings.NewReader(content))

	var results []PromptResult
	for {
		result, ok := reader.Receive(context.Background())
		if !ok {
			return results
		}
		if result.Error != nil {
			t.Fatalf("unexpected error: %v", result.Error)
		}
		results = append(results, result)
	}
}
