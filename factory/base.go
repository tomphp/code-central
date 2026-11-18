package factory

import (
	"context"
	"errors"
	"fmt"

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
func (l *lander) stateOf(r *repository) *baseState {
	s := l.bases[r.name]
	if s == nil {
		s = &baseState{mergedOn: map[string]mergeOn{}}
		l.bases[r.name] = s
	}
	if !r.home && s.branch != r.base {
		s.changedBase()
		s.branch = r.base
	}
	return s
}

// held says why nothing touching r lands this poll under policies, the
// Project's Repositories': it is local-only under Pull Request, its base
// is missing, or under r's Merge + Push isn't level with the remote's. It
// is "" when r's base is ready.
func (l *lander) held(r *repository, policies core.LandingPolicies) string {
	s := l.stateOf(r)
	switch {
	case r.localOnlyUnderPullRequest:
		return "it is local-only under Pull Request"
	case s.missing:
		return "its base, " + l.repos.baseOf(r) + ", is missing"
	case r.policy(policies).PushesBase() && !s.current:
		return "its base, " + l.repos.baseOf(r) + ", isn't level with the remote's"
	}
	return ""
}

// anyBaseHeld reports whether any Repository's base is missing or has
// diverged, as the last poll found, so no Claim starts.
func (l *lander) anyBaseHeld() bool {
	for _, r := range l.repos.ready() {
		if s := l.bases[r.name]; s != nil && (s.missing || s.diverged) {
			return true
		}
	}
	return false
}

// locateBases finds every Repository's base, Home first, as locateBase
// does each.
func (l *lander) locateBases(ctx context.Context, p core.ProjectView) {
	for _, r := range l.repos.ready() {
		l.locateBase(ctx, p, r)
	}
}

// locateBase finds r's base on every poll, before anything is claimed or
// landed: the local branch, else a local branch created at its Push
// remote's (after a fetch of it), else the base is missing. A local-only
// Repository's remotes, if it has any, aren't the Repository's, so it is
// never fetched and its base is the local branch or missing (Spec
// code-central-879). It reports current or missing to Code Central, and
// for the Home Repository checks that .mcp.json is committed on a base
// found for the first time or changed. Nothing touching r lands while its
// base is missing. A held .mcp.json doesn't stop landing; it only stops
// claims.
func (l *lander) locateBase(ctx context.Context, p core.ProjectView, r *repository) {
	s := l.stateOf(r)
	base := l.repos.baseOf(r)
	ref := "refs/heads/" + base
	localOnly, remote := r.localOnly, r.remote
	tip, err := r.git.commit(ref)
	if err != nil && !localOnly && r.git.hasRemote(remote) {
		if out, ferr := r.git.fetch(ctx, remote); ferr != nil {
			if ctx.Err() == nil {
				l.printf("warning: can't fetch %s to look for the base, %s%s: %s", remote, base, r.where(), firstLine(out))
			}
		} else if theirs, rerr := r.git.commit("refs/remotes/" + remote + "/" + base); rerr == nil {
			if _, berr := l.repos.gitOut(r.dir, "branch", "--no-track", base, theirs); berr != nil {
				l.printf("warning: can't create the base, %s, from %s/%s%s: %v", base, remote, base, r.where(), berr)
			} else {
				l.printf("base: created %s at %s from %s/%s%s", base, short(theirs), remote, base, r.where())
				tip, err = theirs, nil
			}
		}
	}
	if err != nil {
		if !s.missing {
			switch {
			case r.home && localOnly:
				l.printf("warning: the base, %s, doesn't exist here, and this repository is local-only: nothing lands and no branch is cut until you create it or change the Base branch in Project Options", base)
			case r.home:
				l.printf("warning: the base, %s, exists neither here nor on %s: nothing lands and no branch is cut until you create it or change the Base branch in Project Options", base, remote)
			case localOnly:
				l.printf("warning: the base, %s, doesn't exist in Repository %s, which is local-only: nothing touching it lands and no branch is cut until you create it or change its Base in Project Options", base, r.name)
			default:
				l.printf("warning: the base, %s, exists neither in Repository %s nor on its %s: nothing touching it lands and no branch is cut until you create it or change its Base in Project Options", base, r.name, remote)
			}
		}
		s.missing = true
		if r.home {
			l.mcpBase = ""
		}
		l.reportBaseState(ctx, r, core.ReportBaseInput{State: core.BaseMissing})
		return
	}
	if s.missing {
		l.printf("base: found %s again%s", base, r.where())
	}
	s.missing = false
	if r.home {
		l.checkMCP(base)
	}
	if localOnly || !r.policy(p.LandingPolicies()).KeepsBaseLevel() {
		// Where the Factory keeps the base level, syncing it reports it.
		// Elsewhere, a local-only Repository's included, it never
		// diverges.
		s.diverged = false
		theirs := tip
		if t, rerr := r.git.commit("refs/remotes/" + remote + "/" + base); rerr == nil && !localOnly {
			theirs = t
		}
		l.reportBaseState(ctx, r, core.ReportBaseInput{State: core.BaseCurrent, Local: tip, Remote: theirs})
	}
}

// checkMCP holds new branches and worktrees while .mcp.json isn't
// committed on the base, and checks again each poll until it is. A base
// already found to have it isn't checked again.
func (l *lander) checkMCP(base string) {
	if l.mcpBase == base && !l.mcpHeld {
		return
	}
	if l.mcpCommitted() {
		if l.mcpHeld {
			l.printf(".mcp.json is committed on %s: cutting branches again", base)
		}
		l.mcpBase, l.mcpHeld = base, false
		return
	}
	if !l.mcpHeld || l.mcpBase != base {
		l.printf("warning: %s: no new branch is cut until it is", l.mcpMessage())
	}
	l.mcpBase, l.mcpHeld = base, true
}

// checkMCPAtStart is start's check of the base it has set: a base the
// Home Repository has must have .mcp.json committed, or the Factory
// doesn't start. One it hasn't yet is left to checkMCP on each poll.
func (l *lander) checkMCPAtStart() error {
	base := l.repos.baseBranch()
	if _, err := l.repos.gitOut(l.repos.home.dir, "rev-parse", "--verify", "-q", base+"^{commit}"); err != nil {
		return nil
	}
	if !l.mcpCommitted() {
		return errors.New(l.mcpMessage())
	}
	l.mcpBase = base
	return nil
}

// mcpCommitted is whether .mcp.json is committed on the base.
func (l *lander) mcpCommitted() bool {
	_, err := l.repos.gitOut(l.repos.home.dir, "cat-file", "-e", l.repos.baseBranch()+":.mcp.json")
	return err == nil
}

// mcpMessage says that .mcp.json isn't committed on the base, and why it
// must be.
func (l *lander) mcpMessage() string {
	b := l.repos.baseBranch()
	return fmt.Sprintf(".mcp.json isn't committed on %s: every worktree starts from %s, so its sessions need it; commit it first", b, b)
}

// setBase takes the base the Project names this poll into the registry.
// A change of base says so and forgets what was known of the old one.
func (l *lander) setBase(base string) {
	if old := l.repos.setBase(base); old != base {
		l.printf("base: %s, was %s", base, old)
		l.stateOf(l.repos.home).changedBase()
		l.oldBases = append(l.oldBases, old)
	}
}
