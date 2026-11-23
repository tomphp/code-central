package core_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// whole asks get_ticket for every part: the whole Ticket.
func whole(ref api.Ref) api.GetTicketInput {
	return api.GetTicketInput{Ticket: ref, Parts: api.TicketParts()}
}

func (f fixture) get(t *testing.T, ref api.Ref) api.TicketView {
	return f.shared().Get(t, ref)
}

func (f fixture) addBlocker(t *testing.T, ticket, blocker api.Ref) api.TicketView {
	t.Helper()
	v, err := record(f.core.AddBlocker(ctx, "acme", api.BlockerInput{Ticket: ticket, Blocker: blocker, Full: true}))
	if err != nil {
		t.Fatalf("add_blocker %s %s: %v", ticket, blocker, err)
	}
	return v
}

// setState writes a Ticket's State straight to the store, for States the
// test reaches only by a long walk through the workflow.
func (f fixture) setState(t *testing.T, n int, s domain.State) {
	t.Helper()
	err := f.store.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", n)
		tk.State = s
		return w.PutTicket("acme", tk)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func blockerIDs(v api.TicketView) []string {
	var out []string
	for _, b := range v.Blockers {
		out = append(out, b.ID+" "+string(b.State))
	}
	return out
}

func TestABlockedReadyTicketStaysReadyButIsNotClaimable(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready) // acme-2, acme-3
	f.addBlocker(t, "acme-2", "acme-3")

	v := f.get(t, "acme-2")
	if v.State != domain.Ready || !v.Blocked || v.Claimable || !slices.Equal(v.BlockedBy, []string{"acme-3"}) {
		t.Fatalf("got state %s blocked %v claimable %v blocked_by %v", v.State, v.Blocked, v.Claimable, v.BlockedBy)
	}
	if got := blockerIDs(v); !slices.Equal(got, []string{"acme-3 Ready"}) {
		t.Fatalf("blockers %v", got)
	}
	// The next claim passes over the older, blocked acme-2.
	if res := f.claim(t, api.ClaimInput{}); res.Ticket.ID != "acme-3" {
		t.Fatalf("claimed %s, want acme-3", res.Ticket.ID)
	}
	if res, _ := f.core.Claim(ctx, "acme", api.ClaimInput{}); res.Ticket != nil {
		t.Fatalf("claimed %s while it is blocked", res.Ticket.ID)
	}
}

func TestClaimingANamedBlockedTicketIsRejectedWithItsBlockers(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready, domain.Draft) // acme-2, acme-3, acme-4
	f.addBlocker(t, "acme-2", "acme-3")
	f.addBlocker(t, "acme-2", "acme-4")

	_, err := f.core.Claim(ctx, "acme", api.ClaimInput{Ticket: "acme-2"})
	var e *api.Error
	if !errors.As(err, &e) || e.Code != api.ActionNotAllowed {
		t.Fatalf("err = %v, want action_not_allowed", err)
	}
	if !strings.Contains(e.Message, "acme-3 (Ready)") || !strings.Contains(e.Message, "acme-4 (Draft)") {
		t.Fatalf("message %q should name each Blocker with its State", e.Message)
	}
	if slices.Contains(e.AllowedActions, "claim") || !slices.Contains(e.AllowedActions, "defer") {
		t.Fatalf("allowed %v: want defer but not claim", e.AllowedActions)
	}
}

func TestOnlyDoneClearsABlocker(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready) // acme-2, acme-3
	f.addBlocker(t, "acme-2", "acme-3")
	for _, s := range []domain.State{domain.InProgress, domain.InReview, domain.Accepted, domain.Failed, domain.Cancelled} {
		f.setState(t, 3, s)
		if v := f.get(t, "acme-2"); !v.Blocked {
			t.Fatalf("a %s Blocker cleared the block", s)
		}
	}
	f.setState(t, 3, domain.Done)
	if v := f.get(t, "acme-2"); v.Blocked || !v.Claimable || !slices.Equal(blockerIDs(v), []string{"acme-3 Done"}) {
		t.Fatalf("blocked %v claimable %v blockers %v after Done", v.Blocked, v.Claimable, blockerIDs(v))
	}
}

func TestCreateTicketTakesBlockers(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready) // acme-2
	v := f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Next", State: domain.Ready, BlockedBy: []api.Ref{"acme-2", "2"}})
	if !slices.Equal(v.BlockedBy, []string{"acme-2"}) || !v.Blocked {
		t.Fatalf("blocked_by %v blocked %v", v.BlockedBy, v.Blocked)
	}
	_, err := f.core.CreateTicket(ctx, "acme", api.CreateTicketInput{Spec: "1", Title: "X", BlockedBy: []api.Ref{"acme-9"}})
	wantCode(t, err, api.Invalid, "blocked_by")
	_, err = f.core.CreateTicket(ctx, "acme", api.CreateTicketInput{Spec: "1", Title: "X", BlockedBy: []api.Ref{"acme-1"}})
	wantCode(t, err, api.Invalid, "blocked_by") // a Spec is not a Ticket
}

func TestBlockersLinkAcrossSpecsWithinOneProject(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)                                               // Spec acme-1, acme-2
	f.spec(t, "acme", "Search")                                           // acme-3
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "3", Title: "Index"}) // acme-4
	v := f.addBlocker(t, "acme-2", "acme-4")
	if !slices.Equal(v.BlockedBy, []string{"acme-4"}) {
		t.Fatalf("blocked_by %v", v.BlockedBy)
	}
	f.project(t, "other")
	_, err := f.core.AddBlocker(ctx, "acme", api.BlockerInput{Ticket: "acme-2", Blocker: "other-4"})
	wantCode(t, err, api.Invalid, "blocker")
	_, err = f.core.AddBlocker(ctx, "acme", api.BlockerInput{Ticket: "acme-2", Blocker: "acme-9"})
	wantCode(t, err, api.Invalid, "blocker")
	_, err = f.core.AddBlocker(ctx, "acme", api.BlockerInput{Ticket: "acme-9", Blocker: "acme-2"})
	wantCode(t, err, api.NotFound, "")
}

func TestAddingABlockerTwiceWritesNothing(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready)
	first := f.addBlocker(t, "acme-2", "acme-3")
	f.clock.Advance(time.Hour)
	again := f.addBlocker(t, "acme-2", "3")
	if !again.Updated.Equal(first.Updated) || len(again.BlockedBy) != 1 {
		t.Fatalf("second add changed the Ticket: %+v", again)
	}
}

func TestABlockerThatWouldFormACycleIsRejected(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready, domain.Ready) // acme-2, acme-3, acme-4
	f.addBlocker(t, "acme-2", "acme-3")
	f.addBlocker(t, "acme-3", "acme-4")

	_, err := f.core.AddBlocker(ctx, "acme", api.BlockerInput{Ticket: "acme-4", Blocker: "acme-2"})
	wantCode(t, err, api.Cycle, "")
	if !strings.Contains(err.Error(), "acme-4 → acme-2 → acme-3 → acme-4") {
		t.Fatalf("message %q should show the cycle", err)
	}
	_, err = f.core.AddBlocker(ctx, "acme", api.BlockerInput{Ticket: "acme-2", Blocker: "acme-2"})
	wantCode(t, err, api.Cycle, "")
	if v := f.get(t, "acme-4"); len(v.BlockedBy) != 0 {
		t.Fatalf("rejected link was written: %v", v.BlockedBy)
	}
}

func TestRemoveBlocker(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready)
	f.addBlocker(t, "acme-2", "acme-3")
	v, err := record(f.core.RemoveBlocker(ctx, "acme", api.BlockerInput{Ticket: "acme-2", Blocker: "acme-3", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	if len(v.BlockedBy) != 0 || v.Blocked || !v.Claimable {
		t.Fatalf("after remove: %+v", v)
	}
	_, err = f.core.RemoveBlocker(ctx, "acme", api.BlockerInput{Ticket: "acme-2", Blocker: "acme-3"})
	wantCode(t, err, api.Invalid, "blocker")
}
