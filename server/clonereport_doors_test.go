package server_test

import (
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/codecentraltest"
)

// The Factory's clones of the Repositories (Spec code-central-848):
// report_repositories, the Factory's, so over HTTP only. A Factory is
// online, so the report stands and the full Project carries each clone.
func init() {
	cases["report_repositories"] = opCase{
		setup: func(t *testing.T, srv *codecentraltest.Server) {
			t.Helper()
			httpOK(t, srv, httpCall{"POST", "/api/p/acme/report_heartbeat", `{"factory":"cc","host":"box","state":"online","interval_seconds":10}`})
		},
		ok:    call{http: reportRepositories(`{"repositories":[{"name":"acme","path":"/src/acme","state":"ready"}]}`)},
		check: acknowledgement("acme", "repositories"),
		full: func(t *testing.T, out map[string]any) {
			clone, _ := repositoryIn(out, "acme")["clone"].(map[string]any)
			if clone["state"] != "ready" || clone["path"] != "/src/acme" {
				t.Fatalf("acme's clone = %v, want ready at /src/acme", clone)
			}
		},
		bad:  call{http: reportRepositories(`{"repositories":[{"name":"acme","path":"/src/acme","state":"missing"}]}`)},
		code: api.Invalid,
	}
}

func reportRepositories(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/report_repositories", body}
}
