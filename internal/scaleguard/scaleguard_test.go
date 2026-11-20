// Package scaleguard holds the test that keeps the Scale tier's tests apart
// from the fast suite (Spec code-central-1144): a scale test lives in a
// *_scale_test.go file built with the scale tag, and nothing else does.
package scaleguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

var skipDirs = map[string]bool{"node_modules": true, ".git": true, "dist": true, "web": true}

func TestTheTreesScaleTestsAreTaggedAndDocumented(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range check(t, os.DirFS(root)) {
		t.Error(p)
	}
}

func TestTheGuardCatchesEachBreach(t *testing.T) {
	const tag = "//go:build scale\n\n"
	for name, tc := range map[string]struct {
		files fstest.MapFS
		want  string
	}{
		"an untagged TestScale": {
			fstest.MapFS{"a/a_test.go": file("", "// Doc.\nfunc TestScaleX(t *testing.T) {}")},
			"outside a *_scale_test.go",
		},
		"an untagged scale file": {
			fstest.MapFS{"a/a_scale_test.go": file("", "// Doc.\nfunc TestScaleX(t *testing.T) {}")},
			"scale build constraint",
		},
		"a missing doc comment": {
			fstest.MapFS{"a/a_scale_test.go": file(tag, "func TestScaleX(t *testing.T) {}")},
			"doc comment",
		},
		"a non-TestScale test in a scale file": {
			fstest.MapFS{"a/a_scale_test.go": file(tag, "// Doc.\nfunc TestOther(t *testing.T) {}")},
			"is not named TestScale",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := strings.Join(check(t, tc.files), "\n")
			if !strings.Contains(got, tc.want) {
				t.Fatalf("problems = %q, want one mentioning %q", got, tc.want)
			}
		})
	}
	good := fstest.MapFS{
		"a/a_scale_test.go": file("//go:build e2e && scale\n\n", "// Doc.\nfunc TestScaleX(t *testing.T) {}\n\nfunc helper() {}"),
	}
	if got := check(t, good); len(got) != 0 {
		t.Fatalf("a compliant file was flagged: %v", got)
	}
}

func file(constraint, body string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(constraint + "package a\n\nimport \"testing\"\n\n" + body + "\n")}
}

// check returns one problem per breach of the Scale tier's rules in the
// Go test files under fsys.
func check(t *testing.T, fsys fs.FS) []string {
	t.Helper()
	var problems []string
	fset := token.NewFileSet()
	err := fs.WalkDir(fsys, ".", func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if skipDirs[e.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		isScale := strings.HasSuffix(path, "_scale_test.go")
		if isScale && !hasScaleConstraint(f) {
			problems = append(problems, path+": a *_scale_test.go file needs a scale build constraint")
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") {
				continue
			}
			scaleName := strings.HasPrefix(fn.Name.Name, "TestScale")
			switch {
			case scaleName && !isScale:
				problems = append(problems, path+": "+fn.Name.Name+" is outside a *_scale_test.go file")
			case !scaleName && isScale:
				problems = append(problems, path+": "+fn.Name.Name+" is not named TestScale… but is in a scale file")
			case scaleName && fn.Doc == nil:
				problems = append(problems, path+": "+fn.Name.Name+" needs a doc comment stating its budget and why it is a scale test")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return problems
}

// hasScaleConstraint reports whether the file's //go:build line names scale.
func hasScaleConstraint(f *ast.File) bool {
	for _, g := range f.Comments {
		if g.Pos() >= f.Package {
			break
		}
		for _, c := range g.List {
			if rest, ok := strings.CutPrefix(c.Text, "//go:build "); ok {
				for _, w := range strings.FieldsFunc(rest, func(r rune) bool { return strings.ContainsRune(" ()&|!", r) }) {
					if w == "scale" {
						return true
					}
				}
			}
		}
	}
	return false
}
