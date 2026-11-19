package core_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

func (f fixture) cancel(t *testing.T, c api.CancelInput, as ...api.Actor) api.TicketView {
	t.Helper()
	cx := ctx
	if len(as) > 0 {
		cx = api.WithActor(ctx, as[0])
	}
	c.Full = true
	v, err := record(f.core.Cancel(cx, "acme", c))
	if err != nil {
		t.Fatalf("cancel %s: %v", c.Ticket, err)
	}
	return v
}

func TestAnAgentCancelsADraftOrReadyTicketByNamingItsReplacements(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Draft, domain.Draft) // acme-2, acme-3, acme-4
	v := f.cancel(t, api.CancelInput{Ticket: "acme-2", Replacements: []api.Ref{"acme-3", "4"}, Reason: "split in two"})
	if v.State != domain.Cancelled || !slices.Equal(v.Replacements, []string{"acme-3", "acme-4"}) {
		t.Fatalf("state %s replacements %v", v.State, v.Replacements)
	}
	if len(v.Comments) != 1 || v.Comments[0].Body != "split in two" || v.Comments[0].Actor != "agent" {
		t.Fatalf("comments %+v", v.Comments)
	}
	if m := v.Moves[len(v.Moves)-1]; m.From != domain.Ready || m.To != domain.Cancelled || m.Actor != "agent" {
		t.Fatalf("move %+v", m)
	}
	// The reason is optional.
	if v := f.cancel(t, api.CancelInput{Ticket: "acme-3", Replacements: []api.Ref{"acme-4"}}); len(v.Comments) != 0 {
		t.Fatalf("comments %+v", v.Comments)
	}
}

func TestAnAgentCannotCancelWithoutAReplacementOrOnceWorkHasStarted(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready) // acme-2, acme-3
	_, err := f.core.Cancel(ctx, "acme", api.CancelInput{Ticket: "acme-2"})
	wantCode(t, err, api.Invalid, "replacements")

	f.claim(t, api.ClaimInput{Ticket: "acme-2"})
	_, err = f.core.Cancel(ctx, "acme", api.CancelInput{Ticket: "acme-2", Replacements: []api.Ref{"acme-3"}})
	wantCode(t, err, api.ActionNotAllowed, "")
}

func TestTheHumanCancelsAnyUnfinishedTicketWithOrWithoutReplacements(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready) // acme-2, acme-3
	f.claim(t, api.ClaimInput{Ticket: "acme-2"})
	v := f.cancel(t, api.CancelInput{Ticket: "acme-2"}, api.Human)
	if v.State != domain.Cancelled || v.Claims[0].Ended == nil || v.Claims[0].EndedBy != "cancel" || v.Moves[len(v.Moves)-1].Actor != "human" {
		t.Fatalf("got %+v", v)
	}
	_, err := f.core.Cancel(human, "acme", api.CancelInput{Ticket: "acme-2"})
	wantCode(t, err, api.ActionNotAllowed, "") // already finished
}

func TestCancelRejectsBadReplacements(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready) // acme-2, acme-3
	for _, c := range []struct {
		name string
		reps []api.Ref
		code api.Code
	}{
		{"unknown", []api.Ref{"acme-9"}, api.Invalid},
		{"a Spec", []api.Ref{"acme-1"}, api.Invalid},
		{"another Project", []api.Ref{"other-3"}, api.Invalid},
		{"itself", []api.Ref{"acme-2"}, api.Invalid},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.core.Cancel(ctx, "acme", api.CancelInput{Ticket: "acme-2", Replacements: c.reps})
			wantCode(t, err, c.code, "replacements")
		})
	}
}

func TestAnythingBlockedByACancelledTicketIsBlockedByItsReplacements(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready, domain.Draft, domain.Draft) // acme-2..acme-5
	f.addBlocker(t, "acme-2", "acme-3")
	before := f.get(t, "acme-2")

	f.cancel(t, api.CancelInput{Ticket: "acme-3", Replacements: []api.Ref{"acme-4", "acme-5"}})
	v := f.get(t, "acme-2")
	if !v.Blocked || !slices.Equal(v.BlockedBy, []string{"acme-3"}) || !v.Updated.Equal(before.Updated) {
		t.Fatalf("blocked %v blocked_by %v updated %v (dependant must not be rewritten)", v.Blocked, v.BlockedBy, v.Updated)
	}
	want := []api.BlockerView{
		{ID: "acme-4", Title: "Tc", State: domain.Draft, Via: "acme-3", Cleared: boolPtr(false)},
		{ID: "acme-5", Title: "Td", State: domain.Draft, Via: "acme-3", Cleared: boolPtr(false)},
	}
	if !reflect.DeepEqual(v.Blockers, want) {
		t.Fatalf("blockers %+v, want %+v", v.Blockers, want)
	}
	f.setState(t, 4, domain.Done)
	f.setState(t, 5, domain.Done)
	if v := f.get(t, "acme-2"); v.Blocked || !v.Claimable {
		t.Fatalf("still blocked once every Replacement is Done: %+v", v.Blockers)
	}
}

func TestACancelledBlockerWithNoReplacementsKeepsItsDependantsBlocked(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready) // acme-2, acme-3
	f.addBlocker(t, "acme-2", "acme-3")
	f.cancel(t, api.CancelInput{Ticket: "acme-3"}, api.Human)
	v := f.get(t, "acme-2")
	if !v.Blocked || v.Claimable || !slices.Equal(blockerIDs(v), []string{"acme-3 Cancelled"}) {
		t.Fatalf("blocked %v claimable %v blockers %v", v.Blocked, v.Claimable, blockerIDs(v))
	}
	// Removing the stranded Blocker frees it.
	if v, err := record(f.core.RemoveBlocker(human, "acme", api.BlockerInput{Ticket: "acme-2", Blocker: "acme-3", Full: true})); err != nil || v.Blocked {
		t.Fatalf("remove: %v, blocked %v", err, v.Blocked)
	}
}

func TestReplacementsThatWouldFormACycleAreRejected(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready, domain.Ready) // acme-2, acme-3, acme-4
	// acme-2 waits on acme-3; replacing acme-3 with acme-2 would make
	// acme-2 wait on itself.
	f.addBlocker(t, "acme-2", "acme-3")
	_, err := f.core.Cancel(ctx, "acme", api.CancelInput{Ticket: "acme-3", Replacements: []api.Ref{"acme-2"}})
	wantCode(t, err, api.Cycle, "")

	// acme-4 waits on acme-2; replacing acme-3 with acme-4 would close
	// acme-2 → acme-4 → acme-2.
	f.addBlocker(t, "acme-4", "acme-2")
	_, err = f.core.Cancel(ctx, "acme", api.CancelInput{Ticket: "acme-3", Replacements: []api.Ref{"acme-4"}})
	wantCode(t, err, api.Cycle, "")
	if v := f.get(t, "acme-3"); v.State != domain.Ready {
		t.Fatalf("rejected cancel was written: %s", v.State)
	}
}

func TestAReplacementThatADependantWaitsOnThroughCancelledTicketsCannotLoopBack(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready, domain.Ready, domain.Ready, domain.Ready) // acme-2 to acme-6
	// acme-2 waits on acme-3, which hands its work to acme-4, then acme-5.
	f.addBlocker(t, "acme-2", "acme-3")
	f.cancel(t, api.CancelInput{Ticket: "acme-3", Replacements: []api.Ref{"acme-4"}})
	f.cancel(t, api.CancelInput{Ticket: "acme-4", Replacements: []api.Ref{"acme-5"}})
	// So replacing acme-5 by acme-2 would make acme-2 wait on itself.
	_, err := f.core.Cancel(ctx, "acme", api.CancelInput{Ticket: "acme-5", Replacements: []api.Ref{"acme-2"}})
	wantCode(t, err, api.Cycle, "")
	var e *api.Error
	if errors.As(err, &e); !strings.Contains(e.Message, "acme-2 → acme-2") {
		t.Fatalf("message %q, want the chain acme-2 → acme-2", e.Message)
	}
	// Replacing it by a Ticket nothing waits through is fine.
	f.cancel(t, api.CancelInput{Ticket: "acme-5", Replacements: []api.Ref{"acme-6"}})
}

func TestAReplacementChainCannotLoopBack(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready, domain.Ready) // acme-2, acme-3, acme-4
	f.cancel(t, api.CancelInput{Ticket: "acme-3", Replacements: []api.Ref{"acme-2"}})
	// acme-3 is replaced by acme-2, so replacing acme-2 by acme-3 loops.
	_, err := f.core.Cancel(ctx, "acme", api.CancelInput{Ticket: "acme-2", Replacements: []api.Ref{"acme-3"}})
	wantCode(t, err, api.Cycle, "")
}
