package tracker

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

var fallbackIDCounter uint64

func newID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return prefix + "_" + hex.EncodeToString(b[:])
	}
	n := atomic.AddUint64(&fallbackIDCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), n)
}
