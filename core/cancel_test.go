package core_test

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

func (f fixture) cancel(t *testing.T, c core.CancelInput, as ...core.Actor) core.TicketView {
	t.Helper()
	cx := ctx
	if len(as) > 0 {
		cx = core.WithActor(ctx, as[0])
	}
	v, err := f.core.Cancel(cx, "acme", c)
	if err != nil {
		t.Fatalf("cancel %s: %v", c.Ticket, err)
	}
	return v
}

func TestAnAgentCancelsADraftOrReadyTicketByNamingItsReplacements(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Draft, store.Draft) // acme-2, acme-3, acme-4
	v := f.cancel(t, core.CancelInput{Ticket: "acme-2", Replacements: []core.Ref{"acme-3", "4"}, Reason: "split in two"})
	if v.State != store.Cancelled || !slices.Equal(v.Replacements, []string{"acme-3", "acme-4"}) {
		t.Fatalf("state %s replacements %v", v.State, v.Replacements)
	}
	if len(v.Comments) != 1 || v.Comments[0].Body != "split in two" || v.Comments[0].Actor != "agent" {
		t.Fatalf("comments %+v", v.Comments)
	}
	if m := v.Moves[len(v.Moves)-1]; m.From != store.Ready || m.To != store.Cancelled || m.Actor != "agent" {
		t.Fatalf("move %+v", m)
	}
	// The reason is optional.
	if v := f.cancel(t, core.CancelInput{Ticket: "acme-3", Replacements: []core.Ref{"acme-4"}}); len(v.Comments) != 0 {
		t.Fatalf("comments %+v", v.Comments)
	}
}

func TestAnAgentCannotCancelWithoutAReplacementOrOnceWorkHasStarted(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready) // acme-2, acme-3
	_, err := f.core.Cancel(ctx, "acme", core.CancelInput{Ticket: "acme-2"})
	wantCode(t, err, core.Invalid, "replacements")

	f.claim(t, core.ClaimInput{Ticket: "acme-2"})
	_, err = f.core.Cancel(ctx, "acme", core.CancelInput{Ticket: "acme-2", Replacements: []core.Ref{"acme-3"}})
	wantCode(t, err, core.ActionNotAllowed, "")
}

func TestTheHumanCancelsAnyUnfinishedTicketWithOrWithoutReplacements(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready) // acme-2, acme-3
	f.claim(t, core.ClaimInput{Ticket: "acme-2"})
	v := f.cancel(t, core.CancelInput{Ticket: "acme-2"}, core.Human)
	if v.State != store.Cancelled || v.Claims[0].Ended == nil || v.Claims[0].EndedBy != "cancel" || v.Moves[len(v.Moves)-1].Actor != "human" {
		t.Fatalf("got %+v", v)
	}
	_, err := f.core.Cancel(human, "acme", core.CancelInput{Ticket: "acme-2"})
	wantCode(t, err, core.ActionNotAllowed, "") // already finished
}

func TestCancelRejectsBadReplacements(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready) // acme-2, acme-3
	for _, c := range []struct {
		name string
		reps []core.Ref
		code core.Code
	}{
		{"unknown", []core.Ref{"acme-9"}, core.Invalid},
		{"a Spec", []core.Ref{"acme-1"}, core.Invalid},
		{"another Project", []core.Ref{"other-3"}, core.Invalid},
		{"itself", []core.Ref{"acme-2"}, core.Invalid},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.core.Cancel(ctx, "acme", core.CancelInput{Ticket: "acme-2", Replacements: c.reps})
			wantCode(t, err, c.code, "replacements")
		})
	}
}

func TestAnythingBlockedByACancelledTicketIsBlockedByItsReplacements(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready, store.Draft, store.Draft) // acme-2..acme-5
	f.addBlocker(t, "acme-2", "acme-3")
	before := f.get(t, "acme-2")

	f.cancel(t, core.CancelInput{Ticket: "acme-3", Replacements: []core.Ref{"acme-4", "acme-5"}})
	v := f.get(t, "acme-2")
	if !v.Blocked || !slices.Equal(v.BlockedBy, []string{"acme-3"}) || !v.Updated.Equal(before.Updated) {
		t.Fatalf("blocked %v blocked_by %v updated %v (dependant must not be rewritten)", v.Blocked, v.BlockedBy, v.Updated)
	}
	want := []core.BlockerView{
		{ID: "acme-4", Title: "Tc", State: store.Draft, Via: "acme-3"},
		{ID: "acme-5", Title: "Td", State: store.Draft, Via: "acme-3"},
	}
	if !slices.Equal(v.Blockers, want) {
		t.Fatalf("blockers %+v, want %+v", v.Blockers, want)
	}
	f.setState(t, 4, store.Done)
	f.setState(t, 5, store.Done)
	if v := f.get(t, "acme-2"); v.Blocked || !v.Claimable {
		t.Fatalf("still blocked once every Replacement is Done: %+v", v.Blockers)
	}
}

func TestACancelledBlockerWithNoReplacementsKeepsItsDependantsBlocked(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready) // acme-2, acme-3
	f.addBlocker(t, "acme-2", "acme-3")
	f.cancel(t, core.CancelInput{Ticket: "acme-3"}, core.Human)
	v := f.get(t, "acme-2")
	if !v.Blocked || v.Claimable || !slices.Equal(blockerIDs(v), []string{"acme-3 Cancelled"}) {
		t.Fatalf("blocked %v claimable %v blockers %v", v.Blocked, v.Claimable, blockerIDs(v))
	}
	// Removing the stranded Blocker frees it.
	if v, err := f.core.RemoveBlocker(human, "acme", core.BlockerInput{Ticket: "acme-2", Blocker: "acme-3"}); err != nil || v.Blocked {
		t.Fatalf("remove: %v, blocked %v", err, v.Blocked)
	}
}

func TestReplacementsThatWouldFormACycleAreRejected(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready, store.Ready) // acme-2, acme-3, acme-4
	// acme-2 waits on acme-3; replacing acme-3 with acme-2 would make
	// acme-2 wait on itself.
	f.addBlocker(t, "acme-2", "acme-3")
	_, err := f.core.Cancel(ctx, "acme", core.CancelInput{Ticket: "acme-3", Replacements: []core.Ref{"acme-2"}})
	wantCode(t, err, core.Cycle, "")

	// acme-4 waits on acme-2; replacing acme-3 with acme-4 would close
	// acme-2 → acme-4 → acme-2.
	f.addBlocker(t, "acme-4", "acme-2")
	_, err = f.core.Cancel(ctx, "acme", core.CancelInput{Ticket: "acme-3", Replacements: []core.Ref{"acme-4"}})
	wantCode(t, err, core.Cycle, "")
	if v := f.get(t, "acme-3"); v.State != store.Ready {
		t.Fatalf("rejected cancel was written: %s", v.State)
	}
}

func TestAReplacementChainCannotLoopBack(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready, store.Ready) // acme-2, acme-3, acme-4
	f.cancel(t, core.CancelInput{Ticket: "acme-3", Replacements: []core.Ref{"acme-2"}})
	// acme-3 is replaced by acme-2, so replacing acme-2 by acme-3 loops.
	_, err := f.core.Cancel(ctx, "acme", core.CancelInput{Ticket: "acme-2", Replacements: []core.Ref{"acme-3"}})
	wantCode(t, err, core.Cycle, "")
}
