// Package events carries change notices from core to whoever listens:
// the long-poll behind `code-central wait`, and later SSE and notify.
// A notice only names what changed; listeners refetch. There is no
// replay, and a subscriber that falls behind misses notices rather than
// slowing a write down.
package events

import "sync"

// The event types.
const (
	TicketChanged  = "ticket.changed"
	SpecChanged    = "spec.changed"
	ProjectChanged = "project.changed"
	// OptionsChanged is about no Project: its Project and ID are empty.
	OptionsChanged = "options.changed"
)

// Event is one change notice. ID is the display ID of the Ticket or Spec
// and is empty for project.changed. Project is empty for options.changed,
// the one notice about no Project; a listener that reads Project treats
// an empty one as not about any Project.
type Event struct {
	Type    string `json:"type"`
	Project string `json:"project"`
	ID      string `json:"id,omitempty"`
}

// Buffer is how many notices a subscriber may fall behind before it
// misses some.
const Buffer = 64

// Bus fans notices out to subscribers. The zero value is not usable;
// call New.
type Bus struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// New returns an empty Bus.
func New() *Bus { return &Bus{subs: map[chan Event]struct{}{}} }

// Publish sends e to every subscriber without blocking.
func (b *Bus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default: // this subscriber is behind; it misses e
		}
	}
}

// Subscribe returns a channel of every notice published from now on, and
// a stop function that closes it. Stop may be called more than once.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, Buffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()
			close(ch)
		})
	}
}
