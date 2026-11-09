package core_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// get_ticket's smaller default and its parts (ticket 192, ADR 0015).

// redone is acme-2 failed under Claim 1, renamed, sent back with a note,
// and asking a Round under Claim 2: two Claims, a Failure Report, a
// comment (the send-back note), a Round, five moves and an edit.
func (f fixture) redone(t *testing.T) {
	t.Helper()
	f.failed(t)
	title := "Login form"
	if _, err := f.core.EditTicket(ctx, "acme", core.EditTicketInput{Ticket: "2", Title: &title}); err != nil {
		t.Fatalf("edit_ticket: %v", err)
	}
	if _, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2", Note: "Migration 12 has run now"}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
	f.claim(t, core.ClaimInput{SessionID: "s2"})
	f.clock.Set(t0.Add(2 * time.Hour))
	if _, err := f.core.PostRound(ctx, "acme", core.PostRoundInput{Ticket: "2", ClaimID: "claim-2", Questions: []core.QuestionInput{qDB}}); err != nil {
		t.Fatalf("post_round: %v", err)
	}
}

// readTicket is get_ticket's result as a caller sees its JSON.
func (f fixture) readTicket(t *testing.T, in core.GetTicketInput) map[string]any {
	t.Helper()
	v, err := f.core.GetTicket(ctx, "acme", in)
	if err != nil {
		t.Fatalf("get_ticket %+v: %v", in, err)
	}
	return asJSON(t, v)
}

func asJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// scalars is every key a read returns whatever parts it asks for (on
// acme-2, which has no branch, link or silence).
var scalars = []string{"blocked", "blocked_by", "blockers", "claimable", "created", "description", "id", "labels", "needs_me_reason", "replacements", "spec", "state", "tags", "title", "updated"}

// redoneCounts is how many of each part redone leaves on acme-2.
var redoneCounts = map[string]int{"claims": 2, "failure_reports": 1, "comments": 1, "rounds": 1, "moves": 5, "edits": 1}

func withKeys(base []string, more ...string) []string {
	out := slices.Concat(base, more)
	slices.Sort(out)
	return out
}

func TestGetTicketByDefaultLeavesOutRoundsMovesAndEditsAndCountsThem(t *testing.T) {
	f := newFixture(t)
	f.redone(t)
	v := f.readTicket(t, core.GetTicketInput{Ticket: "acme-2"})
	if got, want := keys(v), withKeys(scalars, "claims", "failure_reports", "comments", "left_out"); !slices.Equal(got, want) {
		t.Fatalf("keys %v, want %v", got, want)
	}
	for _, p := range []string{"claims", "failure_reports", "comments"} {
		if n := len(v[p].([]any)); n != redoneCounts[p] {
			t.Errorf("%s: %d, want %d", p, n, redoneCounts[p])
		}
	}
	if want := map[string]any{"rounds": float64(1), "moves": float64(5), "edits": float64(1)}; !reflect.DeepEqual(v["left_out"], want) {
		t.Fatalf("left_out %v, want %v", v["left_out"], want)
	}
	// What a redo needs is all there: the send-back note, the Failure
	// Report and how the first Claim ended.
	if c := v["comments"].([]any)[0].(map[string]any); c["body"] != "Migration 12 has run now" {
		t.Fatalf("comment %v", c)
	}
	if r := v["failure_reports"].([]any)[0].(map[string]any); r["what_to_fix"] != "Run migration 12" {
		t.Fatalf("failure report %v", r)
	}
	if c := v["claims"].([]any)[0].(map[string]any); c["ended_by"] != "report_failure" || c["ended"] == nil {
		t.Fatalf("first claim %v", c)
	}
}

func TestGetTicketWithOnePartReturnsTheScalarsAndOnlyThatPart(t *testing.T) {
	for _, part := range core.TicketParts {
		t.Run(part, func(t *testing.T) {
			f := newFixture(t)
			f.redone(t)
			v := f.readTicket(t, core.GetTicketInput{Ticket: "acme-2", Parts: []string{part}})
			if got, want := keys(v), withKeys(scalars, part, "left_out"); !slices.Equal(got, want) {
				t.Fatalf("keys %v, want %v", got, want)
			}
			if n := len(v[part].([]any)); n != redoneCounts[part] {
				t.Fatalf("%s: %d, want %d", part, n, redoneCounts[part])
			}
			want := map[string]any{}
			for p, n := range redoneCounts {
				if p != part {
					want[p] = float64(n)
				}
			}
			if !reflect.DeepEqual(v["left_out"], want) {
				t.Fatalf("left_out %v, want %v", v["left_out"], want)
			}
		})
	}
}

func TestGetTicketWithEveryPartIsTheWholeTicket(t *testing.T) {
	f := newFixture(t)
	f.redone(t)
	v := f.readTicket(t, core.GetTicketInput{Ticket: "acme-2", Parts: core.TicketParts})
	if got, want := keys(v), withKeys(scalars, core.TicketParts...); !slices.Equal(got, want) {
		t.Fatalf("keys %v, want %v: no left_out on the whole Ticket", got, want)
	}
	// It is what a write returns with full.
	w, err := record(f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "Still on it", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, "2"); !reflect.DeepEqual(got, w) {
		t.Fatalf("get_ticket with every part\n%+v\nwant the full write's\n%+v", got, w)
	}
}

func TestGetTicketLeavesOutOnlyPartsThatHaveSomething(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	// A new Ticket has nothing in any part; a part asked for that is empty
	// is still there, as [].
	v := f.readTicket(t, core.GetTicketInput{Ticket: "acme-2", Parts: []string{"rounds"}})
	if !reflect.DeepEqual(v["rounds"], []any{}) {
		t.Fatalf("rounds %v, want []", v["rounds"])
	}
	if _, ok := v["left_out"]; ok {
		t.Fatalf("left_out %v, want none: nothing else has anything", v["left_out"])
	}
	f.move(t, "2", store.Draft)
	f.move(t, "2", store.Ready)
	v = f.readTicket(t, core.GetTicketInput{Ticket: "acme-2", Parts: []string{"rounds"}})
	if want := map[string]any{"moves": float64(2)}; !reflect.DeepEqual(v["left_out"], want) {
		t.Fatalf("left_out %v, want %v", v["left_out"], want)
	}
}

func TestGetTicketRejectsAnUnknownPart(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	_, err := f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: "2", Parts: []string{"rounds", "history"}})
	wantCode(t, err, core.Invalid, "parts")
	if want := `unknown part "history" (the parts are claims, failure_reports, comments, rounds, moves, edits)`; err.(*core.Error).Message != want {
		t.Fatalf("message %q, want %q", err.(*core.Error).Message, want)
	}
}

func TestClaimReturnsTheTicketInGetTicketsDefaultShape(t *testing.T) {
	f := newFixture(t)
	f.failed(t)
	if _, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2", Note: "Migration 12 has run now"}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
	res := f.claim(t, core.ClaimInput{SessionID: "s2"})
	want := f.readTicket(t, core.GetTicketInput{Ticket: "acme-2"})
	if got := asJSON(t, res.Ticket); !reflect.DeepEqual(got, want) {
		t.Fatalf("claim's Ticket\n%v\nwant get_ticket's default\n%v", got, want)
	}
	if _, ok := want["moves"]; ok {
		t.Fatalf("claim's Ticket has moves: %v", want)
	}
}

// commentBodies is the bodies of a read's comments.
func commentBodies(v core.TicketView) []string {
	var out []string
	for _, c := range v.Comments {
		out = append(out, c.Body)
	}
	return out
}

// Each Stage's reads carry what it needs (ticket 192): a redo's claim has
// the Failure Report, send-back, rejection and give-up notes and the
// earlier finish summary, and the Rounds' answers are a part away.
func TestARedosClaimCarriesWhyTheLastClaimEnded(t *testing.T) {
	f := newFixture(t)
	f.failed(t)
	sendBack := "Migration 12 has run now"
	if _, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2", Note: sendBack}); err != nil {
		t.Fatal(err)
	}
	f.claim(t, core.ClaimInput{SessionID: "s2"})
	summary := "Branch acme-2 at abc123: the form posts to /login"
	if _, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-2", Summary: summary}); err != nil {
		t.Fatal(err)
	}
	rejection := "The form forgets the email after an error"
	if _, err := f.core.Reject(human, "acme", core.RejectInput{Ticket: "2", Comment: rejection}); err != nil {
		t.Fatal(err)
	}
	f.claim(t, core.ClaimInput{SessionID: "s3"})
	giveUp := "Out of context: the email is kept, the error message is left"
	if _, err := f.core.GiveUp(ctx, "acme", core.GiveUpInput{Ticket: "2", ClaimID: "claim-3", Note: giveUp}); err != nil {
		t.Fatal(err)
	}

	tk := *f.claim(t, core.ClaimInput{SessionID: "s4"}).Ticket
	if want := []string{sendBack, summary, rejection, giveUp}; !slices.Equal(commentBodies(tk), want) {
		t.Fatalf("comments %q, want %q", commentBodies(tk), want)
	}
	if len(tk.FailureReports) != 1 || tk.FailureReports[0].WhatToFix != "Run migration 12" {
		t.Fatalf("failure reports %+v", tk.FailureReports)
	}
	var endings []string
	for _, c := range tk.Claims {
		endings = append(endings, c.EndedBy)
	}
	if want := []string{"report_failure", "finish", "give_up", ""}; !slices.Equal(endings, want) {
		t.Fatalf("claims ended by %q, want %q", endings, want)
	}
}

func TestTheSpecStageReadsTheGrillingTicketsAnswersAndResolution(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB)
	if _, err := f.core.Answer(human, "acme", core.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite, one file"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.PickUpAnswers(ctx, "acme", core.PickUpAnswersInput{Ticket: "2", ClaimID: "claim-1"}); err != nil {
		t.Fatal(err)
	}
	resolution := "# Resolution\n\nSQLite, one file."
	if _, err := f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: resolution}); err != nil {
		t.Fatal(err)
	}
	// The default says there is a Round to read.
	if v := f.readTicket(t, core.GetTicketInput{Ticket: "2"}); !reflect.DeepEqual(v["left_out"], map[string]any{"rounds": float64(1), "moves": float64(4)}) {
		t.Fatalf("left_out %v", v["left_out"])
	}
	// The spec guide reads the resolution comment, then the Rounds when it
	// points to one.
	v, err := f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: "2", Parts: []string{"comments"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(commentBodies(v), []string{resolution}) {
		t.Fatalf("comments %q", commentBodies(v))
	}
	v, err = f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: "2", Parts: []string{"rounds"}})
	if err != nil {
		t.Fatal(err)
	}
	if q := v.Rounds[0].Questions[0]; q.Answer != "SQLite, one file" {
		t.Fatalf("question %+v", q)
	}
}
