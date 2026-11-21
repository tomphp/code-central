// The Options choosing real git or the Fake git for a Harness, and the
// typed git helpers on Harness, Repository and Remote that serve both.

package factorytest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/git"
)

// Option sets up a Harness New makes.
type Option func(*options)

type options struct {
	fakeGit bool
}

// RealGit runs the Factory, and the harness's repositories, on real git,
// as New does by default; a test that needs raw git (Git, GitIn,
// Remote.Git, or a fake claude's Git Script steps beyond what the Fake
// git models) names it.
func RealGit() Option { return func(o *options) { o.fakeGit = false } }

// FakeGit runs the Factory, and the harness's repositories, on the Fake
// git (git.Fake): in memory, with no git process. The test reads and
// changes them only through the typedHelpers; raw git fails it. A fake
// claude's Git Script steps run on the Fake through the relay
// (relayPath). gitEnv set to "real" runs it on real git all the same.
func FakeGit() Option { return func(o *options) { o.fakeGit = true } }

// typedHelpers are the helpers that serve real git and the Fake git alike.
// They take branch names and commit IDs, not revision syntax like HEAD or
// main^, which the Fake git doesn't model.
const typedHelpers = "Tip, Exists, Log, Parents, File, RemoteTip, CommitOn, MergeByHand"

// gitEnv, set to "real", runs every Harness on real git, even one whose
// test chose FakeGit, to check a run on the Fake git against real git.
const gitEnv = "CODE_CENTRAL_TEST_FACTORY_GIT"

// useFakeGit is whether New runs on the Fake git: the last of opts that
// chooses, unless gitEnv says real git.
func useFakeGit(t testing.TB, opts []Option) bool {
	t.Helper()
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	switch v := os.Getenv(gitEnv); v {
	case "":
		return o.fakeGit
	case "real":
		return false
	default:
		t.Fatalf("factorytest: %s=%q, but the only value it takes is \"real\"", gitEnv, v)
		return false
	}
}

// Tip is the commit at the tip of branch in the repo, failing the test
// when there is no such branch.
func (h *Harness) Tip(branch string) string {
	h.t.Helper()
	return h.tip(h.Repo, branch)
}

// Exists reports whether the repo has branch.
func (h *Harness) Exists(branch string) bool {
	h.t.Helper()
	return h.exists(h.Repo, branch)
}

// Log is the subjects of the commits on rev's first-parent line in the
// repo, newest first, as `git log --first-parent --format=%s` gives them.
func (h *Harness) Log(rev string) []string {
	h.t.Helper()
	return h.log(h.Repo, rev)
}

// Parents is commit's parents in the repo, first parent first.
func (h *Harness) Parents(commit string) []string {
	h.t.Helper()
	return h.parents(h.Repo, commit)
}

// File is the content of the file path, slash-separated, at rev in the
// repo, failing the test when none is committed there.
func (h *Harness) File(rev, path string) string {
	h.t.Helper()
	return h.file(h.Repo, rev, path)
}

// RemoteTip is the repo's remote-tracking branch for branch on remote, as
// its last fetch or push left it, failing the test when it has none.
func (h *Harness) RemoteTip(remote, branch string) string {
	h.t.Helper()
	return h.remoteTip(h.Repo, remote, branch)
}

// CommitOn commits file with content on branch, made from main when it
// doesn't exist, and returns the commit. The repo is on main, and left
// there.
func (h *Harness) CommitOn(branch, file, content string) string {
	h.t.Helper()
	return h.commitOn(h.Repo, branch, file, content)
}

// MergeByHand merges branch into into, as a human would outside the
// Factory: a merge commit "Merge <branch>", or none when into has branch
// already. It returns into's tip, failing the test on a conflict. The repo
// is on main, and left there.
func (h *Harness) MergeByHand(branch, into string) string {
	h.t.Helper()
	return h.mergeByHand(h.Repo, branch, into)
}

// Tip is Harness.Tip in r's clone at Path.
func (r *Repository) Tip(branch string) string {
	r.h.t.Helper()
	return r.h.tip(r.Path, branch)
}

// Exists is Harness.Exists in r's clone at Path.
func (r *Repository) Exists(branch string) bool {
	r.h.t.Helper()
	return r.h.exists(r.Path, branch)
}

// Log is Harness.Log in r's clone at Path.
func (r *Repository) Log(rev string) []string {
	r.h.t.Helper()
	return r.h.log(r.Path, rev)
}

// Parents is Harness.Parents in r's clone at Path.
func (r *Repository) Parents(commit string) []string {
	r.h.t.Helper()
	return r.h.parents(r.Path, commit)
}

// File is Harness.File in r's clone at Path.
func (r *Repository) File(rev, path string) string {
	r.h.t.Helper()
	return r.h.file(r.Path, rev, path)
}

// RemoteTip is Harness.RemoteTip in r's clone at Path.
func (r *Repository) RemoteTip(remote, branch string) string {
	r.h.t.Helper()
	return r.h.remoteTip(r.Path, remote, branch)
}

// CommitOn is Harness.CommitOn in r's clone at Path.
func (r *Repository) CommitOn(branch, file, content string) string {
	r.h.t.Helper()
	return r.h.commitOn(r.Path, branch, file, content)
}

// MergeByHand is Harness.MergeByHand in r's clone at Path.
func (r *Repository) MergeByHand(branch, into string) string {
	r.h.t.Helper()
	return r.h.mergeByHand(r.Path, branch, into)
}

// Tip is Harness.Tip in the bare repository. A bare repository has no
// worktree to commit or merge in, nor remotes of its own, so Remote has
// only the helpers that read.
func (r *Remote) Tip(branch string) string {
	r.h.t.Helper()
	return r.h.tip(r.Path, branch)
}

// Exists is Harness.Exists in the bare repository.
func (r *Remote) Exists(branch string) bool {
	r.h.t.Helper()
	return r.h.exists(r.Path, branch)
}

// Log is Harness.Log in the bare repository.
func (r *Remote) Log(rev string) []string {
	r.h.t.Helper()
	return r.h.log(r.Path, rev)
}

// Parents is Harness.Parents in the bare repository.
func (r *Remote) Parents(commit string) []string {
	r.h.t.Helper()
	return r.h.parents(r.Path, commit)
}

// File is Harness.File in the bare repository.
func (r *Remote) File(rev, path string) string {
	r.h.t.Helper()
	return r.h.file(r.Path, rev, path)
}

// SpecBranches is the names of the repo's branches under spec/.
func (h *Harness) SpecBranches() []string {
	h.t.Helper()
	return h.specBranches(h.Repo)
}

// SpecBranches is Harness.SpecBranches in r's clone at Path.
func (r *Repository) SpecBranches() []string {
	r.h.t.Helper()
	return r.h.specBranches(r.Path)
}

// SpecBranches is Harness.SpecBranches in the bare repository.
func (r *Remote) SpecBranches() []string {
	r.h.t.Helper()
	return r.h.specBranches(r.Path)
}

// Descends reports whether commit is ancestor or descends from it, in the
// repo.
func (h *Harness) Descends(commit, ancestor string) bool {
	h.t.Helper()
	if h.fake != nil {
		return h.fake.Open(h.Repo).IsAncestor(ancestor, commit)
	}
	return exec.Command("git", "-C", h.Repo, "merge-base", "--is-ancestor", ancestor, commit).Run() == nil
}

// WorktreeHead is the commit checked out in ticket's worktree in the repo.
func (h *Harness) WorktreeHead(ticket string) string {
	h.t.Helper()
	return h.head(h.Worktree(ticket))
}

// WorktreeHead is Harness.WorktreeHead in r's clone.
func (r *Repository) WorktreeHead(ticket string) string {
	r.h.t.Helper()
	return r.h.head(r.Worktree(ticket))
}

// AddWorktree adds a worktree on the repo's existing branch at the
// Factory's place for it, Worktree(branch), as the Factory would make it.
func (h *Harness) AddWorktree(branch string) {
	h.t.Helper()
	path := h.Worktree(branch)
	if h.fake != nil {
		must(h.t, h.fake.Open(h.Repo).AddWorktree(path, branch))
		return
	}
	h.git(h.Repo, "worktree", "add", "-q", path, branch)
}

// PushTo pushes the repo's branch from to the remote's branch to, as
// `git push <remote's path> from:refs/heads/to` does: by path, so it leaves
// the repo's remote-tracking branches as they were. It is for a remote made
// with AddRemote.
func (r *Remote) PushTo(from, to string) {
	r.h.t.Helper()
	if r.h.fake != nil {
		r.h.fake.SetRemoteBranch(r.h.Repo, r.Name, to, r.h.tip(r.h.Repo, from))
		return
	}
	r.h.git(r.h.Repo, "push", "-q", r.Path, from+":refs/heads/"+to)
}

// DeleteBranch deletes the remote's branch, as its owner would on the
// Code Host.
func (r *Remote) DeleteBranch(branch string) {
	r.h.t.Helper()
	if r.h.fake != nil {
		must(r.h.t, r.h.fake.Open(r.Path).DeleteRef("refs/heads/"+branch, r.h.tip(r.Path, branch)))
		return
	}
	r.h.git(r.Path, "branch", "-D", branch)
}

// specBranches is SpecBranches in the repository at dir.
func (h *Harness) specBranches(dir string) []string {
	h.t.Helper()
	if h.fake != nil {
		names, err := h.fake.Open(dir).SpecBranches()
		must(h.t, err)
		return names
	}
	out := h.git(dir, "for-each-ref", "--format=%(refname:short)", "refs/heads/spec/")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// head is the commit checked out in the worktree at dir.
func (h *Harness) head(dir string) string {
	h.t.Helper()
	if h.fake != nil {
		out, err := h.fake.Run(dir, "rev-parse", "HEAD")
		must(h.t, err)
		return out
	}
	return h.git(dir, "rev-parse", "HEAD")
}

// tip is Tip in the repository at dir.
func (h *Harness) tip(dir, branch string) string {
	h.t.Helper()
	return h.ref(dir, "refs/heads/"+branch, "branch "+branch)
}

// exists is Exists in the repository at dir.
func (h *Harness) exists(dir, branch string) bool {
	h.t.Helper()
	_, ok := h.lookup(dir, "refs/heads/"+branch)
	return ok
}

// remoteTip is RemoteTip in the clone at dir.
func (h *Harness) remoteTip(dir, remote, branch string) string {
	h.t.Helper()
	return h.ref(dir, "refs/remotes/"+remote+"/"+branch, remote+"/"+branch)
}

// ref is the commit ref names in the repository at dir, failing the test,
// as what, when there is none.
func (h *Harness) ref(dir, ref, what string) string {
	h.t.Helper()
	commit, ok := h.lookup(dir, ref)
	if !ok {
		h.t.Fatalf("factorytest: %s has no %s", dir, what)
	}
	return commit
}

// lookup is the commit the full ref names in the repository at dir, and
// whether it names one, failing the test when dir holds no repository.
func (h *Harness) lookup(dir, ref string) (string, bool) {
	h.t.Helper()
	if h.fake != nil {
		return h.fake.Ref(dir, ref)
	}
	out, err := h.runGit(dir, "rev-parse", "--verify", "-q", ref+"^{commit}")
	if exit := (*exec.ExitError)(nil); errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", false
	}
	if err != nil {
		h.t.Fatalf("git rev-parse %s: %v\n%s", ref, err, out)
	}
	return strings.TrimSpace(out), true
}

// log is Log in the repository at dir.
func (h *Harness) log(dir, rev string) []string {
	h.t.Helper()
	if h.fake == nil {
		out := h.git(dir, "log", "--first-parent", "--format=%s", rev, "--")
		if out == "" {
			return nil
		}
		return strings.Split(out, "\n")
	}
	var subjects []string
	for commit := h.commit(dir, rev); ; {
		subjects = append(subjects, h.fake.Subject(dir, commit))
		parents := h.parents(dir, commit)
		if len(parents) == 0 {
			return subjects
		}
		commit = parents[0]
	}
}

// parents is Parents in the repository at dir.
func (h *Harness) parents(dir, commit string) []string {
	h.t.Helper()
	if h.fake == nil {
		if out := h.git(dir, "log", "-1", "--format=%P", commit, "--"); out != "" {
			return strings.Fields(out)
		}
		return nil
	}
	parents, err := h.fake.Open(dir).ParentsOf(h.commit(dir, commit))
	must(h.t, err)
	if len(parents) == 0 {
		return nil
	}
	return parents
}

// commit is the commit rev names in the Fake git's repository at dir,
// failing the test when it names none.
func (h *Harness) commit(dir, rev string) string {
	h.t.Helper()
	commit, err := h.fake.Open(dir).Commit(rev)
	must(h.t, err)
	return commit
}

// file is File in the repository at dir.
func (h *Harness) file(dir, rev, path string) string {
	h.t.Helper()
	if h.fake == nil {
		out, err := h.runGit(dir, "cat-file", "blob", rev+":"+path)
		if err != nil {
			h.t.Fatalf("factorytest: %s has no file %s at %s: %v\n%s", dir, path, rev, err, out)
		}
		return out
	}
	content, ok := h.fake.File(dir, rev, path)
	if !ok {
		h.t.Fatalf("factorytest: %s has no file %s at %s", dir, path, rev)
	}
	return content
}

// commitOn is CommitOn in the clone at dir.
func (h *Harness) commitOn(dir, branch, file, content string) string {
	h.t.Helper()
	if h.fake != nil {
		old, _ := h.lookup(dir, "refs/heads/"+branch)
		parent := old
		if parent == "" {
			parent = h.tip(dir, "main")
		}
		commit := h.fake.MakeCommit(dir, git.Change{Parents: []string{parent}, Files: map[string]string{file: content}, Message: "Change " + file})
		h.moveBranch(dir, branch, commit, old)
		return commit
	}
	if branch != "main" {
		if !strings.Contains(h.git(dir, "branch", "--list", branch), branch) {
			h.git(dir, "branch", branch, "main")
		}
		h.git(dir, "checkout", "-q", branch)
		defer h.git(dir, "checkout", "-q", "main")
	}
	must(h.t, os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644))
	h.git(dir, "add", file)
	h.git(dir, "commit", "-q", "-m", "Change "+file)
	return h.git(dir, "rev-parse", "HEAD")
}

// mergeByHand is MergeByHand in the clone at dir.
func (h *Harness) mergeByHand(dir, branch, into string) string {
	h.t.Helper()
	msg := "Merge " + branch
	if h.fake == nil {
		if into != "main" {
			h.git(dir, "checkout", "-q", into)
			defer h.git(dir, "checkout", "-q", "main")
		}
		h.git(dir, "merge", "-q", "--no-ff", "-m", msg, branch)
		return h.tip(dir, into)
	}
	g := h.fake.Open(dir)
	head, at := h.tip(dir, branch), h.tip(dir, into)
	if g.IsAncestor(head, at) {
		return at
	}
	tree, conflicts, err := g.MergeTree(at, head)
	must(h.t, err)
	if len(conflicts) > 0 {
		h.t.Fatalf("factorytest: merging %s into %s in %s conflicts in %s", branch, into, dir, strings.Join(conflicts, ", "))
	}
	commit, err := g.CommitTree(tree, msg, at, head)
	must(h.t, err)
	h.moveBranch(dir, into, commit, at)
	return commit
}

// moveBranch moves branch, or makes it when old is "", to commit in the
// Fake git's clone at dir: through its folder when the clone has it
// checked out, as committing there would. It fails the test when another
// worktree has it checked out, as checking it out at dir would.
func (h *Harness) moveBranch(dir, branch, commit, old string) {
	h.t.Helper()
	g := h.fake.Open(dir)
	worktrees, err := g.Worktrees()
	must(h.t, err)
	for _, w := range worktrees {
		if w.Branch != branch {
			continue
		}
		if filepath.Clean(w.Path) != filepath.Clean(dir) {
			h.t.Fatalf("factorytest: %s is checked out in %s, so can't be changed in %s", branch, w.Path, dir)
		}
		if out, err := g.Reset(dir, commit, old); err != nil {
			h.t.Fatalf("factorytest: move %s to %s in %s: %v\n%s", branch, commit, dir, err, out)
		}
		return
	}
	must(h.t, g.UpdateRef("refs/heads/"+branch, commit, old))
}

// relayPath is the harness-only route on the proxy, there only under the
// Fake git, that a fake claude posts its Git Script steps to (relayStep)
// for the harness to run on the Fake git (relayResult). The proxy never
// forwards it to Code Central.
const relayPath = "/factorytest/git"

// relayFile, in the data folder, holds the relay's URL under the Fake git:
// a fake claude runs its Git steps there rather than on real git.
const relayFile = "relay.txt"

// relayStep is a Git Script step a fake claude posts to the relay: git's
// args, to run in the worktree dir, an absolute path.
type relayStep struct {
	Dir  string   `json:"dir"`
	Args []string `json:"args"`
}

// relayResult is the relay's answer to a relayStep: git's trimmed output,
// and why it failed, if it did.
type relayResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// serveRelay runs the relayStep r posts on the Fake git fake and answers
// with its relayResult. A step the Fake git doesn't model fails the test,
// naming the command, as well as the step.
func serveRelay(fake *git.Fake, w http.ResponseWriter, r *http.Request) {
	var step relayStep
	if err := json.NewDecoder(r.Body).Decode(&step); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var res relayResult
	out, err := fake.Run(step.Dir, step.Args...)
	res.Output = out
	if err != nil {
		res.Error = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// relayGit runs a Git Script step through the relay at url, for a fake
// claude under the Fake git: git's args in dir, an absolute path.
func relayGit(url, dir string, args []string) (string, error) {
	b, err := json.Marshal(relayStep{Dir: dir, Args: args})
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: Timeout}).Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("relay: %s: %s", resp.Status, bytes.TrimSpace(body))
	}
	var res relayResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("relay: %w", err)
	}
	if res.Error != "" {
		return res.Output, errors.New(res.Error)
	}
	return res.Output, nil
}
