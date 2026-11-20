// Package git is the Factory's Git port: the Factory opens each
// repository through a Git and calls it through Repo, so that its rules
// can be tested apart from real git. Real is the port through the git
// binary, which the Factory runs with.
package git

import "context"

// Git is what comes before a repository is open.
type Git interface {
	// Open is the repository whose folder is dir.
	Open(dir string) Repo
}

// Repo is one git repository as landing and the sweep use it: the seam
// that lets their decisions be tested against a fake, with real git
// tested on its own (Real).
//
// Landing's helpers (inBase, inOldBase, pushedBase, pushedAlreadyIn,
// remoteTip, mergedOnOldBase) each wrap this seam with a different ref,
// or with a Ticket's or the Factory's own record, not a different git
// question, so they stay separate rather than being gathered into a
// Git facts module.
type Repo interface {
	// Commit resolves rev to a commit, failing when it isn't one in the
	// repository.
	Commit(rev string) (string, error)
	// TreeOf is commit's tree.
	TreeOf(commit string) (string, error)
	// IsAncestor reports whether commit is reachable from rev.
	IsAncestor(commit, rev string) bool
	// MergeBase is a best common ancestor of a and b, failing when they
	// have none.
	MergeBase(a, b string) (string, error)
	// HasRemote reports whether the repository has the remote name.
	HasRemote(name string) bool
	// RemoteURL is the URL of the remote name, failing when the
	// repository hasn't it.
	RemoteURL(name string) (string, error)
	// TopLevel is the top level of the git repository holding the
	// repository's folder, failing when none does.
	TopLevel() (string, error)
	// MergeTree merges head into base without touching any working tree:
	// the merged tree, or the conflicting files.
	MergeTree(base, head string) (tree string, conflicts []string, err error)
	// CommitTree makes a commit of tree with these parents.
	CommitTree(tree, msg string, parents ...string) (string, error)
	// CommitTreeLike makes a commit of tree with these parents, keeping
	// the message and author of the commit like.
	CommitTreeLike(tree, like string, parents ...string) (string, error)
	// ReplayTree applies commit's change onto base without touching any
	// working tree, as a cherry-pick does: the resulting tree, or the
	// conflicting files.
	ReplayTree(base, commit string) (tree string, conflicts []string, err error)
	// FirstParents is the commits on rev's first-parent line that exclude
	// doesn't reach, oldest first.
	FirstParents(rev, exclude string) ([]string, error)
	// ParentsOf is commit's parents, first parent first.
	ParentsOf(commit string) ([]string, error)
	// FastForward fast-forwards the branch checked out in the worktree dir
	// to commit; out is git's output when it can't.
	FastForward(dir, commit string) (out string, err error)
	// Reset moves the branch checked out in the worktree dir from old to
	// commit, which needn't descend from old, keeping uncommitted changes
	// to files the move doesn't touch; out is git's output when changes in
	// the way stop it. A branch found moved from old meanwhile is put back
	// where it was, and reported.
	Reset(dir, commit, old string) (out string, err error)
	// UpdateRef moves ref from old to new, failing unless it is at old.
	UpdateRef(ref, new, old string) error
	// SpecBranches is the names of the branches under spec/.
	SpecBranches() ([]string, error)
	// ForkPoints is the Ticket IDs with a fork ref: the names under
	// ForkRefs.
	ForkPoints() ([]string, error)
	// DeleteRef deletes ref, failing unless it is at old.
	DeleteRef(ref, old string) error
	// Push pushes branch to remote; out is git's output when it fails. With
	// lease it forces, but only over the remote's branch at lease, as
	// `--force-with-lease=<branch>:<lease>` does; with "" it never forces.
	// A stop (ctx) kills it.
	Push(ctx context.Context, remote, branch, lease string) (out string, err error)
	// Fetch fetches remote, updating its remote-tracking refs; out is git's
	// output when it fails. A stop (ctx) kills it.
	Fetch(ctx context.Context, remote string) (out string, err error)
	// RemoteHas reports whether remote has branch now, asking it as `git
	// ls-remote` does, so a branch deleted there since the last fetch,
	// which leaves its remote-tracking ref, isn't counted. A stop (ctx)
	// kills it.
	RemoteHas(ctx context.Context, remote, branch string) (bool, error)
	// Worktrees is what `git worktree list` reports, the repo itself first.
	Worktrees() ([]Worktree, error)
	// Status is the worktree dir's uncommitted changes, untracked files
	// included: "" when it is clean.
	Status(dir string) (string, error)
	// RemoveWorktree removes the clean worktree at path.
	RemoveWorktree(path string) error
}

// Worktree is one entry of `git worktree list`.
type Worktree struct {
	// Path is the worktree's folder.
	Path string
	// Branch is the branch checked out in it; "" when it is detached.
	Branch string
}

// ForkRefs is the namespace of the Factory's fork refs, each named by a
// Ticket ID (Repo.ForkPoints).
const ForkRefs = "refs/code-central/fork/"
