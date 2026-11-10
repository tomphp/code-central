package frontdoor_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// CI Rules (code-central-254): add_ci_rule, edit_ci_rule and
// remove_ci_rule are the human's, so HTTP only. Each rule opens its
// Tickets on a long-lived Spec of its own, named by that Spec's ID.
func init() {
	cases["add_ci_rule"] = opCase{
		ok: call{http: addCIRule(`{"workflow":" ci.yml ","branch":" main ","state":"Ready"}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "ci")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			ciRules(t, out, rule{"acme-4", "ci.yml", "main", "Ready"})
		},
		bad:  call{http: addCIRule(`{"workflow":"ci.yml","branch":"release/*"}`)},
		code: core.Invalid,
	}
	cases["edit_ci_rule"] = opCase{
		setup: withCIRule(`{"workflow":"ci.yml","branch":"main"}`),
		ok:    call{http: editCIRule(`{"spec":"acme-4","workflow":"any","branch":"main","state":"Ready"}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "ci")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			ciRules(t, out, rule{"acme-4", "any", "main", "Ready"})
		},
		bad:  call{http: editCIRule(`{"spec":"acme-1","workflow":"any","branch":"main"}`)},
		code: core.NotFound,
	}
	cases["remove_ci_rule"] = opCase{
		setup: withCIRule(`{"workflow":"ci.yml","branch":"main"}`),
		ok:    call{http: removeCIRule(`{"spec":"acme-4"}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "ci")(t, out)
		},
		full: func(t *testing.T, out map[string]any) { ciRules(t, out) },
		bad:  call{http: removeCIRule(`{"spec":"acme-9"}`)},
		code: core.NotFound,
	}
}

func addCIRule(body string) httpCall    { return httpCall{"POST", "/api/p/acme/add_ci_rule", body} }
func editCIRule(body string) httpCall   { return httpCall{"POST", "/api/p/acme/edit_ci_rule", body} }
func removeCIRule(body string) httpCall { return httpCall{"POST", "/api/p/acme/remove_ci_rule", body} }

func withCIRule(body string) func(*testing.T, *codecentraltest.Server) {
	return func(t *testing.T, srv *codecentraltest.Server) {
		t.Helper()
		httpOK(t, srv, addCIRule(body))
	}
}

// rule is a CI Rule as a Project read returns it, less its created time.
type rule struct{ spec, workflow, branch, state string }

// ciRules checks a Project's CI Rules are exactly want, in order, each
// with a created time.
func ciRules(t *testing.T, project map[string]any, want ...rule) {
	t.Helper()
	c, _ := project["ci"].(map[string]any)
	list, isList := c["rules"].([]any)
	if !isList || len(list) != len(want) {
		t.Fatalf("ci rules = %v, want %v", c["rules"], want)
	}
	for i, r := range list {
		m := r.(map[string]any)
		got := rule{m["spec"].(string), m["workflow"].(string), m["branch"].(string), m["state"].(string)}
		created, _ := m["created"].(string)
		if _, err := time.Parse(time.RFC3339, created); got != want[i] || err != nil {
			t.Fatalf("ci rule %d = %v, want %v with a created time", i, m, want[i])
		}
	}
}

// firstRule is a Project's first CI Rule.
func firstRule(project map[string]any) map[string]any {
	return project["ci"].(map[string]any)["rules"].([]any)[0].(map[string]any)
}

func getAcme(t *testing.T, srv *codecentraltest.Server) map[string]any {
	t.Helper()
	return httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})
}

func TestEveryProjectReadCarriesItsCIRules(t *testing.T) {
	srv := newServer(t)
	ciRules(t, getAcme(t, srv))
	httpOK(t, srv, addCIRule(`{"workflow":"ci.yml","branch":"main","state":"Ready"}`))
	httpOK(t, srv, addCIRule(`{"workflow":"any","branch":"release"}`))
	want := []rule{{"acme-4", "ci.yml", "main", "Ready"}, {"acme-5", "any", "release", "Draft"}}
	ciRules(t, getAcme(t, srv), want...)
	rows := httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any)
	ciRules(t, rows[0].(map[string]any), want...)
	ciRules(t, rows[1].(map[string]any))
}

func TestAddingACIRuleCreatesItsSpecAtTheSameTime(t *testing.T) {
	srv := newServer(t)
	out := httpOK(t, srv, addCIRule(`{"workflow":"ci.yml","branch":"main","full":true}`))
	httpOK(t, srv, addCIRule(`{"workflow":"any","branch":"main"}`))
	for id, title := range map[string]string{"acme-4": "CI: ci.yml on main", "acme-5": "CI: any workflow on main"} {
		spec := httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/" + id, ""})
		if spec["title"] != title || !strings.Contains(spec["body"].(string), "CI Rule") {
			t.Fatalf("%s: title %v body %q, want %q and a body naming the CI Rule", id, spec["title"], spec["body"], title)
		}
	}
	spec := httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/acme-4", ""})
	if created := firstRule(out)["created"]; created != spec["created"] {
		t.Fatalf("rule created %v, Spec created %v: want the same", created, spec["created"])
	}
}

func TestRemovingACIRuleKeepsItsSpec(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, addCIRule(`{"workflow":"ci.yml","branch":"main"}`))
	httpOK(t, srv, addCIRule(`{"workflow":"lint.yml","branch":"main"}`))
	ciRules(t, httpOK(t, srv, removeCIRule(`{"spec":"acme-4","full":true}`)), rule{"acme-5", "lint.yml", "main", "Draft"})
	if spec := httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/acme-4", ""}); spec["title"] != "CI: ci.yml on main" {
		t.Fatalf("the removed rule's Spec = %v, want it kept", spec)
	}
	// Removed, it is no rule to edit or remove again.
	httpRejected(t, srv, removeCIRule(`{"spec":"acme-4"}`), core.NotFound)
	httpRejected(t, srv, editCIRule(`{"spec":"acme-4","workflow":"ci.yml","branch":"main"}`), core.NotFound)
}

func TestEditingACIRuleKeepsItsSpecAndCreatedTimeAndRetitlesTheSpec(t *testing.T) {
	srv := newServer(t)
	created := firstRule(httpOK(t, srv, addCIRule(`{"workflow":"ci.yml","branch":"main","full":true}`)))["created"]
	out := httpOK(t, srv, editCIRule(`{"spec":4,"workflow":"any","branch":"develop","state":"Ready","full":true}`))
	ciRules(t, out, rule{"acme-4", "any", "develop", "Ready"})
	if got := firstRule(out)["created"]; got != created {
		t.Fatalf("created %v after an edit, want %v", got, created)
	}
	spec := httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/acme-4", ""})
	if spec["title"] != "CI: any workflow on develop" {
		t.Fatalf("title %v, want the Spec retitled after its rule", spec["title"])
	}
	// The same rule again writes nothing.
	if out := httpOK(t, srv, editCIRule(`{"spec":"acme-4","workflow":"any","branch":"develop","state":"Ready"}`)); len(out["changed"].([]any)) != 0 {
		t.Fatalf("the same rule changed %v", out["changed"])
	}
}

func TestCIRulesAreRejectedForAnAgent(t *testing.T) {
	srv := newServer(t, withCIRule(`{"workflow":"ci.yml","branch":"main"}`))
	for _, c := range []httpCall{
		addCIRule(`{"workflow":"lint.yml","branch":"main"}`),
		editCIRule(`{"spec":"acme-4","workflow":"any","branch":"main"}`),
		removeCIRule(`{"spec":"acme-4"}`),
	} {
		status, b := httpDoAs(t, srv, core.Agent, c)
		if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
			t.Fatalf("%s: status %d message %q, want a human action", c.path, status, e.Message)
		}
	}
	ciRules(t, getAcme(t, srv), rule{"acme-4", "ci.yml", "main", "Draft"})
	if _, err := srv.Core.GetSpec(context.Background(), "acme", core.GetSpecInput{Spec: "acme-5"}); err == nil {
		t.Fatal("an agent's add_ci_rule created a Spec")
	}
}

func TestACIRuleNamesAWorkflowFileOrAny(t *testing.T) {
	srv := newServer(t)
	for _, w := range []string{"", " ", ".github/workflows/ci.yml", "ci", "CI build", "*.yml"} {
		e := httpRejected(t, srv, addCIRule(`{"workflow":"`+w+`","branch":"main"}`), core.Invalid)
		if e.Field != "workflow" || !strings.Contains(e.Message, "ci.yml") || !strings.Contains(e.Message, "any") {
			t.Fatalf("%q: field %q message %q, want workflow with an example and any", w, e.Field, e.Message)
		}
	}
	ciRules(t, getAcme(t, srv))
}

func TestACIRuleBranchIsAnExactName(t *testing.T) {
	srv := newServer(t)
	for _, b := range []string{"", "  ", "release/*", "feat-?", "[ab]", "my branch"} {
		e := httpRejected(t, srv, addCIRule(`{"workflow":"ci.yml","branch":"`+b+`"}`), core.Invalid)
		if e.Field != "branch" || !strings.Contains(e.Message, "glob") {
			t.Fatalf("%q: field %q message %q, want branch saying no globs", b, e.Field, e.Message)
		}
	}
	ciRules(t, httpOK(t, srv, addCIRule(`{"workflow":"ci.yml","branch":"release/2.x","full":true}`)), rule{"acme-4", "ci.yml", "release/2.x", "Draft"})
}

func TestACIRuleOpensItsTicketsAsDraftOrReady(t *testing.T) {
	srv := newServer(t)
	for _, s := range []string{"In Progress", "Done", "ready"} {
		e := httpRejected(t, srv, addCIRule(`{"workflow":"ci.yml","branch":"main","state":"`+s+`"}`), core.Invalid)
		if e.Field != "state" || !strings.Contains(e.Message, "Draft") || !strings.Contains(e.Message, "Ready") {
			t.Fatalf("%q: field %q message %q, want state naming Draft and Ready", s, e.Field, e.Message)
		}
	}
	ciRules(t, getAcme(t, srv))
}

func TestTwoCIRulesCannotWatchTheSameWorkflowAndBranch(t *testing.T) {
	srv := newServer(t, withCIRule(`{"workflow":"ci.yml","branch":"main"}`), withCIRule(`{"workflow":"any","branch":"main"}`))
	for _, c := range []httpCall{
		addCIRule(`{"workflow":"ci.yml","branch":"main","state":"Ready"}`),
		editCIRule(`{"spec":"acme-5","workflow":"ci.yml","branch":"main"}`),
	} {
		e := httpRejected(t, srv, c, core.Invalid)
		if !strings.Contains(e.Message, "acme-4") {
			t.Fatalf("%s: message %q, want it to name the rule's Spec acme-4", c.path, e.Message)
		}
	}
	// The same branch with another workflow is another rule.
	httpOK(t, srv, addCIRule(`{"workflow":"lint.yml","branch":"main"}`))
}

func TestAnArchivedProjectRejectsCIRules(t *testing.T) {
	srv := newServer(t)
	human := core.WithActor(context.Background(), core.Human)
	must(t, func() error { _, err := srv.Core.ArchiveProject(human, "other"); return err })
	httpRejected(t, srv, httpCall{"POST", "/api/p/other/add_ci_rule", `{"workflow":"ci.yml","branch":"main"}`}, core.ActionNotAllowed)
	httpRejected(t, srv, httpCall{"POST", "/api/p/nope/add_ci_rule", `{"workflow":"ci.yml","branch":"main"}`}, core.UnknownProject)
}

// Turning CI Tracking on or off keeps the rules.
func TestSetCITrackingKeepsTheCIRules(t *testing.T) {
	srv := newServer(t, withCIRule(`{"workflow":"ci.yml","branch":"main"}`))
	ciRules(t, httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/web","full":true}`)), rule{"acme-4", "ci.yml", "main", "Draft"})
}

// A Spec the human retitled keeps its title when its rule is edited.
func TestEditingACIRuleLeavesARetitledSpec(t *testing.T) {
	srv := newServer(t, withCIRule(`{"workflow":"ci.yml","branch":"main"}`))
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/specs/acme-4/edit_spec", `{"title":"Keep main green"}`})
	httpOK(t, srv, editCIRule(`{"spec":"acme-4","workflow":"any","branch":"main"}`))
	if spec := httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/acme-4", ""}); spec["title"] != "Keep main green" {
		t.Fatalf("title %v, want the human's", spec["title"])
	}
}
