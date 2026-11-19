// The services on a Store (Spec code-central-1032): each one's Service
// Registration, live for store.ServiceLife from its last renewal, and the
// Watcher Lease of each Project, which one service holds until it lapses
// and another takes it. Every case runs on its own times, as a clock the
// test moves would give them.

package storetest

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// registration is a service's Registration at store.Format, renewed at.
func registration(id, host string, at time.Time) store.Registration {
	return store.Registration{ID: id, Host: host, Version: "v1.2.3", Format: store.Format, Renewed: at}
}

func register(t *testing.T, s store.Store, r store.Registration) {
	t.Helper()
	if err := s.Services().Register(r); err != nil {
		t.Fatalf("register %s: %v", r.ID, err)
	}
}

// live is the IDs and hosts of the Registrations live at now.
func live(t *testing.T, s store.Store, now time.Time) string {
	t.Helper()
	regs, err := s.Services().Registrations(now)
	if err != nil {
		t.Fatalf("registrations: %v", err)
	}
	out := ""
	for _, r := range regs {
		out += fmt.Sprintf("%s@%s ", r.ID, r.Host)
	}
	return out
}

// holdLease is id holding project's Lease at now, as it then is.
func holdLease(t *testing.T, s store.Store, project, id string, now time.Time) store.Lease {
	t.Helper()
	l, err := s.Services().HoldLease(project, id, now)
	if err != nil {
		t.Fatalf("%s holding %s's lease: %v", id, project, err)
	}
	return l
}

// wantLease fails t unless l is held by holder, on host, until expires.
func wantLease(t *testing.T, l store.Lease, project, holder, host string, expires time.Time) {
	t.Helper()
	if l.Project != project || l.Holder != holder || l.Host != host || !l.Expires.Equal(expires) {
		t.Fatalf("lease = %+v, want %s's held by %s on %q until %v", l, project, holder, host, expires)
	}
}

func runServices(t *testing.T, f Factory) {
	life := store.ServiceLife

	t.Run("a registration is live until its life has passed since it was last renewed", func(t *testing.T) {
		s := f.New(t)
		register(t, s, registration("a", "laptop", t0))
		if got := live(t, s, t0); got != "a@laptop " {
			t.Fatalf("live at once: %q", got)
		}
		if got := live(t, s, t0.Add(life-time.Nanosecond)); got != "a@laptop " {
			t.Fatalf("live just before its life: %q", got)
		}
		if got := live(t, s, t0.Add(life)); got != "" {
			t.Fatalf("live once its life has passed: %q", got)
		}
		register(t, s, registration("a", "laptop", t0.Add(store.ServiceRenewal)))
		if got := live(t, s, t0.Add(life)); got != "a@laptop " {
			t.Fatalf("live after renewing: %q", got)
		}
		regs, _ := s.Services().Registrations(t0.Add(life))
		want := registration("a", "laptop", t0.Add(store.ServiceRenewal))
		if got := regs[0]; got.Version != want.Version || got.Format != want.Format || !got.Renewed.Equal(want.Renewed) {
			t.Fatalf("registration = %+v, want %+v", got, want)
		}
	})

	t.Run("registrations are listed by host, then ID, and a deregistered one is gone", func(t *testing.T) {
		s := f.New(t)
		register(t, s, registration("c", "laptop", t0))
		register(t, s, registration("b", "desktop", t0))
		register(t, s, registration("a", "laptop", t0))
		if got := live(t, s, t0); got != "b@desktop a@laptop c@laptop " {
			t.Fatalf("live: %q", got)
		}
		if err := s.Services().Deregister("b"); err != nil {
			t.Fatal(err)
		}
		if got := live(t, s, t0); got != "a@laptop c@laptop " {
			t.Fatalf("live after deregistering b: %q", got)
		}
	})

	t.Run("a registration at another format is refused, naming both", func(t *testing.T) {
		s := f.New(t)
		r := registration("a", "laptop", t0)
		r.Format = store.Format - 1
		err := s.Services().Register(r)
		var changed *store.FormatChangedError
		if !errors.As(err, &changed) || changed.Stored != store.Format || changed.Own != store.Format-1 {
			t.Fatalf("registering at format %d: %v", r.Format, err)
		}
	})

	t.Run("a watcher lease is held until it lapses, then another service takes it", func(t *testing.T) {
		s := f.New(t)
		register(t, s, registration("a", "laptop", t0))
		register(t, s, registration("b", "desktop", t0))
		wantLease(t, holdLease(t, s, "acme", "a", t0), "acme", "a", "laptop", t0.Add(life))
		wantLease(t, holdLease(t, s, "acme", "b", t0.Add(store.ServiceRenewal)), "acme", "a", "laptop", t0.Add(life))
		// a renews it, so it lasts its life from then.
		renewed := t0.Add(2 * store.ServiceRenewal)
		wantLease(t, holdLease(t, s, "acme", "a", renewed), "acme", "a", "laptop", renewed.Add(life))
		wantLease(t, holdLease(t, s, "acme", "b", renewed.Add(life-time.Nanosecond)), "acme", "a", "laptop", renewed.Add(life))
		// a stopped renewing: once it lapses, b takes it, and a can't
		// take it back while b holds it.
		taken := renewed.Add(life)
		wantLease(t, holdLease(t, s, "acme", "b", taken), "acme", "b", "desktop", taken.Add(life))
		wantLease(t, holdLease(t, s, "acme", "a", taken.Add(time.Second)), "acme", "b", "desktop", taken.Add(life))
	})

	t.Run("each project has a lease of its own", func(t *testing.T) {
		s := f.New(t)
		holdLease(t, s, "acme", "a", t0)
		wantLease(t, holdLease(t, s, "other", "b", t0), "other", "b", "", t0.Add(life))
	})

	t.Run("a deregistered service lets go of its leases at once", func(t *testing.T) {
		s := f.New(t)
		register(t, s, registration("a", "laptop", t0))
		holdLease(t, s, "acme", "a", t0)
		holdLease(t, s, "other", "a", t0)
		holdLease(t, s, "third", "c", t0)
		if err := s.Services().Deregister("a"); err != nil {
			t.Fatal(err)
		}
		at := t0.Add(time.Second)
		wantLease(t, holdLease(t, s, "acme", "b", at), "acme", "b", "", at.Add(life))
		wantLease(t, holdLease(t, s, "other", "b", at), "other", "b", "", at.Add(life))
		wantLease(t, holdLease(t, s, "third", "b", at), "third", "c", "", t0.Add(life))
	})

	if f.Second == nil && f.Reopen != nil {
		// No other service can be on a Store one holds alone, so what the
		// last one left, stopped without deregistering, is no one's.
		t.Run("reopened, a store one service holds alone forgets the services before", func(t *testing.T) {
			s := f.New(t)
			register(t, s, registration("a", "laptop", t0))
			holdLease(t, s, "acme", "a", t0)
			s = f.Reopen(t, s)
			if got := live(t, s, t0); got != "" {
				t.Fatalf("live after reopening: %q", got)
			}
			wantLease(t, holdLease(t, s, "acme", "b", t0), "acme", "b", "", t0.Add(life))
		})
	}

	if f.Second != nil {
		t.Run("a second service sees the first's registration and lease", func(t *testing.T) {
			a := f.New(t)
			b := f.Second(t, a)
			register(t, a, registration("a", "laptop", t0))
			holdLease(t, a, "acme", "a", t0)
			if got := live(t, b, t0); got != "a@laptop " {
				t.Fatalf("live, as the second service reads it: %q", got)
			}
			wantLease(t, holdLease(t, b, "acme", "b", t0.Add(time.Second)), "acme", "a", "laptop", t0.Add(life))
		})
	}
}
