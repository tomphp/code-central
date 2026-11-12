package factory

import (
	"context"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// locateBase finds the Project's base on every poll, before anything is
// claimed or landed: the local branch, else a local branch created at the
// Landing remote's (after a fetch of it), else the base is missing. It
// reports current or missing to Code Central, and checks that .mcp.json is
// committed on a base found for the first time or changed. It returns
// whether landing can go on: it can't while the base is missing. A held
// .mcp.json doesn't stop landing; it only stops claims.
func (f *factory) locateBase(ctx context.Context, landing core.LandingView, policy store.LandingPolicy) bool {
	base := f.baseBranch()
	ref := "refs/heads/" + base
	tip, err := f.repository.commit(ref)
	if err != nil && f.repository.hasRemote(landing.Remote) {
		if out, ferr := f.repository.fetch(ctx, landing.Remote); ferr != nil {
			if ctx.Err() == nil {
				f.printf("warning: can't fetch %s to look for the base, %s: %s", landing.Remote, base, firstLine(out))
			}
		} else if theirs, rerr := f.repository.commit("refs/remotes/" + landing.Remote + "/" + base); rerr == nil {
			if _, berr := f.gitOut(f.repo, "branch", "--no-track", base, theirs); berr != nil {
				f.printf("warning: can't create the base, %s, from %s/%s: %v", base, landing.Remote, base, berr)
			} else {
				f.printf("base: created %s at %s from %s/%s", base, short(theirs), landing.Remote, base)
				tip, err = theirs, nil
			}
		}
	}
	if err != nil {
		if !f.baseMissing {
			f.printf("warning: the base, %s, exists neither here nor on %s: nothing lands and no branch is cut until you create it or change the Base branch in Project Options", base, landing.Remote)
		}
		f.baseMissing = true
		f.mcpBase = ""
		f.reportBaseState(ctx, core.ReportBaseInput{State: core.BaseMissing})
		return false
	}
	if f.baseMissing {
		f.printf("base: found %s again", base)
	}
	f.baseMissing = false
	f.checkMCP(base)
	if !policy.KeepsBaseLevel() {
		// Where the Factory keeps the base level, syncing it reports it.
		remote := tip
		if theirs, rerr := f.repository.commit("refs/remotes/" + landing.Remote + "/" + base); rerr == nil {
			remote = theirs
		}
		f.reportBaseState(ctx, core.ReportBaseInput{State: core.BaseCurrent, Local: tip, Remote: remote})
	}
	return true
}

// checkMCP holds new branches and worktrees while .mcp.json isn't
// committed on the base, and checks again each poll until it is. A base
// already found to have it isn't checked again.
func (f *factory) checkMCP(base string) {
	if f.mcpBase == base && !f.mcpHeld {
		return
	}
	if f.mcpCommitted() {
		if f.mcpHeld {
			f.printf(".mcp.json is committed on %s: cutting branches again", base)
		}
		f.mcpBase, f.mcpHeld = base, false
		return
	}
	if !f.mcpHeld || f.mcpBase != base {
		f.printf("warning: %s: no new branch is cut until it is", f.mcpMessage())
	}
	f.mcpBase, f.mcpHeld = base, true
}
