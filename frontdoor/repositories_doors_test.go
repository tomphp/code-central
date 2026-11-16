package frontdoor_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// Repositories (Spec code-central-848): add_repository, edit_repository,
// remove_repository and set_landing_order are the human's, so HTTP only;
// the CLI's repository command calls them through the client, outside the
// operation table. Every refusal says how to put it right.
func init() {
	cases["add_repository"] = opCase{
		ok: call{http: addRepository(`{"name":"web","clone_url":"git@github.com:acme/web.git"}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "repositories")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			repositories(t, out, "acme", "web")
			if web := repositoryIn(out, "web"); web["base"] != "main" || web["code_host"] != "github" || web["home"] != false {
				t.Fatalf("web = %v, want Base main on GitHub", web)
			}
		},
		bad:  call{http: addRepository(`{"name":"Web","clone_url":"git@github.com:acme/web.git"}`)},
		code: core.Invalid,
	}
	cases["edit_repository"] = opCase{
		setup: withRepository(`{"name":"web","clone_url":"git@github.com:acme/web.git"}`),
		ok:    call{http: editRepository(`{"name":"web","base":"develop","local_path":"/src/web"}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "repositories")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			if web := repositoryIn(out, "web"); web["base"] != "develop" || web["local_path"] != "/src/web" {
				t.Fatalf("web = %v, want Base develop at /src/web", web)
			}
		},
		bad:  call{http: editRepository(`{"name":"acme","base":"trunk"}`)},
		code: core.Invalid,
	}
	cases["remove_repository"] = opCase{
		setup: withRepository(`{"name":"web","clone_url":"git@github.com:acme/web.git"}`),
		ok:    call{http: removeRepository(`{"name":"web"}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "repositories")(t, out)
		},
		full: func(t *testing.T, out map[string]any) { repositories(t, out, "acme") },
		bad:  call{http: removeRepository(`{"name":"acme"}`)},
		code: core.Invalid,
	}
	cases["set_landing_order"] = opCase{
		setup: withRepository(`{"name":"web","clone_url":"git@github.com:acme/web.git"}`),
		ok:    call{http: setLandingOrder(`{"names":["web","acme"]}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "repositories")(t, out)
		},
		full: func(t *testing.T, out map[string]any) { repositories(t, out, "web", "acme") },
		bad:  call{http: setLandingOrder(`{"names":["web"]}`)},
		code: core.Invalid,
	}
}

func addRepository(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/add_repository", body}
}
func editRepository(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/edit_repository", body}
}
func removeRepository(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/remove_repository", body}
}
func setLandingOrder(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_landing_order", body}
}

func withRepository(body string) func(*testing.T, *codecentraltest.Server) {
	return func(t *testing.T, srv *codecentraltest.Server) {
		t.Helper()
		httpOK(t, srv, addRepository(body))
	}
}

// repositories checks a Project's Repositories are named want, in order.
func repositories(t *testing.T, project map[string]any, want ...string) {
	t.Helper()
	list, _ := project["repositories"].([]any)
	var got []string
	for _, r := range list {
		got = append(got, r.(map[string]any)["name"].(string))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("repositories = %v, want %v", got, want)
	}
}

// repositoryIn is the Project's Repository named name, or nil.
func repositoryIn(project map[string]any, name string) map[string]any {
	list, _ := project["repositories"].([]any)
	for _, r := range list {
		if m := r.(map[string]any); m["name"] == name {
			return m
		}
	}
	return nil
}

func TestTheRepositoryOperationsAreRejectedForAnAgentAndAnIntegration(t *testing.T) {
	srv := newServer(t)
	for _, c := range []httpCall{
		addRepository(`{"name":"web","clone_url":"git@github.com:acme/web.git"}`),
		editRepository(`{"name":"web","base":"develop"}`),
		removeRepository(`{"name":"web"}`),
		setLandingOrder(`{"names":["acme"]}`),
	} {
		for _, actor := range []core.Actor{core.Agent, core.Integration} {
			status, b := httpDoAs(t, srv, actor, c)
			if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
				t.Fatalf("%s as %s: status %d, want %d; message %q", c.path, actor, status, httpapi.Status(core.ActionNotAllowed), e.Message)
			}
		}
	}
}

func TestAProjectReadListsTheHomeRepositoryAloneUntilAnotherIsAdded(t *testing.T) {
	srv := newServer(t)
	out := httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})
	repositories(t, out, "acme")
	if home := repositoryIn(out, "acme"); home["home"] != true || home["base"] != "main" {
		t.Fatalf("home = %v, want the Home Repository on main", home)
	}
}

func TestARepositoryRefusalCarriesItsFix(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, addRepository(`{"name":"web","clone_url":"git@github.com:acme/web.git"}`))
	status, b := httpDo(t, srv, addRepository(`{"name":"api","clone_url":"https://github.com/acme/web"}`))
	e := errorJSON(t, b, core.Invalid)
	if status != httpapi.Status(core.Invalid) || e.Field != "clone_url" || !strings.Contains(e.Fix, "edit web") {
		t.Fatalf("status %d, error %+v, want clone_url refused with a fix naming web", status, e)
	}
}
