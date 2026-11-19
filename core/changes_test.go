package core_test

import (
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// Change notices cross services (Spec code-central-1032): a Change
// another service commits to a shared Store gives the same notice and
// Factory wake as a write here.

// sharedStore is a Store shared with another service, over the in-memory
// fake: the test writes as that service straight to the fake, then
// delivers its Change.
type sharedStore struct {
	store.Store
	deliver func(store.Change)
}

func (s *sharedStore) Changes(deliver func(store.Change)) { s.deliver = deliver }

// sharedFixture is a fixture over a sharedStore; its store field is the
// fake, which the other service writes to.
func sharedFixture(t *testing.T) (fixture, *sharedStore) {
	t.Helper()
	mem := store.NewMemory()
	s := &sharedStore{Store: mem}
	clk := clock.NewFake(t0)
	f := fixture{core: core.New(s, clk, &ids.Sequence{}), clock: clk, store: mem}
	if s.deliver == nil {
		t.Fatal("core didn't ask the Store for the Changes other services commit")
	}
	return f, s
}

func TestAForeignChangeEndsAWaitOnTheTicketItChanged(t *testing.T) {
	f, s := sharedFixture(t)
	f.work(t, domain.Ready)
	n := f.clock.Waiters()
	done := make(chan api.WaitResult, 1)
	go func() {
		res, err := f.core.Wait(ctx, "acme", "2", domain.Ready)
		if err != nil {
			panic(err)
		}
		done <- res
	}()
	f.clock.BlockUntil(n + 1)
	// The other service claims acme-2.
	f.moveTo(t, 2, domain.InProgress)
	s.deliver(store.Change{Project: "acme", Type: events.TicketChanged, ID: "acme-2", Wakes: false})
	if res := <-done; res.State != domain.InProgress {
		t.Fatalf("wait returned %s, want In Progress", res.State)
	}
}

// A Store that may have missed Changes, as a Postgres Store whose LISTEN
// connection dropped, delivers a project-wide Change: a wait refetches.
func TestAForeignProjectWideChangeEndsAWaitOnATicketThatMoved(t *testing.T) {
	f, s := sharedFixture(t)
	f.work(t, domain.Ready)
	n := f.clock.Waiters()
	done := make(chan api.WaitResult, 1)
	go func() {
		res, err := f.core.Wait(ctx, "acme", "2", domain.Ready)
		if err != nil {
			panic(err)
		}
		done <- res
	}()
	f.clock.BlockUntil(n + 1)
	f.moveTo(t, 2, domain.InProgress)
	s.deliver(store.Change{Project: "acme", Type: events.ProjectChanged, Wakes: true})
	if res := <-done; res.State != domain.InProgress {
		t.Fatalf("wait returned %s, want In Progress", res.State)
	}
}

func TestAForeignChangeSendsItsNotice(t *testing.T) {
	f, s := sharedFixture(t)
	f.work(t, domain.Ready)
	notices, stop := f.core.Events().Subscribe()
	defer stop()
	s.deliver(store.Change{Project: "acme", Type: events.SpecChanged, ID: "acme-1"})
	if got, want := <-notices, (events.Event{Type: events.SpecChanged, Project: "acme", ID: "acme-1"}); got != want {
		t.Fatalf("notice %+v, want %+v", got, want)
	}
}

func TestAForeignChangeThatWakesTheFactoriesWakesThem(t *testing.T) {
	f, s := sharedFixture(t)
	f.work(t, domain.Ready)
	call := f.wakeAsync("acme", "main")
	s.deliver(store.Change{Project: "acme", Type: events.TicketChanged, ID: "acme-2", Wakes: true})
	if w := <-call; !w.Woken {
		t.Fatal("next_factory_wake wasn't woken by a foreign Change that wakes the Factories")
	}
}

func TestAForeignChangeThatDoesNotWakeTheFactoriesLeavesThemAsleep(t *testing.T) {
	f, s := sharedFixture(t)
	f.work(t, domain.Ready)
	call := f.wakeAsync("acme", "main")
	s.deliver(store.Change{Project: "acme", Type: events.TicketChanged, ID: "acme-2"})
	if w := f.ended(call); w.Woken {
		t.Fatal("next_factory_wake was woken by a foreign Change that doesn't wake the Factories")
	}
}

// A write here announces its Change to the other services, waking the
// Factories as it wakes them here.
func TestAWriteAnnouncesItsChange(t *testing.T) {
	f, rs := retryFixture()
	f.work(t, domain.Draft)
	rs.announced = nil
	f.approve(t, "2")
	want := []store.Change{{Project: "acme", Type: events.TicketChanged, ID: "acme-2", Wakes: true}}
	if len(rs.announced) != 1 || rs.announced[0] != want[0] {
		t.Fatalf("announced %+v, want %+v", rs.announced, want)
	}
}
