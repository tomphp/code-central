package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// specStageClaimed submits Requirement acme-4, passes its grilling Stage,
// claims its spec Stage, acme-6, as claim-1 and acme-3 as claim-2.
func specStageClaimed(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	submittedRequirement(t, srv)
	must(t, func() error {
		_, err := srv.Core.Move(core.WithActor(context.Background(), core.Human), "acme", core.MoveInput{Ticket: "5", ToState: store.Done})
		return err
	})
	for _, ref := range []core.Ref{"acme-6", "acme-3"} {
		must(t, func() error {
			_, err := srv.Core.Claim(context.Background(), "acme", core.ClaimInput{Ticket: ref})
			return err
		})
	}
}

// finish takes the Spec recommendation on every door: required on a spec
// Stage, refused on any other Ticket, and stored on the spec Stage.
func TestFinishTakesTheRecommendationOnEveryDoor(t *testing.T) {
	type finishes struct {
		missing, elsewhere core.Error
		ok                 map[string]any
	}
	for door, finish := range map[string]func(t *testing.T, srv *codecentraltest.Server) finishes{
		"mcp": func(t *testing.T, srv *codecentraltest.Server) finishes {
			s := connect(t, srv, "acme")
			return finishes{
				missing:   mcpRejected(t, s, "finish", map[string]any{"ticket": "acme-6", "claim_id": "claim-1", "summary": "Small."}, core.Invalid),
				elsewhere: mcpRejected(t, s, "finish", map[string]any{"ticket": "acme-3", "claim_id": "claim-2", "summary": "Done.", "recommendation": "split"}, core.Invalid),
				ok:        mcpOK(t, s, "finish", map[string]any{"ticket": "acme-6", "claim_id": "claim-1", "summary": "Small.", "recommendation": "implement", "full": true}),
			}
		},
		"http": func(t *testing.T, srv *codecentraltest.Server) finishes {
			return finishes{
				missing:   httpRejected(t, srv, httpCall{"POST", "/api/p/acme/tickets/6/finish", `{"claim_id":"claim-1","summary":"Small."}`}, core.Invalid),
				elsewhere: httpRejected(t, srv, httpCall{"POST", "/api/p/acme/tickets/3/finish", `{"claim_id":"claim-2","summary":"Done.","recommendation":"split"}`}, core.Invalid),
				ok:        httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/6/finish", `{"claim_id":"claim-1","summary":"Small.","recommendation":"implement","full":true}`}),
			}
		},
		"cli": func(t *testing.T, srv *codecentraltest.Server) finishes {
			return finishes{
				missing:   cliRejected(t, srv, []string{"finish", "acme-6", "--claim", "claim-1", "--summary", "Small."}, core.Invalid),
				elsewhere: cliRejected(t, srv, []string{"finish", "acme-3", "--claim", "claim-2", "--summary", "Done.", "--recommendation", "split"}, core.Invalid),
				ok:        cliOK(t, srv, []string{"finish", "acme-6", "--claim", "claim-1", "--summary", "Small.", "--recommendation", "implement", "--full"}),
			}
		},
	} {
		t.Run(door, func(t *testing.T) {
			srv := newServer(t, specStageClaimed)
			got := finish(t, srv)
			for name, e := range map[string]core.Error{"missing": got.missing, "elsewhere": got.elsewhere} {
				if e.Field != "recommendation" {
					t.Errorf("%s: field %q (%s), want recommendation", name, e.Field, e.Message)
				}
			}
			tk, _ := got.ok["ticket"].(map[string]any)
			field("state", "In Review")(t, tk)
			field("recommendation", "implement")(t, tk)
		})
	}
}
