package core_test

import (
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

func ptr(s string) *string { return &s }

// defining is a Project acme with Spec acme-1 ("Login", body "body\n")
// and Draft Ticket acme-2 ("Form", description "Build it.\n").
func defining(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", Description: "Build it.\n"})
	return f
}

func TestEditTicketRecordsThePreviousTextOfChangedFields(t *testing.T) {
	f := defining(t)
	f.clock.Advance(time.Minute)
	v, err := f.core.EditTicket(core.WithActor(ctx, core.Human), "acme", core.EditTicketInput{
		Ticket: "acme-2", Title: ptr("Form"), Description: ptr("Build it well.\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.Title != "Form" || v.Description != "Build it well.\n" || !v.Updated.Equal(t0.Add(time.Minute)) {
		t.Fatalf("view: %+v", v)
	}
	want := core.EditView{Number: 1, Actor: "human", Time: t0.Add(time.Minute), Previous: map[string]string{"description": "Build it.\n"}}
	if len(v.Edits) != 1 || !editEqual(v.Edits[0], want) {
		t.Fatalf("edits %+v, want only %+v (an unchanged title is not recorded)", v.Edits, want)
	}

	v, err = f.core.EditTicket(ctx, "acme", core.EditTicketInput{Ticket: "2", Title: ptr("Login form")})
	if err != nil {
		t.Fatal(err)
	}
	want = core.EditView{Number: 2, Actor: "agent", Time: t0.Add(time.Minute), Previous: map[string]string{"title": "Form"}}
	if len(v.Edits) != 2 || !editEqual(v.Edits[1], want) {
		t.Fatalf("edits %+v, want a second entry %+v", v.Edits, want)
	}
	got, _ := f.core.GetTicket(ctx, "acme", "2")
	if len(got.Edits) != 2 || got.Title != "Login form" {
		t.Fatalf("get_ticket: %+v", got)
	}
}

func TestAnEditThatChangesNothingRecordsNothing(t *testing.T) {
	f := defining(t)
	f.clock.Advance(time.Minute)
	for _, in := range []core.EditTicketInput{
		{Ticket: "2"},
		{Ticket: "2", Title: ptr("Form"), Description: ptr("Build it.\n")},
	} {
		v, err := f.core.EditTicket(ctx, "acme", in)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Edits) != 0 || !v.Updated.Equal(t0) {
			t.Fatalf("a no-op edit wrote: %+v", v)
		}
	}
}

func TestEditTicketRejects(t *testing.T) {
	f := defining(t)
	_, err := f.core.EditTicket(ctx, "acme", core.EditTicketInput{Ticket: "2", Title: ptr(" ")})
	wantCode(t, err, core.Invalid, "title")
	_, err = f.core.EditTicket(ctx, "acme", core.EditTicketInput{Ticket: "1", Title: ptr("T")})
	wantCode(t, err, core.NotFound, "")
	_, err = f.core.EditTicket(ctx, "nope", core.EditTicketInput{Ticket: "2"})
	wantCode(t, err, core.UnknownProject, "")
}

func TestEditSpecReplacesTheBodyWholeAndKeepsTheOldOne(t *testing.T) {
	f := defining(t)
	f.clock.Advance(time.Minute)
	v, err := f.core.EditSpec(core.WithActor(ctx, core.Human), "acme", core.EditSpecInput{Spec: "acme-1", Body: ptr("# Login\n\nNew.\n"), Full: true})
	if err != nil {
		t.Fatal(err)
	}
	sp := v.Record
	if sp.Body != "# Login\n\nNew.\n" || sp.Title != "Login" || !sp.Updated.Equal(t0.Add(time.Minute)) {
		t.Fatalf("view: %+v", sp)
	}
	want := core.EditView{Number: 1, Actor: "human", Time: t0.Add(time.Minute), Previous: map[string]string{"body": "body\n"}}
	if len(sp.Edits) != 1 || !editEqual(sp.Edits[0], want) {
		t.Fatalf("edits %+v, want %+v", sp.Edits, want)
	}
	v, err = f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Title: ptr("Sign in"), Body: ptr("# Login\n\nNew.\n"), Full: true})
	if err != nil {
		t.Fatal(err)
	}
	sp = v.Record
	want = core.EditView{Number: 2, Actor: "agent", Time: t0.Add(time.Minute), Previous: map[string]string{"title": "Login"}}
	if len(sp.Edits) != 2 || !editEqual(sp.Edits[1], want) || sp.Title != "Sign in" {
		t.Fatalf("edits %+v, want a second entry %+v", sp.Edits, want)
	}
	v, _ = f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Title: ptr("Sign in"), Full: true})
	if len(v.Record.Edits) != 2 {
		t.Fatalf("a no-op edit was recorded: %+v", v.Record.Edits)
	}
}

func TestEditSpecRejects(t *testing.T) {
	f := defining(t)
	_, err := f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Title: ptr("")})
	wantCode(t, err, core.Invalid, "title")
	_, err = f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "2", Body: ptr("x")})
	wantCode(t, err, core.NotFound, "")
	_, err = f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "other-1", Body: ptr("x")})
	wantCode(t, err, core.Invalid, "spec")
}

func TestLabelsAreAddedAndRemovedByNameOnSpecsAndTickets(t *testing.T) {
	f := defining(t)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "API", Labels: []string{"api"}}) // 3
	f.clock.Advance(time.Minute)

	v, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "3", Labels: []string{"auth", "api"}})
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != "acme-3" || v.Kind != "ticket" || !slices.Equal(v.Labels, []string{"api", "auth"}) {
		t.Fatalf("add to a Ticket: %+v (an existing Label is kept once)", v)
	}
	v, err = f.core.RemoveLabels(ctx, "acme", core.LabelsInput{Target: "acme-3", Labels: []string{"api", "missing"}})
	if err != nil || !slices.Equal(v.Labels, []string{"auth"}) {
		t.Fatalf("remove from a Ticket: %+v, %v", v, err)
	}
	tk, _ := f.core.GetTicket(ctx, "acme", "3")
	if !slices.Equal(tk.Labels, []string{"auth"}) || len(tk.Edits) != 0 || !tk.Updated.Equal(t0.Add(time.Minute)) {
		t.Fatalf("ticket after: %+v (Label changes aren't recorded in edits)", tk)
	}

	v, err = f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "1", Labels: []string{"auth"}})
	if err != nil || v.ID != "acme-1" || v.Kind != "spec" || !slices.Equal(v.Labels, []string{"auth"}) {
		t.Fatalf("add to a Spec: %+v, %v", v, err)
	}
	sp, _ := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "1"})
	if !slices.Equal(sp.Labels, []string{"auth"}) || len(sp.Edits) != 0 {
		t.Fatalf("spec after: %+v", sp)
	}
}

func TestLabelsReject(t *testing.T) {
	f := defining(t)
	for _, l := range []string{"", " ", "a\nb"} {
		_, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "2", Labels: []string{l}})
		wantCode(t, err, core.Invalid, "labels")
	}
	_, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "2"})
	wantCode(t, err, core.Invalid, "labels")
	_, err = f.core.RemoveLabels(ctx, "acme", core.LabelsInput{Target: "9", Labels: []string{"x"}})
	wantCode(t, err, core.NotFound, "")
	_, err = f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "other-2", Labels: []string{"x"}})
	wantCode(t, err, core.Invalid, "target")
}

func TestSetTagsMergesAndRemoveTagsRemoves(t *testing.T) {
	f := defining(t)
	v, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "2", Tags: map[string]string{"type": "afk", "scratch.status": "ready"}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "2", Tags: map[string]string{"type": "hitl"}})
	if err != nil || !maps.Equal(v.Tags, map[string]string{"type": "hitl", "scratch.status": "ready"}) {
		t.Fatalf("set_tags merges and overwrites only the keys named: %+v, %v", v, err)
	}
	v, err = f.core.RemoveTags(ctx, "acme", core.RemoveTagsInput{Target: "2", Keys: []string{"type", "missing"}})
	if err != nil || !maps.Equal(v.Tags, map[string]string{"scratch.status": "ready"}) {
		t.Fatalf("remove_tags: %+v, %v", v, err)
	}
	tk, _ := f.core.GetTicket(ctx, "acme", "2")
	if tk.Tags["scratch.status"] != "ready" || len(tk.Edits) != 0 {
		t.Fatalf("ticket: %+v (Tag changes aren't recorded in edits)", tk)
	}
	v, err = f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "acme-1", Tags: map[string]string{"area": "web"}})
	if err != nil || v.Kind != "spec" || v.Tags["area"] != "web" {
		t.Fatalf("set_tags on a Spec: %+v, %v", v, err)
	}
}

func TestTagsReject(t *testing.T) {
	f := defining(t)
	for _, tags := range []map[string]string{
		nil,
		{"Type": "afk"},
		{"a b": "x"},
		{"": "x"},
		{"type": ""},
		{"type": "two\nlines"},
	} {
		_, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "2", Tags: tags})
		wantCode(t, err, core.Invalid, "tags")
	}
	for _, keys := range [][]string{nil, {"Type"}} {
		_, err := f.core.RemoveTags(ctx, "acme", core.RemoveTagsInput{Target: "2", Keys: keys})
		wantCode(t, err, core.Invalid, "keys")
	}
	_, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "7", Tags: map[string]string{"a": "b"}})
	wantCode(t, err, core.NotFound, "")
}

func TestCommentWorksOnAnyTicketWithNoClaim(t *testing.T) {
	f := defining(t)
	// A Ticket some agent holds, written straight to the store.
	f.store.Update(func(w store.Writer) error {
		return w.PutTicket("acme", store.Ticket{Number: 3, Title: "Held", State: store.InProgress, Spec: 1, Created: t0, Updated: t0})
	})
	f.clock.Advance(time.Minute)
	v, err := f.core.Comment(core.WithActor(ctx, core.Human), "acme", core.CommentInput{Ticket: "3", Body: "Try the other API.\n"})
	if err != nil {
		t.Fatal(err)
	}
	want := core.CommentView{Number: 1, Actor: "human", Time: t0.Add(time.Minute), Body: "Try the other API.\n"}
	if len(v.Comments) != 1 || v.Comments[0] != want || v.State != store.InProgress || !v.Updated.Equal(want.Time) {
		t.Fatalf("view: %+v", v)
	}
	v, _ = f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "acme-3", Body: "Noted."})
	if len(v.Comments) != 2 || v.Comments[1].Number != 2 || v.Comments[1].Actor != "agent" {
		t.Fatalf("second comment: %+v", v.Comments)
	}

	_, err = f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "3", Body: "  \n"})
	wantCode(t, err, core.Invalid, "body")
	_, err = f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "1", Body: "x"})
	wantCode(t, err, core.NotFound, "")
}

func TestApproveMovesDraftToReady(t *testing.T) {
	f := defining(t)
	f.clock.Advance(time.Minute)
	v, err := f.core.Approve(core.WithActor(ctx, core.Human), "acme", core.TicketInput{Ticket: "2"})
	if err != nil {
		t.Fatal(err)
	}
	want := core.MoveView{From: store.Draft, To: store.Ready, Actor: "human", Time: t0.Add(time.Minute)}
	if v.State != store.Ready || !v.Claimable || len(v.Moves) != 1 || v.Moves[0] != want {
		t.Fatalf("view: %+v", v)
	}
	_, err = f.core.Approve(ctx, "acme", core.TicketInput{Ticket: "2"})
	wantAllowed(t, err, "defer")
}

func TestDeferMovesReadyToDraftWithTheReasonAsAComment(t *testing.T) {
	f := defining(t)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "API", State: store.Ready}) // 3
	_, err := f.core.Defer(ctx, "acme", core.DeferInput{Ticket: "3", Reason: " "})
	wantCode(t, err, core.Invalid, "reason")
	f.clock.Advance(time.Minute)
	v, err := f.core.Defer(ctx, "acme", core.DeferInput{Ticket: "3", Reason: "Needs the schema first."})
	if err != nil {
		t.Fatal(err)
	}
	at := t0.Add(time.Minute)
	if v.State != store.Draft || v.Claimable || len(v.Moves) != 1 || v.Moves[0] != (core.MoveView{From: store.Ready, To: store.Draft, Actor: "agent", Time: at}) {
		t.Fatalf("view: %+v", v)
	}
	if len(v.Comments) != 1 || v.Comments[0] != (core.CommentView{Number: 1, Actor: "agent", Time: at, Body: "Needs the schema first."}) {
		t.Fatalf("comments: %+v", v.Comments)
	}
	_, err = f.core.Defer(ctx, "acme", core.DeferInput{Ticket: "2", Reason: "x"})
	wantAllowed(t, err, "approve")
}

// wantAllowed checks err is action_not_allowed naming want among the
// allowed actions.
func wantAllowed(t *testing.T, err error, want string) {
	t.Helper()
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.ActionNotAllowed || !slices.Contains(e.AllowedActions, want) {
		t.Fatalf("err = %v, want action_not_allowed allowing %s", err, want)
	}
}

func editEqual(a, b core.EditView) bool {
	if a.Number != b.Number || a.Actor != b.Actor || !a.Time.Equal(b.Time) || len(a.Previous) != len(b.Previous) {
		return false
	}
	for k, v := range b.Previous {
		if got, ok := a.Previous[k]; !ok || got != v {
			return false
		}
	}
	return true
}
