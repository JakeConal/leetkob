package sandbox

import (
	"io"
	"sync"
)

// capped retains at most limit bytes across concurrent readers. One extra
// byte trips the cap so a disabled limiter (limit larger than the write, or
// a reader that ignores the cap) is observable to tests.
type capped struct {
	mu    sync.Mutex
	limit int
	used  int
	over  bool
	hit   chan struct{}
}

func (c *capped) add(p []byte) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.over || c.used >= c.limit {
		c.trip()
		return nil
	}
	room := c.limit - c.used
	if len(p) <= room {
		c.used += len(p)
		return append([]byte(nil), p...)
	}
	c.used = c.limit
	c.trip()
	return append([]byte(nil), p[:room]...)
}

func (c *capped) trip() {
	if c.over {
		return
	}
	c.over = true
	if c.hit == nil {
		return
	}
	select {
	case c.hit <- struct{}{}:
	default:
	}
}

func (c *capped) tripped() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.over
}

func readInto(r io.Reader, cap *capped, dst *[]byte) {
	buf := make([]byte, 2048)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			kept := cap.add(buf[:n])
			if len(kept) > 0 {
				*dst = append(*dst, kept...)
			}
			if cap.tripped() {
				_, _ = io.Copy(io.Discard, r)
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// collectLimited reads both streams until EOF. The retained bytes are at
// most limit, and truncated is true when either stream offered more.
func collectLimited(stdout, stderr io.Reader, limit int) (out, errOut []byte, truncated bool) {
	if limit < 0 {
		limit = 0
	}
	shared := &capped{limit: limit}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		readInto(stdout, shared, &out)
	}()
	go func() {
		defer wg.Done()
		readInto(stderr, shared, &errOut)
	}()
	wg.Wait()
	return out, errOut, shared.tripped()
}
