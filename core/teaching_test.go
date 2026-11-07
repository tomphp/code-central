package core_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
)

func TestAProjectRemembersItsMode(t *testing.T) {
	f := newFixture(t)
	if p := f.project(t, "acme"); p.Mode != core.ModeCommitted {
		t.Fatalf("default mode %q", p.Mode)
	}
	p, err := f.core.CreateProject(ctx, core.CreateProjectInput{Key: "solo", Mode: core.ModeLocal})
	if err != nil || p.Mode != core.ModeLocal {
		t.Fatalf("create local: %+v %v", p, err)
	}
	_, err = f.core.CreateProject(ctx, core.CreateProjectInput{Key: "odd", Mode: "shared"})
	wantCode(t, err, core.Invalid, "mode")

	p, err = f.core.SetProjectMode(ctx, "acme", core.SetProjectModeInput{Mode: core.ModeLocal})
	if err != nil || p.Mode != core.ModeLocal {
		t.Fatalf("set_project_mode: %+v %v", p, err)
	}
	if got, _ := f.core.GetProject(ctx, "acme"); got.Mode != core.ModeLocal {
		t.Fatalf("stored mode %q", got.Mode)
	}
	_, err = f.core.SetProjectMode(ctx, "acme", core.SetProjectModeInput{Mode: "shared"})
	wantCode(t, err, core.Invalid, "mode")
	_, err = f.core.SetProjectMode(ctx, "nope", core.SetProjectModeInput{Mode: core.ModeLocal})
	wantCode(t, err, core.UnknownProject, "")
}

func TestTheTeachingFilesMapTheSkillsToCodeCentral(t *testing.T) {
	files := core.TeachingFiles()
	byPath := map[string]string{}
	for _, f := range files {
		byPath[f.Path] = f.Body
	}
	tracker, labels := byPath["docs/agents/issue-tracker.md"], byPath["docs/agents/triage-labels.md"]
	if len(files) != 2 || tracker == "" || labels == "" {
		t.Fatalf("teaching files %v", files)
	}
	for _, want := range []string{
		"get_workflow", "create_spec", "create_ticket", "get_ticket", "get_spec", "comment",
		"`wayfinder:map`", "add_blocker", "find_tickets(spec, claimable)", "claim(ticket)", "finish",
	} {
		if !strings.Contains(tracker, want) {
			t.Errorf("issue-tracker.md doesn't mention %s", want)
		}
	}
	for _, want := range []string{
		"get_workflow", "| `needs-triage` | `Draft` State", "| `ready-for-agent` | `Ready` State",
		"| `wontfix` | `Cancelled` State", "| `needs-info` | `needs-info` Label", "| `ready-for-human` | `ready-for-human` Label",
	} {
		if !strings.Contains(labels, want) {
			t.Errorf("triage-labels.md doesn't mention %s", want)
		}
	}
}

func TestEveryGeneratedTeachingFileStaysWithinAbout400Tokens(t *testing.T) {
	for _, f := range core.TeachingFiles() {
		if n := tokens(f.Body); n > 400 {
			t.Errorf("%s is about %d tokens, over the 400 budget", f.Path, n)
		}
	}
}
