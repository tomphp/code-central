package store_test

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// These tests are the Migration chain over a SQLite Store (Spec
// code-central-1032, seam 3): the Store is staged as a File Store
// layout, the chain runs over it, and the result is loaded into a new
// database swapped in after a VACUUM INTO backup.

func openSQLiteFormat(t *testing.T, path string, format int, steps ...store.MigrationStep) *store.SQLStore {
	t.Helper()
	s, err := store.OpenSQLiteFormat(path, format, steps)
	if err != nil {
		t.Fatalf("open as format %d: %v", format, err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedSQLiteFormat1 writes a format-1 SQLite Store with a Project, a
// Spec and a Ticket, and closes it.
func seedSQLiteFormat1(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "code-central.db")
	s := openSQLiteFormat(t, path, 1)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutProject(domain.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed"})
	})
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutSpec("acme", domain.Spec{Number: 1, Title: "Login", Created: t0, Updated: t0, Body: "# Login\n"})
	})
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutTicket("acme", domain.Ticket{Number: 2, Title: "Form", State: domain.Ready, Spec: 1, Created: t0, Updated: t0,
			Moves: []domain.Move{{From: domain.Draft, To: domain.Ready, Actor: "agent", Time: t0}}}, store.Moves)
	})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// renaming is a step that renames Ticket Form in its file, so a test can
// see the chain ran.
func renaming(from int) store.MigrationStep {
	return store.MigrationStep{From: from, Name: "rename Form", File: func(rel string, data []byte) ([]byte, error) {
		return bytes.Replace(data, []byte("title: Form\n"), []byte("title: Form (migrated)\n"), 1), nil
	}}
}

// dump is every record s holds, History included, as text to compare.
func dump(t *testing.T, s store.Store) string {
	t.Helper()
	var b bytes.Buffer
	if err := s.View(func(r store.Reader) error {
		fmt.Fprintf(&b, "options %+v\n", r.Options())
		for _, p := range r.Projects() {
			fmt.Fprintf(&b, "project %+v\n", p)
			for rec := range r.Records(p.Key) {
				if rec.Spec != nil {
					fmt.Fprintf(&b, "spec %+v\n", *rec.Spec)
				}
				if rec.Ticket != nil {
					fmt.Fprintf(&b, "ticket %+v\n", *rec.Ticket)
				}
			}
			fmt.Fprintf(&b, "ci runs %+v\n", r.CIRuns(p.Key))
		}
		return nil
	}); err != nil {
		t.Fatalf("read: %v", err)
	}
	return b.String()
}

func ticketTitle(t *testing.T, s store.Store, n int) string {
	t.Helper()
	var title string
	if err := s.View(func(r store.Reader) error {
		tk, ok := r.Ticket("acme", n)
		if !ok {
			t.Fatalf("no ticket %d", n)
		}
		title = tk.Title
		return nil
	}); err != nil {
		t.Fatalf("read: %v", err)
	}
	return title
}

func TestAnOlderSQLiteStoreIsMigratedThroughTheChainAndBackedUp(t *testing.T) {
	path := seedSQLiteFormat1(t)
	old := openSQLiteFormat(t, path, 1)
	before := dump(t, old)
	old.Close()

	s := openSQLiteFormat(t, path, 2, renaming(1))

	bak := path + ".format-1.bak"
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 1, To: 2, Backup: bak}) {
		t.Fatalf("Migrated() = %+v", m)
	}
	if got := ticketTitle(t, s, 2); got != "Form (migrated)" {
		t.Fatalf("ticket 2's title = %q: the chain did not run", got)
	}

	// The migrated Store is writable, and keeps its writes at format 2.
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutTicket("acme", domain.Ticket{Number: 3, Title: "More", State: domain.Draft, Spec: 1, Created: t0, Updated: t0})
	})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got := siblings(t, path); fmt.Sprint(got) != "[code-central.db code-central.db.format-1.bak code-central.db.lock]" {
		t.Fatalf("next to the database: %v (want no staging left)", got)
	}
	again := openSQLiteFormat(t, path, 2)
	if m := again.Migrated(); m != nil {
		t.Fatalf("reopening at format 2 migrated again: %+v", m)
	}
	if got := ticketTitle(t, again, 3); got != "More" {
		t.Fatalf("ticket 3 after reopening = %q", got)
	}

	// The backup is the old Store, at the old format.
	backup := openSQLiteFormat(t, bak, 1)
	if got := dump(t, backup); got != before {
		t.Fatalf("the backup is not the old Store:\n%s\nwant\n%s", got, before)
	}
}

// writeFormat46 writes a File Store layout at format 46 into dir: a
// Project whose GitHub Issues setting is the one repository format 47
// made a list, a Spec, two Tickets (one with History, one with its parts
// left out, as older files may), the Options, CI Runs and a day's Usage.
func writeFormat46(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 46\n")
	writeFile(t, filepath.Join(dir, "options.yaml"), "gh_path: /opt/bin/gh\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n"+
			"github_issues:\n  on: true\n  repository: acme/web\n  label: good first issue\n  new_issues_need_me: true\n  spec: 1\n")
	writeFile(t, filepath.Join(dir, "projects/acme/specs/1.md"),
		"---\nid: 1\ntitle: \"Login: the form\"\nlabels: [auth]\ntags: {effort: high}\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-03T03:04:05Z\nedits: []\n---\n# Login\n\nA body with \"quotes\".\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: Form\nstate: Ready\nspec: 1\nlabels: [ui]\ntags: {}\nblocked_by: [3]\nreplacements: []\n"+
			"created: 2026-01-02T03:04:05Z\nupdated: 2026-01-04T03:04:05Z\ndescription: |\n  Line one.\n  Line two.\n"+
			"claims: []\nrounds: []\nfailure_reports: []\n"+
			"comments:\n  - number: 1\n    actor: human\n    time: 2026-01-03T03:04:05Z\n    body: |\n      Looks good.\n"+
			"moves:\n  - from: Draft\n    to: Ready\n    actor: agent\n    time: 2026-01-03T03:04:05Z\nedits: []\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/3.yaml"),
		"title: \"true\"\nstate: Draft\nspec: 1\nlabels: []\ntags: {}\nblocked_by: []\nreplacements: []\n"+
			"created: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\nmoves: []\n")
	writeFile(t, filepath.Join(dir, "projects/acme/ci-runs.yaml"),
		"runs:\n  - id: 7\n    attempt: 1\n    workflow: ci.yml\n    name: CI\n    branch: main\n    commit: abc123\n    status: completed\n    conclusion: success\n    url: https://example.com/runs/7\n")
	writeFile(t, filepath.Join(dir, "projects/acme/usage/2026-01-03.jsonl"),
		`{"type":"mcp_call","time":"2026-01-03T03:04:05Z","op":"get_ticket","id":"acme-2","request_bytes":20,"response_bytes":300}`+"\n"+
			`{"type":"mcp_call","time":"2026-01-03T03:05:05Z","op":"finish","request_bytes":40,"response_bytes":60,"error":"stale_claim"}`+"\n")
}

// The real chain migrates a format-46 SQLite Store to the records the
// same fixture migrates to as a File Store, Usage included.
func TestTheRealChainMigratesAFormat46SQLiteStoreAsItDoesTheFileStore(t *testing.T) {
	files := filepath.Join(t.TempDir(), "code-central")
	writeFormat46(t, files)
	fixture := filepath.Join(t.TempDir(), "fixture")
	writeFormat46(t, fixture)
	path := filepath.Join(t.TempDir(), "code-central.db")
	store.LoadSQLiteFixture(t, fixture, path)

	want := openFile(t, files)
	s := openSQLite(t, path)

	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 46, To: store.Format, Backup: path + ".format-46.bak"}) {
		t.Fatalf("Migrated() = %+v", m)
	}
	if got, w := dump(t, s), dump(t, want); got != w {
		t.Fatalf("the migrated SQLite Store holds\n%s\nwant, as the File Store migrated\n%s", got, w)
	}
	if !bytes.Contains([]byte(dump(t, s)), []byte("Repositories:[acme/web]")) {
		t.Fatalf("the format 47 step did not run:\n%s", dump(t, s))
	}
	days := []string{"2026-01-03"}
	got, err := s.Usage().Read("", days)
	if err != nil {
		t.Fatal(err)
	}
	w, err := want.Usage().Read("", days)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", w) || len(got.Calls) != 2 {
		t.Fatalf("usage after migrating = %+v, want %+v", got, w)
	}
}

// A Migration that fails, wherever it fails, leaves the original database
// as it was, at its format, and nothing beside it: no new database, no
// staging and no backup.
func TestAFailedSQLiteMigrationLeavesTheOriginalUntouched(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T)
		steps []store.MigrationStep
		want  string
	}{
		{name: "a step fails", steps: []store.MigrationStep{{From: 1, Name: "break tickets", File: func(rel string, data []byte) ([]byte, error) {
			if bytes.Contains([]byte(rel), []byte("/tickets/")) {
				return nil, errors.New("cannot rewrite")
			}
			return data, nil
		}}}, want: `step "break tickets" failed on projects/acme/tickets/2.yaml`},
		{name: "a step leaves a record this build refuses", steps: []store.MigrationStep{{From: 1, Name: "unknown key", File: func(rel string, data []byte) ([]byte, error) {
			if bytes.Contains([]byte(rel), []byte("/tickets/")) {
				return append(data, "surprise: 1\n"...), nil
			}
			return data, nil
		}}}, want: "surprise"},
		{name: "no step from the format", want: "no migration from format 1"},
		{name: "the swap fails", setup: func(t *testing.T) { store.FailSQLiteMigrationBeforeSwap(t, errors.New("disk full")) },
			steps: []store.MigrationStep{renaming(1)}, want: "disk full"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := seedSQLiteFormat1(t)
			old := openSQLiteFormat(t, path, 1)
			before := dump(t, old)
			old.Close()
			if c.setup != nil {
				c.setup(t)
			}

			_, err := store.OpenSQLiteFormat(path, 2, c.steps)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("open: %v, want an error naming %q", err, c.want)
			}
			if got := siblings(t, path); fmt.Sprint(got) != "[code-central.db code-central.db.lock]" {
				t.Fatalf("next to the database: %v, want only it and its lock", got)
			}
			again := openSQLiteFormat(t, path, 1)
			if got := dump(t, again); got != before {
				t.Fatalf("the database changed:\n%s\nwant\n%s", got, before)
			}
		})
	}
}

// A backup left by an earlier Migration is never overwritten: the
// Migration refuses, naming it, and the database is untouched.
func TestASQLiteMigrationRefusesWhenItsBackupExists(t *testing.T) {
	path := seedSQLiteFormat1(t)
	writeFile(t, path+".format-1.bak", "an earlier backup")

	_, err := store.OpenSQLiteFormat(path, 2, []store.MigrationStep{renaming(1)})
	if err == nil || !strings.Contains(err.Error(), "format-1.bak already exists") {
		t.Fatalf("open: %v, want a refusal naming the backup", err)
	}
	if got := readFile(t, path+".format-1.bak"); got != "an earlier backup" {
		t.Fatalf("the backup was overwritten: %q", got)
	}
	if got := ticketTitle(t, openSQLiteFormat(t, path, 1), 2); got != "Form" {
		t.Fatalf("ticket 2 = %q, want the database untouched", got)
	}
}

// A database at a format newer than this build's is refused, by name,
// and left as it is.
func TestANewerSQLiteStoreIsRefusedAndLeftAsItIs(t *testing.T) {
	path := seedSQLiteFormat1(t)

	_, err := store.OpenSQLiteFormat(path, 0, nil)
	if err == nil || !strings.Contains(err.Error(), "code-central.db: unknown format 1 (this code-central understands format 0)") {
		t.Fatalf("open by an older build: %v, want a refusal naming the database and both formats", err)
	}
	if got := siblings(t, path); fmt.Sprint(got) != "[code-central.db code-central.db.lock]" {
		t.Fatalf("next to the database: %v, want no backup", got)
	}
	if got := ticketTitle(t, openSQLiteFormat(t, path, 1), 2); got != "Form" {
		t.Fatalf("ticket 2 = %q, want the database untouched", got)
	}
}
