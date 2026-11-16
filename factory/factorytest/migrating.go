package factorytest

// TEMPORARY: the helpers the migration to Settle deletes, kept until no
// test uses them. This file goes before the work is finished.

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

func (h *Harness) WaitForState(id string, state store.State) core.TicketView {
	h.t.Helper()
	var tk core.TicketView
	h.WaitFor(fmt.Sprintf("%s to be %s", id, state), func() bool {
		tk = h.Ticket(id)
		if tk.State != state && (tk.State == store.Failed || tk.State == store.Cancelled) {
			h.t.Fatalf("waiting for %s to be %s, it is %s%s", id, state, tk.State, lastFailure(tk))
		}
		return tk.State == state
	})
	return tk
}

func (r *Running) WaitForOutput(s string) {
	r.h.t.Helper()
	r.h.WaitFor(fmt.Sprintf("%q on stdout (have %q, stderr %q)", s, r.stdout, r.stderr), func() bool {
		return strings.Contains(r.stdout.String(), s)
	})
}

func (r *Running) WaitForLongPoll() { r.h.t.Helper(); r.Settle() }

func (r *Running) WaitForTurn(what string) {
	r.h.t.Helper()
	r.h.WaitFor(what, func() bool { return len(r.h.Calls()) > 0 })
}

func (r *Running) WaitForClaimed(id string, state store.State) {
	r.h.t.Helper()
	r.h.WaitForState(id, state)
}

func (h *Harness) WaitForWorktree(ticket string) {
	h.t.Helper()
	git, err := realGit()
	must(h.t, err)
	h.WaitFor("the worktree of "+ticket, func() bool {
		cmd := exec.Command(git, "rev-parse", "HEAD")
		cmd.Dir = h.Worktree(ticket)
		cmd.Env = h.Environ()
		return cmd.Run() == nil
	})
}
