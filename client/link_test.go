package client_test

import (
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
)

// An Integration's loop over the client: pull items in, keep them up to
// date, record the external state, and find what it already knows.
func TestAnIntegrationRoundTrip(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	if _, err := srv.Client.CreateProject(ctx, api.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	gh := client.New(srv.URL)
	gh.Actor = api.Integration

	sp, err := gh.CreateLinkedSpec(ctx, "acme", api.CreateLinkedSpecInput{Title: "Epic", Link: api.LinkInput{Integration: "github", ID: "1"}, Full: true})
	if err != nil || sp.Record.Link == nil {
		t.Fatalf("create_linked_spec: %+v, %v", sp, err)
	}
	tk, err := gh.CreateLinkedTicket(ctx, "acme", api.CreateLinkedTicketInput{Spec: api.Ref(sp.Record.ID), Title: "Issue", Link: api.LinkInput{Integration: "github", ID: "2"}})
	if err != nil || tk.State != domain.Draft {
		t.Fatalf("create_linked_ticket: %+v, %v", tk, err)
	}
	title := "Epic, renamed"
	if s, err := gh.UpdateLinkedSpec(ctx, "acme", api.UpdateLinkedSpecInput{Spec: api.Ref(sp.Record.ID), Title: &title}); err != nil || s.Edit != 1 || s.Record != nil {
		t.Fatalf("update_linked_spec: %+v, %v", s, err)
	}
	if v, err := gh.UpdateLinkedTicket(ctx, "acme", api.UpdateLinkedTicketInput{Ticket: api.Ref(tk.ID), Title: &title, Full: true}); err != nil || v.Record.Edits[0].Actor != "integration" {
		t.Fatalf("update_linked_ticket: %+v, %v", v, err)
	}
	if row, err := gh.SetExternalState(ctx, "acme", api.SetExternalStateInput{Target: api.Ref(tk.ID), ExternalState: domain.ExternalDeleted}); err != nil || row.Link.ExternalState != domain.ExternalDeleted {
		t.Fatalf("set_external_state: %+v, %v", row, err)
	}
	human := client.New(srv.URL)
	human.Actor = api.Human
	if row, err := human.Unlink(ctx, "acme", api.UnlinkInput{Target: api.Ref(tk.ID)}); err != nil || !row.Link.Detached {
		t.Fatalf("unlink: %+v, %v", row, err)
	}
	rows, err := gh.FindLinked(ctx, "acme", api.FindLinkedInput{Integration: "github", ID: "2"})
	if err != nil || len(rows) != 1 || rows[0].ID != tk.ID || !rows[0].Link.Detached {
		t.Fatalf("find_linked: %+v, %v", rows, err)
	}
	_, err = srv.Client.UpdateLinkedSpec(ctx, "acme", api.UpdateLinkedSpecInput{Spec: api.Ref(sp.Record.ID), Title: &title})
	wantCode(t, err, api.ActionNotAllowed, "")
}
