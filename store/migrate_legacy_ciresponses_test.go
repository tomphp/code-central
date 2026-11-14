package store_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// Format 37 (code-central-585) moved the human's responses stored on a
// Ticket's pushed commit before code-central-432 into the Project's CI
// responses, as reads overlaid them: Land anyway from either place, and
// the later Re-run request with its attempts.
func TestTheRealChainMigratesAFormat36StoreMovingPushedCommitResponsesOntoTheProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 36\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"), "key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n"+
		"ci:\n  tracking: true\n  provider: github_actions\n  repository: acme/web\n"+
		"ci_responses:\n"+
		"  - commit: inproject\n    land_anyway: 2026-01-02T04:00:00Z\n"+
		"  - commit: both\n    land_anyway: 2026-01-02T04:00:00Z\n"+
		"  - commit: projectlater\n    rerun: 2026-01-02T06:00:00Z\n    reruns:\n      - id: 21\n        attempt: 2\n"+
		"  - commit: pushedlater\n    rerun: 2026-01-02T05:00:00Z\n    reruns:\n      - id: 31\n        attempt: 1\n    fix_ticket: 9\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"), "title: Accepted\nstate: Accepted\nspec: 1\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n"+
		"pushed_commits:\n"+
		"  - commit: onpushed\n    branch: main\n    pushed: 2026-01-02T03:04:05Z\n    land_anyway: 2026-01-02T03:05:05Z\n"+
		"  - commit: inproject\n    pushed: 2026-01-02T03:06:05Z\n"+
		"  - commit: both\n    pushed: 2026-01-02T03:07:05Z\n    land_anyway: 2026-01-02T03:08:05Z\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/3.yaml"), "title: Also accepted\nstate: Accepted\nspec: 1\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n"+
		"pushed_commits:\n"+
		"  - commit: projectlater\n    pushed: 2026-01-02T03:04:05Z\n    rerun: 2026-01-02T05:00:00Z\n    reruns:\n      - id: 20\n        attempt: 1\n"+
		"  - commit: pushedlater\n    pushed: 2026-01-02T03:05:05Z\n    rerun: 2026-01-02T06:00:00Z\n    reruns:\n      - id: 30\n        attempt: 1\n      - id: 32\n        attempt: 3\n")
	untouched := "title: Never responded to\nstate: Accepted\nspec: 1\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\npushed_commits:\n  - commit: onpushed\n    pushed: 2026-01-02T03:04:05Z\n"
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/4.yaml"), untouched)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-36.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 36, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 36 with its backup", m)
	}
	after := snapshot(t, dir)
	if got := after["store.yaml"]; got != fmt.Sprintf("format: %d\n", store.Format) {
		t.Errorf("store.yaml = %q", got)
	}
	if got := after["projects/acme/tickets/4.yaml"]; got != untouched {
		t.Errorf("a Ticket with no responses changed:\n%s", got)
	}
	for _, rel := range []string{"projects/acme/tickets/2.yaml", "projects/acme/tickets/3.yaml"} {
		if strings.Contains(after[rel], "land_anyway") || strings.Contains(after[rel], "rerun") {
			t.Errorf("%s still holds a response:\n%s", rel, after[rel])
		}
	}
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	want := []store.CIResponse{
		// Only in CIResponses: kept as it was.
		{Commit: "inproject", LandAnyway: at("2026-01-02T04:00:00Z")},
		// On both: Land anyway stays.
		{Commit: "both", LandAnyway: at("2026-01-02T04:00:00Z")},
		// The Project's Re-run is the later: it and its attempts win.
		{Commit: "projectlater", Rerun: at("2026-01-02T06:00:00Z"), Reruns: []store.CIRunAttempt{{ID: 21, Attempt: 2}}},
		// The pushed commit's Re-run is the later: it and its attempts
		// win, and the fix Ticket stays.
		{Commit: "pushedlater", Rerun: at("2026-01-02T06:00:00Z"), Reruns: []store.CIRunAttempt{{ID: 30, Attempt: 1}, {ID: 32, Attempt: 3}}, FixTicket: 9},
		// Only on the pushed commit: added.
		{Commit: "onpushed", LandAnyway: at("2026-01-02T03:05:05Z")},
	}
	s.View(func(r store.Reader) error {
		if p, _ := r.Project("acme"); !reflect.DeepEqual(p.CIResponses, want) {
			t.Errorf("CI responses after migrating =\n%+v\nwant\n%+v", p.CIResponses, want)
		}
		tk, _ := r.Ticket("acme", 2)
		wantPushed := []store.PushedCommit{
			{Commit: "onpushed", Branch: "main", Pushed: at("2026-01-02T03:04:05Z")},
			{Commit: "inproject", Pushed: at("2026-01-02T03:06:05Z")},
			{Commit: "both", Pushed: at("2026-01-02T03:07:05Z")},
		}
		if !reflect.DeepEqual(tk.PushedCommits, wantPushed) {
			t.Errorf("acme-2's pushed commits after migrating = %+v, want %+v", tk.PushedCommits, wantPushed)
		}
		return nil
	})
}

// A Project with no CI responses gains one only for a pushed commit that
// held a response, and a store with none to move copies every file
// unchanged.
func TestTheLegacyCIResponsesMigrationAddsCIResponsesToAProjectWithNone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 36\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"), "key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"), "title: Accepted\nstate: Accepted\nspec: 1\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n"+
		"pushed_commits:\n  - commit: abc123\n    pushed: 2026-01-02T03:04:05Z\n    rerun: 2026-01-02T03:05:05Z\n    reruns:\n      - id: 1\n        attempt: 1\n")
	writeFile(t, filepath.Join(dir, "projects/other/project.yaml"), "key: other\ndisplay_name: Other\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	after := snapshot(t, dir)
	if got := after["projects/other/project.yaml"]; got != before["projects/other/project.yaml"] {
		t.Errorf("a Project with nothing to move changed:\n%s", got)
	}
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		want := []store.CIResponse{{Commit: "abc123", Rerun: time.Date(2026, 1, 2, 3, 5, 5, 0, time.UTC), Reruns: []store.CIRunAttempt{{ID: 1, Attempt: 1}}}}
		if !reflect.DeepEqual(p.CIResponses, want) {
			t.Errorf("CI responses after migrating = %+v, want %+v", p.CIResponses, want)
		}
		return nil
	})
}
