package sandbox

import (
	"bytes"
	"sync"
)

// limitedBuffer collects at most limit bytes and drains everything else,
// so a submission that floods stdout cannot block on a full pipe, grow
// controller memory without bound, or prevent the container from exiting.
// Writes never fail; overflow is reported separately.
type limitedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

// Write accepts the whole slice so os/exec keeps reading, but only stores
// the first limit bytes.
func (w *limitedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		w.truncated = w.truncated || len(p) > 0
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = w.buf.Write(p[:remaining])
		w.truncated = true
		return len(p), nil
	}
	_, _ = w.buf.Write(p)
	return len(p), nil
}

// String returns the collected prefix.
func (w *limitedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// Truncated reports whether more than the cap was observed.
func (w *limitedBuffer) Truncated() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.truncated
}

// Len returns the number of collected bytes.
func (w *limitedBuffer) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Len()
}
