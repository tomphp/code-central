package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/ops"
)

// The Integration contract (ticket 42, ADR 0004).

var integration = api.WithActor(ctx, api.Integration)

func ghLink(id string) api.LinkInput {
	return api.LinkInput{Integration: "github", ID: id, URL: "https://github.com/acme/web/issues/" + id}
}

// linked is Project acme with linked Spec acme-1 and, under it, linked
// Draft Ticket acme-2.
func (f fixture) linked(t *testing.T) (api.SpecView, api.TicketView) {
	t.Helper()
	f.project(t, "acme")
	sp, err := f.core.CreateLinkedSpec(integration, "acme", api.CreateLinkedSpecInput{Title: "Login", Body: "Users log in.\n", Link: ghLink("10"), Full: true})
	if err != nil {
		t.Fatalf("create_linked_spec: %v", err)
	}
	tk, err := record(f.core.CreateLinkedTicket(integration, "acme", api.CreateLinkedTicketInput{Spec: "1", Title: "Form", Description: "Build it.\n", Link: ghLink("11"), Full: true}))
	if err != nil {
		t.Fatalf("create_linked_ticket: %v", err)
	}
	return *sp.Record, tk
}

func wantLink(t *testing.T, got *api.LinkView, want api.LinkView) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("link %+v, want %+v", got, want)
	}
}

func TestAnIntegrationCreatesLinkedSpecsAndTicketsInDraftOrReady(t *testing.T) {
	f := newFixture(t)
	sp, tk := f.linked(t)
	wantLink(t, sp.Link, api.LinkView{Integration: "github", ID: "10", URL: "https://github.com/acme/web/issues/10", ExternalState: domain.ExternalOpen})
	if tk.State != domain.Draft || tk.Spec.ID != "acme-1" || tk.Description != "Build it.\n" {
		t.Fatalf("ticket %+v", tk)
	}
	wantLink(t, tk.Link, api.LinkView{Integration: "github", ID: "11", URL: "https://github.com/acme/web/issues/11", ExternalState: domain.ExternalOpen})
	if got := f.get(t, "2"); got.Link == nil || got.Link.ID != "11" {
		t.Fatalf("get_ticket link %+v", got.Link)
	}
	ready, err := record(f.core.CreateLinkedTicket(integration, "acme", api.CreateLinkedTicketInput{Spec: "1", Title: "API", State: domain.Ready, Link: ghLink("12"), Full: true}))
	if err != nil || ready.State != domain.Ready {
		t.Fatalf("ready: %+v, %v", ready, err)
	}
	rows, _ := f.core.FindTickets(ctx, "acme", api.FindTicketsInput{})
	if rows[0].Link == nil || rows[0].Link.ID != "11" {
		t.Fatalf("find_tickets row link %+v", rows[0].Link)
	}

	_, err = f.core.CreateLinkedTicket(integration, "acme", api.CreateLinkedTicketInput{Spec: "1", Title: "X", State: domain.InProgress, Link: ghLink("13")})
	wantCode(t, err, api.Invalid, "state")
	_, err = f.core.CreateLinkedTicket(integration, "acme", api.CreateLinkedTicketInput{Spec: "1", Title: "X", Link: api.LinkInput{Integration: "github"}})
	wantCode(t, err, api.Invalid, "link.id")
	_, err = f.core.CreateLinkedTicket(integration, "acme", api.CreateLinkedTicketInput{Spec: "1", Title: "X", Link: api.LinkInput{Integration: "github", ID: "14", URL: "javascript:alert(1)"}})
	wantCode(t, err, api.Invalid, "link.url")
}

func TestAnItemIsLinkedToAnExternalItemAtMostOnce(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	_, err := f.core.CreateLinkedTicket(integration, "acme", api.CreateLinkedTicketInput{Spec: "1", Title: "Again", Link: ghLink("11")})
	wantCode(t, err, api.Invalid, "link.id")
	_, err = f.core.CreateLinkedSpec(integration, "acme", api.CreateLinkedSpecInput{Title: "Again", Link: ghLink("10")})
	wantCode(t, err, api.Invalid, "link.id")
	// An external item with no parent is both a Spec and its one Ticket.
	if _, err := f.core.CreateLinkedTicket(integration, "acme", api.CreateLinkedTicketInput{Spec: "1", Title: "Login", Link: ghLink("10")}); err != nil {
		t.Fatalf("a Ticket linked to the Spec's own item: %v", err)
	}
}

func TestOnlyAnIntegrationCreatesLinkedItems(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	for _, as := range []api.Actor{api.Human, api.Agent} {
		c := api.WithActor(ctx, as)
		_, err := f.core.CreateLinkedSpec(c, "acme", api.CreateLinkedSpecInput{Title: "S", Link: ghLink("1")})
		wantCode(t, err, api.ActionNotAllowed, "")
		_, err = f.core.CreateLinkedTicket(c, "acme", api.CreateLinkedTicketInput{Spec: "1", Title: "T", Link: ghLink("2")})
		wantCode(t, err, api.ActionNotAllowed, "")
	}
}

func TestOnlyTheIntegrationWritesALinkedDefinition(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	title := "Renamed"
	for _, as := range []api.Actor{api.Human, api.Agent} {
		c := api.WithActor(ctx, as)
		_, err := f.core.EditTicket(c, "acme", api.EditTicketInput{Ticket: "2", Title: &title})
		wantCode(t, err, api.ActionNotAllowed, "")
		_, err = f.core.EditSpec(c, "acme", api.EditSpecInput{Spec: "1", Body: &title})
		wantCode(t, err, api.ActionNotAllowed, "")
		_, err = f.core.UpdateLinkedTicket(c, "acme", api.UpdateLinkedTicketInput{Ticket: "2", Title: &title})
		wantCode(t, err, api.ActionNotAllowed, "")
		_, err = f.core.SetExternalState(c, "acme", api.SetExternalStateInput{Target: "2", ExternalState: domain.ExternalClosed})
		wantCode(t, err, api.ActionNotAllowed, "")
	}
	// Delivery stays Code Central's: Labels, Tags and comments still work.
	if _, err := f.core.AddLabels(ctx, "acme", api.LabelsInput{Target: "2", Labels: []string{"ui"}}); err != nil {
		t.Fatalf("add_labels on a linked Ticket: %v", err)
	}
	if got := f.get(t, "2"); got.Title != "Form" || len(got.Edits) != 0 {
		t.Fatalf("definition changed: %+v", got)
	}
}

func TestTheIntegrationsDefinitionUpdatesGoThroughEditHistory(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	other, _ := f.core.CreateLinkedSpec(integration, "acme", api.CreateLinkedSpecInput{Title: "Accounts", Link: ghLink("20")})
	f.clock.Advance(time.Minute)
	title, desc, parent := "Login form", "Build the form.\n", api.Ref(other.ID)
	v, err := record(f.core.UpdateLinkedTicket(integration, "acme", api.UpdateLinkedTicketInput{Ticket: "2", Title: &title, Description: &desc, Spec: &parent, Full: true}))
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
	sp, err := f.core.UpdateLinkedSpec(integration, "acme", api.UpdateLinkedSpecInput{Spec: "1", Body: &body, Full: true})
	if err != nil || sp.Record.Body != body || len(sp.Record.Edits) != 1 || sp.Record.Edits[0].Actor != "integration" || sp.Record.Edits[0].Previous["body"] != "Users log in.\n" {
		t.Fatalf("spec %+v, %v", sp, err)
	}
	// edit_ticket as the integration is a definition write too.
	again := "Login form v2"
	if v, err := record(f.core.EditTicket(integration, "acme", api.EditTicketInput{Ticket: "2", Title: &again, Full: true})); err != nil || v.Title != again {
		t.Fatalf("edit_ticket as integration: %+v, %v", v, err)
	}

	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Local"}) // acme-4, unlinked
	_, err = f.core.UpdateLinkedTicket(integration, "acme", api.UpdateLinkedTicketInput{Ticket: "4", Title: &title})
	wantCode(t, err, api.ActionNotAllowed, "")
	missing := api.Ref("99")
	_, err = f.core.UpdateLinkedTicket(integration, "acme", api.UpdateLinkedTicketInput{Ticket: "2", Spec: &missing})
	wantCode(t, err, api.Invalid, "spec")
}

// An external item that moved, such as a work item to another Azure
// DevOps project, keeps its Link ID and has its Link's URL refreshed,
// which isn't a definition edit.
func TestTheIntegrationRefreshesALinksURL(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	moved := "https://example.com/acme/api/issues/2"
	v, err := record(f.core.UpdateLinkedTicket(integration, "acme", api.UpdateLinkedTicketInput{Ticket: "2", URL: &moved, Full: true}))
	if err != nil || v.Link.URL != moved || len(v.Edits) != 0 {
		t.Fatalf("ticket %+v, %v; want the Link's URL refreshed with no edit", v, err)
	}
	bad := "not a url"
	_, err = f.core.UpdateLinkedTicket(integration, "acme", api.UpdateLinkedTicketInput{Ticket: "2", URL: &bad})
	wantCode(t, err, api.Invalid, "url")
}

func TestTheIntegrationCantMoveState(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	_, err := f.core.Approve(integration, "acme", api.TicketInput{Ticket: "2"})
	wantCode(t, err, api.ActionNotAllowed, "")
	_, err = f.core.Cancel(integration, "acme", api.CancelInput{Ticket: "2", Replacements: []api.Ref{"2"}})
	wantCode(t, err, api.ActionNotAllowed, "")
	_, err = f.core.Move(integration, "acme", api.MoveInput{Ticket: "2", ToState: domain.Ready})
	wantCode(t, err, api.ActionNotAllowed, "")
	if got := f.get(t, "2"); got.State != domain.Draft {
		t.Fatalf("state %s", got.State)
	}
}

func (f fixture) setExternal(t *testing.T, ref api.Ref, s domain.ExternalState) api.LinkedRow {
	t.Helper()
	row, err := f.core.SetExternalState(integration, "acme", api.SetExternalStateInput{Target: ref, ExternalState: s})
	if err != nil {
		t.Fatalf("set_external_state %s %s: %v", ref, s, err)
	}
	return row
}

func TestALinkedTicketClosedExternallyNeedsMeUntilReopened(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	f.clock.Advance(time.Hour)
	row := f.setExternal(t, "2", domain.ExternalClosed)
	if row.Kind != "ticket" || row.ID != "acme-2" || row.Link.ExternalState != domain.ExternalClosed {
		t.Fatalf("row %+v", row)
	}
	wantReason(t, f.get(t, "2"), api.ReasonClosedExternally)
	if e := f.get(t, "2").Edits; len(e) != 1 || e[0].Actor != "integration" || e[0].Previous["external_state"] != "open" {
		t.Fatalf("edits %+v", e)
	}
	nm, _ := f.core.NeedsMe(ctx)
	if nm.Count != 1 || nm.Groups[0].Reason != api.ReasonClosedExternally {
		t.Fatalf("needs me %+v", nm)
	}
	r := nm.Groups[0].Tickets[0]
	if !r.Since.Equal(t0.Add(time.Hour)) || r.Link == nil || r.Link.URL != "https://github.com/acme/web/issues/11" {
		t.Fatalf("needs me row %+v", r)
	}
	for _, s := range []domain.ExternalState{domain.ExternalDeleted, domain.ExternalOutOfScope} {
		f.setExternal(t, "2", s)
		wantReason(t, f.get(t, "2"), api.ReasonClosedExternally)
	}
	f.setExternal(t, "2", domain.ExternalOpen)
	wantReason(t, f.get(t, "2"), "")

	_, err := f.core.SetExternalState(integration, "acme", api.SetExternalStateInput{Target: "2", ExternalState: "gone"})
	wantCode(t, err, api.Invalid, "external_state")
}

func TestClosedExternallyIsLastInTheNeedsMeOrder(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	f.setExternal(t, "2", domain.ExternalClosed)
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Review me"}) // acme-3
	f.setState(t, 3, domain.InReview)
	nm, _ := f.core.NeedsMe(ctx)
	if len(nm.Groups) != 2 || nm.Groups[0].Reason != api.ReasonInReview || nm.Groups[1].Reason != api.ReasonClosedExternally {
		t.Fatalf("groups %+v", nm.Groups)
	}
	wf, _ := f.core.GetWorkflow(ctx)
	if last := wf.NeedsMe[len(wf.NeedsMe)-1]; last.Reason != api.ReasonClosedExternally {
		t.Fatalf("published needs_me order ends with %s", last.Reason)
	}
}

func TestAFinishedTicketIsNotClosedExternally(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	f.setExternal(t, "2", domain.ExternalClosed)
	f.move(t, "2", domain.Done)
	wantReason(t, f.get(t, "2"), "")
}

func TestUnlinkDetachesTheLinkMakingTheDefinitionEditable(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	f.setExternal(t, "2", domain.ExternalClosed)

	_, err := f.core.Unlink(ctx, "acme", api.UnlinkInput{Target: "2"})
	wantCode(t, err, api.ActionNotAllowed, "")
	_, err = f.core.Unlink(integration, "acme", api.UnlinkInput{Target: "2"})
	wantCode(t, err, api.ActionNotAllowed, "")

	row, err := f.core.Unlink(human, "acme", api.UnlinkInput{Target: "2"})
	if err != nil || !row.Link.Detached {
		t.Fatalf("unlink: %+v, %v", row, err)
	}
	v := f.get(t, "2")
	wantReason(t, v, "")
	if last := v.Edits[len(v.Edits)-1]; last.Actor != "human" || last.Previous["detached"] != "false" {
		t.Fatalf("edits %+v", v.Edits)
	}
	title := "Mine now"
	if v, err := record(f.core.EditTicket(ctx, "acme", api.EditTicketInput{Ticket: "2", Title: &title, Full: true})); err != nil || v.Title != title {
		t.Fatalf("edit after unlink: %+v, %v", v, err)
	}
	_, err = f.core.UpdateLinkedTicket(integration, "acme", api.UpdateLinkedTicketInput{Ticket: "2", Title: &title})
	wantCode(t, err, api.ActionNotAllowed, "")
	// A Spec unlinks the same way.
	if row, err := f.core.Unlink(human, "acme", api.UnlinkInput{Target: "1"}); err != nil || row.Kind != "spec" || !row.Link.Detached {
		t.Fatalf("unlink spec: %+v, %v", row, err)
	}
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Local"}) // acme-3
	_, err = f.core.Unlink(human, "acme", api.UnlinkInput{Target: "3"})
	wantCode(t, err, api.Invalid, "target")
}

func TestLookupsByLinkFindDetachedLinksToo(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	f.core.Unlink(human, "acme", api.UnlinkInput{Target: "2"})
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Local"})

	all, err := f.core.FindLinked(integration, "acme", api.FindLinkedInput{Integration: "github"})
	if err != nil || len(all) != 2 || all[0].ID != "acme-1" || all[0].Kind != "spec" || all[1].ID != "acme-2" || all[1].State != domain.Draft || !all[1].Link.Detached {
		t.Fatalf("find_linked: %+v, %v", all, err)
	}
	one, _ := f.core.FindLinked(integration, "acme", api.FindLinkedInput{Integration: "github", ID: "11"})
	if len(one) != 1 || one[0].ID != "acme-2" {
		t.Fatalf("by id: %+v", one)
	}
	none, _ := f.core.FindLinked(integration, "acme", api.FindLinkedInput{Integration: "azure"})
	if none == nil || len(none) != 0 {
		t.Fatalf("other integration: %+v", none)
	}
	// The detached item is still known, so it isn't linked again.
	_, err = f.core.CreateLinkedTicket(integration, "acme", api.CreateLinkedTicketInput{Spec: "1", Title: "Form", Link: ghLink("11")})
	wantCode(t, err, api.Invalid, "link.id")
	_, err = f.core.FindLinked(integration, "acme", api.FindLinkedInput{})
	wantCode(t, err, api.Invalid, "integration")
}

func TestTheLinkOperationsAreHTTPOnly(t *testing.T) {
	for name, aud := range map[string]ops.Audience{
		"create_linked_spec":   ops.IntegrationOnly,
		"create_linked_ticket": ops.IntegrationOnly,
		"update_linked_spec":   ops.IntegrationOnly,
		"update_linked_ticket": ops.IntegrationOnly,
		"set_external_state":   ops.IntegrationOnly,
		"find_linked":          ops.IntegrationOnly,
		"unlink":               ops.HumanOnly,
	} {
		if op, ok := ops.Lookup(name); !ok || op.Audience != aud {
			t.Errorf("operation %s: %+v, %v", name, op, ok)
		}
	}
}
