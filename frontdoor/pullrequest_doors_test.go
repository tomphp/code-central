package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// The Pull Request Watcher's own (code-central-388): get_pull_request_watch
// and report_pull_request, over HTTP only.
func init() {
	cases["get_pull_request_watch"] = opCase{
		setup: pushedAcceptedForPullRequest,
		ok:    call{http: httpCall{"GET", "/api/p/acme/pull-request-watch", ""}},
		check: func(t *testing.T, out map[string]any) {
			field("code_host", "github")(t, out)
			field("repository", "acme/web")(t, out)
			field("base", "main")(t, out)
			work, _ := out["work"].([]any)
			if len(work) != 1 || work[0].(map[string]any)["branch"] != "acme-3" {
				t.Fatalf("work = %v, want acme-3's branch", out["work"])
			}
		},
		bad:  call{http: httpCall{"GET", "/api/p/nope/pull-request-watch", ""}},
		code: core.UnknownProject,
	}
	cases["report_pull_request"] = opCase{
		setup: pushedAcceptedForPullRequest,
		ok:    call{http: reportPullRequest(`{"branch":"acme-3","number":7,"link":"https://github.com/acme/web/pull/7","state":"open"}`)},
		check: acknowledgement("acme", "pull_request"),
		full: func(t *testing.T, out map[string]any) {
			field("branch", "acme-3")(t, out)
			tickets, _ := out["tickets"].([]any)
			if len(tickets) != 1 || tickets[0] != "acme-3" {
				t.Fatalf("tickets = %v, want acme-3", out["tickets"])
			}
		},
		bad:  call{http: reportPullRequest(`{"branch":"acme-3","number":7,"link":"https://github.com/acme/web/pull/7","state":"merged"}`)},
		code: core.Invalid,
	}
}

func reportPullRequest(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/report_pull_request", body}
}

// pushedAcceptedForPullRequest turns Pull Request on for acme, with the
// Push remote and base the Factory reports, and makes acme-3 Accepted and
// pushed as its branch.
func pushedAcceptedForPullRequest(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Client.ReportPushRemote(context.Background(), "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/web.git", Base: "main"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.SetLandingMethod(core.WithActor(context.Background(), core.Human), "acme",
			core.SetLandingMethodInput{Method: store.LandingPullRequest, Remote: "origin"})
		return err
	})
	acceptedTicket(t, srv)
	must(t, func() error {
		_, err := srv.Client.ReportPushedCommit(context.Background(), "acme", core.ReportPushedCommitInput{Ticket: "acme-3", Commit: "abc123", Branch: "acme-3"})
		return err
	})
}
