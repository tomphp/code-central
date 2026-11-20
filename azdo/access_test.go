package azdo_test

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/internal/fakeazdo"
)

func TestReadyIsAClientWhenThePATIsSet(t *testing.T) {
	fake := fakeazdo.New(t)
	a := azdo.Access{Environ: []string{"ADO_PAT=" + pat}, PATEnv: "ADO_PAT", Watcher: "CI Watcher", Scope: "Build: Read & execute"}
	c, r, err := a.Ready(fake.ProjectURL())
	if err != nil || r.Problem != "" || c == nil {
		t.Fatalf("Ready = %v, %+v, %v; want a client", c, r, err)
	}
	if c.ProjectURL() != fake.ProjectURL() {
		t.Fatalf("ProjectURL = %q", c.ProjectURL())
	}
}

func TestReadyWithoutThePATIsThePATMissingProblem(t *testing.T) {
	a := azdo.Access{Environ: []string{"OTHER=x"}, PATEnv: "MY_PAT", Watcher: "CI Watcher", Scope: "Build: Read & execute"}
	c, r, err := a.Ready("https://dev.azure.com/acme/web")
	if err != nil || c != nil {
		t.Fatalf("Ready = %v, %v; want no client and no error", c, err)
	}
	want := api.WatcherCheckResult{Problem: api.ProblemPATMissing,
		Reason: "The environment variable MY_PAT is not set, or is empty, in the CI Watcher's environment.",
		Fix:    azdo.PATMissingFix(runtime.GOOS, "MY_PAT", "Build: Read & execute")}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("result = %+v, want %+v", r, want)
	}
}

func TestReadyReadsTheDefaultPATVariableWhenNoneIsNamed(t *testing.T) {
	fake := fakeazdo.New(t)
	a := azdo.Access{Environ: []string{domain.DefaultPATEnv + "=" + pat}, Watcher: "Pull Request Watcher", Scope: "Code: Read & write"}
	if _, r, err := a.Ready(fake.ProjectURL()); err != nil || r.Problem != "" {
		t.Fatalf("Ready = %+v, %v; want a client", r, err)
	}
	_, r, _ := azdo.Access{Watcher: "Pull Request Watcher", Scope: "Code: Read & write"}.Ready(fake.ProjectURL())
	if r.Problem != api.ProblemPATMissing || r.Fix != azdo.PATMissingFix(runtime.GOOS, domain.DefaultPATEnv, "Code: Read & write") {
		t.Fatalf("result = %+v, want the PAT missing problem naming %s", r, domain.DefaultPATEnv)
	}
}

func TestReadyRefusesAProjectURLThatDoesNotParse(t *testing.T) {
	a := azdo.Access{Environ: []string{"ADO_PAT=" + pat}, PATEnv: "ADO_PAT", Watcher: "CI Watcher", Scope: "Build: Read & execute"}
	if c, r, err := a.Ready("https://dev.azure.com/acme"); err == nil || c != nil || r.Problem != "" {
		t.Fatalf("Ready = %v, %+v, %v; want the URL's error and no problem", c, r, err)
	}
}

func TestRejectedIsThePATRejectedProblemWithTheScope(t *testing.T) {
	r := azdo.Access{PATEnv: "MY_PAT", Scope: "Code: Read & write"}.Rejected()
	want := api.WatcherCheckResult{Problem: api.ProblemPATRejected,
		Reason: "Azure DevOps rejected the PAT in MY_PAT: it may have expired or been revoked.",
		Fix:    "Make a new PAT with Code: Read & write, and set it in MY_PAT for the service."}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("result = %+v, want %+v", r, want)
	}
}
