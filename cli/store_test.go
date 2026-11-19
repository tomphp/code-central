package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/store/storetest"
)

// moveHome is a home whose settings hold other keys, with a File Store
// in its data folder filled with one of every record Store Move copies,
// whose contents it returns.
func moveHome(t *testing.T) (env map[string]string, source string, before storetest.Contents) {
	t.Helper()
	home, data := t.TempDir(), t.TempDir()
	writeHomeSettings(t, home, "# my settings\nport: 7790\nnotifications: off # quiet\n")
	source = filepath.Join(data, "code-central")
	s, err := store.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	storetest.Fill(t, s)
	before = storetest.ContentsOf(t, s)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return map[string]string{"HOME": home, "XDG_DATA_HOME": data}, source, before
}

func settingsText(t *testing.T, env map[string]string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(env["HOME"], ".config", "code-central", "settings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// contentsAt opens the Store at l and reads what it holds.
func contentsAt(t *testing.T, l store.Location) storetest.Contents {
	t.Helper()
	s, err := store.OpenAt(l)
	if err != nil {
		t.Fatalf("opening %s: %v", l, err)
	}
	defer s.Close()
	return storetest.ContentsOf(t, s)
}

func TestAStoreMovedToSQLiteAndBackHoldsTheSameRecordsAndUsage(t *testing.T) {
	t.Parallel()
	env, source, before := moveHome(t)
	db := filepath.Join(t.TempDir(), "central.db")

	r := run(t, env, "", "store", "move", "--to", "sqlite:"+db)
	if r.code != 0 {
		t.Fatalf("move to sqlite: exit %d: %s", r.code, r.stderr)
	}
	for _, want := range []string{"files at " + source, "sqlite at " + db, "acme: 2 Specs, 4 Tickets, 2 CI Runs, 2 Usage days", "the Options", "untouched"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the move's output doesn't say %q:\n%s", want, r.stdout)
		}
	}
	want := "# my settings\nport: 7790\nnotifications: off # quiet\nstore:\n  kind: sqlite\n  path: " + db + "\n"
	if got := settingsText(t, env); got != want {
		t.Errorf("settings.yaml after the move:\n%s\nwant:\n%s", got, want)
	}
	storetest.SameContents(t, before, contentsAt(t, store.Location{Kind: store.Files, Path: source}))

	back := filepath.Join(t.TempDir(), "store")
	r = run(t, env, "", "store", "move", "--to", "files:"+back)
	if r.code != 0 {
		t.Fatalf("move back to files: exit %d: %s", r.code, r.stderr)
	}
	if got := settingsText(t, env); !strings.Contains(got, "store:\n  kind: files\n  path: "+back+"\n") || !strings.Contains(got, "port: 7790") {
		t.Errorf("settings.yaml after moving back:\n%s", got)
	}
	storetest.SameContents(t, before, contentsAt(t, store.Location{Kind: store.Files, Path: back}))
}

func TestAStoreMoveWithoutAPathTakesTheKindsDefaultAndWritesNoPath(t *testing.T) {
	t.Parallel()
	env, source, _ := moveHome(t)
	r := run(t, env, "", "store", "move", "--to", "sqlite")
	if r.code != 0 {
		t.Fatalf("move: exit %d: %s", r.code, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(source, "code-central.db")); err != nil {
		t.Errorf("the database isn't in the data folder: %v", err)
	}
	if got := settingsText(t, env); !strings.HasSuffix(got, "store:\n  kind: sqlite\n") {
		t.Errorf("settings.yaml after the move:\n%s", got)
	}
}

func TestAStoreMoveWritesTheSettingsFileWhenThereIsNone(t *testing.T) {
	t.Parallel()
	env, _, _ := moveHome(t)
	os.Remove(filepath.Join(env["HOME"], ".config", "code-central", "settings.yaml"))
	db := filepath.Join(t.TempDir(), "central.db")
	if r := run(t, env, "", "store", "move", "--to", "sqlite:"+db); r.code != 0 {
		t.Fatalf("move: exit %d: %s", r.code, r.stderr)
	}
	if got, want := settingsText(t, env), "store:\n  kind: sqlite\n  path: "+db+"\n"; got != want {
		t.Errorf("settings.yaml:\n%s\nwant:\n%s", got, want)
	}
}

func TestAStoreMoveReplacesAnEarlierStoreSetting(t *testing.T) {
	t.Parallel()
	env, source, _ := moveHome(t)
	writeHomeSettings(t, env["HOME"], "store:\n  kind: files\n  path: "+source+"\nport: 7790\n")
	db := filepath.Join(t.TempDir(), "central.db")
	if r := run(t, env, "", "store", "move", "--to", "sqlite:"+db); r.code != 0 {
		t.Fatalf("move: exit %d: %s", r.code, r.stderr)
	}
	if got, want := settingsText(t, env), "store:\n  kind: sqlite\n  path: "+db+"\nport: 7790\n"; got != want {
		t.Errorf("settings.yaml:\n%s\nwant:\n%s", got, want)
	}
}

// refused runs a store move that must fail with exit 1, naming each of
// says, and leave settings.yaml as it was.
func refused(t *testing.T, env map[string]string, says []string, args ...string) result {
	t.Helper()
	settings := settingsText(t, env)
	r := run(t, env, "", append([]string{"store", "move"}, args...)...)
	if r.code != 1 {
		t.Fatalf("exit %d, want 1: stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	for _, s := range says {
		if !strings.Contains(r.stderr, s) {
			t.Errorf("the refusal doesn't say %q: %s", s, r.stderr)
		}
	}
	if got := settingsText(t, env); got != settings {
		t.Errorf("settings.yaml changed:\n%s", got)
	}
	return r
}

func TestAStoreMoveRefusesATargetThatIsNotEmpty(t *testing.T) {
	t.Parallel()
	env, source, before := moveHome(t)
	db := filepath.Join(t.TempDir(), "central.db")
	s, err := store.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(w store.Writer) error {
		return w.PutProject(domain.Project{Key: "other", DisplayName: "other", Mode: "committed"})
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	refused(t, env, []string{"sqlite at " + db, "isn't empty"}, "--to", "sqlite:"+db)
	storetest.SameContents(t, before, contentsAt(t, store.Location{Kind: store.Files, Path: source}))
}

func TestAStoreMoveRefusesASourceAServiceHasOpen(t *testing.T) {
	t.Parallel()
	env, source, _ := moveHome(t)
	s, err := store.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	db := filepath.Join(t.TempDir(), "central.db")
	refused(t, env, []string{source, "locked", "stop"}, "--to", "sqlite:"+db)
	if _, err := os.Stat(db); err == nil {
		t.Error("the target was made")
	}
}

// Not parallel: the copy it breaks is the package's.
func TestAStoreMoveWhoseCountsDifferStopsAndLeavesTheSettings(t *testing.T) {
	env, _, _ := moveHome(t)
	cli.AddATicketOnMove(t)
	db := filepath.Join(t.TempDir(), "central.db")
	refused(t, env, []string{"acme: 4 Tickets in the source, 5 in the target", "remove"}, "--to", "sqlite:"+db)
}

func TestAStoreMoveRefusesTheStoreItMovesFrom(t *testing.T) {
	t.Parallel()
	env, source, _ := moveHome(t)
	refused(t, env, []string{"files at " + source, "the Store serve opens"}, "--to", "files:"+source)
}

func TestAStoreMoveNamesWhatIsWrongWithItsArguments(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		args []string
		says string
	}{
		{"no --to", nil, "--to"},
		{"an unknown kind", []string{"--to", "mysql"}, "mysql"},
		{"a path for postgres", []string{"--to", "postgres:/tmp/x"}, "postgres"},
		{"--url-env for sqlite", []string{"--to", "sqlite", "--url-env", "DB"}, "--url-env"},
		{"an extra argument", []string{"--to", "sqlite", "now"}, "now"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			env, _, _ := moveHome(t)
			r := run(t, env, "", append([]string{"store", "move"}, c.args...)...)
			if r.code != 1 || !strings.Contains(r.stderr, c.says) || !strings.Contains(r.stderr, "usage: code-central store move") {
				t.Fatalf("exit %d, stderr %q; want a usage error naming %s", r.code, r.stderr, c.says)
			}
		})
	}
}

func TestAStoreMoveToPostgresWithoutItsURLNamesTheVariable(t *testing.T) {
	t.Parallel()
	env, _, _ := moveHome(t)
	refused(t, env, []string{"CC_TEST_DB"}, "--to", "postgres", "--url-env", "CC_TEST_DB")
	refused(t, env, []string{"CODE_CENTRAL_DATABASE_URL"}, "--to", "postgres")
}

// A Store moves into Postgres and back out whole; the settings name the
// variable the URL is in, never the URL.
func TestAStoreMovedToPostgresAndBackHoldsTheSameRecordsAndUsage(t *testing.T) {
	t.Parallel()
	url := storetest.FreshPostgres(t)
	env, _, before := moveHome(t)
	env["CC_TEST_DB"] = url
	r := run(t, env, "", "store", "move", "--to", "postgres", "--url-env", "CC_TEST_DB")
	if r.code != 0 {
		t.Fatalf("move to postgres: exit %d: %s", r.code, r.stderr)
	}
	if got := settingsText(t, env); !strings.HasSuffix(got, "store:\n  kind: postgres\n  url_env: CC_TEST_DB\n") || strings.Contains(got, url) {
		t.Errorf("settings.yaml after the move:\n%s", got)
	}
	if strings.Contains(r.stdout, url) {
		t.Errorf("the output shows the URL: %s", r.stdout)
	}
	back := filepath.Join(t.TempDir(), "store")
	if r := run(t, env, "", "store", "move", "--to", "files:"+back); r.code != 0 {
		t.Fatalf("move back to files: exit %d: %s", r.code, r.stderr)
	}
	storetest.SameContents(t, before, contentsAt(t, store.Location{Kind: store.Files, Path: back}))
}

// A Postgres Store takes no lock, so its live Service Registrations are
// what show a service still runs on it.
func TestAStoreMoveRefusesAPostgresSourceAServiceIsRegisteredOn(t *testing.T) {
	t.Parallel()
	url := storetest.FreshPostgres(t)
	env, _, _ := moveHome(t)
	env["CC_TEST_DB"] = url
	if r := run(t, env, "", "store", "move", "--to", "postgres", "--url-env", "CC_TEST_DB"); r.code != 0 {
		t.Fatalf("move to postgres: exit %d: %s", r.code, r.stderr)
	}
	s, err := store.OpenPostgres(url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	reg := store.Registration{ID: "a", Host: "desktop", Version: "v1.2.3", Format: store.Format, Renewed: time.Now()}
	if err := s.Services().Register(reg); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(t.TempDir(), "central.db")
	refused(t, env, []string{"desktop (v1.2.3)", "stop"}, "--to", "sqlite:"+db)
	if _, err := os.Stat(db); err == nil {
		t.Error("the target was made")
	}
}
