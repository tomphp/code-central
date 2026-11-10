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

// An agent's default read leaves out what it doesn't use (code-central-320):
// a Done Blocker's title and the Claims' session fields, which parts:
// [claims] returns in full.

// sessioned is acme-2 claimed, then blocked by acme-3 (Done) and acme-4 (Ready),
// its Claim by s1 with a working dir, a resume command and a Session Event.
func (f fixture) sessioned(t *testing.T) {
	t.Helper()
	f.work(t, store.Ready, store.Ready, store.Ready)
	f.move(t, "3", store.Done)
	if _, err := f.core.Claim(ctx, "acme", core.ClaimInput{Ticket: "acme-2", SessionID: "s1", WorkingDir: str("/work/acme-2"), ResumeCommand: str("claude --resume s1")}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	for _, b := range []core.Ref{"3", "4"} {
		if _, err := f.core.AddBlocker(ctx, "acme", core.BlockerInput{Ticket: "2", Blocker: b}); err != nil {
			t.Fatalf("add_blocker %s: %v", b, err)
		}
	}
	f.core.PostSessionEvents(ctx, "s1", []core.SessionEvent{{Time: f.clock.Now(), Kind: "tool_call", Text: "Bash: go test ./..."}})
}

func TestGetTicketByDefaultShowsADoneBlockerAsItsIDAndStateOnly(t *testing.T) {
	f := newFixture(t)
	f.sessioned(t)
	v := f.readTicket(t, core.GetTicketInput{Ticket: "acme-2"})
	want := []any{
		map[string]any{"id": "acme-3", "state": "Done"},
		map[string]any{"id": "acme-4", "title": "Tc", "state": "Ready", "cleared": false},
	}
	if !reflect.DeepEqual(v["blockers"], want) {
		t.Fatalf("blockers %v, want %v", v["blockers"], want)
	}
}

func TestGetTicketByDefaultLeavesOutTheClaimsSessionFieldsAndCountsThem(t *testing.T) {
	f := newFixture(t)
	f.sessioned(t)
	v := f.readTicket(t, core.GetTicketInput{Ticket: "acme-2"})
	c := v["claims"].([]any)[0].(map[string]any)
	for _, k := range []string{"working_dir", "resume_command", "latest_event"} {
		if _, ok := c[k]; ok {
			t.Errorf("claim has %s: %v", k, c)
		}
	}
	if c["session_id"] != "s1" {
		t.Errorf("claim session_id %v, want s1", c["session_id"])
	}
	if want := map[string]any{"claim_sessions": float64(1), "moves": float64(1)}; !reflect.DeepEqual(v["left_out"], want) {
		t.Fatalf("left_out %v, want %v", v["left_out"], want)
	}
	// claim returns the same.
	f.clock.Advance(time.Minute)
	res := f.claim(t, core.ClaimInput{Ticket: "acme-2", SessionID: "s1"})
	if got := asJSON(t, res.Ticket); !reflect.DeepEqual(got["claims"], v["claims"]) {
		t.Fatalf("claim's claims %v, want %v", got["claims"], v["claims"])
	}
}

func TestGetTicketWithTheClaimsPartReturnsThemInFull(t *testing.T) {
	f := newFixture(t)
	f.sessioned(t)
	v := f.readTicket(t, core.GetTicketInput{Ticket: "acme-2", Parts: []string{"claims"}})
	c := v["claims"].([]any)[0].(map[string]any)
	if c["working_dir"] != "/work/acme-2" || c["resume_command"] != "claude --resume s1" {
		t.Fatalf("claim %v", c)
	}
	if e, _ := c["latest_event"].(map[string]any); e["text"] != "Bash: go test ./..." {
		t.Fatalf("latest_event %v", c["latest_event"])
	}
	if _, ok := v["left_out"].(map[string]any)["claim_sessions"]; ok {
		t.Fatalf("left_out %v counts claim_sessions", v["left_out"])
	}
	if b := v["blockers"].([]any)[0].(map[string]any); b["title"] != "Tb" {
		t.Fatalf("Done Blocker %v, want its title", b)
	}
}
