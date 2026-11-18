package cli

import (
	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/ops"
)

// Starting a Retro.
func init() {
	commands["submit-retro"] = derive(ops.SubmitRetro, (*client.Client).SubmitRetro, "start a Retro on a finished Ticket or Spec")
}
