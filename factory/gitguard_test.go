package factory_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The git guard holds Spec code-central-1148's line: every git call of
// the Factory's goes through the Git port (factory/git), so the Factory's
// own files run no git. It parses the non-test files of the package and
// of its Harnesses, on every OS's build, and flags each process they
// start but the ones allowed,
// however os/exec is imported, exec.Command taken as a value,
// os.StartProcess, and any gitOut, the helper that ran git before the
// port.

// gitGuardAllow is a process the Factory may start: the program, as
// written, in file; why says what it is, for the reader.
type gitGuardAllow struct {
	file, program, why string
}

var gitGuardAllowlist = []gitGuardAllow{
	{file: "harness/claude/discover.go", program: "h.o.Path", why: "reads claude --help"},
	{file: "harness/claude/args.go", program: "h.o.Path", why: "checks the Claude Args"},
	{file: "harness/claude/claude.go", program: "h.o.Path", why: "runs a turn"},
	{file: "process_unix.go", program: "ps", why: "lists the processes"},
}

func TestTheFactoryRunsGitOnlyThroughTheGitPort(t *testing.T) {
	var files []string
	for _, pattern := range []string{"*.go", "harness/*.go", "harness/*/*.go"} {
		matched, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, matched...)
	}
	fset := token.NewFileSet()
	for _, path := range files {
		name := filepath.ToSlash(path)
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		execName, osName := importedAs(file, "os/exec"), importedAs(file, "os")
		called := map[ast.Expr]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Ident:
				if n.Name == "gitOut" {
					t.Errorf("%s: gitOut runs git outside the Git port: add the operation to git.Repo or git.Git", fset.Position(n.Pos()))
				}
			case *ast.CallExpr:
				program, ok := started(n, execName)
				if !ok {
					break
				}
				called[n.Fun] = true
				if !slices.ContainsFunc(gitGuardAllowlist, func(a gitGuardAllow) bool { return a.file == name && a.program == program }) {
					t.Errorf("%s: the Factory starts %s itself: run git through the Git port, or allow another program in gitGuardAllowlist", fset.Position(n.Pos()), program)
				}
			case *ast.SelectorExpr:
				switch {
				case isCommand(n, execName) && !called[n]:
					t.Errorf("%s: %s is taken as a value, so the guard can't see what it starts: call it directly", fset.Position(n.Pos()), types.ExprString(n))
				case selects(n, osName, "StartProcess"):
					t.Errorf("%s: the Factory starts a process with os.StartProcess: run git through the Git port, or use exec.Command and allow it in gitGuardAllowlist", fset.Position(n.Pos()))
				}
			}
			return true
		})
	}
}

// importedAs is the name file refers to the package at path by, "" when
// it doesn't import it.
func importedAs(file *ast.File, path string) string {
	for _, spec := range file.Imports {
		if p, _ := strconv.Unquote(spec.Path.Value); p != path {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name
		}
		return path[strings.LastIndex(path, "/")+1:]
	}
	return ""
}

// selects is whether sel is pkg.name, pkg being an imported package's
// name in the file.
func selects(sel *ast.SelectorExpr, pkg, name string) bool {
	id, ok := sel.X.(*ast.Ident)
	return ok && pkg != "" && id.Name == pkg && sel.Sel.Name == name
}

// isCommand is whether sel is os/exec's Command or CommandContext, the
// package named execName.
func isCommand(sel *ast.SelectorExpr, execName string) bool {
	return selects(sel, execName, "Command") || selects(sel, execName, "CommandContext")
}

// started is the program call starts, as written, when it is
// exec.Command or exec.CommandContext, os/exec being named execName.
func started(call *ast.CallExpr, execName string) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isCommand(sel, execName) {
		return "", false
	}
	arg := 0
	if sel.Sel.Name == "CommandContext" {
		arg = 1
	}
	if len(call.Args) <= arg {
		return "", false
	}
	return types.ExprString(call.Args[arg]), true
}
