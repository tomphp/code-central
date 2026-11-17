package factory

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// baseState is what the Factory knows of one Repository's base from poll
// to poll (Spec code-central-848: each Repository's base is located,
// synced and advanced apart, and a broken one holds back only the work
// touching it).
type baseState struct {
	// missing is whether the base exists neither locally nor on the Push
	// remote, as the last poll found.
	missing bool
	// diverged is whether the local base holds commits the Push remote's
	// doesn't (and, under Merge + Push, the remote's holds ones it doesn't
	// and the Factory couldn't move the local base onto it), so nothing
	// touching the Repository lands and no branch is cut. current is
	// whether it was level with the remote's this poll, fast-forwarded or
	// moved if need be.
	diverged, current bool
	// branch is the base these are of, for a Repository other than the
	// Home Repository, whose base setBase follows.
	branch string
	// mergedOn is where the Factory merged each Ticket it hasn't landed
	// yet, by Ticket ID, so a change of base leaves it to finish there.
	mergedOn map[string]mergeOn
}

// changedBase forgets what was known of the base before it changed,
// keeping the merges made into the old one to finish there.
func (s *baseState) changedBase() { s.diverged, s.current = false, false }

// stateOf is what the Factory knows of r's base. A Repository other than
// the Home Repository whose Base has changed starts afresh, but for its
// merges.
func (f *factory) stateOf(r *repository) *baseState {
	s := f.bases[r.name]
	if s == nil {
		s = &baseState{mergedOn: map[string]mergeOn{}}
		f.bases[r.name] = s
	}
	if !r.home && s.branch != r.base {
		s.changedBase()
		s.branch = r.base
	}
	return s
}

// held says why nothing touching r lands this poll under policy, the
// Project's: its base is missing, or under r's Merge + Push isn't level
// with the remote's. It is "" when r's base is ready.
func (f *factory) held(r *repository, policy core.LandingPolicy) string {
	s := f.stateOf(r)
	switch {
	case s.missing:
		return "its base, " + f.baseOf(r) + ", is missing"
	case r.policy(policy).PushesBase() && !s.current:
		return "its base, " + f.baseOf(r) + ", isn't level with the remote's"
	}
	return ""
}

// anyBaseHeld reports whether any Repository's base is missing or has
// diverged, as the last poll found, so no Claim starts.
func (f *factory) anyBaseHeld() bool {
	for _, r := range f.readyRepositories() {
		if s := f.bases[r.name]; s != nil && (s.missing || s.diverged) {
			return true
		}
	}
	return false
}

// locateBases finds every Repository's base, Home first, as locateBase
// does each.
func (f *factory) locateBases(ctx context.Context, p core.ProjectView) {
	for _, r := range f.readyRepositories() {
		f.locateBase(ctx, p, r)
	}
}

// locateBase finds r's base on every poll, before anything is claimed or
// landed: the local branch, else a local branch created at the Landing
// remote's (after a fetch of it), else the base is missing. A local-only
// Repository's remotes, if it has any, aren't the Repository's, so it is
// never fetched and its base is the local branch or missing (Spec
// code-central-879). It reports current or missing to Code Central, and
// for the Home Repository checks that .mcp.json is committed on a base
// found for the first time or changed. Nothing touching r lands while its
// base is missing. A held .mcp.json doesn't stop landing; it only stops
// claims.
func (f *factory) locateBase(ctx context.Context, p core.ProjectView, r *repository) {
	s := f.stateOf(r)
	base := f.baseOf(r)
	ref := "refs/heads/" + base
	localOnly := r.view().LocalOnly()
	tip, err := r.git.commit(ref)
	if err != nil && !localOnly && r.git.hasRemote(p.Landing.Remote) {
		if out, ferr := r.git.fetch(ctx, p.Landing.Remote); ferr != nil {
			if ctx.Err() == nil {
				f.printf("warning: can't fetch %s to look for the base, %s%s: %s", p.Landing.Remote, base, r.where(), firstLine(out))
			}
		} else if theirs, rerr := r.git.commit("refs/remotes/" + p.Landing.Remote + "/" + base); rerr == nil {
			if _, berr := f.gitOut(r.dir, "branch", "--no-track", base, theirs); berr != nil {
				f.printf("warning: can't create the base, %s, from %s/%s%s: %v", base, p.Landing.Remote, base, r.where(), berr)
			} else {
				f.printf("base: created %s at %s from %s/%s%s", base, short(theirs), p.Landing.Remote, base, r.where())
				tip, err = theirs, nil
			}
		}
	}
	if err != nil {
		if !s.missing {
			switch {
			case r.home:
				f.printf("warning: the base, %s, exists neither here nor on %s: nothing lands and no branch is cut until you create it or change the Base branch in Project Options", base, p.Landing.Remote)
			case localOnly:
				f.printf("warning: the base, %s, doesn't exist in Repository %s, which is local-only: nothing touching it lands and no branch is cut until you create it or change its Base in Project Options", base, r.name)
			default:
				f.printf("warning: the base, %s, exists neither in Repository %s nor on its %s: nothing touching it lands and no branch is cut until you create it or change its Base in Project Options", base, r.name, p.Landing.Remote)
			}
		}
		s.missing = true
		if r.home {
			f.mcpBase = ""
		}
		f.reportBaseState(ctx, r, core.ReportBaseInput{State: core.BaseMissing})
		return
	}
	if s.missing {
		f.printf("base: found %s again%s", base, r.where())
	}
	s.missing = false
	if r.home {
		f.checkMCP(base)
	}
	if !r.policy(p.LandingPolicy()).KeepsBaseLevel() {
		// Where the Factory keeps the base level, syncing it reports it.
		// Elsewhere it never diverges.
		s.diverged = false
		remote := tip
		if theirs, rerr := r.git.commit("refs/remotes/" + p.Landing.Remote + "/" + base); rerr == nil && !localOnly {
			remote = theirs
		}
		f.reportBaseState(ctx, r, core.ReportBaseInput{State: core.BaseCurrent, Local: tip, Remote: remote})
	}
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
