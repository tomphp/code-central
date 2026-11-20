package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Real is git through the binary at Binary, with every call run in the
// environment Environ: the Factory's Git unless a test gives it another.
type Real struct {
	// Binary is the path of the git binary.
	Binary string
	// Environ is the environment each git call runs in.
	Environ []string
}

// Open is the repository at dir, through the git binary. It runs no git:
// a dir that isn't a repository fails on its first call instead.
func (g Real) Open(dir string) Repo {
	return realRepo{binary: g.Binary, dir: dir, environ: g.Environ}
}

func (g Real) Clone(ctx context.Context, remote, url, path string) (string, error) {
	cmd := exec.CommandContext(ctx, g.Binary, "clone", "--quiet", "--origin", remote, url, path)
	// A prompt for credentials would wait forever: nobody answers it.
	cmd.Env = append(slices.Clone(g.Environ), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (g Real) TopLevel(dir string) (string, error) { return g.Open(dir).TopLevel() }

func (g Real) Version() (string, error) { return run(g.Binary, g.Environ, "", "version") }

// realRepo is the repository at dir, through the git binary.
type realRepo struct {
	binary  string
	dir     string
	environ []string
}

// run runs the git binary in dir and returns its trimmed output.
func run(binary string, environ []string, dir string, args ...string) (string, error) {
	cmd := exec.Command(binary, args...)
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

func (g realRepo) out(args ...string) (string, error) {
	return run(g.binary, g.environ, g.dir, args...)
}

func (g realRepo) Commit(rev string) (string, error) {
	return g.out("rev-parse", "--verify", "-q", rev+"^{commit}")
}

func (g realRepo) TreeOf(commit string) (string, error) {
	return g.out("rev-parse", "--verify", "-q", commit+"^{tree}")
}

func (g realRepo) IsAncestor(commit, rev string) bool {
	_, err := g.out("merge-base", "--is-ancestor", commit, rev)
	return err == nil
}

func (g realRepo) MergeBase(a, b string) (string, error) { return g.out("merge-base", a, b) }

func (g realRepo) HasRemote(name string) bool {
	_, err := g.RemoteURL(name)
	return err == nil
}

func (g realRepo) RemoteURL(name string) (string, error) { return g.out("remote", "get-url", name) }

func (g realRepo) TopLevel() (string, error) {
	top, err := g.out("rev-parse", "--show-toplevel")
	// git writes C:/... on Windows; the Factory's paths are C:\...
	return filepath.FromSlash(top), err
}

func (g realRepo) MergeTree(base, head string) (tree string, conflicts []string, err error) {
	return g.mergeTreeWith(nil, base, head)
}

func (g realRepo) ReplayTree(base, commit string) (tree string, conflicts []string, err error) {
	return g.mergeTreeWith([]string{"--merge-base=" + commit + "^"}, base, commit)
}

// mergeTreeWith is merge-tree of base and head with these extra options.
func (g realRepo) mergeTreeWith(options []string, base, head string) (tree string, conflicts []string, err error) {
	args := append([]string{"merge-tree", "--write-tree", "--name-only", "--no-messages"}, options...)
	cmd := exec.Command(g.binary, append(args, base, head)...)
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

func (g realRepo) CommitTree(tree, msg string, parents ...string) (string, error) {
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

func (g realRepo) CommitTreeLike(tree, like string, parents ...string) (string, error) {
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
	return run(g.binary, environ, g.dir, commitTreeArgs(tree, fields[3], parents)...)
}

func (g realRepo) FirstParents(rev, exclude string) ([]string, error) {
	out, err := g.out("rev-list", "--first-parent", "--reverse", rev, "^"+exclude)
	return strings.Fields(out), err
}

func (g realRepo) ParentsOf(commit string) ([]string, error) {
	out, err := g.out("log", "-1", "--format=%P", commit)
	return strings.Fields(out), err
}

func (g realRepo) FastForward(dir, commit string) (string, error) {
	return g.inWorktree(dir, "merge", "--ff-only", "-q", commit)
}

func (g realRepo) Reset(dir, commit, old string) (string, error) {
	out, err := g.inWorktree(dir, "reset", "--keep", "-q", commit)
	if err != nil {
		return out, err
	}
	// reset moves the branch from wherever it is, and leaves where that
	// was in ORIG_HEAD: a commit made on it since the Factory last looked
	// would be dropped, so it goes back.
	if was, err := run(g.binary, g.environ, dir, "rev-parse", "--verify", "-q", "ORIG_HEAD"); err == nil && was != old {
		if out, err := g.inWorktree(dir, "reset", "--keep", "-q", was); err != nil {
			return out, fmt.Errorf("the branch moved to %s meanwhile, and can't be put back: %w", was, err)
		}
		return "", fmt.Errorf("the branch moved to %s meanwhile", was)
	}
	return "", nil
}

// inWorktree runs git in the worktree dir and returns its combined
// output, for git's own account of why it couldn't.
func (g realRepo) inWorktree(dir string, args ...string) (string, error) {
	cmd := exec.Command(g.binary, args...)
	cmd.Dir = dir
	cmd.Env = g.environ
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (g realRepo) UpdateRef(ref, new, old string) error {
	_, err := g.out("update-ref", ref, new, old)
	return err
}

func (g realRepo) SpecBranches() ([]string, error) {
	out, err := g.out("for-each-ref", "--format=%(refname:short)", "refs/heads/spec/")
	return strings.Fields(out), err
}

func (g realRepo) ForkPoints() ([]string, error) {
	out, err := g.out("for-each-ref", "--format=%(refname)", ForkRefs)
	var ids []string
	for _, ref := range strings.Fields(out) {
		ids = append(ids, strings.TrimPrefix(ref, ForkRefs))
	}
	return ids, err
}

func (g realRepo) DeleteRef(ref, old string) error {
	_, err := g.out("update-ref", "-d", ref, old)
	return err
}

func (g realRepo) Push(ctx context.Context, remote, branch, lease string) (string, error) {
	args := []string{"push", remote, branch}
	if lease != "" {
		args = []string{"push", "--force-with-lease=" + branch + ":" + lease, remote, branch}
	}
	cmd := exec.CommandContext(ctx, g.binary, args...)
	cmd.Dir = g.dir
	// A prompt for credentials would wait forever: nobody answers it.
	cmd.Env = append(slices.Clone(g.environ), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (g realRepo) Fetch(ctx context.Context, remote string) (string, error) {
	cmd := exec.CommandContext(ctx, g.binary, "fetch", "--quiet", remote)
	cmd.Dir = g.dir
	cmd.Env = append(slices.Clone(g.environ), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (g realRepo) RemoteHas(ctx context.Context, remote, branch string) (bool, error) {
	args := []string{"ls-remote", "--heads", remote, "refs/heads/" + branch}
	cmd := exec.CommandContext(ctx, g.binary, args...)
	cmd.Dir = g.dir
	cmd.Env = append(slices.Clone(g.environ), "GIT_TERMINAL_PROMPT=0")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (g realRepo) Worktrees() ([]Worktree, error) {
	out, err := g.out("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var list []Worktree
	for _, block := range strings.Split(out, "\n\n") {
		var w Worktree
		for _, line := range strings.Split(block, "\n") {
			if p, ok := strings.CutPrefix(line, "worktree "); ok {
				// git writes C:/... on Windows; the Factory's paths are C:\...
				w.Path = filepath.FromSlash(p)
			} else if b, ok := strings.CutPrefix(line, "branch "); ok {
				w.Branch = strings.TrimPrefix(b, "refs/heads/")
			}
		}
		if w.Path != "" {
			list = append(list, w)
		}
	}
	return list, nil
}

func (g realRepo) Status(dir string) (string, error) {
	return run(g.binary, g.environ, dir, "status", "--porcelain", "--untracked-files=all")
}

func (g realRepo) RemoveWorktree(path string) error {
	_, err := g.out("worktree", "remove", path)
	return err
}

func (g realRepo) AddWorktree(path, branch string) error {
	_, err := g.out("worktree", "add", "-q", path, branch)
	return err
}

func (g realRepo) AddWorktreeOnNewBranch(path, branch, from string) error {
	_, err := g.out("worktree", "add", "-q", "-b", branch, path, from)
	return err
}

func (g realRepo) AddDetachedWorktree(path, rev string) error {
	_, err := g.out("worktree", "add", "-q", "--detach", path, rev)
	return err
}

func (g realRepo) CheckoutDetached(dir, rev string) error {
	_, err := run(g.binary, g.environ, dir, "checkout", "-q", "--detach", rev)
	return err
}

func (g realRepo) PruneWorktrees() error {
	_, err := g.out("worktree", "prune")
	return err
}

func (g realRepo) CreateBranch(branch, commit string) error {
	_, err := g.out("branch", "--no-track", branch, commit)
	return err
}

func (g realRepo) HasFile(rev, path string) bool {
	_, err := g.out("cat-file", "-e", rev+":"+path)
	return err == nil
}

func (g realRepo) CurrentBranch(dir string) (string, error) {
	b, err := run(g.binary, g.environ, dir, "symbolic-ref", "--short", "-q", "HEAD")
	if err == nil && b == "" {
		return "", errors.New("git symbolic-ref: HEAD names no branch")
	}
	return b, err
}

func (g realRepo) LastCommit(dir string) (id, subject string, err error) {
	out, err := run(g.binary, g.environ, dir, "log", "-1", "--format=%h %s")
	id, subject, _ = strings.Cut(out, " ")
	return id, subject, err
}
