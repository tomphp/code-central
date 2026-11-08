package client_test

import (
	"testing"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// An Integration's loop over the client: pull items in, keep them up to
// date, record the external state, and find what it already knows.
func TestAnIntegrationRoundTrip(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	if _, err := srv.Client.CreateProject(ctx, core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	gh := client.New(srv.URL)
	gh.Actor = core.Integration

	sp, err := gh.CreateLinkedSpec(ctx, "acme", core.CreateLinkedSpecInput{Title: "Epic", Link: core.LinkInput{Integration: "github", ID: "1"}, Full: true})
	if err != nil || sp.Record.Link == nil {
		t.Fatalf("create_linked_spec: %+v, %v", sp, err)
	}
	tk, err := gh.CreateLinkedTicket(ctx, "acme", core.CreateLinkedTicketInput{Spec: core.Ref(sp.Record.ID), Title: "Issue", Link: core.LinkInput{Integration: "github", ID: "2"}})
	if err != nil || tk.State != store.Draft {
		t.Fatalf("create_linked_ticket: %+v, %v", tk, err)
	}
	title := "Epic, renamed"
	if s, err := gh.UpdateLinkedSpec(ctx, "acme", core.UpdateLinkedSpecInput{Spec: core.Ref(sp.Record.ID), Title: &title}); err != nil || s.Edit != 1 || s.Record != nil {
		t.Fatalf("update_linked_spec: %+v, %v", s, err)
	}
	if v, err := gh.UpdateLinkedTicket(ctx, "acme", core.UpdateLinkedTicketInput{Ticket: core.Ref(tk.ID), Title: &title}); err != nil || v.Edits[0].Actor != "integration" {
		t.Fatalf("update_linked_ticket: %+v, %v", v, err)
	}
	if row, err := gh.SetExternalState(ctx, "acme", core.SetExternalStateInput{Target: core.Ref(tk.ID), ExternalState: store.ExternalDeleted}); err != nil || row.Link.ExternalState != store.ExternalDeleted {
		t.Fatalf("set_external_state: %+v, %v", row, err)
	}
	human := client.New(srv.URL)
	human.Actor = core.Human
	if row, err := human.Unlink(ctx, "acme", core.UnlinkInput{Target: core.Ref(tk.ID)}); err != nil || !row.Link.Detached {
		t.Fatalf("unlink: %+v, %v", row, err)
	}
	rows, err := gh.FindLinked(ctx, "acme", core.FindLinkedInput{Integration: "github", ID: "2"})
	if err != nil || len(rows) != 1 || rows[0].ID != tk.ID || !rows[0].Link.Detached {
		t.Fatalf("find_linked: %+v, %v", rows, err)
	}
	_, err = srv.Client.UpdateLinkedSpec(ctx, "acme", core.UpdateLinkedSpecInput{Spec: core.Ref(sp.Record.ID), Title: &title})
	wantCode(t, err, core.ActionNotAllowed, "")
}
