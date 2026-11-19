//go:build !js

package store

import (
	"bytes"
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// OpenSQLiteFormat opens path as a build whose format is format and whose
// migration chain is steps, so tests can migrate to a test-only format.
func OpenSQLiteFormat(path string, format int, steps []MigrationStep) (*SQLStore, error) {
	return openSQLite(path, format, steps)
}

// LoadSQLiteFixture writes the File Store layout at dir, at whatever
// format its store.yaml holds, into a new SQLite Store at path, each file
// as its row's document without decoding it, as a code-central of that
// format would have kept it. The indexed columns are left empty: nothing
// but a Migration reads a database at an older format.
func LoadSQLiteFixture(t testing.TB, dir, path string) {
	t.Helper()
	loadFixture(t, dir, "sqlite", sqliteDSN(path), sqlite)
}

// LoadPostgresFixture is LoadSQLiteFixture into the empty Postgres Store
// at url.
func LoadPostgresFixture(t testing.TB, dir, url string) {
	t.Helper()
	loadFixture(t, dir, "pgx", url, postgres)
}

// loadFixture is LoadSQLiteFixture into the empty database at dsn,
// opened with driver, in dialect d.
func loadFixture(t testing.TB, dir, driver, dsn string, d dialect) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range d.schema() {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, d.rebind(q), args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	doc := func(rel string, data []byte) (*yaml.Node, string) {
		t.Helper()
		var n yaml.Node
		if err := yaml.Unmarshal(data, &n); err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		return n.Content[0], jsonText(t, n.Content[0])
	}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		parts := strings.Split(rel, "/")
		n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(parts[len(parts)-1], ".yaml"), ".md"))
		switch {
		case rel == "store.yaml":
			var f storeFile
			if _, err := decodeStrict(data, &f); err != nil {
				return err
			}
			exec(`INSERT INTO meta (key, value) VALUES ('format', ?)`, strconv.Itoa(f.Format))
		case rel == "options.yaml":
			_, j := doc(rel, data)
			exec(`INSERT INTO options (id, doc) VALUES (1, ?)`, j)
		case len(parts) == 3 && parts[2] == "project.yaml":
			_, j := doc(rel, data)
			exec(`INSERT INTO projects (key, doc) VALUES (?, ?)`, parts[1], j)
		case len(parts) == 3 && parts[2] == ciRunsName:
			_, j := doc(rel, data)
			exec(`INSERT INTO ci_runs (project, doc) VALUES (?, ?)`, parts[1], j)
		case len(parts) == 4 && parts[2] == "specs":
			front, body, ok := splitFrontmatter(data)
			if !ok {
				t.Fatalf("%s: no frontmatter", rel)
			}
			node, _ := doc(rel, front)
			putKey(node, "body", stringNode(string(body)))
			exec(`INSERT INTO specs (project, number, created, updated, link_integration, link_id, doc) VALUES (?, ?, 0, 0, '', '', ?)`,
				parts[1], n, jsonText(t, node))
		case len(parts) == 4 && parts[2] == "tickets":
			node, _ := doc(rel, data)
			args := []any{parts[1], n}
			for _, part := range History {
				v := takeKey(node, string(part))
				if v == nil {
					v = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
				}
				args = append(args, jsonText(t, v))
			}
			args = append(args, jsonText(t, node))
			exec(`INSERT INTO tickets (project, number, state, spec, created, updated, link_integration, link_id, head,
				claims, rounds, comments, moves, edits, failure_reports, doc) VALUES (?, ?, '', 0, 0, 0, '', '', '{}', ?, ?, ?, ?, ?, ?, ?)`, args...)
		case len(parts) == 4 && parts[2] == "usage":
			for line := range bytes.SplitSeq(data, []byte("\n")) {
				if len(bytes.TrimSpace(line)) > 0 {
					typ, doc := usageRow(line)
					exec(`INSERT INTO usage_records (project, day, type, doc) VALUES (?, ?, ?, ?)`, parts[1], strings.TrimSuffix(parts[3], ".jsonl"), typ, string(doc))
				}
			}
		default:
			t.Fatalf("%s: not a file of a File Store", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// jsonText is the node tree n as a JSON document's text.
func jsonText(t testing.TB, n *yaml.Node) string {
	t.Helper()
	j, err := jsonOf(n)
	if err != nil {
		t.Fatal(err)
	}
	return string(j)
}

// FailSQLiteMigrationBeforeSwap makes the next SQLite Migrations fail with
// err once the backup is written, just before the new database is swapped
// in.
func FailSQLiteMigrationBeforeSwap(t interface{ Cleanup(func()) }, err error) {
	failSQLiteMigrationBeforeSwap = err
	t.Cleanup(func() { failSQLiteMigrationBeforeSwap = nil })
}
