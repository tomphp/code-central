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
		"httpapi", "mcp", "server", "internal/wiretypes", "cli", "factory/factorytest",
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

// The clients of the service link neither core nor store, however
// indirectly: they talk to the service over HTTP and name the wire
// through api. factory/factorytest is not among them: it is test tooling
// that composes the real service in process (codecentraltest), so it links
// both by design.
func TestClientsDoNotLinkCoreOrStore(t *testing.T) {
	for _, pkg := range []string{
		"client", "factory", "cmd/code-central-factory",
		"ciwatcher", "issuewatcher", "pullrequestwatcher", "workitemwatcher",
		"gh", "azdo", "ratelimit", "watchers", "notify",
	} {
		t.Run(pkg, func(t *testing.T) {
			deps := goDeps(t, "../../"+pkg)
			for _, banned := range []string{module + "/core", module + "/store"} {
				if _, ok := deps[banned]; !ok {
					continue
				}
				t.Errorf("%s links %s via %s", pkg, banned, strings.Join(importChain(deps, module+"/"+pkg, banned), " -> "))
			}
		})
	}
}

// api is a near-leaf: it may import only domain and internal/fieldwalk.
func TestAPIImportsOnlyDomainAndFieldwalk(t *testing.T) {
	for pkg := range goDeps(t, "../../api") {
		if !strings.HasPrefix(pkg, module+"/") {
			continue
		}
		switch pkg {
		case module + "/api", module + "/domain", module + "/internal/fieldwalk":
		default:
			t.Errorf("api depends on %s; it may import only domain and internal/fieldwalk", pkg)
		}
	}
}

// goDeps maps each package a package depends on, itself included, to
// the packages it imports directly.
func goDeps(t *testing.T, pkg string) map[string][]string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", "-f", `{{.ImportPath}} {{join .Imports " "}}`, pkg).Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	deps := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			deps[fields[0]] = fields[1:]
		}
	}
	return deps
}

// importChain returns a shortest path of imports from one package to
// another through deps.
func importChain(deps map[string][]string, from, to string) []string {
	prev := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == to {
			var chain []string
			for p := to; p != ""; p = prev[p] {
				chain = append([]string{p}, chain...)
			}
			return chain
		}
		for _, next := range deps[cur] {
			if _, seen := prev[next]; !seen {
				prev[next] = cur
				queue = append(queue, next)
			}
		}
	}
	return []string{from, to}
}
