package core_test

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// work is a Project acme with Spec acme-1 and the given Tickets on it,
// created a minute apart so their claim order is their creation order.
func (f fixture) work(t *testing.T, states ...store.State) []core.TicketView {
	t.Helper()
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	var out []core.TicketView
	for i, s := range states {
		out = append(out, f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "T" + string(rune('a'+i)), State: s}))
		f.clock.Advance(time.Minute)
	}
	return out
}

var human = core.WithActor(ctx, core.Human)

func (f fixture) claim(t *testing.T, in core.ClaimInput) core.ClaimResult {
	t.Helper()
	res, err := f.core.Claim(ctx, "acme", in)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if res.Ticket == nil {
		t.Fatal("claim: nothing claimed")
	}
	return res
}

func TestClaimTakesTheNextReadyTicketInClaimOrder(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Draft, store.Ready, store.Ready) // acme-2 Draft, acme-3 and acme-4 Ready
	f.clock.Set(t0.Add(time.Hour))

	res := f.claim(t, core.ClaimInput{SessionID: "sess-1"})
	if res.ClaimID != "claim-1" || res.Ticket.ID != "acme-3" || res.Ticket.State != store.InProgress {
		t.Fatalf("got claim %q on %s (%s), want claim-1 on acme-3 In Progress", res.ClaimID, res.Ticket.ID, res.Ticket.State)
	}
	want := core.MoveView{From: store.Ready, To: store.InProgress, Actor: "agent", Time: t0.Add(time.Hour), Claim: 1}
	if len(res.Ticket.Moves) != 1 || res.Ticket.Moves[0] != want {
		t.Fatalf("moves %+v, want [%+v]", res.Ticket.Moves, want)
	}
	if c := res.Ticket.Claims; len(c) != 1 || c[0].Number != 1 || c[0].SessionID != "sess-1" || !c[0].Started.Equal(t0.Add(time.Hour)) || c[0].Ended != nil {
		t.Fatalf("claims %+v", c)
	}
	if next := f.claim(t, core.ClaimInput{}); next.Ticket.ID != "acme-4" || next.ClaimID != "claim-2" {
		t.Fatalf("second claim took %s with %q, want acme-4 with claim-2", next.Ticket.ID, next.ClaimID)
	}
}

func TestClaimOrderBreaksTiesOnNumber(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "S")
	for range 3 { // same created time
		f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "T", State: store.Ready})
	}
	for _, want := range []string{"acme-2", "acme-3", "acme-4"} {
		if got := f.claim(t, core.ClaimInput{}).Ticket.ID; got != want {
			t.Fatalf("claimed %s, want %s", got, want)
		}
	}
}

func TestClaimWithNothingClaimableReturnsNoTicket(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Draft)
	res, err := f.core.Claim(ctx, "acme", core.ClaimInput{})
	if err != nil || res.Ticket != nil || res.ClaimID != "" {
		t.Fatalf("got %+v, %v; want {ticket: null}", res, err)
	}
	b, _ := json.Marshal(res)
	if string(b) != `{"ticket":null}` {
		t.Fatalf("JSON %s, want {\"ticket\":null}", b)
	}
}

func TestClaimNarrowedToASpec(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready) // acme-2 on acme-1
	f.spec(t, "acme", "Other")
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "3", Title: "Later", State: store.Ready}) // acme-4
	if got := f.claim(t, core.ClaimInput{Spec: "acme-3"}).Ticket.ID; got != "acme-4" {
		t.Fatalf("claimed %s, want acme-4 from Spec acme-3", got)
	}
	res, err := f.core.Claim(ctx, "acme", core.ClaimInput{Spec: "3"})
	if err != nil || res.Ticket != nil {
		t.Fatalf("got %+v, %v; want nothing left in acme-3", res, err)
	}
	_, err = f.core.Claim(ctx, "acme", core.ClaimInput{Spec: "9"})
	wantCode(t, err, core.Invalid, "spec")
}

func TestClaimANamedTicket(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready, store.Draft) // acme-2, acme-3 Ready; acme-4 Draft
	if got := f.claim(t, core.ClaimInput{Ticket: "acme-3"}).Ticket.ID; got != "acme-3" {
		t.Fatalf("claimed %s, want acme-3", got)
	}

	_, err := f.core.Claim(ctx, "acme", core.ClaimInput{Ticket: "4"})
	wantCode(t, err, core.ActionNotAllowed, "")
	e := err.(*core.Error)
	if !strings.Contains(e.Message, "Draft") {
		t.Fatalf("message %q should give the Ticket's State as the reason", e.Message)
	}
	if !slices.Contains(e.AllowedActions, "approve") || slices.Contains(e.AllowedActions, "claim") {
		t.Fatalf("allowed actions %v, want those from Draft", e.AllowedActions)
	}

	_, err = f.core.Claim(ctx, "acme", core.ClaimInput{Ticket: "3"})
	wantCode(t, err, core.ActionNotAllowed, "")
	_, err = f.core.Claim(ctx, "acme", core.ClaimInput{Ticket: "9"})
	wantCode(t, err, core.NotFound, "")
}

func TestClaimRejectsBothSpecAndTicket(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	_, err := f.core.Claim(ctx, "acme", core.ClaimInput{Spec: "1", Ticket: "2"})
	wantCode(t, err, core.Invalid, "ticket")
}

func TestClaimOnAnUnknownProject(t *testing.T) {
	f := newFixture(t)
	_, err := f.core.Claim(ctx, "nope", core.ClaimInput{})
	wantCode(t, err, core.UnknownProject, "")
}

func TestTheClaimIDIsLeftOutOfEveryRead(t *testing.T) {
	f := newFixture(t)
	f.core = core.New(f.store, f.clock, fixedID("SECRETCLAIMID"))
	f.work(t, store.Ready)
	res := f.claim(t, core.ClaimInput{})
	if res.ClaimID != "SECRETCLAIMID" {
		t.Fatalf("claim ID %q", res.ClaimID)
	}
	view, _ := json.Marshal(res.Ticket)
	got, err := f.core.GetTicket(ctx, "acme", "2")
	if err != nil {
		t.Fatal(err)
	}
	read, _ := json.Marshal(got)
	rows, _ := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{})
	found, _ := json.Marshal(rows)
	for _, b := range [][]byte{view, read, found} {
		if strings.Contains(string(b), "SECRETCLAIMID") {
			t.Fatalf("a read shows the Claim ID: %s", b)
		}
	}
}

func TestTwoConcurrentClaimsNeverTakeTheSameTicket(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready, store.Ready, store.Ready, store.Ready)
	const agents = 20
	var wg sync.WaitGroup
	got := make(chan string, agents)
	for range agents {
		wg.Go(func() {
			res, err := f.core.Claim(ctx, "acme", core.ClaimInput{})
			if err != nil {
				t.Error(err)
				return
			}
			if res.Ticket != nil {
				got <- res.Ticket.ID
			}
		})
	}
	wg.Wait()
	close(got)
	seen := map[string]bool{}
	for id := range got {
		if seen[id] {
			t.Fatalf("%s was claimed twice", id)
		}
		seen[id] = true
	}
	if len(seen) != 5 {
		t.Fatalf("%d Tickets claimed, want all 5", len(seen))
	}
}

type fixedID string

func (f fixedID) ClaimID() string { return string(f) }
