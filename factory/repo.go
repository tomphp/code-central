package factory

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// repo is the git repository as landing and the sweep use it: the seam
// that lets their decisions be tested against a fake, with real git
// tested on its own (gitRepo).
type repo interface {
	// commit resolves rev to a commit, failing when it isn't one in the
	// repository.
	commit(rev string) (string, error)
	// isAncestor reports whether commit is reachable from rev.
	isAncestor(commit, rev string) bool
	// hasRemote reports whether the repository has the remote name.
	hasRemote(name string) bool
	// mergeTree merges head into base without touching any working tree:
	// the merged tree, or the conflicting files.
	mergeTree(base, head string) (tree string, conflicts []string, err error)
	// commitTree makes a commit of tree with these parents.
	commitTree(tree, msg string, parents ...string) (string, error)
	// commitTreeLike makes a commit of tree with these parents, keeping
	// the message and author of the commit like.
	commitTreeLike(tree, like string, parents ...string) (string, error)
	// replayTree applies commit's change onto base without touching any
	// working tree, as a cherry-pick does: the resulting tree, or the
	// conflicting files.
	replayTree(base, commit string) (tree string, conflicts []string, err error)
	// firstParents is the commits on rev's first-parent line that exclude
	// doesn't reach, oldest first.
	firstParents(rev, exclude string) ([]string, error)
	// parentsOf is commit's parents, first parent first.
	parentsOf(commit string) ([]string, error)
	// fastForward fast-forwards the branch checked out in the worktree dir
	// to commit; out is git's output when it can't.
	fastForward(dir, commit string) (out string, err error)
	// reset moves the branch checked out in the worktree dir from old to
	// commit, which needn't descend from old, keeping uncommitted changes
	// to files the move doesn't touch; out is git's output when changes in
	// the way stop it. A branch found moved from old meanwhile is put back
	// where it was, and reported.
	reset(dir, commit, old string) (out string, err error)
	// updateRef moves ref from old to new, failing unless it is at old.
	updateRef(ref, new, old string) error
	// specBranches is the names of the branches under spec/.
	specBranches() ([]string, error)
	// deleteRef deletes ref, failing unless it is at old.
	deleteRef(ref, old string) error
	// push pushes branch to remote, never forcing; out is git's output
	// when it fails. A stop (ctx) kills it.
	push(ctx context.Context, remote, branch string) (out string, err error)
	// fetch fetches remote, updating its remote-tracking refs; out is git's
	// output when it fails. A stop (ctx) kills it.
	fetch(ctx context.Context, remote string) (out string, err error)
	// worktrees is what `git worktree list` reports, the repo itself first.
	worktrees() ([]gitWorktree, error)
	// status is the worktree dir's uncommitted changes, untracked files
	// included: "" when it is clean.
	status(dir string) (string, error)
	// removeWorktree removes the clean worktree at path.
	removeWorktree(path string) error
}

// gitRepo is the repository at dir, through the git binary.
type gitRepo struct {
	git     string
	dir     string
	environ []string
}

// gitOut runs git in dir and returns its trimmed output.
func gitOut(git string, environ []string, dir string, args ...string) (string, error) {
	cmd := exec.Command(git, args...)
	cmd.Dir = dir
	cmd.Env = environ
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func (g gitRepo) out(args ...string) (string, error) { return gitOut(g.git, g.environ, g.dir, args...) }

func (g gitRepo) commit(rev string) (string, error) {
	return g.out("rev-parse", "--verify", "-q", rev+"^{commit}")
}

func (g gitRepo) isAncestor(commit, rev string) bool {
	_, err := g.out("merge-base", "--is-ancestor", commit, rev)
	return err == nil
}

func (g gitRepo) hasRemote(name string) bool {
	_, err := g.out("remote", "get-url", name)
	return err == nil
}

func (g gitRepo) mergeTree(base, head string) (tree string, conflicts []string, err error) {
	return g.mergeTreeWith(nil, base, head)
}

func (g gitRepo) replayTree(base, commit string) (tree string, conflicts []string, err error) {
	return g.mergeTreeWith([]string{"--merge-base=" + commit + "^"}, base, commit)
}

// mergeTreeWith is merge-tree of base and head with these extra options.
func (g gitRepo) mergeTreeWith(options []string, base, head string) (tree string, conflicts []string, err error) {
	args := append([]string{"merge-tree", "--write-tree", "--name-only", "--no-messages"}, options...)
	cmd := exec.Command(g.git, append(args, base, head)...)
	cmd.Dir = g.dir
	cmd.Env = g.environ
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return "", nil, fmt.Errorf("git merge-tree: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	tree = lines[0]
	if err == nil {
		return tree, nil, nil
	}
	for _, l := range lines[1:] {
		if l == "" {
			break
		}
		conflicts = append(conflicts, l)
	}
	return tree, conflicts, nil
}

func (g gitRepo) commitTree(tree, msg string, parents ...string) (string, error) {
	return g.out(commitTreeArgs(tree, msg, parents)...)
}

// commitTreeArgs is the commit-tree command for tree with these parents
// and message.
func commitTreeArgs(tree, msg string, parents []string) []string {
	args := []string{"commit-tree", tree}
	for _, p := range parents {
		args = append(args, "-p", p)
	}
	return append(args, "-m", msg)
}

func (g gitRepo) commitTreeLike(tree, like string, parents ...string) (string, error) {
	// The author's name, email and date, then the message.
	info, err := g.out("log", "-1", "--format=%an%n%ae%n%aD%n%B", like)
	if err != nil {
		return "", err
	}
	fields := strings.SplitN(info, "\n", 4)
	if len(fields) != 4 {
		return "", fmt.Errorf("git log: can't read %s: %q", like, info)
	}
	environ := append(slices.Clone(g.environ), "GIT_AUTHOR_NAME="+fields[0], "GIT_AUTHOR_EMAIL="+fields[1], "GIT_AUTHOR_DATE="+fields[2])
	return gitOut(g.git, environ, g.dir, commitTreeArgs(tree, fields[3], parents)...)
}

func (g gitRepo) firstParents(rev, exclude string) ([]string, error) {
	out, err := g.out("rev-list", "--first-parent", "--reverse", rev, "^"+exclude)
	return strings.Fields(out), err
}

func (g gitRepo) parentsOf(commit string) ([]string, error) {
	out, err := g.out("log", "-1", "--format=%P", commit)
	return strings.Fields(out), err
}

func (g gitRepo) fastForward(dir, commit string) (string, error) {
	return g.inWorktree(dir, "merge", "--ff-only", "-q", commit)
}

func (g gitRepo) reset(dir, commit, old string) (string, error) {
	out, err := g.inWorktree(dir, "reset", "--keep", "-q", commit)
	if err != nil {
		return out, err
	}
	// reset moves the branch from wherever it is, and leaves where that
	// was in ORIG_HEAD: a commit made on it since the Factory last looked
	// would be dropped, so it goes back.
	if was, err := gitOut(g.git, g.environ, dir, "rev-parse", "--verify", "-q", "ORIG_HEAD"); err == nil && was != old {
		if out, err := g.inWorktree(dir, "reset", "--keep", "-q", was); err != nil {
			return out, fmt.Errorf("the branch moved to %s meanwhile, and can't be put back: %w", was, err)
		}
		return "", fmt.Errorf("the branch moved to %s meanwhile", was)
	}
	return "", nil
}

// inWorktree runs git in the worktree dir and returns its combined
// output, for git's own account of why it couldn't.
func (g gitRepo) inWorktree(dir string, args ...string) (string, error) {
	cmd := exec.Command(g.git, args...)
	cmd.Dir = dir
	cmd.Env = g.environ
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (g gitRepo) updateRef(ref, new, old string) error {
	_, err := g.out("update-ref", ref, new, old)
	return err
}

func (g gitRepo) specBranches() ([]string, error) {
	out, err := g.out("for-each-ref", "--format=%(refname:short)", "refs/heads/spec/")
	return strings.Fields(out), err
}

func (g gitRepo) deleteRef(ref, old string) error {
	_, err := g.out("update-ref", "-d", ref, old)
	return err
}

func (g gitRepo) push(ctx context.Context, remote, branch string) (string, error) {
	cmd := exec.CommandContext(ctx, g.git, "push", remote, branch)
	cmd.Dir = g.dir
	// A prompt for credentials would wait forever: nobody answers it.
	cmd.Env = append(slices.Clone(g.environ), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (g gitRepo) fetch(ctx context.Context, remote string) (string, error) {
	cmd := exec.CommandContext(ctx, g.git, "fetch", "--quiet", remote)
	cmd.Dir = g.dir
	cmd.Env = append(slices.Clone(g.environ), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (g gitRepo) worktrees() ([]gitWorktree, error) {
	out, err := g.out("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var list []gitWorktree
	for _, block := range strings.Split(out, "\n\n") {
		var w gitWorktree
		for _, line := range strings.Split(block, "\n") {
			if p, ok := strings.CutPrefix(line, "worktree "); ok {
				// git writes C:/... on Windows; the Factory's paths are C:\...
				w.path = filepath.FromSlash(p)
			} else if b, ok := strings.CutPrefix(line, "branch "); ok {
				w.branch = strings.TrimPrefix(b, "refs/heads/")
			}
		}
		if w.path != "" {
			list = append(list, w)
		}
	}
	return list, nil
}

func (g gitRepo) status(dir string) (string, error) {
	return gitOut(g.git, g.environ, dir, "status", "--porcelain", "--untracked-files=all")
}

func (g gitRepo) removeWorktree(path string) error {
	_, err := g.out("worktree", "remove", path)
	return err
}
