package factory

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/tomoram/code-central/core"
)

// The Factory's Repositories (Spec code-central-848): a registry of the
// Project's Repositories by Repository Name, rebuilt from the Project on
// every poll. The Home Repository is the git top level the Factory starts
// in; each other one has a clone at its local path, else beside the Home
// Repository, which the Factory makes when nothing is there and adopts
// when it is a clone of the Clone URL. A local-only Repository, with no
// Clone URL, is never cloned: its git repository is adopted with no
// remote check (Spec code-central-879). Anything else there, or a path
// another Repository has, is Repository missing, left untouched, and no
// new Claim starts while any Repository is missing.

// repository is one of the Project's Repositories as the Factory has it.
type repository struct {
	name string
	home bool
	// dir is where its clone is: for the Home Repository, the git top
	// level the Factory starts in.
	dir string
	// git is the seam for every git call in the clone; nil while missing.
	git repo
	// worktrees is the folder of its worktrees, beside the clone:
	// <dir>.worktrees for the Home Repository, and <dir's
	// parent>/<name>.worktrees for each other one.
	worktrees string
	// cloneURL is the Clone URL the Project lists; empty for the Home
	// Repository.
	cloneURL string
	// base is the Base the Project lists; empty for the Home Repository,
	// whose Base is the Project's (baseOf).
	base string
	// remote is its Push remote's name, as the Project lists it: its
	// own, else the Project's. Every push, fetch and remote-tracking ref
	// in its clone is of this remote. Only the poll reads it.
	remote string
	// localOnly is whether the Project lists it as local-only, with no
	// Clone URL: the Home Repository once the Factory reported its clone
	// has no Push remote (Spec code-central-890). It is never pushed or
	// fetched.
	localOnly bool
	// localOnlyUnderPullRequest is whether it is local-only under Pull
	// Request, as only a Home that lost its remote can be: nothing lands
	// in it.
	localOnlyUnderPullRequest bool
	// problem says why it is Repository missing; empty while its clone is
	// ready.
	problem string
	// cloned is whether this poll made its clone.
	cloned bool
}

// listedAs takes from v, r as the Project lists it, what every poll
// reads again: its Push remote, and whether it is local-only, and so
// under Pull Request.
func (r *repository) listedAs(v core.RepositoryView) {
	r.remote, r.localOnly, r.localOnlyUnderPullRequest = v.PushRemote, v.LocalOnly, v.LocalOnlyUnderPullRequest
}

// policy is r's Landing Policy among the Project's, policies, which every
// landing step in r asks: its Own Landing, else the Default Landing, with
// Push off in a local-only Repository (ADR 0039, Spec code-central-890).
// A function given the policies with r asks this.
func (r *repository) policy(policies core.LandingPolicies) core.LandingPolicy {
	return policies.In(r.name)
}

// where ends a line about something in r, saying where it is: " in
// Repository <name>", or nothing for the Home Repository, as the lines
// of a Project with the Home Repository alone have always said.
func (r *repository) where() string {
	if r.home {
		return ""
	}
	return " in Repository " + r.name
}

// repositories is the registry the landing and the sessions share: the
// Home Repository, the Project's Repositories by Repository Name and in
// landing order, the Project's base, and how a Ticket's or the Chat's
// worktrees are named and set up in them (worktree.go). start fills it
// in; after that only the poll's landing step writes it (replace,
// setBase), and turns and the Chat loop read it from their own
// goroutines, under its mutexes.
type repositories struct {
	// git is the git binary and environ the environment every git call of
	// the Factory's runs with; printf writes the Factory's lines.
	git     string
	environ []string
	printf  func(format string, args ...any)
	// home is the Home Repository: the git top level the Factory starts
	// in, its repo as landing and the sweep use it, and its worktree
	// folder, <repo-parent>/<repo-dir>.worktrees. start sets it, and
	// nothing changes it after.
	home *repository
	// byName is the Project's Repositories by Repository Name, home among
	// them, rebuilt from the Project on every poll, and inOrder the same
	// in landing order; mu guards both.
	mu      sync.Mutex
	byName  map[string]*repository
	inOrder []*repository
	// base is the Project's base as the last read of it said; baseMu
	// guards it, because turns read it from their own goroutines.
	baseMu sync.Mutex
	base   string
}

// newRepositories is a registry with no Repository yet, whose git calls
// run with environ and whose lines go to printf: start sets the rest.
func newRepositories(environ []string, printf func(format string, args ...any)) *repositories {
	return &repositories{environ: environ, printf: printf, byName: map[string]*repository{}}
}

// gitOut runs git in dir and returns its trimmed output.
func (rs *repositories) gitOut(dir string, args ...string) (string, error) {
	return gitOut(rs.git, rs.environ, dir, args...)
}

// newRepository is the Repository name whose clone is at dir.
func (rs *repositories) newRepository(name, dir string) *repository {
	return &repository{name: name, dir: dir, git: gitRepo{git: rs.git, dir: dir, environ: rs.environ}, worktrees: filepath.Join(filepath.Dir(dir), filepath.Base(dir)+".worktrees")}
}

// replace makes byName and inOrder the Repositories, as a sync found
// them.
func (rs *repositories) replace(byName map[string]*repository, inOrder []*repository) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.byName, rs.inOrder = byName, inOrder
}

// named is the Repository called name, as the last poll found it; nil
// when the Project has none.
func (rs *repositories) named(name string) *repository {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.byName[name]
}

// several reports whether the Project has more than one Repository, as
// the last poll found. A turn's end reads it too (the finish check), from
// its own goroutine.
func (rs *repositories) several() bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return len(rs.inOrder) > 1
}

// ready is the Project's Repositories with a clone, in landing order, as
// the last poll found them: the Home Repository alone before the first.
// The Chat loop reads them too, from its own goroutine.
func (rs *repositories) ready() []*repository {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	var out []*repository
	for _, r := range rs.inOrder {
		if r.problem == "" {
			out = append(out, r)
		}
	}
	return out
}

// anyMissing reports whether any Repository is missing, as the last poll
// found.
func (rs *repositories) anyMissing() bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, r := range rs.byName {
		if r.problem != "" {
			return true
		}
	}
	return false
}

// baseBranch is the Project's base as of the last poll's read: the Home
// Repository's Base.
func (rs *repositories) baseBranch() string {
	rs.baseMu.Lock()
	defer rs.baseMu.Unlock()
	return rs.base
}

// setBase takes the base the Project names, returning the one before.
func (rs *repositories) setBase(base string) (old string) {
	rs.baseMu.Lock()
	defer rs.baseMu.Unlock()
	old, rs.base = rs.base, base
	return old
}

// baseOf is r's Base: the Project's for the Home Repository.
func (rs *repositories) baseOf(r *repository) string {
	if r.home {
		return rs.baseBranch()
	}
	return r.base
}

// inBaseOf reports whether commit is in r's base.
func (rs *repositories) inBaseOf(r *repository, commit string) bool {
	return r.git.isAncestor(commit, "refs/heads/"+rs.baseOf(r))
}

// syncRepositories rebuilds the registry from p's Repositories: it
// clones each Repository with nothing at its path, adopts each matching
// clone, and reports every clone to Code Central. A Project with the
// Home Repository alone has nothing to clone or report, once a report
// has cleared any from when it had more.
func (f *factory) syncRepositories(ctx context.Context, p core.ProjectView) {
	next := map[string]*repository{}
	var inOrder []*repository
	var report []core.RepositoryReport
	// taken is each path an earlier Repository, or the Home, resolves
	// to, with whose it is: a later one there is Repository missing.
	taken := map[string]string{}
	for _, v := range p.Repositories {
		if v.Home {
			taken[f.repos.home.dir] = v.Name
		}
	}
	for _, v := range p.Repositories {
		r := f.repos.home
		if v.Home {
			r.listedAs(v)
		} else {
			r = f.cloneOf(ctx, v, taken)
			if ctx.Err() != nil {
				// A stop cut the clone off: it isn't missing.
				return
			}
			was := f.repos.named(v.Name)
			f.sayClone(r, was)
			if r.problem == "" && (was == nil || was.problem != "") {
				// As the Home Repository's are at start.
				f.repos.prune(r)
			}
		}
		f.sayLocalOnlyUnderPullRequest(r)
		next[v.Name] = r
		inOrder = append(inOrder, r)
		state := core.CloneReady
		if r.problem != "" {
			state = core.CloneMissing
		}
		report = append(report, core.RepositoryReport{Name: v.Name, Path: r.dir, State: state, Problem: r.problem})
	}
	f.repos.replace(next, inOrder)
	// Back to the Home Repository alone, one more report clears the last.
	several := len(p.Repositories) > 1
	if !several && !f.reportedSeveral {
		return
	}
	f.reportedSeveral = several
	if _, err := f.client.ReportRepositories(ctx, f.project, core.ReportRepositoriesInput{Repositories: report}); err != nil && ctx.Err() == nil {
		if !f.reportRepositoriesFailing {
			f.printf("warning: can't report the Repositories' clones to Code Central: %v", err)
		}
		f.reportRepositoriesFailing = true
		return
	}
	f.reportRepositoriesFailing = false
}

// cloneOf is the Repository v with its clone: cloned under its Push
// remote's name when its path holds nothing, adopted when the path holds
// a clone whose Push remote is v's Clone URL, and otherwise missing,
// with the path untouched. A local-only Repository, with no Clone URL, is
// never cloned: whatever git repository of its own is at its path is
// adopted with no remote check (Spec code-central-879). A path in taken
// is another Repository's, so missing too; v's path is added to it.
func (f *factory) cloneOf(ctx context.Context, v core.RepositoryView, taken map[string]string) *repository {
	dir := v.LocalPath
	if dir == "" {
		dir = filepath.Join(filepath.Dir(f.repos.home.dir), v.Name)
	}
	r := f.repos.newRepository(v.Name, dir)
	r.cloneURL, r.base = v.CloneURL, v.Base
	r.listedAs(v)
	remote := r.remote
	// Named after the Repository, whatever its local path's folder is
	// called (ADR 0048).
	r.worktrees = filepath.Join(filepath.Dir(dir), v.Name+".worktrees")
	missing := func(format string, args ...any) *repository {
		r.git, r.problem = nil, fmt.Sprintf(format, args...)
		return r
	}
	for path, other := range taken {
		if samePath(path, dir) {
			return missing("%s is Repository %s's clone", dir, other)
		}
	}
	taken[dir] = v.Name
	switch _, err := os.Lstat(dir); {
	case errors.Is(err, fs.ErrNotExist) && v.CloneURL == "":
		return missing("nothing at %s", dir)
	case errors.Is(err, fs.ErrNotExist):
		if out, err := cloneRepository(ctx, f.repos.git, f.repos.environ, remote, v.CloneURL, dir); err != nil {
			if out == "" {
				out = err.Error()
			}
			return missing("git clone failed: %s", firstLine(out))
		}
		r.cloned = true
		return r
	case err != nil:
		return missing("can't read %s: %v", dir, err)
	}
	top, err := r.git.topLevel()
	if err != nil {
		return missing("%s isn't a git repository", dir)
	}
	if !samePath(top, dir) {
		return missing("%s is inside the git repository at %s, not a clone of its own", dir, top)
	}
	if v.CloneURL == "" {
		return r
	}
	url, err := r.git.remoteURL(remote)
	if err != nil {
		return missing("%s has no remote %s, the Push remote", dir, remote)
	}
	if core.NormalCloneURL(url) != core.NormalCloneURL(v.CloneURL) {
		return missing("%s's remote %s is %s, not %s", dir, remote, url, v.CloneURL)
	}
	return r
}

// sayClone says what became of r's clone since was, the registry's entry
// before this poll: cloned, adopted, or newly missing.
func (f *factory) sayClone(r, was *repository) {
	switch {
	case r.problem != "":
		if was == nil || was.problem != r.problem {
			f.printf("warning: Repository %s is missing: %s: no new Claim starts until it has a clone", r.name, r.problem)
		}
	case r.cloned:
		f.printf("repository %s: cloned %s to %s", r.name, r.cloneURL, r.dir)
	case was == nil || was.problem != "" || was.dir != r.dir:
		f.printf("repository %s: adopted the clone at %s", r.name, r.dir)
	}
}

// sayLocalOnlyUnderPullRequest says once that r, a Home that lost its
// remote, as only it can be, is local-only under Pull Request, which
// holds back the Tickets landing in it (held), and says so again should
// it be once more after it was put right.
func (f *factory) sayLocalOnlyUnderPullRequest(r *repository) {
	if !r.localOnlyUnderPullRequest {
		delete(f.saidLocalOnly, r.name)
		return
	}
	if f.saidLocalOnly[r.name] {
		return
	}
	f.saidLocalOnly[r.name] = true
	f.printf("warning: Repository %s is local-only under Pull Request: nothing lands in it until you give it another Landing in Project Options, or add its Push remote, %s, to its clone and restart the Factory", r.name, r.remote)
}

// samePath reports whether a and b name one folder, through any
// symlinks.
func samePath(a, b string) bool {
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
