package factory

import (
	"context"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/core"
)

// The Factory's landing part (Spec code-central-934): on each poll it
// reads the Project and its Landing Policy, keeps the Repositories'
// clones and bases current, lands Accepted work, per Ticket and per Spec,
// detects hand merges and merged pull requests, and sweeps the worktrees
// and Spec Branches the Factory is done with. It reports all of it
// through the Client. It holds no *factory, so it can't reach turns,
// Claims, Chat or Heartbeats: the session part calls poll once per poll,
// asks claimingHeld before claiming, and answers the two landerHooks.

// landerHooks is what the landing part asks of the sessions.
type landerHooks struct {
	// stopped reports whether the Factory is halted, draining or paused:
	// nothing more lands this poll.
	stopped func(ctx context.Context) bool
	// turnRunningOn reports whether a turn is running on ticket, whose
	// worktrees the sweep keeps.
	turnRunningOn func(ticket string) bool
}

// lander is the landing part. Its state is the poll's own: once start
// has set it up, only poll and claimingHeld, both on the poll loop, read
// or write it.
type lander struct {
	// repos is the registry the sessions share, which landing alone
	// writes (replace, setBase); its git calls are landing's.
	repos   *repositories
	client  *client.Client
	project string
	// work hears of the landing's work in flight, as the sessions' does,
	// and printf writes its lines to the Factory's output.
	work   work
	printf func(format string, args ...any)
	hooks  landerHooks

	// reportedSeveral is whether the last report of the Repositories'
	// clones was of several, and reportRepositoriesFailing whether it
	// failed, so a failure warns once.
	reportedSeveral           bool
	reportRepositoriesFailing bool
	// warned is the dirty finished worktrees, and the finished Specs'
	// Spec Branches kept for not being in their base, already said.
	warned map[string]bool
	// warnedMerged is "<ticket>@<commit>" of Tickets merged in a State
	// that can't land, already warned of.
	warnedMerged map[string]bool
	// leftOnBranch is "<ticket>@<commit>" of Accepted Tickets left for the
	// human to merge under the Branch Landing Method, already said.
	leftOnBranch map[string]bool
	// saidHeld is "<ticket>@<why>" of Accepted Tickets a broken base or a
	// missing clone holds back from landing, already said.
	saidHeld map[string]bool
	// oldBases are the bases the Project named earlier in this run.
	oldBases []string
	// bases is what the Factory knows of each Repository's base, by
	// Repository Name: whether it is missing, has diverged or was level
	// with the remote's this poll, and the merges made into it that
	// haven't landed yet.
	bases map[string]*baseState
	// mcpBase is the base whose .mcp.json was last found committed, and
	// mcpHeld whether the file is missing from the base, so no branch or
	// worktree is cut.
	mcpBase string
	mcpHeld bool
	// reportBaseFailing is whether the last report_base failed, so a
	// failure warns once.
	reportBaseFailing bool
	// reportedRemote is the Home's Push remote name whose URL, or its
	// absence, was last reported to Code Central, and remoteReported
	// whether one was; reportRemoteFailing is whether the last report
	// failed, so a failure warns once.
	reportedRemote      string
	remoteReported      bool
	reportRemoteFailing bool
	// saidLocalOnly is the Repositories said to be local-only under Pull
	// Request, and not put right since.
	saidLocalOnly map[string]bool
	// saidOnCI is "<ticket>@<pushed commit>@<CI result>" of Tickets the
	// CI gate holds, already said.
	saidOnCI map[string]bool
	// unreported is, by "<ticket>@<Repository Name>", the commit a push
	// under Merge + Push left that Repository's remote base holding, whose
	// report as the Ticket's pushed commit there failed, to report next
	// poll.
	unreported map[string]string
}

// newLander is the landing part of project's Factory over the registry
// repos, reporting through c, with nothing found out yet: start sets the
// .mcp.json state it finds.
func newLander(repos *repositories, c *client.Client, project string, w work, printf func(format string, args ...any), h landerHooks) *lander {
	return &lander{repos: repos, client: c, project: project, work: w, printf: printf, hooks: h, warned: map[string]bool{}, warnedMerged: map[string]bool{}, leftOnBranch: map[string]bool{}, saidHeld: map[string]bool{}, bases: map[string]*baseState{}, saidLocalOnly: map[string]bool{}, saidOnCI: map[string]bool{}, unreported: map[string]string{}}
}

// poll is a poll's landing: it reads the Landing Method and the
// Repositories, reports the Home's Push remote when it needs to, makes
// sure each Repository has a clone, lands what it can unless the Factory
// is stopped, and sweeps the worktrees the Factory is done with. ok says
// whether the Project was read, and p is it.
func (l *lander) poll(ctx context.Context) (p core.ProjectView, ok bool) {
	p, ok = l.landing(ctx)
	if ok {
		p = l.reportPushRemote(ctx, p)
		l.syncRepositories(ctx, p)
		l.locateBases(ctx, p)
	}
	if ok && !l.hooks.stopped(ctx) {
		l.land(ctx, p)
	}
	policies := p.LandingPolicies()
	l.sweep(ctx, policies)
	l.sweepSpecBranches(ctx, policies)
	return p, ok
}

// claimingHeld reports whether nothing may be claimed, as every claim
// cuts a branch or a worktree from each Repository's base: a base
// disagrees with the host's or is missing, the Home Repository's lacks
// .mcp.json, or a Repository is missing, so a Ticket would start without
// a checkout of every Repository.
func (l *lander) claimingHeld() bool {
	return l.anyBaseHeld() || l.mcpHeld || l.repos.anyMissing()
}
