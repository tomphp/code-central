// Package docguard holds the test that keeps docs/agents/ from naming Go
// identifiers that no longer exist (Ticket code-central-1021).
package docguard

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	span       = regexp.MustCompile("`([^`\n]+)`")
	qualified  = regexp.MustCompile(`\b([a-z][a-z0-9]*)\.([A-Z][A-Za-z0-9_]*)`)
	called     = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\(`)
	testName   = regexp.MustCompile(`^Test[A-Z][A-Za-z0-9_]*$`)
	builtins   = map[string]bool{"init": true, "main": true, "make": true, "new": true, "len": true, "append": true}
	skipDirs   = map[string]bool{"node_modules": true, ".git": true, "dist": true, "web": true}
	rootToRepo = filepath.Join("..", "..")
)

// declarations holds every name the module's Go files declare: anywhere
// (funcs, methods, types, values, fields) and per package name.
type declarations struct {
	all   map[string]bool
	byPkg map[string]map[string]bool
}

func collect(t *testing.T) declarations {
	t.Helper()
	d := declarations{all: map[string]bool{}, byPkg: map[string]map[string]bool{}}
	fset := token.NewFileSet()
	err := filepath.WalkDir(rootToRepo, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if skipDirs[e.Name()] && path != rootToRepo {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		pkg := f.Name.Name
		if d.byPkg[pkg] == nil {
			d.byPkg[pkg] = map[string]bool{}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name != "_" {
				d.all[id.Name] = true
				d.byPkg[pkg][id.Name] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// inWeb reports whether name appears in the web sources, for a doc that
// names a TypeScript function such as useApi().
func inWeb(t *testing.T, names map[string]bool) map[string]bool {
	t.Helper()
	found := map[string]bool{}
	root := filepath.Join(rootToRepo, "web", "src")
	_ = filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || !(strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for n := range names {
			if !found[n] && strings.Contains(string(b), n) {
				found[n] = true
			}
		}
		return nil
	})
	return found
}

type ref struct {
	file string
	line int
	text string // the identifier as written
	pkg  string // set for pkg.Name references
	name string
}

// references returns the Go-looking identifiers in a backquoted span:
// pkg.Name, name( and TestName. Bare words and arguments are left alone,
// because they cannot be told from prose.
func references(file string, line int, code string) []ref {
	var out []ref
	for _, m := range qualified.FindAllStringSubmatch(code, -1) {
		if len(m[2]) == 1 { // a placeholder such as ops.X
			continue
		}
		out = append(out, ref{file, line, m[0], m[1], m[2]})
	}
	for _, m := range called.FindAllStringSubmatch(code, -1) {
		out = append(out, ref{file, line, m[1] + "(", "", m[1]})
	}
	if testName.MatchString(code) {
		out = append(out, ref{file, line, code, "", code})
	}
	return out
}

func TestAgentDocsNameOnlyIdentifiersThatExist(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(rootToRepo, "docs", "agents", "*.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no docs/agents/*.md found: %v", err)
	}
	d := collect(t)

	var missing []ref
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(nil, 1<<20)
		for line := 1; sc.Scan(); line++ {
			for _, m := range span.FindAllStringSubmatch(sc.Text(), -1) {
				for _, r := range references(file, line, m[1]) {
					if r.pkg != "" {
						// pkg.Name only counts when pkg is a Go package here.
						if scope, ok := d.byPkg[r.pkg]; ok && !scope[r.name] {
							missing = append(missing, r)
						}
						continue
					}
					if !d.all[r.name] && !builtins[r.name] {
						missing = append(missing, r)
					}
				}
			}
		}
		f.Close()
		if err := sc.Err(); err != nil {
			t.Fatal(err)
		}
	}

	// A name that is not Go may be TypeScript.
	names := map[string]bool{}
	for _, r := range missing {
		names[r.name] = true
	}
	ts := inWeb(t, names)
	for _, r := range missing {
		if r.pkg == "" && ts[r.name] {
			continue
		}
		t.Errorf("%s:%d names `%s`, which no longer exists; update the doc (or drop the backquotes if it is not Go)", r.file, r.line, r.text)
	}
}
