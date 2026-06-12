package budget

import (
	"testing"
	"time"

	contextstatus "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/status"
)

func TestFormatElapsed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		elapsedMs int64
		want      string
	}{
		{0, "0.00s"},
		{766, "0.77s"},
		{2910, "2.91s"},
		{2559, "2.56s"},
		{59999, "60.00s"},
		{60000, "1m0s"},
		{65000, "1m5s"},
		{125000, "2m5s"},
	}
	for _, tc := range tests {
		if got := FormatElapsed(tc.elapsedMs); got != tc.want {
			t.Fatalf("FormatElapsed(%d) = %q, want %q", tc.elapsedMs, got, tc.want)
		}
	}
}

func TestFormatConversationStatusLine(t *testing.T) {
	t.Parallel()

	line := FormatConversationStatusLine(testConversation(766*time.Millisecond, 318, 37, 634, 318), 128000)
	want := "[Stats: elapsed=0.77s prompt↑=318 completion↓=37 total↑↓=634 content=318 0.25%(128K)]"
	if line != want {
		t.Fatalf("FormatConversationStatusLine() = %q, want %q", line, want)
	}
}

func TestFormatConversationStatusLineEmpty(t *testing.T) {
	t.Parallel()

	if got := FormatConversationStatusLine(contextstatus.Status{}, 128000); got != "" {
		t.Fatalf("FormatConversationStatusLine(empty) = %q, want empty", got)
	}
}

func TestFormatConversationStatusLineWithoutContext(t *testing.T) {
	t.Parallel()

	line := FormatConversationStatusLine(testConversation(100*time.Millisecond, 50, 5, 55, 0), 0)
	want := "[Stats: elapsed=0.10s prompt↑=50 completion↓=5 total↑↓=55 content=0]"
	if line != want {
		t.Fatalf("FormatConversationStatusLine() = %q, want %q", line, want)
	}
}

func TestFormatConversationStatusLineOverOneMinute(t *testing.T) {
	t.Parallel()

	line := FormatConversationStatusLine(testConversation(65*time.Second, 1000, 200, 6200, 5000), 128000)
	want := "[Stats: elapsed=1m5s prompt↑=1000 completion↓=200 total↑↓=6200 content=5000 3.91%(128K)]"
	if line != want {
		t.Fatalf("FormatConversationStatusLine() = %q, want %q", line, want)
	}
}

func TestFormatPercentTwoDecimals(t *testing.T) {
	t.Parallel()

	if got, want := FormatPercent(1.0125), "1.01%"; got != want {
		t.Fatalf("FormatPercent(1.0125) = %q, want %q", got, want)
	}
	if got, want := FormatPercent(0.2484375), "0.25%"; got != want {
		t.Fatalf("FormatPercent(0.2484375) = %q, want %q", got, want)
	}
}

func TestFormatConversationStatusLinePercentRounding(t *testing.T) {
	t.Parallel()

	tiny := FormatConversationStatusLine(testConversation(0, 0, 0, 99, 4), 1000)
	if tiny != "[Stats: elapsed=0.00s prompt↑=0 completion↓=0 total↑↓=99 content=4 0.40%(1K)]" {
		t.Fatalf("tiny percent line = %q", tiny)
	}

	rounded := FormatConversationStatusLine(testConversation(0, 0, 0, 999, 415), 1000)
	if rounded != "[Stats: elapsed=0.00s prompt↑=0 completion↓=0 total↑↓=999 content=415 41.50%(1K)]" {
		t.Fatalf("rounded percent line = %q", rounded)
	}
}

func testConversation(elapsed time.Duration, prompt, completion, total, contextTokens int) contextstatus.Status {
	return contextstatus.Status{
		Elapsed:       elapsed,
		Prompt:        prompt,
		Completion:    completion,
		Total:         total,
		ContextTokens: contextTokens,
	}
}
