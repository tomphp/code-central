// Package importguard holds the test that keeps the clients of the domain
// model from importing the store (Spec code-central-925).
package importguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const module = "github.com/tomoram/code-central"

// goList returns the non-test imports each package names directly.
func goList(t *testing.T, pkg string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-f", `{{join .Imports "\n"}}`, pkg).Output()
	if err != nil {
		t.Fatalf("go list %s: %v", pkg, err)
	}
	return strings.Fields(string(out))
}

// These packages name the domain model's records through domain, so a
// direct import of store would pull persistence into a client.
func TestClientsDoNotImportStore(t *testing.T) {
	for _, pkg := range []string{
		"httpapi", "client", "mcp", "server", "internal/wiretypes",
		"factory", "factory/factorytest", "cmd/code-central-factory",
		"ciwatcher", "issuewatcher", "pullrequestwatcher", "workitemwatcher",
	} {
		t.Run(pkg, func(t *testing.T) {
			for _, imp := range goList(t, "../../"+pkg) {
				if imp == module+"/store" {
					t.Errorf("%s imports store; name the record through domain instead", pkg)
				}
			}
		})
	}
}

// world and codecentraltest build a store, so they may import it, but
// only for NewMemory and Store.
func TestTestSupportUsesStoreOnlyForMemory(t *testing.T) {
	for _, dir := range []string{"../../world", "../../codecentraltest"} {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			files, err := filepath.Glob(filepath.Join(dir, "*.go"))
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range files {
				if strings.HasSuffix(file, "_test.go") {
					continue
				}
				for _, use := range storeSelectors(t, file) {
					if use != "NewMemory" && use != "Store" {
						t.Errorf("%s uses store.%s; only NewMemory and Store belong here", file, use)
					}
				}
			}
		})
	}
}

// storeSelectors lists the names a file takes from the store package.
func storeSelectors(t *testing.T, file string) []string {
	t.Helper()
	src, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var uses []string
	ast.Inspect(src, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "store" {
				uses = append(uses, sel.Sel.Name)
			}
		}
		return true
	})
	return uses
}

// In cli only the commands that open the store (serve, service, and
// store move, ADR 0070) import it.
func TestOnlyServeCommandsImportStoreInCLI(t *testing.T) {
	opens := map[string]bool{"serve.go": true, "service.go": true, "store.go": true}
	files, err := filepath.Glob("../../cli/*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		base := filepath.Base(file)
		for _, imp := range src.Imports {
			if strings.Trim(imp.Path.Value, `"`) == module+"/store" && !opens[base] {
				t.Errorf("cli/%s imports store; only serve.go, service.go and store.go may", base)
			}
		}
	}
}

// domain is a leaf: any client may import it, so it may import no
// Code Central package.
func TestDomainImportsNothingOfCodeCentral(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "../../domain").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.HasPrefix(pkg, module+"/") && pkg != module+"/domain" {
			t.Errorf("domain depends on %s; it may import nothing of Code Central", pkg)
		}
	}
}
