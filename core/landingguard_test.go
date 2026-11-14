package core

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The guard holds ADR 0039's line: no code in core or the Factory reads a
// landing setting to decide, outside the policy and the code that saves
// and shows the settings. It type-checks the packages' non-test files and
// flags every read of a guarded field on a guarded type.

// guardedFields are the four landing inputs.
var guardedFields = []string{"Method", "Push", "LandPerSpec", "Tracking"}

// guardedTypes hold the landing inputs, as package path and type name.
var guardedTypes = []string{
	"github.com/tomoram/code-central/store.Landing",
	"github.com/tomoram/code-central/store.CI",
	"github.com/tomoram/code-central/core.LandingView",
}

// guardAllow is a whole file, or one top-level function in it, that may
// read the guarded fields. File is relative to the repository's root; why
// says what the code is, for the reader.
type guardAllow struct {
	file, fn, why string
}

var landingGuardAllowlist = []guardAllow{
	{file: "core/landingpolicy.go", why: "the policy and its builders, including ProjectView.LandingPolicy()"},
	{file: "core/humanops.go", fn: "SetLandingMethod", why: "saves the Landing settings"},
	{file: "core/core.go", fn: "landingViewWith", why: "builds the LandingView"},
	{file: "core/ci.go", why: "builds, checks and saves the CI settings, and builds CIView"},
}

// landingRead is one read of a guarded field: where, which field, the
// type the field belongs to, and the top-level function it is in (empty
// outside one).
type landingRead struct {
	file  string
	line  int
	fn    string
	field string
	owner string
}

// guardedPackage is one type-checked package: its files keyed by their
// path from the repository's root.
type guardedPackage struct {
	files map[string]*ast.File
	info  *types.Info
	fset  *token.FileSet
}

// checkGuarded type-checks the files with the standard library alone,
// importing from source. Packages share imp, so each import is compiled
// once; the Factory's import of core is imp's own copy, since the
// packages core reaches it through (client) import that one too.
func checkGuarded(t *testing.T, fset *token.FileSet, imp types.Importer, path string, files map[string]*ast.File) guardedPackage {
	t.Helper()
	info := &types.Info{Selections: map[*ast.SelectorExpr]*types.Selection{}}
	var list []*ast.File
	for _, name := range slices.Sorted(maps.Keys(files)) {
		list = append(list, files[name])
	}
	if _, err := (&types.Config{Importer: imp}).Check(path, fset, list, info); err != nil {
		t.Fatalf("type-check %s: %v", path, err)
	}
	return guardedPackage{files: files, info: info, fset: fset}
}

// parseGuarded parses the non-test Go files of root/dir that build on
// this platform.
func parseGuarded(t *testing.T, fset *token.FileSet, root, dir string) map[string]*ast.File {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, dir))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		abs := filepath.Join(root, dir)
		if ok, err := build.Default.MatchFile(abs, name); err != nil {
			t.Fatal(err)
		} else if !ok {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(abs, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files[dir+"/"+name] = f
	}
	return files
}

// landingReads finds every selection of a guarded field whose type is a
// guarded type or a pointer to one. Composite-literal keys aren't
// selections, and a same-named field on another type doesn't match.
func landingReads(pkg guardedPackage) []landingRead {
	var reads []landingRead
	for _, file := range slices.Sorted(maps.Keys(pkg.files)) {
		for _, decl := range pkg.files[file].Decls {
			fn := ""
			if fd, ok := decl.(*ast.FuncDecl); ok {
				fn = fd.Name.Name
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				s, ok := pkg.info.Selections[sel]
				if !ok || s.Kind() != types.FieldVal || !slices.Contains(guardedFields, sel.Sel.Name) {
					return true
				}
				owner := fieldOwner(s)
				if !slices.Contains(guardedTypes, owner) {
					return true
				}
				reads = append(reads, landingRead{file: file, line: pkg.fset.Position(sel.Sel.Pos()).Line, fn: fn, field: sel.Sel.Name, owner: owner})
				return true
			})
		}
	}
	return reads
}

// fieldOwner names the struct type the selected field is declared in,
// following any embedded fields on the way, as package path and name.
func fieldOwner(s *types.Selection) string {
	t := s.Recv()
	idx := s.Index()
	for _, i := range idx[:len(idx)-1] {
		st, ok := deref(t).Underlying().(*types.Struct)
		if !ok {
			return ""
		}
		t = st.Field(i).Type()
	}
	named, ok := deref(t).(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return ""
	}
	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
}

func deref(t types.Type) types.Type {
	if p, ok := t.Underlying().(*types.Pointer); ok {
		return p.Elem()
	}
	return t
}

// allowed reports whether the allowlist covers the read.
func allowed(allow []guardAllow, r landingRead) bool {
	return slices.ContainsFunc(allow, func(a guardAllow) bool {
		return a.file == r.file && (a.fn == "" || a.fn == r.fn)
	})
}

// staleEntries are the allowlist entries naming a file, or a top-level
// function in it, that the packages don't have.
func staleEntries(allow []guardAllow, pkgs ...guardedPackage) []string {
	var stale []string
	for _, a := range allow {
		var f *ast.File
		for _, p := range pkgs {
			if pf, ok := p.files[a.file]; ok {
				f = pf
			}
		}
		switch {
		case f == nil:
			stale = append(stale, a.file+": no such file")
		case a.fn != "" && !slices.ContainsFunc(f.Decls, func(d ast.Decl) bool {
			fd, ok := d.(*ast.FuncDecl)
			return ok && fd.Name.Name == a.fn
		}):
			stale = append(stale, fmt.Sprintf("%s: no top-level function %s", a.file, a.fn))
		}
	}
	return stale
}

// guardFailure is the failure message for the reads the allowlist doesn't
// cover, or empty when it covers them all.
func guardFailure(allow []guardAllow, reads []landingRead) string {
	var b strings.Builder
	for _, r := range reads {
		if allowed(allow, r) {
			continue
		}
		fmt.Fprintf(&b, "\t%s:%d: %s of %s\n", r.file, r.line, r.field, r.owner)
	}
	if b.Len() == 0 {
		return ""
	}
	return "these read a landing setting to decide; ask the Landing Policy instead " +
		"(see docs/agents/recipes.md, \"Adding or changing a landing rule\"):\n" + b.String()
}

func TestNoLandingSettingIsReadOutsideThePolicy(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "source", nil)

	core := checkGuarded(t, fset, imp, "github.com/tomoram/code-central/core", parseGuarded(t, fset, root, "core"))
	factory := checkGuarded(t, fset, imp, "github.com/tomoram/code-central/factory", parseGuarded(t, fset, root, "factory"))

	if stale := staleEntries(landingGuardAllowlist, core, factory); len(stale) > 0 {
		t.Errorf("the allowlist names what isn't there:\n\t%s", strings.Join(stale, "\n\t"))
	}
	if msg := guardFailure(landingGuardAllowlist, append(landingReads(core), landingReads(factory)...)); msg != "" {
		t.Error(msg)
	}
}

// guardFixture is a package that reads a landing setting to decide, beside
// reads the guard must let through.
const guardFixture = `package fixture

import "github.com/tomoram/code-central/store"

// specView has a field named like a landing setting, on another type.
type specView struct{ LandPerSpec bool }

func decides(p store.Project, s specView) bool {
	_ = store.Landing{Method: store.LandingMerge, Push: true}
	return p.Landing.Method == store.LandingMerge && s.LandPerSpec
}

func viaPointer(c *store.CI) bool {
	return c.Tracking
}
`

func checkGuardFixture(t *testing.T) guardedPackage {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	// The file is named as if in core, so its imports resolve from there.
	f, err := parser.ParseFile(fset, filepath.Join(root, "core", "fixture.go"), guardFixture, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	return checkGuarded(t, fset, importer.ForCompiler(fset, "source", nil), "fixture", map[string]*ast.File{"fixture/fixture.go": f})
}

func TestTheLandingGuardFlagsAReadOfALandingSetting(t *testing.T) {
	pkg := checkGuardFixture(t)
	msg := guardFailure(landingGuardAllowlist, landingReads(pkg))
	want := "these read a landing setting to decide; ask the Landing Policy instead " +
		"(see docs/agents/recipes.md, \"Adding or changing a landing rule\"):\n" +
		"\tfixture/fixture.go:10: Method of github.com/tomoram/code-central/store.Landing\n" +
		"\tfixture/fixture.go:14: Tracking of github.com/tomoram/code-central/store.CI\n"
	if msg != want {
		t.Errorf("failure =\n%s\nwant\n%s", msg, want)
	}

	allow := []guardAllow{{file: "fixture/fixture.go", fn: "decides"}, {file: "fixture/fixture.go", fn: "viaPointer"}}
	if msg := guardFailure(allow, landingReads(pkg)); msg != "" {
		t.Errorf("allowlisted functions flagged:\n%s", msg)
	}
	if msg := guardFailure([]guardAllow{{file: "fixture/fixture.go"}}, landingReads(pkg)); msg != "" {
		t.Errorf("allowlisted file flagged:\n%s", msg)
	}
}

func TestTheLandingGuardFailsOnAStaleAllowlistEntry(t *testing.T) {
	pkg := checkGuardFixture(t)
	got := staleEntries([]guardAllow{
		{file: "fixture/fixture.go", fn: "decides"},
		{file: "fixture/fixture.go", fn: "gone"},
		{file: "fixture/missing.go"},
	}, pkg)
	want := []string{"fixture/fixture.go: no top-level function gone", "fixture/missing.go: no such file"}
	if !slices.Equal(got, want) {
		t.Errorf("stale = %q, want %q", got, want)
	}
}
