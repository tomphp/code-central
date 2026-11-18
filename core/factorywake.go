package core

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The Factory's wake (Spec code-central-743): each Factory keeps one
// next_factory_wake call open, and a write its poll acts on ends every
// open call on the write's Project, woken, so the Factory polls at once
// instead of on its next --poll tick. The call carries no work: the
// Factory's poll decides what to do. The open calls are held in memory
// only, and a wake with no call open is not remembered.

// FactoryWake is what next_factory_wake returns: Woken when a change the
// Factory's poll acts on ended the call, and empty when it timed out, a
// newer call replaced it or its request ended.
type FactoryWake struct {
	Woken bool `json:"woken,omitempty"`
}

// NextFactoryWakeInput is the Factory's next_factory_wake(factory, host),
// named as its Heartbeat names it.
type NextFactoryWakeInput struct {
	Factory string `json:"-" query:"factory"`
	Host    string `json:"-" query:"host"`
}

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
func (c *Core) longPollKey(key, factory, host string) (heartbeatKey, error) {
	name, host := strings.TrimSpace(factory), strings.TrimSpace(host)
	switch {
	case name == "":
		return heartbeatKey{}, invalid("factory", "factory is required: the Factory's name, as its Heartbeat gives it")
	case host == "":
		return heartbeatKey{}, invalid("host", "host is required: the machine the Factory runs on, as its Heartbeat gives it")
	}
	if err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
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
func (c *Core) WaitingFactoryWakes(key string) int {
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
func (c *Core) NextFactoryWake(ctx context.Context, key string, in NextFactoryWakeInput) (FactoryWake, error) {
	k, err := c.longPollKey(key, in.Factory, in.Host)
	if err != nil {
		return FactoryWake{}, err
	}
	p := newLongPoll()
	c.factoryWakes.open(k, p)
	defer c.factoryWakes.close(k, p)
	timeout := c.clock.After(WaitTimeout)
	select {
	case <-p.wake:
		return FactoryWake{Woken: true}, nil
	case <-p.replaced:
	case <-ctx.Done():
	case <-timeout:
		// A wake that came as the call timed out still counts.
		select {
		case <-p.wake:
			return FactoryWake{Woken: true}, nil
		default:
		}
	}
	return FactoryWake{}, nil
}

// wakingStates are the States a move into wakes the Factories: Ready
// (claimable again), Answered (to resume), Accepted (to land), and Done
// and Cancelled (which may clear another Ticket's last Blocker).
var wakingStates = []domain.State{domain.Ready, domain.Answered, domain.Accepted, domain.Done, domain.Cancelled}

// wakesTheFactory says whether writing Ticket t, over the record r holds
// before the write, is a change the Factory's poll acts on: t moves into
// one of the wakingStates (a new Ticket counts as moving into its
// State), a live Claim on it ends by a move its holder didn't make, or
// its Blockers shrink.
func wakesTheFactory(r store.Reader, key string, t domain.Ticket) bool {
	was, found := r.Ticket(key, t.Number)
	if !found {
		return slices.Contains(wakingStates, t.State)
	}
	movedIn := was.State != t.State && slices.Contains(wakingStates, t.State)
	return movedIn || claimWithdrawn(was, t) || blockersShrank(was, t)
}

// blockersShrank says whether t has lost a Blocker was had, which may
// leave it claimable.
func blockersShrank(was, t domain.Ticket) bool {
	return slices.ContainsFunc(was.BlockedBy, func(n int) bool { return !slices.Contains(t.BlockedBy, n) })
}

// claimWithdrawn says whether the Claim live on was has ended on t by an
// action other than its holder's own, such as cancel, release or a move
// by hand: the Factory may be running its turn, and has a slot back.
func claimWithdrawn(was, t domain.Ticket) bool {
	live, ok := liveClaim(was)
	if !ok {
		return false
	}
	for _, c := range t.Claims {
		if c.Number == live.Number {
			return !c.Live() && !isHolderAction(c.EndedBy)
		}
	}
	return false
}

// projectWakesTheFactory says whether writing Project p, over the record
// r holds before the write, changes a setting the Factory reads: the
// base, Landing, CI, Models, Stage Guides, Auto-accept or Archived. The
// Push remote the Factory reports, its Reported Models and UI-only
// settings such as Focus Kinds don't count.
func projectWakesTheFactory(r store.Reader, p domain.Project) bool {
	was, found := r.Project(p.Key)
	if !found {
		return false
	}
	ci := func(c domain.CI) domain.CI {
		c.PushRemoteURL, c.NoPushRemote = "", false
		return c
	}
	return was.Base() != p.Base() || was.Landing != p.Landing || !ci(was.CI).Equal(ci(p.CI)) ||
		!was.Models.Equal(p.Models) || was.StageGuides != p.StageGuides ||
		!slices.Equal(was.AutoAccept, p.AutoAccept) || !was.Archived.Equal(p.Archived)
}

// specWakesTheFactory says whether writing Spec s, over the record r
// holds before the write, turns a Map's guided Tag on: the Factory then
// starts its next decision.
func specWakesTheFactory(r store.Reader, key string, s domain.Spec) bool {
	guided := func(s domain.Spec) bool { return isMap(s.Labels) && s.Tags[GuidedTag] == GuidedOn }
	was, found := r.Spec(key, s.Number)
	return guided(s) && !(found && guided(was))
}
