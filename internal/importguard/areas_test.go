package importguard

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

const (
	corePkg       = module + "/core"
	foundationPkg = corePkg + "/foundation"
)

// areas lists the packages under core that each own part of what *Core
// once did: every package under core but the shared foundation and the
// test fixture.
func areas(t *testing.T) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "./core/...")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list ./core/...: %v", err)
	}
	var found []string
	for _, pkg := range strings.Fields(string(out)) {
		if !slices.Contains([]string{corePkg, foundationPkg, corePkg + "/coretest"}, pkg) {
			found = append(found, pkg)
		}
	}
	return found
}

// imports maps every package of the module to what it imports directly,
// not counting its tests.
func imports(t *testing.T) map[string][]string {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "./...")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list ./...: %v", err)
	}
	all := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			all[fields[0]] = fields[1:]
		}
	}
	return all
}

func TestThereAreAreasToGuard(t *testing.T) {
	if len(areas(t)) == 0 {
		t.Fatal("core has no area under it: the guard would pass for nothing")
	}
}

// An area imports neither core nor another area, only the foundation, so
// the areas can be built, tested and reasoned about one at a time.
func TestAnAreaImportsOnlyTheFoundation(t *testing.T) {
	all, as := imports(t), areas(t)
	for _, area := range as {
		for _, imp := range all[area] {
			if imp == corePkg || slices.Contains(as, imp) {
				t.Errorf("%s imports %s; an area may import only %s", area, imp, foundationPkg)
			}
		}
	}
}

func TestTheFoundationImportsNeitherCoreNorAnArea(t *testing.T) {
	all, as := imports(t), areas(t)
	for _, imp := range all[foundationPkg] {
		if imp == corePkg || slices.Contains(as, imp) {
			t.Errorf("foundation imports %s; it sits under both", imp)
		}
	}
}

// What serves the operations sits above core, so nothing under core
// links it.
func TestNothingUnderCoreLinksTheServingPackages(t *testing.T) {
	banned := []string{"ops", "ops/bind", "server", "httpapi", "mcp"}
	for _, pkg := range append(areas(t), foundationPkg) {
		t.Run(strings.TrimPrefix(pkg, corePkg+"/"), func(t *testing.T) {
			deps := goDeps(t, "../../"+strings.TrimPrefix(pkg, module+"/"))
			for _, b := range banned {
				if _, ok := deps[module+"/"+b]; ok {
					t.Errorf("%s links %s via %s", pkg, b, strings.Join(importChain(deps, pkg, module+"/"+b), " -> "))
				}
			}
		})
	}
}

// Core composes the areas and ops/bind binds their operations. The test
// worlds build them whole, and app wires a setting into one.
func TestOnlyTheCompositionPackagesImportAnArea(t *testing.T) {
	allowed := []string{"core", "ops/bind", "world", "codecentraltest", "core/coretest", "app"}
	for pkg, imps := range imports(t) {
		for _, area := range areas(t) {
			if slices.Contains(imps, area) && !slices.Contains(allowed, strings.TrimPrefix(pkg, module+"/")) {
				t.Errorf("%s imports %s; only %s may", pkg, area, strings.Join(allowed, ", "))
			}
		}
	}
}
