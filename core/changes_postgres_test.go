//go:build !js

package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/store/storetest"
)

// Two services on one Postgres Store (Spec code-central-1032, User
// Stories 9 and 10): a write through one ends a wait and wakes the
// Factories on the other. Each is a fixture over its own Store handle.
func twoServices(t *testing.T) (here, there fixture) {
	t.Helper()
	url := storetest.FreshPostgres(t)
	service := func() fixture {
		s, err := store.OpenPostgres(url)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		t.Cleanup(func() { s.Close() })
		clk := clock.NewFake(t0)
		return fixture{core: core.New(s, clk, &ids.Sequence{}), clock: clk, store: s}
	}
	return service(), service()
}

func TestAClaimOnOneServiceEndsAWaitOnAnother(t *testing.T) {
	here, there := twoServices(t)
	here.work(t, domain.Ready)
	n := there.clock.Waiters()
	done := make(chan core.WaitResult, 1)
	go func() {
		res, err := there.core.Wait(ctx, "acme", "2", domain.Ready)
		if err != nil {
			panic(err)
		}
		done <- res
	}()
	there.clock.BlockUntil(n + 1)
	if _, err := here.core.Claim(ctx, "acme", core.ClaimInput{Ticket: "acme-2"}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if res := within(t, done); res.State != domain.InProgress {
		t.Fatalf("wait returned %s, want In Progress", res.State)
	}
}

func TestNewWorkOnOneServiceWakesTheFactoriesOnAnother(t *testing.T) {
	here, there := twoServices(t)
	here.work(t)
	call := there.wakeAsync("acme", "main")
	here.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "New work", State: domain.Ready})
	if w := within(t, call); !w.Woken {
		t.Fatal("next_factory_wake wasn't woken by new work on another service")
	}
}

// within is what ch gives within a few seconds, failing t instead of
// holding it when nothing comes: the other service's notice never did.
func within[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(15 * time.Second):
		t.Fatal("nothing heard from the other service")
		var zero T
		return zero
	}
}
