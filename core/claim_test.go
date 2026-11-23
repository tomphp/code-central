package core_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/core/coretest"
	"github.com/tomoram/code-central/domain"
)

// work is a Project acme with Spec acme-1 and the given Tickets on it,
// created a minute apart so their claim order is their creation order.
func (f fixture) work(t *testing.T, states ...domain.State) []api.TicketView {
	return f.shared().Work(t, states...)
}

// longLived gives s the long-lived Label.
func (f fixture) longLived(t *testing.T, s api.SpecView) { f.shared().LongLived(t, s) }

var human = coretest.Human

func (f fixture) claim(t *testing.T, in api.ClaimInput) api.ClaimResult {
	return f.shared().Claim(t, in)
}

func TestClaimTakesTheNextReadyTicketInClaimOrder(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Draft, domain.Ready, domain.Ready) // acme-2 Draft, acme-3 and acme-4 Ready
	f.clock.Set(t0.Add(time.Hour))

	res := f.claim(t, api.ClaimInput{SessionID: "sess-1"})
	if res.ClaimID != "claim-1" || res.Ticket.ID != "acme-3" || res.Ticket.State != domain.InProgress {
		t.Fatalf("got claim %q on %s (%s), want claim-1 on acme-3 In Progress", res.ClaimID, res.Ticket.ID, res.Ticket.State)
	}
	want := api.MoveView{From: domain.Ready, To: domain.InProgress, Actor: "agent", Time: t0.Add(time.Hour), Claim: 1}
	if moves := f.get(t, "acme-3").Moves; len(moves) != 1 || moves[0] != want {
		t.Fatalf("moves %+v, want [%+v]", moves, want)
	}
	if c := res.Ticket.Claims; len(c) != 1 || c[0].Number != 1 || c[0].SessionID != "sess-1" || !c[0].Started.Equal(t0.Add(time.Hour)) || c[0].Ended != nil {
		t.Fatalf("claims %+v", c)
	}
	if next := f.claim(t, api.ClaimInput{}); next.Ticket.ID != "acme-4" || next.ClaimID != "claim-2" {
		t.Fatalf("second claim took %s with %q, want acme-4 with claim-2", next.Ticket.ID, next.ClaimID)
	}
}

func TestClaimOrderBreaksTiesOnNumber(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "S")
	for range 3 { // same created time
		f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "T", State: domain.Ready})
	}
	for _, want := range []string{"acme-2", "acme-3", "acme-4"} {
		if got := f.claim(t, api.ClaimInput{}).Ticket.ID; got != want {
			t.Fatalf("claimed %s, want %s", got, want)
		}
	}
}

func TestClaimWithNothingClaimableReturnsNoTicket(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Draft)
	res, err := f.core.Claim(ctx, "acme", api.ClaimInput{})
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
	f.work(t, domain.Ready) // acme-2 on acme-1
	f.spec(t, "acme", "Other")
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "3", Title: "Later", State: domain.Ready}) // acme-4
	if got := f.claim(t, api.ClaimInput{Spec: "acme-3"}).Ticket.ID; got != "acme-4" {
		t.Fatalf("claimed %s, want acme-4 from Spec acme-3", got)
	}
	res, err := f.core.Claim(ctx, "acme", api.ClaimInput{Spec: "3"})
	if err != nil || res.Ticket != nil {
		t.Fatalf("got %+v, %v; want nothing left in acme-3", res, err)
	}
	_, err = f.core.Claim(ctx, "acme", api.ClaimInput{Spec: "9"})
	wantCode(t, err, api.Invalid, "spec")
}

func TestClaimANamedTicket(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready, domain.Draft) // acme-2, acme-3 Ready; acme-4 Draft
	if got := f.claim(t, api.ClaimInput{Ticket: "acme-3"}).Ticket.ID; got != "acme-3" {
		t.Fatalf("claimed %s, want acme-3", got)
	}

	_, err := f.core.Claim(ctx, "acme", api.ClaimInput{Ticket: "4"})
	wantCode(t, err, api.ActionNotAllowed, "")
	e := err.(*api.Error)
	if !strings.Contains(e.Message, "Draft") {
		t.Fatalf("message %q should give the Ticket's State as the reason", e.Message)
	}
	if !slices.Contains(e.AllowedActions, "approve") || slices.Contains(e.AllowedActions, "claim") {
		t.Fatalf("allowed actions %v, want those from Draft", e.AllowedActions)
	}

	_, err = f.core.Claim(ctx, "acme", api.ClaimInput{Ticket: "3"})
	wantCode(t, err, api.ActionNotAllowed, "")
	_, err = f.core.Claim(ctx, "acme", api.ClaimInput{Ticket: "9"})
	wantCode(t, err, api.NotFound, "")
}

func TestClaimRejectsBothSpecAndTicket(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	_, err := f.core.Claim(ctx, "acme", api.ClaimInput{Spec: "1", Ticket: "2"})
	wantCode(t, err, api.Invalid, "ticket")
}

func TestClaimOnAnUnknownProject(t *testing.T) {
	f := newFixture(t)
	_, err := f.core.Claim(ctx, "nope", api.ClaimInput{})
	wantCode(t, err, api.UnknownProject, "")
}

func TestTheClaimIDIsLeftOutOfEveryRead(t *testing.T) {
	f := newFixture(t)
	f.core = core.New(f.store, f.clock, fixedID("SECRETCLAIMID"))
	f.work(t, domain.Ready)
	res := f.claim(t, api.ClaimInput{})
	if res.ClaimID != "SECRETCLAIMID" {
		t.Fatalf("claim ID %q", res.ClaimID)
	}
	view, _ := json.Marshal(res.Ticket)
	got, err := f.core.GetTicket(ctx, "acme", whole("2"))
	if err != nil {
		t.Fatal(err)
	}
	read, _ := json.Marshal(got)
	rows, _ := f.core.FindTickets(ctx, "acme", api.FindTicketsInput{})
	found, _ := json.Marshal(rows)
	for _, b := range [][]byte{view, read, found} {
		if strings.Contains(string(b), "SECRETCLAIMID") {
			t.Fatalf("a read shows the Claim ID: %s", b)
		}
	}
}

func TestTwoConcurrentClaimsNeverTakeTheSameTicket(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready, domain.Ready, domain.Ready, domain.Ready)
	const agents = 20
	var wg sync.WaitGroup
	got := make(chan string, agents)
	for range agents {
		wg.Go(func() {
			res, err := f.core.Claim(ctx, "acme", api.ClaimInput{})
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

func TestTheHoldingSessionClaimsItsTicketAgainAndGetsTheSameClaim(t *testing.T) {
	for _, c := range []struct {
		state domain.State
		setup func(f fixture)
	}{
		{domain.InProgress, func(f fixture) {
			f.work(t, domain.Ready)
			f.claim(t, api.ClaimInput{SessionID: "sess-1"})
		}},
		{domain.NeedsAnswer, func(f fixture) { f.asking(t, qDB) }},
		{domain.Answered, func(f fixture) {
			f.asking(t, qDB)
			if _, err := f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"}); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(string(c.state), func(t *testing.T) {
			f := newFixture(t)
			c.setup(f)
			f.clock.Advance(time.Hour)
			before := f.get(t, "2")
			notices, stop := f.core.Events().Subscribe()
			defer stop()

			dir, resume := "/elsewhere", "other --resume"
			res := f.claim(t, api.ClaimInput{Ticket: "acme-2", SessionID: "sess-1", WorkingDir: &dir, ResumeCommand: &resume})
			if res.ClaimID != "claim-1" || res.Ticket.State != c.state || res.StageGuide == nil || res.StageGuide.Stage != "implementation" {
				t.Fatalf("got claim %q on %s, guide %+v; want claim-1 in %s with the implementation guide", res.ClaimID, res.Ticket.State, res.StageGuide, c.state)
			}
			after := f.get(t, "2")
			b, _ := json.Marshal(before)
			a, _ := json.Marshal(after)
			if string(a) != string(b) {
				t.Fatalf("claiming again wrote the Ticket:\nbefore %s\nafter  %s", b, a)
			}
			now, err := f.core.GetTicket(ctx, "acme", api.GetTicketInput{Ticket: "2"})
			if err != nil {
				t.Fatal(err)
			}
			if r, n := asJSON(t, res.Ticket), asJSON(t, now); !reflect.DeepEqual(r, n) {
				t.Fatalf("claiming again gave the view %v, want the Ticket as it is %v", r, n)
			}
			select {
			case e := <-notices:
				t.Fatalf("claiming again sent a notice %+v", e)
			default:
			}
		})
	}
}

func TestClaimingAgainTakesNoNewClaimID(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready)
	f.claim(t, api.ClaimInput{SessionID: "sess-1"})
	f.claim(t, api.ClaimInput{Ticket: "2", SessionID: "sess-1"})
	if got := f.claim(t, api.ClaimInput{}).ClaimID; got != "claim-2" {
		t.Fatalf("the next fresh Claim got %q, want claim-2", got)
	}
}

func TestOnlyANamedTicketAndAMatchingSessionGetTheirClaimBack(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready) // acme-2, acme-3
	f.claim(t, api.ClaimInput{SessionID: "sess-1"})

	// A bare or Spec-narrowed claim takes the next Ticket, not your own.
	if res := f.claim(t, api.ClaimInput{SessionID: "sess-1"}); res.Ticket.ID != "acme-3" || res.ClaimID != "claim-2" {
		t.Fatalf("bare claim took %s with %q, want acme-3 with claim-2", res.Ticket.ID, res.ClaimID)
	}
	res, err := f.core.Claim(ctx, "acme", api.ClaimInput{Spec: "1", SessionID: "sess-1"})
	if err != nil || res.Ticket != nil {
		t.Fatalf("Spec-narrowed claim got %+v, %v; want nothing claimable", res, err)
	}

	// An empty session ID never matches, not even a Claim made without one.
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Td", State: domain.Ready}) // acme-4
	f.claim(t, api.ClaimInput{Ticket: "4"})
	_, err = f.core.Claim(ctx, "acme", api.ClaimInput{Ticket: "4"})
	wantMessage(t, err, api.ActionNotAllowed, "",
		"acme-4 is held by Claim 1 of another session; claim another Ticket, or ask the human to release it; if this session holds it, call claim again with your session_id")
}

func TestClaimingATicketAnotherSessionHoldsSaysWhoHoldsIt(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB) // acme-2 Needs Answer under Claim 1 of sess-1
	_, err := f.core.Claim(ctx, "acme", api.ClaimInput{Ticket: "2", SessionID: "sess-2"})
	wantMessage(t, err, api.ActionNotAllowed, "",
		"acme-2 is held by Claim 1 of another session; claim another Ticket, or ask the human to release it")
	_, err = f.core.Claim(ctx, "acme", api.ClaimInput{Ticket: "2"})
	wantMessage(t, err, api.ActionNotAllowed, "",
		"acme-2 is held by Claim 1 of another session; claim another Ticket, or ask the human to release it; if this session holds it, call claim again with your session_id")
}
