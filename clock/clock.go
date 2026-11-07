// Package clock is the time source injected wherever time matters, so no
// test ever sleeps on real time.
package clock

import (
	"sync"
	"time"
)

// Clock tells the time and makes timers.
type Clock interface {
	Now() time.Time
	// After sends the time on the returned channel once d has passed.
	After(d time.Duration) <-chan time.Time
}

// System is the real clock.
type System struct{}

// Now returns the current time.
func (System) Now() time.Time { return time.Now() }

// After is time.After.
func (System) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Fake is a clock that moves only when told to. Timers made with After
// fire when Set or Advance reaches their deadline. It is safe for
// concurrent use.
type Fake struct {
	mu      sync.Mutex
	cond    *sync.Cond
	now     time.Time
	waiters []waiter
}

type waiter struct {
	at time.Time
	ch chan time.Time
}

// NewFake returns a Fake clock reading t.
func NewFake(t time.Time) *Fake {
	f := &Fake{now: t}
	f.cond = sync.NewCond(&f.mu)
	return f
}

// Now returns the fake time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// After returns a channel that receives the fake time once the clock has
// been moved d past now. A d of zero or less fires at once.
func (f *Fake) After(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- f.now
		return ch
	}
	f.waiters = append(f.waiters, waiter{at: f.now.Add(d), ch: ch})
	f.cond.Broadcast()
	return ch
}

// Waiters is the number of timers that have not fired yet.
func (f *Fake) Waiters() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.waiters)
}

// BlockUntil blocks until at least n timers are waiting, so a test can
// move the clock only after the code under test has started its timer.
func (f *Fake) BlockUntil(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for len(f.waiters) < n {
		f.cond.Wait()
	}
}

// Set moves the clock to t.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
	f.fire()
}

// Advance moves the clock forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	f.fire()
}

func (f *Fake) fire() {
	kept := f.waiters[:0]
	for _, w := range f.waiters {
		if w.at.After(f.now) {
			kept = append(kept, w)
			continue
		}
		w.ch <- f.now
	}
	f.waiters = kept
	f.cond.Broadcast()
}
