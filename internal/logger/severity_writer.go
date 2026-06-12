package logger

import (
	"bytes"
	"io"
	"sync"
)

// severityLabelWriter rewrites klog's one-letter severity prefix into a readable label.
// severityLabelWriter 将 klog 的单字母级别前缀改写为可读标签。
type severityLabelWriter struct {
	mu      sync.Mutex
	inner   io.Writer
	initial byte
	label   []byte
	pending []byte
}

func newSeverityLabelWriter(inner io.Writer, initial byte, label string) io.Writer {
	return &severityLabelWriter{
		inner:   inner,
		initial: initial,
		label:   []byte(label),
	}
}

func (w *severityLabelWriter) Write(p []byte) (int, error) {
	if w == nil || w.inner == nil {
		return len(p), nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending = append(w.pending, p...)
	for {
		lineEnd := bytes.IndexByte(w.pending, '\n')
		if lineEnd < 0 {
			break
		}
		line := append([]byte(nil), w.pending[:lineEnd+1]...)
		if err := w.writeLine(line); err != nil {
			return len(p), err
		}
		w.pending = w.pending[lineEnd+1:]
	}
	return len(p), nil
}

func (w *severityLabelWriter) writeLine(line []byte) error {
	if len(line) > 0 && line[0] == w.initial {
		line = append(append([]byte(nil), w.label...), line[1:]...)
	}
	_, err := w.inner.Write(line)
	return err
}
