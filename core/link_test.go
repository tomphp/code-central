package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// The Integration contract (ticket 42, ADR 0004).

var integration = core.WithActor(ctx, core.Integration)

func ghLink(id string) core.LinkInput {
	return core.LinkInput{Integration: "github", ID: id, URL: "https://github.com/acme/web/issues/" + id}
}

// linked is Project acme with linked Spec acme-1 and, under it, linked
// Draft Ticket acme-2.
func (f fixture) linked(t *testing.T) (core.SpecView, core.TicketView) {
	t.Helper()
	f.project(t, "acme")
	sp, err := f.core.CreateLinkedSpec(integration, "acme", core.CreateLinkedSpecInput{Title: "Login", Body: "Users log in.\n", Link: ghLink("10")})
	if err != nil {
		t.Fatalf("create_linked_spec: %v", err)
	}
	tk, err := f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "1", Title: "Form", Description: "Build it.\n", Link: ghLink("11")})
	if err != nil {
		t.Fatalf("create_linked_ticket: %v", err)
	}
	return sp, tk
}

func wantLink(t *testing.T, got *core.LinkView, want core.LinkView) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("link %+v, want %+v", got, want)
	}
}

func TestAnIntegrationCreatesLinkedSpecsAndTicketsInDraftOrReady(t *testing.T) {
	f := newFixture(t)
	sp, tk := f.linked(t)
	wantLink(t, sp.Link, core.LinkView{Integration: "github", ID: "10", URL: "https://github.com/acme/web/issues/10", ExternalState: store.ExternalOpen})
	if tk.State != store.Draft || tk.Spec.ID != "acme-1" || tk.Description != "Build it.\n" {
		t.Fatalf("ticket %+v", tk)
	}
	wantLink(t, tk.Link, core.LinkView{Integration: "github", ID: "11", URL: "https://github.com/acme/web/issues/11", ExternalState: store.ExternalOpen})
	if got := f.get(t, "2"); got.Link == nil || got.Link.ID != "11" {
		t.Fatalf("get_ticket link %+v", got.Link)
	}
	ready, err := f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "1", Title: "API", State: store.Ready, Link: ghLink("12")})
	if err != nil || ready.State != store.Ready {
		t.Fatalf("ready: %+v, %v", ready, err)
	}
	rows, _ := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{})
	if rows[0].Link == nil || rows[0].Link.ID != "11" {
		t.Fatalf("find_tickets row link %+v", rows[0].Link)
	}

	_, err = f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "1", Title: "X", State: store.InProgress, Link: ghLink("13")})
	wantCode(t, err, core.Invalid, "state")
	_, err = f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "1", Title: "X", Link: core.LinkInput{Integration: "github"}})
	wantCode(t, err, core.Invalid, "link.id")
	_, err = f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "1", Title: "X", Link: core.LinkInput{Integration: "github", ID: "14", URL: "javascript:alert(1)"}})
	wantCode(t, err, core.Invalid, "link.url")
}

func TestAnItemIsLinkedToAnExternalItemAtMostOnce(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	_, err := f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "1", Title: "Again", Link: ghLink("11")})
	wantCode(t, err, core.Invalid, "link.id")
	_, err = f.core.CreateLinkedSpec(integration, "acme", core.CreateLinkedSpecInput{Title: "Again", Link: ghLink("10")})
	wantCode(t, err, core.Invalid, "link.id")
	// An external item with no parent is both a Spec and its one Ticket.
	if _, err := f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "1", Title: "Login", Link: ghLink("10")}); err != nil {
		t.Fatalf("a Ticket linked to the Spec's own item: %v", err)
	}
}

func TestOnlyAnIntegrationCreatesLinkedItems(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	for _, as := range []core.Actor{core.Human, core.Agent} {
		c := core.WithActor(ctx, as)
		_, err := f.core.CreateLinkedSpec(c, "acme", core.CreateLinkedSpecInput{Title: "S", Link: ghLink("1")})
		wantCode(t, err, core.ActionNotAllowed, "")
		_, err = f.core.CreateLinkedTicket(c, "acme", core.CreateLinkedTicketInput{Spec: "1", Title: "T", Link: ghLink("2")})
		wantCode(t, err, core.ActionNotAllowed, "")
	}
}

func TestOnlyTheIntegrationWritesALinkedDefinition(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	title := "Renamed"
	for _, as := range []core.Actor{core.Human, core.Agent} {
		c := core.WithActor(ctx, as)
		_, err := f.core.EditTicket(c, "acme", core.EditTicketInput{Ticket: "2", Title: &title})
		wantCode(t, err, core.ActionNotAllowed, "")
		_, err = f.core.EditSpec(c, "acme", core.EditSpecInput{Spec: "1", Body: &title})
		wantCode(t, err, core.ActionNotAllowed, "")
		_, err = f.core.UpdateLinkedTicket(c, "acme", core.UpdateLinkedTicketInput{Ticket: "2", Title: &title})
		wantCode(t, err, core.ActionNotAllowed, "")
		_, err = f.core.SetExternalState(c, "acme", core.SetExternalStateInput{Target: "2", ExternalState: store.ExternalClosed})
		wantCode(t, err, core.ActionNotAllowed, "")
	}
	// Delivery stays Code Central's: Labels, Tags and comments still work.
	if _, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "2", Labels: []string{"ui"}}); err != nil {
		t.Fatalf("add_labels on a linked Ticket: %v", err)
	}
	if got := f.get(t, "2"); got.Title != "Form" || len(got.Edits) != 0 {
		t.Fatalf("definition changed: %+v", got)
	}
}

func TestTheIntegrationsDefinitionUpdatesGoThroughEditHistory(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	other, _ := f.core.CreateLinkedSpec(integration, "acme", core.CreateLinkedSpecInput{Title: "Accounts", Link: ghLink("20")})
	f.clock.Advance(time.Minute)
	title, desc, parent := "Login form", "Build the form.\n", core.Ref(other.ID)
	v, err := f.core.UpdateLinkedTicket(integration, "acme", core.UpdateLinkedTicketInput{Ticket: "2", Title: &title, Description: &desc, Spec: &parent})
	if err != nil {
		t.Fatal(err)
	}
	if v.Title != title || v.Description != desc || v.Spec.ID != "acme-3" {
		t.Fatalf("ticket %+v", v)
	}
	e := v.Edits[0]
	if len(v.Edits) != 1 || e.Actor != "integration" || !e.Time.Equal(t0.Add(time.Minute)) ||
		e.Previous["title"] != "Form" || e.Previous["description"] != "Build it.\n" || e.Previous["spec"] != "acme-1" {
		t.Fatalf("edits %+v", v.Edits)
	}
	body := "New body.\n"
	sp, err := f.core.UpdateLinkedSpec(integration, "acme", core.UpdateLinkedSpecInput{Spec: "1", Body: &body})
	if err != nil || sp.Body != body || len(sp.Edits) != 1 || sp.Edits[0].Actor != "integration" || sp.Edits[0].Previous["body"] != "Users log in.\n" {
		t.Fatalf("spec %+v, %v", sp, err)
	}
	// edit_ticket as the integration is a definition write too.
	again := "Login form v2"
	if v, err := f.core.EditTicket(integration, "acme", core.EditTicketInput{Ticket: "2", Title: &again}); err != nil || v.Title != again {
		t.Fatalf("edit_ticket as integration: %+v, %v", v, err)
	}

	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Local"}) // acme-4, unlinked
	_, err = f.core.UpdateLinkedTicket(integration, "acme", core.UpdateLinkedTicketInput{Ticket: "4", Title: &title})
	wantCode(t, err, core.ActionNotAllowed, "")
	missing := core.Ref("99")
	_, err = f.core.UpdateLinkedTicket(integration, "acme", core.UpdateLinkedTicketInput{Ticket: "2", Spec: &missing})
	wantCode(t, err, core.Invalid, "spec")
}

func TestTheIntegrationCantMoveState(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	_, err := f.core.Approve(integration, "acme", core.TicketInput{Ticket: "2"})
	wantCode(t, err, core.ActionNotAllowed, "")
	_, err = f.core.Cancel(integration, "acme", core.CancelInput{Ticket: "2", Replacements: []core.Ref{"2"}})
	wantCode(t, err, core.ActionNotAllowed, "")
	_, err = f.core.Move(integration, "acme", core.MoveInput{Ticket: "2", ToState: store.Ready})
	wantCode(t, err, core.ActionNotAllowed, "")
	if got := f.get(t, "2"); got.State != store.Draft {
		t.Fatalf("state %s", got.State)
	}
}

func (f fixture) setExternal(t *testing.T, ref core.Ref, s store.ExternalState) core.LinkedRow {
	t.Helper()
	row, err := f.core.SetExternalState(integration, "acme", core.SetExternalStateInput{Target: ref, ExternalState: s})
	if err != nil {
		t.Fatalf("set_external_state %s %s: %v", ref, s, err)
	}
	return row
}

func TestALinkedTicketClosedExternallyNeedsMeUntilReopened(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	f.clock.Advance(time.Hour)
	row := f.setExternal(t, "2", store.ExternalClosed)
	if row.Kind != "ticket" || row.ID != "acme-2" || row.Link.ExternalState != store.ExternalClosed {
		t.Fatalf("row %+v", row)
	}
	wantReason(t, f.get(t, "2"), core.ReasonClosedExternally)
	if e := f.get(t, "2").Edits; len(e) != 1 || e[0].Actor != "integration" || e[0].Previous["external_state"] != "open" {
		t.Fatalf("edits %+v", e)
	}
	nm, _ := f.core.NeedsMe(ctx)
	if nm.Count != 1 || nm.Groups[0].Reason != core.ReasonClosedExternally {
		t.Fatalf("needs me %+v", nm)
	}
	r := nm.Groups[0].Tickets[0]
	if !r.Since.Equal(t0.Add(time.Hour)) || r.Link == nil || r.Link.URL != "https://github.com/acme/web/issues/11" {
		t.Fatalf("needs me row %+v", r)
	}
	for _, s := range []store.ExternalState{store.ExternalDeleted, store.ExternalOutOfScope} {
		f.setExternal(t, "2", s)
		wantReason(t, f.get(t, "2"), core.ReasonClosedExternally)
	}
	f.setExternal(t, "2", store.ExternalOpen)
	wantReason(t, f.get(t, "2"), "")

	_, err := f.core.SetExternalState(integration, "acme", core.SetExternalStateInput{Target: "2", ExternalState: "gone"})
	wantCode(t, err, core.Invalid, "external_state")
}

func TestClosedExternallyIsLastInTheNeedsMeOrder(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	f.setExternal(t, "2", store.ExternalClosed)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Review me"}) // acme-3
	f.setState(t, 3, store.InReview)
	nm, _ := f.core.NeedsMe(ctx)
	if len(nm.Groups) != 2 || nm.Groups[0].Reason != core.ReasonInReview || nm.Groups[1].Reason != core.ReasonClosedExternally {
		t.Fatalf("groups %+v", nm.Groups)
	}
	wf, _ := f.core.GetWorkflow(ctx)
	if last := wf.NeedsMe[len(wf.NeedsMe)-1]; last.Reason != core.ReasonClosedExternally {
		t.Fatalf("published needs_me order ends with %s", last.Reason)
	}
}

func TestAFinishedTicketIsNotClosedExternally(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	f.setExternal(t, "2", store.ExternalClosed)
	f.move(t, "2", store.Done)
	wantReason(t, f.get(t, "2"), "")
}

func TestUnlinkDetachesTheLinkMakingTheDefinitionEditable(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	f.setExternal(t, "2", store.ExternalClosed)

	_, err := f.core.Unlink(ctx, "acme", core.UnlinkInput{Target: "2"})
	wantCode(t, err, core.ActionNotAllowed, "")
	_, err = f.core.Unlink(integration, "acme", core.UnlinkInput{Target: "2"})
	wantCode(t, err, core.ActionNotAllowed, "")

	row, err := f.core.Unlink(human, "acme", core.UnlinkInput{Target: "2"})
	if err != nil || !row.Link.Detached {
		t.Fatalf("unlink: %+v, %v", row, err)
	}
	v := f.get(t, "2")
	wantReason(t, v, "")
	if last := v.Edits[len(v.Edits)-1]; last.Actor != "human" || last.Previous["detached"] != "false" {
		t.Fatalf("edits %+v", v.Edits)
	}
	title := "Mine now"
	if v, err := f.core.EditTicket(ctx, "acme", core.EditTicketInput{Ticket: "2", Title: &title}); err != nil || v.Title != title {
		t.Fatalf("edit after unlink: %+v, %v", v, err)
	}
	_, err = f.core.UpdateLinkedTicket(integration, "acme", core.UpdateLinkedTicketInput{Ticket: "2", Title: &title})
	wantCode(t, err, core.ActionNotAllowed, "")
	// A Spec unlinks the same way.
	if row, err := f.core.Unlink(human, "acme", core.UnlinkInput{Target: "1"}); err != nil || row.Kind != "spec" || !row.Link.Detached {
		t.Fatalf("unlink spec: %+v, %v", row, err)
	}
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Local"}) // acme-3
	_, err = f.core.Unlink(human, "acme", core.UnlinkInput{Target: "3"})
	wantCode(t, err, core.Invalid, "target")
}

func TestLookupsByLinkFindDetachedLinksToo(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	f.core.Unlink(human, "acme", core.UnlinkInput{Target: "2"})
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Local"})

	all, err := f.core.FindLinked(integration, "acme", core.FindLinkedInput{Integration: "github"})
	if err != nil || len(all) != 2 || all[0].ID != "acme-1" || all[0].Kind != "spec" || all[1].ID != "acme-2" || all[1].State != store.Draft || !all[1].Link.Detached {
		t.Fatalf("find_linked: %+v, %v", all, err)
	}
	one, _ := f.core.FindLinked(integration, "acme", core.FindLinkedInput{Integration: "github", ID: "11"})
	if len(one) != 1 || one[0].ID != "acme-2" {
		t.Fatalf("by id: %+v", one)
	}
	none, _ := f.core.FindLinked(integration, "acme", core.FindLinkedInput{Integration: "azure"})
	if none == nil || len(none) != 0 {
		t.Fatalf("other integration: %+v", none)
	}
	// The detached item is still known, so it isn't linked again.
	_, err = f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "1", Title: "Form", Link: ghLink("11")})
	wantCode(t, err, core.Invalid, "link.id")
	_, err = f.core.FindLinked(integration, "acme", core.FindLinkedInput{})
	wantCode(t, err, core.Invalid, "integration")
}

func TestTheLinkOperationsAreHTTPOnly(t *testing.T) {
	for name, aud := range map[string]core.Audience{
		"create_linked_spec":   core.IntegrationOnly,
		"create_linked_ticket": core.IntegrationOnly,
		"update_linked_spec":   core.IntegrationOnly,
		"update_linked_ticket": core.IntegrationOnly,
		"set_external_state":   core.IntegrationOnly,
		"find_linked":          core.IntegrationOnly,
		"unlink":               core.HumanOnly,
	} {
		if op, ok := core.Operation(name); !ok || op.Audience != aud {
			t.Errorf("operation %s: %+v, %v", name, op, ok)
		}
	}
}
