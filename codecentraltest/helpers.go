// Small helpers tests share alongside a test service.

package codecentraltest

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"
)

// SyncBuffer is a log written by the service's goroutines, or a child
// process, while the test reads it.
type SyncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

// Write appends p.
func (s *SyncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

// String is everything written so far.
func (s *SyncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// EventuallyWithin is how long Eventually waits.
const EventuallyWithin = 15 * time.Second

// Eventually waits, in real time, for ok to hold, for what another
// process or goroutine does, and fails t with the message format and
// args make if it doesn't within EventuallyWithin.
func Eventually(t testing.TB, ok func() bool, format string, args ...any) {
	t.Helper()
	deadline := time.Now().Add(EventuallyWithin)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal(fmt.Sprintf(format, args...))
		}
		time.Sleep(10 * time.Millisecond)
	}
}
