package logger

import (
	"bytes"
	"testing"
)

func TestSeverityLabelWriterRewritesCompleteLines(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	writer := newSeverityLabelWriter(&buf, 'I', "[Info] ")
	if _, err := writer.Write([]byte("I0613 04:00:00.000000 file.go:1] message\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := buf.String()
	want := "[Info] 0613 04:00:00.000000 file.go:1] message\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestSeverityLabelWriterBuffersPartialLines(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	writer := newSeverityLabelWriter(&buf, 'E', "[Error] ")
	if _, err := writer.Write([]byte("E0613 04:")); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("partial line should be buffered, got %q", buf.String())
	}
	if _, err := writer.Write([]byte("00:00.000000 file.go:1] boom\n")); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}

	got := buf.String()
	want := "[Error] 0613 04:00:00.000000 file.go:1] boom\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
