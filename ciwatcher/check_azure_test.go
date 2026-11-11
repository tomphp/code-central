package ciwatcher_test

import (
	"context"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/internal/fakeazdo"
	"github.com/tomoram/code-central/store"
)

const secretPAT = "s3cret-pat-value"

// checkAzure runs the check mode for the fake's project with environ.
func checkAzure(srv *fakeazdo.Server, environ ...string) core.WatcherCheckResult {
	in := core.CICheckInput{Provider: store.CIAzurePipelines, ProjectURL: srv.ProjectURL(), PATEnv: "ADO_PAT"}
	return ciwatcher.Check(context.Background(), in, gh.Env{Environ: environ})
}

func TestAnAzureCheckWithAWorkingPATPassesAndSendsItAsBasicAuth(t *testing.T) {
	srv := fakeazdo.New(t)
	if r := checkAzure(srv, "ADO_PAT="+secretPAT); r != (core.WatcherCheckResult{}) {
		t.Fatalf("check = %+v, want no problem", r)
	}
	reqs := srv.Requests()
	if len(reqs) != 2 || !strings.Contains(reqs[0].Path, "/_apis/projects/web") || !strings.Contains(reqs[1].Path, "/_apis/build/builds") {
		t.Fatalf("requests = %+v, want the project read, then one build listed", reqs)
	}
	if reqs[1].Query["$top"][0] != "1" {
		t.Fatalf("builds query = %v, want one build", reqs[1].Query)
	}
	// Basic auth with an empty user name: base64(":" + PAT).
	if want := "Basic OnMzY3JldC1wYXQtdmFsdWU="; reqs[0].Authorization != want {
		t.Fatalf("Authorization = %q, want %q", reqs[0].Authorization, want)
	}
}

func TestAnUnsetOrEmptyPATVariableIsPATMissingWithThePerOSFix(t *testing.T) {
	for name, environ := range map[string][]string{"unset": nil, "empty": {"ADO_PAT="}} {
		t.Run(name, func(t *testing.T) {
			srv := fakeazdo.New(t)
			r := checkAzure(srv, environ...)
			if r.Problem != core.ProblemPATMissing || !strings.Contains(r.Reason, "ADO_PAT") || !strings.Contains(r.Fix, "restart") {
				t.Fatalf("check = %+v, want pat_missing naming ADO_PAT and a restart", r)
			}
			want := map[string]string{"darwin": "launchctl setenv ADO_PAT", "linux": "systemctl --user set-environment", "windows": "user environment variable"}[runtime.GOOS]
			if !strings.Contains(r.Fix, want) {
				t.Fatalf("fix = %q, want it to say %q", r.Fix, want)
			}
			if n := len(srv.Requests()); n != 0 {
				t.Fatalf("%d requests, want none without a PAT", n)
			}
		})
	}
}

func TestARejectedPATIsPATRejected(t *testing.T) {
	srv := fakeazdo.New(t, fakeazdo.Answer{Path: "/", Status: 401})
	r := checkAzure(srv, "ADO_PAT="+secretPAT)
	if r.Problem != core.ProblemPATRejected || !strings.Contains(r.Fix, "Build: Read & execute") || !strings.Contains(r.Fix, "ADO_PAT") {
		t.Fatalf("check = %+v, want pat_rejected naming the scope and ADO_PAT", r)
	}
}

func TestAProjectNotFoundOrUnreachableIsProjectUnreadable(t *testing.T) {
	srv := fakeazdo.New(t, fakeazdo.Answer{Path: "/", Status: 404})
	if r := checkAzure(srv, "ADO_PAT="+secretPAT); r.Problem != core.ProblemProjectUnreadable || !strings.Contains(r.Reason, srv.ProjectURL()) {
		t.Fatalf("404: check = %+v, want project_unreadable naming the project", r)
	}
	down := fakeazdo.New(t)
	url := down.ProjectURL()
	down.Close()
	in := core.CICheckInput{Provider: store.CIAzurePipelines, ProjectURL: url, PATEnv: "ADO_PAT"}
	r := ciwatcher.Check(context.Background(), in, gh.Env{Environ: []string{"ADO_PAT=" + secretPAT}})
	if r.Problem != core.ProblemProjectUnreadable {
		t.Fatalf("unreachable: check = %+v, want project_unreadable", r)
	}
}

func TestAPATWithoutTheBuildScopeIsPATScope(t *testing.T) {
	srv := fakeazdo.New(t, fakeazdo.Answer{Path: fakeazdo.ProjectPath + "/_apis/build", Status: 403})
	r := checkAzure(srv, "ADO_PAT="+secretPAT)
	if r.Problem != core.ProblemPATScope || !strings.Contains(r.Fix, "Build: Read & execute") {
		t.Fatalf("check = %+v, want pat_scope naming the scope", r)
	}
}

func TestNoCheckAnswerEverCarriesThePAT(t *testing.T) {
	for _, status := range []int{401, 403, 404, 500} {
		srv := fakeazdo.New(t, fakeazdo.Answer{Path: "/", Status: status, Body: "no"})
		r := checkAzure(srv, "ADO_PAT="+secretPAT)
		if strings.Contains(r.Reason+r.Fix, secretPAT) {
			t.Fatalf("status %d: check = %+v leaks the PAT", status, r)
		}
	}
}

func TestAnAzureCheckThrottledIsAFailedCheckWithWhenToTryAgain(t *testing.T) {
	srv := fakeazdo.New(t, fakeazdo.Answer{Path: "/", Status: 429}.WithHeader("Retry-After", "60"))
	r := checkAzure(srv, "ADO_PAT="+secretPAT)
	if r.Problem != core.ProblemCheckFailed || !strings.Contains(r.Reason, "Azure DevOps") {
		t.Fatalf("check = %+v, want check_failed naming Azure DevOps", r)
	}
}

// watchingAzure is acme watching the fake's project with the PAT variable.
func watchingAzure(t *testing.T, srv *fakeazdo.Server, environ ...string) *watching {
	t.Helper()
	w := newWatching(t)
	human := core.WithActor(context.Background(), core.Human)
	if _, err := w.srv.Core.SetCITracking(human, "acme", core.SetCITrackingInput{Tracking: true, Provider: store.CIAzurePipelines, ProjectURL: srv.ProjectURL(), PATEnv: "MY_PAT"}); err != nil {
		t.Fatal(err)
	}
	r, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); pw.Close() })
	go ciwatcher.Watch(ctx, ciwatcher.Config{Client: w.srv.Client, Project: "acme", Clock: w.srv.Clock, Stdin: r, Log: io.Discard,
		Env: gh.Env{Environ: environ}})
	return w
}

func TestTheCIWatcherReadsThePATFromItsOwnEnvironmentByTheVariableGetCIWatchNames(t *testing.T) {
	srv := fakeazdo.New(t)
	w := watchingAzure(t, srv, "MY_PAT="+secretPAT)
	w.status(core.WatcherWatching)
	if auth := srv.Requests()[0].Authorization; auth != "Basic OnMzY3JldC1wYXQtdmFsdWU=" {
		t.Fatalf("Authorization = %q, want the PAT from MY_PAT", auth)
	}
}

func TestAMissingPATIsAnErrorTheWatcherReportsWithoutLeakingAnything(t *testing.T) {
	srv := fakeazdo.New(t)
	w := watchingAzure(t, srv)
	got := w.status(core.WatcherError)
	if got.Problem != core.ProblemPATMissing || !strings.Contains(got.Message, "MY_PAT") {
		t.Fatalf("status %+v, want pat_missing naming MY_PAT", got)
	}
}
