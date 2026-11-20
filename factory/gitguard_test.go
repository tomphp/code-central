package factory_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The git guard holds Spec code-central-1148's line: every git call of
// the Factory's goes through the Git port (factory/git), so the Factory's
// own files run no git. It parses the package's non-test files, on every
// OS's build, and flags each process they start but the ones allowed, and
// any gitOut, the helper that ran git before the port.

// gitGuardAllow is a process the Factory may start: the program, as
// written, in file; why says what it is, for the reader.
type gitGuardAllow struct {
	file, program, why string
}

var gitGuardAllowlist = []gitGuardAllow{
	{file: "claudehelp.go", program: "f.claude", why: "reads claude --help"},
	{file: "claudeargs.go", program: "f.claude", why: "checks the Claude Args"},
	{file: "claudeturn.go", program: "r.claude", why: "runs a turn"},
	{file: "process_unix.go", program: "ps", why: "lists the processes"},
}

func TestTheFactoryRunsGitOnlyThroughTheGitPort(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Ident:
				if n.Name == "gitOut" {
					t.Errorf("%s: gitOut runs git outside the Git port: add the operation to git.Repo or git.Git", fset.Position(n.Pos()))
				}
			case *ast.CallExpr:
				program, ok := started(n)
				if ok && !slices.ContainsFunc(gitGuardAllowlist, func(a gitGuardAllow) bool { return a.file == name && a.program == program }) {
					t.Errorf("%s: the Factory starts %s itself: run git through the Git port, or allow another program in gitGuardAllowlist", fset.Position(n.Pos()), program)
				}
			}
			return true
		})
	}
}

// started is the program call starts, as written, when it is
// exec.Command or exec.CommandContext.
func started(call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "exec" {
		return "", false
	}
	arg := 0
	switch sel.Sel.Name {
	case "Command":
	case "CommandContext":
		arg = 1
	default:
		return "", false
	}
	if len(call.Args) <= arg {
		return "", false
	}
	return types.ExprString(call.Args[arg]), true
}
