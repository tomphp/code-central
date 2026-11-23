package factories

import (
	"context"
	"strings"
	"sync"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/store"
)

// The Factory's wake (Spec code-central-743): each Factory keeps one
// next_factory_wake call open, and a write its poll acts on ends every
// open call on the write's Project, woken, so the Factory polls at once
// instead of on its next --poll tick. The call carries no work: the
// Factory's poll decides what to do. The open calls are held in memory
// only, and a wake with no call open is not remembered.

// longPoll is one open long-poll call: next_chat_work or
// next_factory_wake. wake fires when there may be something for it;
// replaced is closed when a newer call from the same Factory takes its
// place.
type longPoll struct {
	wake     chan struct{}
	replaced chan struct{}
}

// newLongPoll is a call not yet woken or replaced.
func newLongPoll() *longPoll {
	return &longPoll{wake: make(chan struct{}, 1), replaced: make(chan struct{})}
}

// longPollKey is the key of Factory factory's long-poll call on Project
// key from host, both named as its Heartbeat names them. Either missing is
// invalid, and an unknown Project is refused; an archived one is not, as
// the call is a read.
func (c *Factories) longPollKey(key, factory, host string) (heartbeatKey, error) {
	name, host := strings.TrimSpace(factory), strings.TrimSpace(host)
	switch {
	case name == "":
		return heartbeatKey{}, api.InvalidField("factory", "factory is required: the Factory's name, as its Heartbeat gives it")
	case host == "":
		return heartbeatKey{}, api.InvalidField("host", "host is required: the machine the Factory runs on, as its Heartbeat gives it")
	}
	if err := c.f.Store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return api.UnknownProjectError(key)
		}
		return nil
	}); err != nil {
		return heartbeatKey{}, err
	}
	return heartbeatKey{key, name, host}, nil
}

// notify wakes p, at most once until it looks.
func (p *longPoll) notify() {
	select {
	case p.wake <- struct{}{}:
	default: // already woken
	}
}

// factoryWakes is each Factory's one open next_factory_wake call per
// Project.
type factoryWakes struct {
	mu    sync.Mutex
	polls map[heartbeatKey]*longPoll
	// woken is the open calls given a wake: a call takes its wake from
	// its channel before it closes, so the channel can't say so.
	woken map[*longPoll]bool
}

// newFactoryWakes is an empty holder.
func newFactoryWakes() *factoryWakes {
	return &factoryWakes{polls: map[heartbeatKey]*longPoll{}, woken: map[*longPoll]bool{}}
}

// open makes p Factory k's open call, ending the one it replaces, empty,
// and handing p any wake that one has yet to take.
func (fw *factoryWakes) open(k heartbeatKey, p *longPoll) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if old := fw.polls[k]; old != nil {
		close(old.replaced)
		select {
		case <-old.wake:
			p.notify()
			fw.woken[p] = true
		default:
		}
	}
	fw.polls[k] = p
}

// close forgets p, unless a newer call has taken its place.
func (fw *factoryWakes) close(k heartbeatKey, p *longPoll) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.polls[k] == p {
		delete(fw.polls, k)
	}
	delete(fw.woken, p)
}

// wake wakes every open call on the Project.
func (fw *factoryWakes) wake(key string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	for k, p := range fw.polls {
		if k.project == key {
			p.notify()
			fw.woken[p] = true
		}
	}
}

// WaitingFactoryWakes is how many next_factory_wake calls on Project key
// are open and not yet woken: those a write the Factory's poll acts on
// would wake now. A woken call stays uncounted while it returns. It is
// for tests, which must know a wake will reach a Factory before they make
// one.
func (c *Factories) WaitingFactoryWakes(key string) int {
	fw := c.factoryWakes
	fw.mu.Lock()
	defer fw.mu.Unlock()
	n := 0
	for k, p := range fw.polls {
		if k.project == key && !fw.woken[p] {
			n++
		}
	}
	return n
}

// NextFactoryWake is the Factory's next_factory_wake long-poll. It holds
// until a write the Factory's poll acts on reaches the Project, and then
// returns woken; or, for at most WaitTimeout on the injected clock or
// until ctx ends, and then returns empty. A newer call from the same
// Factory and host ends this one, empty.
func (c *Factories) NextFactoryWake(ctx context.Context, key string, in api.NextFactoryWakeInput) (api.FactoryWake, error) {
	k, err := c.longPollKey(key, in.Factory, in.Host)
	if err != nil {
		return api.FactoryWake{}, err
	}
	p := newLongPoll()
	c.factoryWakes.open(k, p)
	defer c.factoryWakes.close(k, p)
	timeout := c.f.Clock.After(foundation.WaitTimeout)
	select {
	case <-p.wake:
		return api.FactoryWake{Woken: true}, nil
	case <-p.replaced:
	case <-ctx.Done():
	case <-timeout:
		// A wake that came as the call timed out still counts.
		select {
		case <-p.wake:
			return api.FactoryWake{Woken: true}, nil
		default:
		}
	}
	return api.FactoryWake{}, nil
}

// Wake wakes every open next_factory_wake call on Project key, as a write
// its poll acts on does.
func (c *Factories) Wake(key string) { c.factoryWakes.wake(key) }
