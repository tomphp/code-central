package frontdoor_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Azure Pipelines settings (code-central-364), through set_ci_tracking and
// Project reads.

// azureCI checks a Project's ci object for the Azure fields.
func azureCI(t *testing.T, project map[string]any, provider, repository, projectURL, patEnv string) {
	t.Helper()
	c, _ := project["ci"].(map[string]any)
	if c == nil || c["provider"] != provider || c["repository"] != repository || c["project_url"] != projectURL || c["pat_env"] != patEnv {
		t.Fatalf("ci = %v, want %s, repository %q, project URL %q, PAT variable %q", project["ci"], provider, repository, projectURL, patEnv)
	}
}

func TestEveryProjectReadCarriesTheAzureSettingsWithADOPATAsTheDefault(t *testing.T) {
	srv := newServer(t)
	azureCI(t, getAcme(t, srv), "github_actions", "", "", "ADO_PAT")
	for _, row := range httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any) {
		azureCI(t, row.(map[string]any), "github_actions", "", "", "ADO_PAT")
	}
	out := httpOK(t, srv, setCITracking(`{"tracking":true,"provider":"azure_pipelines","project_url":" https://dev.azure.com/acme/web/_build?definitionId=3 ","full":true}`))
	azureCI(t, out, "azure_pipelines", "", "https://dev.azure.com/acme/web", "ADO_PAT")
	azureCI(t, getAcme(t, srv), "azure_pipelines", "", "https://dev.azure.com/acme/web", "ADO_PAT")
	out = httpOK(t, srv, setCITracking(`{"tracking":true,"provider":"azure_pipelines","project_url":"https://dev.azure.com/acme/web","pat_env":"ACME_PAT","full":true}`))
	azureCI(t, out, "azure_pipelines", "", "https://dev.azure.com/acme/web", "ACME_PAT")
}

func TestEachProviderKeepsItsOwnFieldsWhenTheProviderIsSwitched(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setCITracking(`{"tracking":false,"provider":"azure_pipelines","repository":"acme/web","project_url":"https://dev.azure.com/acme/web","pat_env":"ACME_PAT"}`))
	out := httpOK(t, srv, setCITracking(`{"tracking":false,"provider":"github_actions","repository":"acme/web","project_url":"https://dev.azure.com/acme/web","pat_env":"ACME_PAT","full":true}`))
	azureCI(t, out, "github_actions", "acme/web", "https://dev.azure.com/acme/web", "ACME_PAT")
}

func TestTurningAzurePipelinesOnNeedsAProjectURL(t *testing.T) {
	srv := newServer(t)
	for _, body := range []string{`{"tracking":true,"provider":"azure_pipelines"}`, `{"tracking":true,"provider":"azure_pipelines","repository":"acme/web","project_url":" "}`} {
		e := httpRejected(t, srv, setCITracking(body), core.Invalid)
		if e.Field != "project_url" || e.Message != "CI Tracking needs the Azure DevOps project to watch, as its URL" {
			t.Fatalf("%s: field %q message %q", body, e.Field, e.Message)
		}
	}
	// Off, the project URL may stay empty.
	httpOK(t, srv, setCITracking(`{"tracking":false,"provider":"azure_pipelines"}`))
}

func TestAProjectURLThatDoesNotParseIsRefusedOnProjectURL(t *testing.T) {
	srv := newServer(t)
	for _, u := range []string{"web", "https://dev.azure.com/acme", "https://dev.azure.com/", "ftp://dev.azure.com/acme/web"} {
		for _, tracking := range []string{"true", "false"} {
			e := httpRejected(t, srv, setCITracking(`{"tracking":`+tracking+`,"provider":"azure_pipelines","project_url":"`+u+`"}`), core.Invalid)
			if e.Field != "project_url" || !strings.Contains(e.Message, "Azure DevOps project URL") {
				t.Fatalf("%q: field %q message %q, want project_url", u, e.Field, e.Message)
			}
		}
	}
}

func TestAPATEnvironmentVariableIsANameAndIsRefusedOnPATEnvOtherwise(t *testing.T) {
	srv := newServer(t)
	for _, name := range []string{"1PAT", "MY-PAT", "my pat", "$PAT", "ghp_abc=def"} {
		e := httpRejected(t, srv, setCITracking(`{"tracking":false,"provider":"azure_pipelines","pat_env":"`+name+`"}`), core.Invalid)
		if e.Field != "pat_env" || !strings.Contains(e.Message, name) {
			t.Fatalf("%q: field %q message %q, want pat_env naming it", name, e.Field, e.Message)
		}
	}
	azureCI(t, httpOK(t, srv, setCITracking(`{"tracking":false,"provider":"azure_pipelines","pat_env":"_Acme_PAT_2","full":true}`)), "azure_pipelines", "", "", "_Acme_PAT_2")
}

func TestAnUnknownCIProviderIsRefusedNamingBoth(t *testing.T) {
	srv := newServer(t)
	e := httpRejected(t, srv, setCITracking(`{"tracking":true,"provider":"circleci","repository":"acme/web"}`), core.Invalid)
	if e.Field != "provider" || !strings.Contains(e.Message, `"github_actions"`) || !strings.Contains(e.Message, `"azure_pipelines"`) {
		t.Fatalf("field %q message %q, want provider naming both", e.Field, e.Message)
	}
}

func TestTheCheckRunsAgainWhenTheProviderProjectURLOrPATVariableChangesWhileOn(t *testing.T) {
	checks := &ciChecks{}
	srv := newCICheckServer(t, checks)
	on := func(extra string) {
		httpOK(t, srv, setCITracking(`{"tracking":true,"provider":"azure_pipelines","project_url":"https://dev.azure.com/acme/web"`+extra+`}`))
	}
	on(``)
	want := core.CICheckInput{Provider: store.CIAzurePipelines, ProjectURL: "https://dev.azure.com/acme/web", PATEnv: "ADO_PAT"}
	if got := checks.ran(); !slices.Equal(got, []core.CICheckInput{want}) {
		t.Fatalf("checks = %+v, want %+v", got, want)
	}
	on(`,"pat_env":"ADO_PAT","ignored_workflows":["Nightly build"]`)
	if n := len(checks.ran()); n != 1 {
		t.Fatalf("naming the default PAT variable and an ignored workflow ran %d checks, want 1", n)
	}
	on(`,"pat_env":"ACME_PAT"`)
	httpOK(t, srv, setCITracking(`{"tracking":true,"provider":"azure_pipelines","project_url":"https://dev.azure.com/acme/site","pat_env":"ACME_PAT"}`))
	httpOK(t, srv, setCITracking(`{"tracking":true,"provider":"github_actions","repository":"acme/web","project_url":"https://dev.azure.com/acme/site","pat_env":"ACME_PAT"}`))
	got := checks.ran()
	if len(got) != 4 || got[1].PATEnv != "ACME_PAT" || got[2].ProjectURL != "https://dev.azure.com/acme/site" || got[3].Provider != store.CIGitHubActions || got[3].Repository != "acme/web" {
		t.Fatalf("checks = %+v, want one per change", got)
	}
}

func TestTheAzureCheckProblemsAreRefusedOnTheirFields(t *testing.T) {
	for problem, field := range map[core.WatcherProblem]string{
		core.ProblemPATMissing: "tracking", core.ProblemPATRejected: "tracking", core.ProblemPATScope: "tracking", core.ProblemProjectUnreadable: "project_url",
	} {
		t.Run(string(problem), func(t *testing.T) {
			checks := &ciChecks{result: core.WatcherCheckResult{Problem: problem, Reason: "azure says no", Fix: "Fix it."}}
			srv := newCICheckServer(t, checks)
			e := httpRejected(t, srv, setCITracking(`{"tracking":true,"provider":"azure_pipelines","project_url":"https://dev.azure.com/acme/web"}`), core.Invalid)
			if e.Field != field || e.Message != "azure says no" || e.Fix != "Fix it." {
				t.Fatalf("refusal = %+v, want the check's reason and fix on %s", e, field)
			}
		})
	}
}

func TestChangingTheProviderResetsTheCIWatchersStatusAndLeavesTheIgnoredWorkflowsAndRules(t *testing.T) {
	srv := newServer(t, withCIRule(`{"workflow":"ci.yml","branch":"main"}`))
	httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/web","ignored_workflows":["nightly.yml"]}`))
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/report_ci_watcher_status", `{"state":"watching","message":"Watching acme/web"}`})
	out := httpOK(t, srv, setCITracking(`{"tracking":true,"provider":"azure_pipelines","project_url":"https://dev.azure.com/acme/web","ignored_workflows":["nightly.yml"],"full":true}`))
	if w, _ := out["ci"].(map[string]any)["watcher"].(map[string]any); w["state"] != "not_reported" {
		t.Fatalf("watcher = %v, want its status reset", w)
	}
	ciRules(t, out, rule{"acme-4", "ci.yml", "main", "Draft"})
	if c := out["ci"].(map[string]any); !slices.Equal(anyStrings(c["ignored_workflows"]), []string{"nightly.yml"}) {
		t.Fatalf("ignored = %v, want them left alone", c["ignored_workflows"])
	}
}

func anyStrings(v any) []string {
	var out []string
	for _, s := range v.([]any) {
		out = append(out, s.(string))
	}
	return out
}

func TestIgnoredWorkflowsAndCIRulesAreValidatedByTheProjectsProvider(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setCITracking(`{"tracking":false,"provider":"azure_pipelines"}`))
	for _, w := range []string{"a/b", `a\b`, "build*", "x?", "[x]"} {
		e := httpRejected(t, srv, setCITracking(`{"tracking":false,"provider":"azure_pipelines","ignored_workflows":["`+strings.ReplaceAll(w, `\`, `\\`)+`"]}`), core.Invalid)
		if e.Field != "ignored_workflows" || !strings.Contains(e.Message, "pipeline") {
			t.Fatalf("%q: field %q message %q, want ignored_workflows asking for a pipeline's name", w, e.Field, e.Message)
		}
		e = httpRejected(t, srv, addCIRule(`{"workflow":"`+strings.ReplaceAll(w, `\`, `\\`)+`","branch":"main"}`), core.Invalid)
		if e.Field != "workflow" || !strings.Contains(e.Message, "pipeline") || !strings.Contains(e.Message, "any") {
			t.Fatalf("%q: field %q message %q, want workflow asking for a pipeline's name or any", w, e.Field, e.Message)
		}
	}
	out := httpOK(t, srv, setCITracking(`{"tracking":false,"provider":"azure_pipelines","ignored_workflows":["Nightly build","ci.yml"],"full":true}`))
	if got := anyStrings(out["ci"].(map[string]any)["ignored_workflows"]); !slices.Equal(got, []string{"Nightly build", "ci.yml"}) {
		t.Fatalf("ignored = %v", got)
	}
	httpOK(t, srv, addCIRule(`{"workflow":"Build and test","branch":"main"}`))
	httpOK(t, srv, addCIRule(`{"workflow":"any","branch":"release"}`))
	ciRules(t, getAcme(t, srv), rule{"acme-4", "Build and test", "main", "Draft"}, rule{"acme-5", "any", "release", "Draft"})
	httpRejected(t, srv, editCIRule(`{"spec":"acme-4","workflow":"ci/x","branch":"main"}`), core.Invalid)
	httpOK(t, srv, editCIRule(`{"spec":"acme-4","workflow":"Deploy web","branch":"main"}`))

	// GitHub Actions keeps its workflow file pattern, and switching back
	// leaves the Azure names alone.
	httpOK(t, srv, setCITracking(`{"tracking":false,"provider":"github_actions","ignored_workflows":["ci.yml"]}`))
	httpRejected(t, srv, addCIRule(`{"workflow":"Build and test","branch":"dev"}`), core.Invalid)
	if rules := getAcme(t, srv)["ci"].(map[string]any)["rules"].([]any); len(rules) != 2 {
		t.Fatalf("rules = %v, want both left alone", rules)
	}
}

func TestAPartiallySucceededRunCountsAsPassedAndIsKeptAsPartiallySucceeded(t *testing.T) {
	srv := newServer(t, func(t *testing.T, srv *codecentraltest.Server) {
		pushedAccepted(t, srv)
		reportRuns(t, srv, ciRun(1, "Build", "abc123", store.CICompleted, store.CIPartiallySucceeded))
	})
	if tk := ticket3(t, srv); tk.CIResult != core.CIPassed {
		t.Fatalf("ci result %q, want passed", tk.CIResult)
	}
	runs, err := srv.Client.GetTicket(t.Context(), "acme", "acme-3")
	if err != nil || len(runs.CIRuns) != 1 || runs.CIRuns[0].Conclusion != store.CIPartiallySucceeded {
		t.Fatalf("runs = %+v, err %v, want the run kept as partially succeeded", runs.CIRuns, err)
	}
}

func TestGetCIWatchCarriesTheProjectURLAndThePATVariable(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setCITracking(`{"tracking":true,"provider":"azure_pipelines","project_url":"https://dev.azure.com/acme/web"}`))
	w, err := srv.Client.GetCIWatch(t.Context(), "acme")
	if err != nil || w.Provider != store.CIAzurePipelines || w.ProjectURL != "https://dev.azure.com/acme/web" || w.PATEnv != "ADO_PAT" {
		t.Fatalf("watch = %+v, err %v, want the project URL and ADO_PAT", w, err)
	}
}
