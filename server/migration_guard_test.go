package server_test

// A temporary guard for the migration onto ops (ADR 0063): while the
// legacy route and tool maps still serve the operations not yet defined
// in ops, every operation must be served exactly one way in each door.
// Ticket 1005 removes the legacy maps and this test with them.

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/mcp"
	"github.com/tomoram/code-central/ops"
)

func TestEveryOperationIsDefinedExactlyOneWay(t *testing.T) {
	legacyRoutes, legacyTools := httpapi.LegacyRoutes(), mcp.LegacyTools()
	for _, op := range ops.All() {
		_, defined := ops.DefinitionOf(op.Name)
		if legacy := slices.Contains(legacyRoutes, op.Name); defined == legacy {
			t.Errorf("%s: defined in ops %v, in httpapi's legacy routes %v; it must be exactly one", op.Name, defined, legacy)
		}
		if op.Audience != ops.AgentAndHuman {
			continue
		}
		if legacy := slices.Contains(legacyTools, op.Name); defined == legacy {
			t.Errorf("%s: defined in ops %v, in mcp's legacy tools %v; it must be exactly one", op.Name, defined, legacy)
		}
	}
	for _, name := range legacyRoutes {
		if _, ok := ops.Lookup(name); !ok {
			t.Errorf("httpapi's legacy routes serve %s, which ops.All() doesn't list", name)
		}
	}
	for _, name := range legacyTools {
		if _, ok := ops.Lookup(name); !ok {
			t.Errorf("mcp's legacy tools serve %s, which ops.All() doesn't list", name)
		}
	}
}
