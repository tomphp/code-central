package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

const implementNextStep = "The spec Stage is Done and the tickets Stage is Cancelled, and acme-8 now builds the whole Spec."

// Implement, the human's other way to accept a spec Stage. It has an HTTP
// route only: no MCP tool and no CLI command.
func init() {
	cases["implement"] = opCase{
		setup: specStageInReview,
		ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/6/implement", `{}`}},
		check: func(t *testing.T, out map[string]any) {
			ticketAcknowledgement("acme-6", "Done", "state", "moves", "blocked", "needs_me_reason")(t, out)
			field("next_step", implementNextStep)(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			field("next_step", implementNextStep)(t, out)
			tk, _ := out["ticket"].(map[string]any)
			field("id", "acme-6")(t, tk)
			field("state", "Done")(t, tk)
		},
		bad:  call{http: httpCall{"POST", "/api/p/acme/tickets/5/implement", `{}`}},
		code: core.ActionNotAllowed,
	}
}

// specStageInReview submits Requirement acme-4 and moves its spec Stage,
// acme-6, to In Review.
func specStageInReview(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	submittedRequirement(t, srv)
	must(t, func() error {
		_, err := srv.Core.Move(core.WithActor(context.Background(), core.Human), "acme", core.MoveInput{Ticket: "6", ToState: store.InReview})
		return err
	})
}

// A linked Requirement after Implement is building, and its Spec comment
// is due, as after Split into Tickets.
func TestAnIssueWantsBuildingAndTheSpecCommentAfterImplement(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	httpOK(t, srv, reportIssueWritten(`{"target":"acme-6","key":"context"}`))
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/7/move", `{"to_state":"Done"}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/8/move", `{"to_state":"In Review"}`})
	wantDue(t, writesDue(t, srv), "acme-6", "code-central:requirement", "")
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/8/implement", `{}`})
	wantDue(t, writesDue(t, srv), "acme-6", "code-central:building", "", "spec")
}
