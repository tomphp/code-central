package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// Finished work and Archived Projects (ticket 39): the human's, so HTTP
// only. Project other has no Tickets, so it can be archived; acme can't.
func init() {
	cases["archive_project"] = opCase{
		ok: call{http: httpCall{"POST", "/api/p/other/archive_project", `{}`}},
		check: func(t *testing.T, out map[string]any) {
			field("key", "other")(t, out)
			if out["archived"] != true || out["archived_at"] == nil {
				t.Fatalf("archive_project: %v", out)
			}
		},
		bad:  call{http: httpCall{"POST", "/api/p/acme/archive_project", `{}`}},
		code: core.ActionNotAllowed,
	}
	cases["unarchive_project"] = opCase{
		setup: otherArchived,
		ok:    call{http: httpCall{"POST", "/api/p/other/unarchive_project", ``}},
		check: func(t *testing.T, out map[string]any) {
			if out["archived"] != false || out["archived_at"] != nil {
				t.Fatalf("unarchive_project: %v", out)
			}
		},
		bad:  call{http: httpCall{"POST", "/api/p/nope/unarchive_project", `{}`}},
		code: core.UnknownProject,
	}
}

func otherArchived(t *testing.T, srv *codecentraltest.Server) {
	human := core.WithActor(context.Background(), core.Human)
	must(t, func() error { _, err := srv.Core.ArchiveProject(human, "other"); return err })
}

// The refusal reaches an agent over MCP and the CLI in the one error
// shape, listing unarchive_project; reads by URL keep working.
func TestAnArchivedProjectsRefusalThroughEveryDoor(t *testing.T) {
	srv := newServer(t, func(t *testing.T, srv *codecentraltest.Server) {
		human := core.WithActor(context.Background(), core.Human)
		for _, n := range []core.Ref{"2", "3"} {
			must(t, func() error {
				_, err := srv.Core.Move(human, "acme", core.MoveInput{Ticket: n, ToState: "Done"})
				return err
			})
		}
		must(t, func() error { _, err := srv.Core.ArchiveProject(human, "acme"); return err })
	})
	want := func(t *testing.T, e core.Error) {
		t.Helper()
		if len(e.AllowedActions) != 1 || e.AllowedActions[0] != "unarchive_project" {
			t.Fatalf("refusal %+v, want allowed_actions [unarchive_project]", e)
		}
	}
	want(t, mcpRejected(t, connect(t, srv, "acme"), "create_spec", map[string]any{"title": "More"}, core.ActionNotAllowed))
	want(t, cliRejected(t, srv, []string{"create-spec", "--title", "More"}, core.ActionNotAllowed))
	httpRejected(t, srv, httpCall{"POST", "/api/p/acme/create_ticket", `{"spec":1,"title":"More"}`}, core.ActionNotAllowed)
	field("state", "Done")(t, httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/2", ""}))
}
