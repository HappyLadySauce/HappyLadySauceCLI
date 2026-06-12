package security

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func waitForFinalize(t *testing.T, finalized *atomic.Int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for finalized.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if finalized.Load() != 1 {
		t.Fatalf("finalize calls = %d, want 1", finalized.Load())
	}
}

func TestProxyStreamReaderWithFinalizeRunsOnCloseWithoutConsumption(t *testing.T) {
	t.Parallel()

	var finalized atomic.Int32
	inner := schema.StreamReaderFromArray([]string{"ok"})
	reader := proxyStreamReaderWithFinalize(inner, func() {
		finalized.Add(1)
	})
	reader.Close()
	waitForFinalize(t, &finalized)
}

func TestProxyStreamReaderWithFinalizeRunsOnceOnEOF(t *testing.T) {
	t.Parallel()

	var finalized atomic.Int32
	inner := schema.StreamReaderFromArray([]string{"ok"})
	reader := proxyStreamReaderWithFinalize(inner, func() {
		finalized.Add(1)
	})
	defer reader.Close()

	if got, err := reader.Recv(); err != nil || got != "ok" {
		t.Fatalf("Recv() = %q, %v; want ok, nil", got, err)
	}
	if _, err := reader.Recv(); err == nil {
		t.Fatal("expected EOF")
	}
	waitForFinalize(t, &finalized)
}
