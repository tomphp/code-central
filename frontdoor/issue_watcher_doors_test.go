package frontdoor_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// The Issue Watcher's own operations (code-central-285): get_issue_watch,
// the read it polls, and report_issue_watcher_status. Both are the
// integration Actor's, over HTTP only.
func init() {
	cases["get_issue_watch"] = opCase{
		setup: githubIssuesOn,
		ok:    call{http: getIssueWatch("acme")},
		check: func(t *testing.T, out map[string]any) {
			field("project", "acme")(t, out)
			field("repository", "acme/web")(t, out)
		},
		bad:  call{http: getIssueWatch("nope")},
		code: core.UnknownProject,
	}
	cases["report_issue_watcher_status"] = opCase{
		setup: githubIssuesOn,
		ok:    call{http: reportIssueWatcherStatus(`{"state":"watching"}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "github_issues")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			issueWatcher(t, out, "watching", "", "")
		},
		bad:  call{http: reportIssueWatcherStatus(`{"state":"asleep"}`)},
		code: core.Invalid,
	}
}

func getIssueWatch(key string) httpCall {
	return httpCall{"GET", "/api/p/" + key + "/integration/issue-watch", ""}
}

func reportIssueWatcherStatus(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/integration/report_issue_watcher_status", body}
}

// githubIssuesOn turns GitHub Issues on for acme, from acme/web, with the
// filter label triage.
func githubIssuesOn(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/web","label":"triage"}`))
}

// issueWatcher checks the Issue Watcher's status in a Project's
// github_issues object.
func issueWatcher(t *testing.T, project map[string]any, state, message, fix string) {
	t.Helper()
	g, _ := project["github_issues"].(map[string]any)
	w, _ := g["watcher"].(map[string]any)
	if w == nil || w["state"] != state || str(w["message"]) != message || str(w["fix"]) != fix {
		t.Fatalf("github_issues.watcher = %v, want %s with message %q and fix %q", g["watcher"], state, message, fix)
	}
}

func TestGetIssueWatchHasTheGitHubIssuesSettingsAndGHPath(t *testing.T) {
	srv := newServer(t, githubIssuesOn)
	gh := filepath.Join(t.TempDir(), "gh")
	httpOK(t, srv, httpCall{"POST", "/api/set_options", `{"gh_path":` + strconv.Quote(gh) + `}`})

	got := httpOK(t, srv, getIssueWatch("acme"))
	want := map[string]any{"project": "acme", "on": true, "repository": "acme/web", "label": "triage", "gh_path": gh, "spec": "acme-4"}
	if issues, ok := got["issues"].([]any); !ok || len(issues) != 0 {
		t.Fatalf("get_issue_watch issues = %v, want none", got["issues"])
	}
	delete(got, "issues")
	if !mapsEqual(got, want) {
		t.Fatalf("get_issue_watch = %v, want %v", got, want)
	}
}

func TestGetIssueWatchSaysWhenGitHubIssuesIsOff(t *testing.T) {
	srv := newServer(t)
	got := httpOK(t, srv, getIssueWatch("acme"))
	want := map[string]any{"project": "acme", "on": false, "repository": "", "label": "", "gh_path": ""}
	if issues, ok := got["issues"].([]any); !ok || len(issues) != 0 {
		t.Fatalf("get_issue_watch issues = %v, want none", got["issues"])
	}
	delete(got, "issues")
	if !mapsEqual(got, want) {
		t.Fatalf("get_issue_watch = %v, want %v", got, want)
	}
}

// createIncomingIssue creates a Draft Ticket on spec linked to acme/web's
// issue n, as the Issue Watcher does, and returns its ID.
func createIncomingIssue(t *testing.T, srv *codecentraltest.Server, spec string, n int) string {
	t.Helper()
	body := fmt.Sprintf(`{"spec":%q,"title":"Issue %d","description":"Body %d","full":true,"link":{"integration":"github","id":"acme/web#%d","url":"https://github.com/acme/web/issues/%d"}}`, spec, n, n, n, n)
	return httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/create_linked_ticket", body})["id"].(string)
}

// get_issue_watch lists the Incoming Issues: the Tickets on the Issues
// Spec that aren't finished and whose Link isn't detached, each with its
// Link, title and description.
func TestGetIssueWatchListsTheIncomingIssuesThatArentFinishedOrDetached(t *testing.T) {
	srv := newServer(t, githubIssuesOn)
	waiting := createIncomingIssue(t, srv, "acme-4", 1)
	cancelled := createIncomingIssue(t, srv, "acme-4", 2)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + cancelled + "/cancel", `{}`})
	unlinked := createIncomingIssue(t, srv, "acme-4", 3)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/unlink", `{"target":"` + unlinked + `"}`})
	createIncomingIssue(t, srv, "1", 4) // on another Spec
	ready := createIncomingIssue(t, srv, "acme-4", 5)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + ready + "/approve", `{}`})

	issues, _ := httpOK(t, srv, getIssueWatch("acme"))["issues"].([]any)
	var got []string
	for _, i := range issues {
		i := i.(map[string]any)
		l := i["link"].(map[string]any)
		got = append(got, fmt.Sprintf("%s %s %s %s %s %s", i["ticket"], i["title"], i["description"], l["integration"], l["id"], l["url"]))
	}
	want := []string{
		waiting + " Issue 1 Body 1 github acme/web#1 https://github.com/acme/web/issues/1",
		ready + " Issue 5 Body 5 github acme/web#5 https://github.com/acme/web/issues/5",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("get_issue_watch issues = %q, want %q", got, want)
	}
}

// The status report is the Issue Watcher's alone: neither an agent nor
// the human may make it.
func TestReportIssueWatcherStatusIsRefusedFromAnyoneButTheIntegrationActor(t *testing.T) {
	srv := newServer(t, githubIssuesOn)
	for _, actor := range []core.Actor{core.Agent, core.Human} {
		status, b := httpDoAs(t, srv, actor, reportIssueWatcherStatus(`{"state":"watching"}`))
		if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "integration") {
			t.Fatalf("as %s: status %d, want %d; message %q", actor, status, httpapi.Status(core.ActionNotAllowed), e.Message)
		}
	}
	issueWatcher(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "not_reported", "", "")
}

func TestTheIssueWatchersStatusShowsInGitHubIssues(t *testing.T) {
	srv := newServer(t, githubIssuesOn)
	project := func() map[string]any { return httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}) }
	issueWatcher(t, project(), "not_reported", "", "")

	httpOK(t, srv, reportIssueWatcherStatus(`{"state":"error","problem":"no_triage","message":"gh can't triage acme/web","fix":"Give it triage."}`))
	issueWatcher(t, project(), "error", "gh can't triage acme/web", "Give it triage.")
	// The CI Watcher's status is its own.
	ciWatcher(t, project(), "not_reported", "", "")

	httpOK(t, srv, reportIssueWatcherStatus(`{"state":"watching","rate_limited_until":"2026-01-05T10:15:00Z"}`))
	g := project()["github_issues"].(map[string]any)["watcher"].(map[string]any)
	if g["state"] != "watching" || g["rate_limited_until"] != "2026-01-05T10:15:00Z" {
		t.Fatalf("github_issues.watcher = %v, want watching, rate-limited until 10:15", g)
	}

	// Turning GitHub Issues off forgets it.
	httpOK(t, srv, setGitHubIssues(`{"on":false,"repository":"acme/web"}`))
	issueWatcher(t, project(), "not_reported", "", "")
}

// Only an Issue Watcher's problems are accepted, and only while GitHub
// Issues is on.
func TestReportIssueWatcherStatusRefusesWhatIsntTheIssueWatchers(t *testing.T) {
	srv := newServer(t)
	status, b := httpDo(t, srv, reportIssueWatcherStatus(`{"state":"watching"}`))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "GitHub Issues is off") {
		t.Fatalf("status %d; message %q, want GitHub Issues off", status, e.Message)
	}
	githubIssuesOn(t, srv)
	for _, body := range []string{
		`{"state":"error","problem":"crashed","message":"crashed"}`,
		`{"state":"error","problem":"logged_out"}`,
		`{"state":"error","message":"no","rate_limited_until":"2026-01-05T10:15:00Z"}`,
	} {
		status, b := httpDo(t, srv, reportIssueWatcherStatus(body))
		if errorJSON(t, b, core.Invalid); status != httpapi.Status(core.Invalid) {
			t.Fatalf("%s: status %d, want invalid", body, status)
		}
	}
}
