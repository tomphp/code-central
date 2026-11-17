package server_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// The Factory's wake (Spec code-central-743): next_factory_wake, over
// HTTP only. The human approves Draft acme-2 into Ready while the call
// holds, which wakes it.
func init() {
	cases["next_factory_wake"] = opCase{
		setup: approveOnceTheWakeHolds,
		ok:    call{http: httpCall{"GET", "/api/p/acme/factory/next-wake?factory=f&host=h", ""}},
		check: func(t *testing.T, out map[string]any) {
			t.Helper()
			if out["woken"] != true {
				t.Fatalf("woken = %v, want true", out["woken"])
			}
		},
		bad:  call{http: httpCall{"GET", "/api/p/acme/factory/next-wake?host=h", ""}},
		code: core.Invalid,
	}
}

// approveOnceTheWakeHolds approves acme-2 as soon as a call starts its
// timeout on the server's clock.
func approveOnceTheWakeHolds(t *testing.T, srv *codecentraltest.Server) {
	n := srv.Clock.Waiters()
	go func() {
		srv.Clock.BlockUntil(n + 1)
		if _, err := srv.Core.Approve(core.WithActor(context.Background(), core.Human), "acme", core.TicketInput{Ticket: "2"}); err != nil {
			t.Errorf("approve: %v", err)
		}
	}()
}

func TestTheClientsNextFactoryWakeIsWokenAndEndsEmptyAtTheTimeout(t *testing.T) {
	srv := newServer(t, approveOnceTheWakeHolds)
	in := core.NextFactoryWakeInput{Factory: "f", Host: "h"}
	if w, err := srv.Client.NextFactoryWake(context.Background(), "acme", in); err != nil || !w.Woken {
		t.Fatalf("next_factory_wake = %+v, %v; want woken", w, err)
	}
	done := make(chan core.FactoryWake, 1)
	n := srv.Clock.Waiters()
	go func() {
		w, err := srv.Client.NextFactoryWake(context.Background(), "acme", in)
		if err != nil {
			t.Errorf("next_factory_wake: %v", err)
		}
		done <- w
	}()
	srv.Clock.BlockUntil(n + 1)
	srv.Clock.Advance(core.WaitTimeout)
	if w := <-done; w.Woken {
		t.Fatalf("next_factory_wake = %+v, want empty", w)
	}
}
