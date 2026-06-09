package channel

import (
	"context"
	"strings"
	"testing"
)

func TestReadLoop_SingleLine(t *testing.T) {
	results := runReadLoop(t, "hello\n")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Text != "hello" {
		t.Fatalf("expected hello, got %q", results[0].Text)
	}
}

func TestReadLoop_BackslashContinuation(t *testing.T) {
	input := "line one\\\nline two\n"
	results := runReadLoop(t, input)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	want := "line one\nline two"
	if results[0].Text != want {
		t.Fatalf("expected %q, got %q", want, results[0].Text)
	}
}

func TestReadLoop_MultilineBlock(t *testing.T) {
	input := "\"\"\"\nfirst\n\nsecond\n\"\"\"\n"
	results := runReadLoop(t, input)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	want := "first\n\nsecond"
	if results[0].Text != want {
		t.Fatalf("expected %q, got %q", want, results[0].Text)
	}
}

func TestReadLoop_SkipsEmptyLines(t *testing.T) {
	results := runReadLoop(t, "\n\nhello\n\n")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Text != "hello" {
		t.Fatalf("expected hello, got %q", results[0].Text)
	}
}

func TestReadLoop_LiteralBackslashAtEnd(t *testing.T) {
	results := runReadLoop(t, "C:\\\\\n")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Text != `C:\` {
		t.Fatalf("expected C:\\, got %q", results[0].Text)
	}
}

func TestReadLoop_LiteralBackslashN(t *testing.T) {
	results := runReadLoop(t, `print("\n")\n`)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	want := `print("\n")\n`
	if results[0].Text != want {
		t.Fatalf("expected %q, got %q", want, results[0].Text)
	}
}

func TestReadLoop_ForwardSlashUnchanged(t *testing.T) {
	results := runReadLoop(t, "https://example.com/a/b\n")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	want := "https://example.com/a/b"
	if results[0].Text != want {
		t.Fatalf("expected %q, got %q", want, results[0].Text)
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

func TestReadLoop_MultipleMessages(t *testing.T) {
	results := runReadLoop(t, "one\n\"\"\"\ntwo\nthree\n\"\"\"\nfour\n")
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Text != "one" || results[1].Text != "two\nthree" || results[2].Text != "four" {
		t.Fatalf("unexpected messages: %#v", results)
	}
}

func runReadLoop(t *testing.T, input string) []contentResult {
	t.Helper()

	in := NewContentChannel(context.Background(), strings.NewReader(input))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go in.readLoop(ctx, strings.NewReader(input))

	var results []contentResult
	for result := range in.ContentCh() {
		if result.Error != nil {
			t.Fatalf("unexpected error: %v", result.Error)
		}
		results = append(results, result)
	}
	return results
}
