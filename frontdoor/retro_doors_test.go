package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Starting a Retro (code-central-667). acme-2 is Done, so the Retros Spec
// is acme-4 and the Retro acme-5; Ready acme-3 is unfinished.
func init() {
	cases["submit_retro"] = opCase{
		setup: func(t *testing.T, srv *codecentraltest.Server) {
			_, err := srv.Core.Move(core.WithActor(context.Background(), core.Human), "acme", core.MoveInput{Ticket: "2", ToState: store.Done})
			if err != nil {
				t.Fatal(err)
			}
		},
		ok: call{
			mcp:  map[string]any{"target": "acme-2", "note": "Why so slow?"},
			http: httpCall{"POST", "/api/p/acme/submit_retro", `{"target":"acme-2","note":"Why so slow?"}`},
			cli:  []string{"acme-2", "--note", "Why so slow?"},
		},
		check: ticketAcknowledgement("acme-5", "Ready", "title", "state", "spec", "labels", "tags", "description", "moves", "claimable"),
		full: func(t *testing.T, out map[string]any) {
			field("id", "acme-5")(t, out)
			field("title", "Retro: Login form")(t, out)
			field("state", "Ready")(t, out)
			sp, _ := out["spec"].(map[string]any)
			field("id", "acme-4")(t, sp)
			field("title", "Retros")(t, sp)
			tags, _ := out["tags"].(map[string]any)
			field("retro-of", "acme-2")(t, tags)
			field("description", "Retro on Ticket [acme-2](acme-2): Login form.\n\n## Note\n\nWhy so slow?\n\n"+
				"## Claims to read\n\nNone: no agent ever claimed this work, so there is no session to read.\n")(t, out)
			labels("retro")(t, out)
		},
		bad: call{
			mcp:  map[string]any{"target": "acme-3"},
			http: httpCall{"POST", "/api/p/acme/submit_retro", `{"target":"acme-3"}`},
			cli:  []string{"acme-3"},
		},
		code: core.Invalid,
	}
}
