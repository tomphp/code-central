package cli

import (
	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/ops"
)

// Landing Accepted work (ADR 0006).
func init() {
	commands["land"] = derive(ops.Land, (*client.Client).Land, "the work is in the base of a Changed Repository, the Home Repository's unless named: records the commit that put it there, and once it has landed in every one, Accepted or In Review to Done")
}
