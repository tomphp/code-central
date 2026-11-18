package factory_test

import (
	"os/exec"
	"strings"
	"testing"
)

// The Factory is a client of the service, so it must not link the command
// shell (docs/architecture.md, anti-pattern 1).
func TestFactoryDoesNotImportCLI(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".", "../cmd/code-central-factory").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.HasSuffix(pkg, "/code-central/cli") {
			t.Errorf("factory links %s; resolve through client instead", pkg)
		}
	}
}
