package git

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Fake is git in memory, for tests: one world of repositories, each known
// by its folder, whose commits and trees are real data with
// content-addressed IDs. It merges by the whole-file rule (Fake.merge),
// and fails the test it was made with on whatever it doesn't model,
// rather than quietly doing something real git wouldn't.
type Fake struct {
	tb    testing.TB
	mu    sync.Mutex
	repos map[string]*fakeRepository
}

// NewFake is a Fake with no repositories, failing tb on what it doesn't
// model.
func NewFake(tb testing.TB) *Fake {
	return &Fake{tb: tb, repos: map[string]*fakeRepository{}}
}

// fakeAuthor is the identity CommitTree gives, the harness's own.
const fakeAuthor = "Factory Test <factory@example.com>"

// fakeRepository is one repository in a Fake.
type fakeRepository struct {
	// dir is its folder, where the ref head is checked out.
	dir  string
	head string
	// commits and trees are its objects by ID, and refs each ref's
	// commit by its full name.
	commits map[string]fakeCommit
	trees   map[string]fakeTree
	refs    map[string]string
}

// fakeTree is a tree: each file's content by its slash-separated path.
type fakeTree map[string]string

// fakeCommit is a commit: its tree's ID, its parents' IDs, first parent
// first, and its author, as "Name <email>", and message.
type fakeCommit struct {
	tree    string
	parents []string
	author  string
	message string
}

// Change is a commit for a test to make: its first parent's tree, or an
// empty one when it has no parents, with Files written and Removed
// deleted.
type Change struct {
	// Parents are its parents, first parent first; none makes a root
	// commit.
	Parents []string
	// Files are the files it writes, by slash-separated path, with their
	// content.
	Files map[string]string
	// Removed are the paths it deletes.
	Removed []string
	// Message is its message.
	Message string
	// Author is its author as "Name <email>"; "" is the identity
	// CommitTree gives.
	Author string
}

// Init makes a repository in the folder dir, with main checked out there
// at a root commit of files with the message msg, and returns that
// commit. It fails the test when dir holds a repository already.
func (f *Fake) Init(dir string, files map[string]string, msg string) string {
	f.tb.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	dir = filepath.Clean(dir)
	if f.repos[dir] != nil {
		f.tb.Fatalf("fake git: %s holds a repository already", dir)
	}
	r := &fakeRepository{dir: dir, head: "refs/heads/main", commits: map[string]fakeCommit{}, trees: map[string]fakeTree{}, refs: map[string]string{}}
	tree := fakeTree(maps.Clone(files))
	c := r.writeCommit(fakeCommit{tree: r.writeTree(tree), author: fakeAuthor, message: msg})
	r.refs[r.head] = c
	if err := r.checkOut(fakeTree{}, tree); err != nil {
		f.tb.Fatal(err)
	}
	f.repos[dir] = r
	return c
}

// MakeCommit makes c in the repository at dir and returns its ID. It
// moves no ref and touches no folder.
func (f *Fake) MakeCommit(dir string, c Change) string {
	f.tb.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	g := fakeRepo{f: f, dir: filepath.Clean(dir)}
	r, err := g.repository()
	if err != nil {
		f.tb.Fatal(err)
	}
	tree := fakeTree{}
	var parents []string
	for i, p := range c.Parents {
		id, err := g.resolve(r, p)
		if err != nil {
			f.tb.Fatal(err)
		}
		if i == 0 {
			tree = maps.Clone(r.treeOf(id))
		}
		parents = append(parents, id)
	}
	maps.Copy(tree, c.Files)
	for _, p := range c.Removed {
		delete(tree, p)
	}
	if p, ok := tree.clash(); ok {
		f.tb.Fatalf("fake git: %s is both a file and a folder", p)
	}
	author := c.Author
	if author == "" {
		author = fakeAuthor
	}
	return r.writeCommit(fakeCommit{tree: r.writeTree(tree), parents: parents, author: author, message: c.Message})
}

func (f *Fake) Open(dir string) Repo { return fakeRepo{f: f, dir: filepath.Clean(dir)} }

func (f *Fake) Clone(ctx context.Context, remote, url, path string) (string, error) {
	return "", f.notModelled("clone")
}

func (f *Fake) TopLevel(dir string) (string, error) { return "", f.notModelled("the top level") }

func (f *Fake) Version() (string, error) { return "", f.notModelled("the version") }

// notModelled fails the test on something the Fake doesn't model, and is
// the error to return for it. It fails it with Errorf, not Fatalf: the
// Factory calls git from goroutines of its own.
func (f *Fake) notModelled(what string, args ...any) error {
	err := fmt.Errorf("fake git: not modelled: "+what, args...)
	f.tb.Errorf("%v", err)
	return err
}

// fakeID is the ID of the object of kind made of fields: a SHA-1 of them,
// each length-prefixed, so that equal objects have equal IDs, with no
// counter or clock, and different ones different IDs.
func fakeID(kind string, fields ...string) string {
	h := sha1.New()
	io.WriteString(h, kind)
	for _, f := range fields {
		fmt.Fprintf(h, "\x00%d:%s", len(f), f)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (r *fakeRepository) writeTree(t fakeTree) string {
	var fields []string
	for _, p := range slices.Sorted(maps.Keys(t)) {
		fields = append(fields, p, t[p])
	}
	id := fakeID("tree", fields...)
	r.trees[id] = t
	return id
}

// writeCommit writes c, ending its message in a newline, as commit-tree
// does.
func (r *fakeRepository) writeCommit(c fakeCommit) string {
	if c.message != "" && !strings.HasSuffix(c.message, "\n") {
		c.message += "\n"
	}
	id := fakeID("commit", append([]string{c.tree, c.author, c.message}, c.parents...)...)
	r.commits[id] = c
	return id
}

func (r *fakeRepository) treeOf(commit string) fakeTree { return r.trees[r.commits[commit].tree] }

// ancestors is the commits reachable from these, themselves included.
func (r *fakeRepository) ancestors(commits ...string) map[string]bool {
	seen := map[string]bool{}
	for len(commits) > 0 {
		c := commits[len(commits)-1]
		commits = commits[:len(commits)-1]
		if !seen[c] {
			seen[c] = true
			commits = append(commits, r.commits[c].parents...)
		}
	}
	return seen
}

// mergeBases is a and b's best common ancestors: those no other common
// ancestor descends from.
func (r *fakeRepository) mergeBases(a, b string) []string {
	ofA := r.ancestors(a)
	var common, parents []string
	for c := range r.ancestors(b) {
		if ofA[c] {
			common = append(common, c)
			parents = append(parents, r.commits[c].parents...)
		}
	}
	below := r.ancestors(parents...)
	var best []string
	for _, c := range common {
		if !below[c] {
			best = append(best, c)
		}
	}
	slices.Sort(best)
	return best
}

// moveTo moves the branch checked out in the folder to commit, as the git
// command does: it writes into the folder only the files that differ
// between the two commits, and refuses, with git's account of why, when
// one of them has a change of its own or is untracked where commit adds
// it. A tracked file that is missing isn't a change.
func (r *fakeRepository) moveTo(commit, command string) (string, error) {
	was, now := r.treeOf(r.refs[r.head]), r.treeOf(commit)
	var changed, untracked []string
	for _, p := range touched(was, now) {
		content, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(p)))
		old, tracked := was[p]
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return "", err
		case !tracked:
			untracked = append(untracked, p)
		case string(content) != old:
			changed = append(changed, p)
		}
	}
	switch {
	case changed != nil:
		return gitFailed("error: Your local changes to the following files would be overwritten by %s:\n\t%s\nPlease commit your changes or stash them before you %s.\nAborting", command, strings.Join(changed, "\n\t"), command)
	case untracked != nil:
		return gitFailed("error: The following untracked working tree files would be overwritten by %s:\n\t%s\nPlease move or remove them before you %s.\nAborting", command, strings.Join(untracked, "\n\t"), command)
	}
	if err := r.checkOut(was, now); err != nil {
		return "", err
	}
	r.refs[r.head] = commit
	return "", nil
}

// touched is the paths whose files differ between two trees, sorted.
func touched(was, now fakeTree) []string {
	var paths []string
	for p, content := range now {
		if old, ok := was[p]; !ok || old != content {
			paths = append(paths, p)
		}
	}
	for p := range was {
		if _, ok := now[p]; !ok {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	return paths
}

// checkOut writes the files that differ from was to now into the folder,
// removing those now hasn't, with any folder that leaves empty.
func (r *fakeRepository) checkOut(was, now fakeTree) error {
	for p := range was {
		if _, ok := now[p]; ok {
			continue
		}
		name := filepath.Join(r.dir, filepath.FromSlash(p))
		if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		for d := filepath.Dir(name); d != r.dir && os.Remove(d) == nil; d = filepath.Dir(d) {
		}
	}
	for p, content := range now {
		if old, ok := was[p]; ok && old == content {
			continue
		}
		name := filepath.Join(r.dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// gitFailed is the output and error of a git command that failed with
// this account of why.
func gitFailed(format string, args ...any) (string, error) {
	out := fmt.Sprintf(format, args...)
	if strings.HasPrefix(out, "fatal: ") {
		return out, errors.New("exit status 128")
	}
	return out, errors.New("exit status 1")
}

// clash is a path of t that is also a folder of another of its paths, as
// no git tree can have.
func (t fakeTree) clash() (string, bool) {
	for p := range t {
		for d := path.Dir(p); d != "."; d = path.Dir(d) {
			if _, ok := t[d]; ok {
				return d, true
			}
		}
	}
	return "", false
}

// fakeRepo is the repository at dir in a Fake.
type fakeRepo struct {
	f   *Fake
	dir string
}

// lock locks the Fake for one call and is the repository, or an error
// when dir holds none.
func (g fakeRepo) lock() (*fakeRepository, func(), error) {
	g.f.mu.Lock()
	r, err := g.repository()
	return r, g.f.mu.Unlock, err
}

func (g fakeRepo) repository() (*fakeRepository, error) {
	if r := g.f.repos[g.dir]; r != nil {
		return r, nil
	}
	return nil, fmt.Errorf("fatal: not a git repository: %s", g.dir)
}

// resolve resolves rev to a commit, as rev-parse does for the forms the
// Factory passes: a ref, a name under refs/, refs/tags/, refs/heads/ or
// refs/remotes/, or a commit's ID, whole or abbreviated. Any other syntax
// isn't modelled.
func (g fakeRepo) resolve(r *fakeRepository, rev string) (string, error) {
	if rev == "HEAD" || strings.ContainsAny(rev, "^~:@{}") || strings.Contains(rev, "..") {
		return "", g.f.notModelled("the revision %q", rev)
	}
	if _, ok := r.commits[rev]; ok {
		return rev, nil
	}
	for _, ref := range []string{rev, "refs/" + rev, "refs/tags/" + rev, "refs/heads/" + rev, "refs/remotes/" + rev} {
		if id, ok := r.refs[ref]; ok {
			return id, nil
		}
	}
	var found []string
	if len(rev) >= 4 && strings.Trim(rev, "0123456789abcdef") == "" {
		for id := range r.commits {
			if strings.HasPrefix(id, rev) {
				found = append(found, id)
			}
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	return "", fmt.Errorf("fatal: %s isn't a commit", rev)
}

func (g fakeRepo) Commit(rev string) (string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	return g.resolve(r, rev)
}

func (g fakeRepo) TreeOf(commit string) (string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	c, err := g.resolve(r, commit)
	if err != nil {
		return "", err
	}
	return r.commits[c].tree, nil
}

func (g fakeRepo) IsAncestor(commit, rev string) bool {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return false
	}
	c, err := g.resolve(r, commit)
	if err != nil {
		return false
	}
	of, err := g.resolve(r, rev)
	return err == nil && r.ancestors(of)[c]
}

func (g fakeRepo) MergeBase(a, b string) (string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	bases, err := g.mergeBase(r, a, b)
	if err != nil {
		return "", err
	}
	return bases, nil
}

// mergeBase is a and b's one best common ancestor, failing when they
// have none; several aren't modelled.
func (g fakeRepo) mergeBase(r *fakeRepository, a, b string) (string, error) {
	ca, err := g.resolve(r, a)
	if err != nil {
		return "", err
	}
	cb, err := g.resolve(r, b)
	if err != nil {
		return "", err
	}
	switch bases := r.mergeBases(ca, cb); len(bases) {
	case 0:
		return "", fmt.Errorf("fatal: %s and %s have no common ancestor", a, b)
	case 1:
		return bases[0], nil
	default:
		return "", g.f.notModelled("%s and %s have several merge bases", a, b)
	}
}

func (g fakeRepo) MergeTree(base, head string) (tree string, conflicts []string, err error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", nil, err
	}
	mb, err := g.mergeBase(r, base, head)
	if err != nil {
		return "", nil, fmt.Errorf("fatal: refusing to merge unrelated histories: %w", err)
	}
	ours, _ := g.resolve(r, base)
	theirs, _ := g.resolve(r, head)
	return g.merge(r, r.treeOf(mb), r.treeOf(ours), r.treeOf(theirs))
}

func (g fakeRepo) ReplayTree(base, commit string) (tree string, conflicts []string, err error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", nil, err
	}
	ours, err := g.resolve(r, base)
	if err != nil {
		return "", nil, err
	}
	theirs, err := g.resolve(r, commit)
	if err != nil {
		return "", nil, err
	}
	parents := r.commits[theirs].parents
	if len(parents) == 0 {
		return "", nil, fmt.Errorf("fatal: %s has no parent to replay it from", commit)
	}
	return g.merge(r, r.treeOf(parents[0]), r.treeOf(ours), r.treeOf(theirs))
}

// merge writes the merge of the trees ours and theirs from base, by the
// whole-file rule, file by file: a file only one side changed, or both
// changed the same way, merges cleanly; one both changed differently
// conflicts when it is a single line, or absent, on every side, keeping
// ours; and anything else isn't modelled. A file added on both sides, or
// changed on one and deleted on the other, counts as changed on both. A
// merge that needs git's rename detection isn't modelled either.
func (g fakeRepo) merge(r *fakeRepository, base, ours, theirs fakeTree) (tree string, conflicts []string, err error) {
	if why := unmodelledRename(base, ours, theirs); why != "" {
		return "", nil, g.f.notModelled("merging %s", why)
	}
	merged := fakeTree{}
	var paths []string
	for _, t := range []fakeTree{base, ours, theirs} {
		paths = append(paths, slices.Collect(maps.Keys(t))...)
	}
	slices.Sort(paths)
	for _, p := range slices.Compact(paths) {
		b, inBase := base[p]
		o, inOurs := ours[p]
		t, inTheirs := theirs[p]
		switch {
		case inOurs == inTheirs && o == t, inTheirs == inBase && t == b:
			if inOurs {
				merged[p] = o
			}
		case inOurs == inBase && o == b:
			if inTheirs {
				merged[p] = t
			}
		case oneLine(b, inBase) && oneLine(o, inOurs) && oneLine(t, inTheirs):
			conflicts = append(conflicts, p)
			if inOurs {
				merged[p] = o
			}
		default:
			return "", nil, g.f.notModelled("merging %s, changed on both sides and not one line on each", p)
		}
	}
	if p, ok := merged.clash(); ok {
		return "", nil, g.f.notModelled("merging %s, a file on one side and a folder on the other", p)
	}
	return r.writeTree(merged), conflicts, nil
}

// oneLine reports whether a file, when present, is one line: what the
// whole-file rule knows git conflicts on.
func oneLine(content string, present bool) bool {
	return !present || content != "" && !strings.Contains(strings.TrimSuffix(content, "\n"), "\n")
}

// unmodelledRename is why merging ours and theirs from base would need
// git's rename detection, or "" when it wouldn't. That is when a side
// deleted a file it may have renamed, having added one git may find
// similar, and the other side changed or deleted the file, or added
// another to its folder, which the side left empty.
func unmodelledRename(base, ours, theirs fakeTree) string {
	for _, sides := range [][2]fakeTree{{ours, theirs}, {theirs, ours}} {
		side, other := sides[0], sides[1]
		for p, was := range base {
			if _, kept := side[p]; kept || was == "" {
				continue
			}
			renamed := false
			for q, content := range side {
				if _, ok := base[q]; !ok && !unalike(was, content) {
					renamed = true
				}
			}
			if !renamed {
				continue
			}
			if now, ok := other[p]; !ok || now != was {
				return p + ", deleted on one side, maybe renamed, and changed on the other"
			}
			if dir := path.Dir(p); dir != "." && !holds(base, side, dir, false) && holds(base, other, dir, true) {
				return "the folder " + dir + ", emptied on one side, maybe renamed, and added to on the other"
			}
		}
	}
	return ""
}

// unalike reports whether git's rename detection can find nothing in
// common between two files: each one line, shorter than the 64 bytes it
// compares at a time, and different.
func unalike(a, b string) bool {
	short := func(s string) bool { return len(s) < 64 && oneLine(s, true) }
	return a != b && short(a) && short(b)
}

// holds reports whether t has a file in dir or, with added, a file in dir
// that base hasn't.
func holds(base, t fakeTree, dir string, added bool) bool {
	for p := range t {
		if _, ok := base[p]; strings.HasPrefix(p, dir+"/") && !(added && ok) {
			return true
		}
	}
	return false
}

func (g fakeRepo) CommitTree(tree, msg string, parents ...string) (string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	return g.commitTree(r, fakeCommit{tree: tree, author: fakeAuthor, message: msg}, parents)
}

func (g fakeRepo) CommitTreeLike(tree, like string, parents ...string) (string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	c, err := g.resolve(r, like)
	if err != nil {
		return "", err
	}
	return g.commitTree(r, fakeCommit{tree: tree, author: r.commits[c].author, message: r.commits[c].message}, parents)
}

// commitTree writes c with these parents, once its tree and parents are
// found.
func (g fakeRepo) commitTree(r *fakeRepository, c fakeCommit, parents []string) (string, error) {
	if _, ok := r.trees[c.tree]; !ok {
		return "", fmt.Errorf("fatal: not a valid tree: %s", c.tree)
	}
	for _, p := range parents {
		id, err := g.resolve(r, p)
		if err != nil {
			return "", err
		}
		c.parents = append(c.parents, id)
	}
	return r.writeCommit(c), nil
}

func (g fakeRepo) FirstParents(rev, exclude string) ([]string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return nil, err
	}
	from, err := g.resolve(r, rev)
	if err != nil {
		return nil, err
	}
	to, err := g.resolve(r, exclude)
	if err != nil {
		return nil, err
	}
	excluded := r.ancestors(to)
	var line []string
	for c := from; c != "" && !excluded[c]; {
		line = append(line, c)
		c = ""
		if parents := r.commits[line[len(line)-1]].parents; len(parents) > 0 {
			c = parents[0]
		}
	}
	slices.Reverse(line)
	return line, nil
}

func (g fakeRepo) ParentsOf(commit string) ([]string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return nil, err
	}
	c, err := g.resolve(r, commit)
	if err != nil {
		return nil, err
	}
	return slices.Clone(r.commits[c].parents), nil
}

func (g fakeRepo) FastForward(dir, commit string) (string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	if err := g.inFolder(r, dir); err != nil {
		return "", err
	}
	to, err := g.resolve(r, commit)
	if err != nil {
		return gitFailed("merge: %s - not something we can merge", commit)
	}
	at := r.refs[r.head]
	switch {
	case r.ancestors(at)[to]:
		return "", nil
	case !r.ancestors(to)[at]:
		return gitFailed("fatal: Not possible to fast-forward, aborting.")
	}
	return r.moveTo(to, "merge")
}

func (g fakeRepo) Reset(dir, commit, old string) (string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	if err := g.inFolder(r, dir); err != nil {
		return "", err
	}
	to, err := g.resolve(r, commit)
	if err != nil {
		return gitFailed("fatal: ambiguous argument '%s': unknown revision or path not in the working tree.", commit)
	}
	was := r.refs[r.head]
	if out, err := r.moveTo(to, "reset"); err != nil {
		return out, err
	}
	// As Real's does, a reset of a branch that moved since the caller
	// looked puts it back.
	if was != old {
		if out, err := r.moveTo(was, "reset"); err != nil {
			return out, fmt.Errorf("the branch moved to %s meanwhile, and can't be put back: %w", was, err)
		}
		return "", fmt.Errorf("the branch moved to %s meanwhile", was)
	}
	return "", nil
}

// inFolder fails unless dir is the repository's own folder: worktrees
// aren't modelled.
func (g fakeRepo) inFolder(r *fakeRepository, dir string) error {
	if filepath.Clean(dir) != r.dir {
		return g.f.notModelled("the worktree %s", dir)
	}
	return nil
}

func (g fakeRepo) UpdateRef(ref, new, old string) error {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(ref, "refs/") {
		return g.f.notModelled("the ref %q", ref)
	}
	id, err := g.resolve(r, new)
	if err != nil {
		return err
	}
	if err := g.checkRef(r, ref, old); err != nil {
		return err
	}
	r.refs[ref] = id
	return nil
}

// checkRef fails unless ref is at old, or, with old "", doesn't exist and
// clashes with no ref as a folder of it or a ref in it.
func (g fakeRepo) checkRef(r *fakeRepository, ref, old string) error {
	at, exists := r.refs[ref]
	switch {
	case old == "" && exists:
		return fmt.Errorf("fatal: cannot lock ref '%s': reference already exists", ref)
	case old == "":
		for other := range r.refs {
			if strings.HasPrefix(ref, other+"/") || strings.HasPrefix(other, ref+"/") {
				return fmt.Errorf("fatal: cannot lock ref '%s': '%s' exists", ref, other)
			}
		}
		return nil
	case !exists:
		return fmt.Errorf("fatal: cannot lock ref '%s': unable to resolve reference '%s'", ref, ref)
	}
	want, err := g.resolve(r, old)
	if err != nil {
		return err
	}
	if at != want {
		return fmt.Errorf("fatal: cannot lock ref '%s': is at %s but expected %s", ref, at, want)
	}
	return nil
}

func (g fakeRepo) SpecBranches() ([]string, error) {
	return g.refsUnder("refs/heads/spec/", "refs/heads/")
}

func (g fakeRepo) ForkPoints() ([]string, error) { return g.refsUnder(ForkRefs, ForkRefs) }

// refsUnder is the refs under prefix, sorted, each without trim.
func (g fakeRepo) refsUnder(prefix, trim string) ([]string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, ref := range slices.Sorted(maps.Keys(r.refs)) {
		if strings.HasPrefix(ref, prefix) {
			names = append(names, strings.TrimPrefix(ref, trim))
		}
	}
	return names, nil
}

func (g fakeRepo) DeleteRef(ref, old string) error {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return err
	}
	if old == "" {
		return fmt.Errorf("fatal: %q: not a valid old SHA1", old)
	}
	if err := g.checkRef(r, ref, old); err != nil {
		return err
	}
	delete(r.refs, ref)
	return nil
}

func (g fakeRepo) HasRemote(name string) bool {
	g.f.notModelled("remotes")
	return false
}

func (g fakeRepo) RemoteURL(name string) (string, error) { return "", g.f.notModelled("remotes") }

func (g fakeRepo) TopLevel() (string, error) { return "", g.f.notModelled("the top level") }

func (g fakeRepo) Push(ctx context.Context, remote, branch, lease string) (string, error) {
	return "", g.f.notModelled("push")
}

func (g fakeRepo) Fetch(ctx context.Context, remote string) (string, error) {
	return "", g.f.notModelled("fetch")
}

func (g fakeRepo) RemoteHas(ctx context.Context, remote, branch string) (bool, error) {
	return false, g.f.notModelled("ls-remote")
}

func (g fakeRepo) Worktrees() ([]Worktree, error) { return nil, g.f.notModelled("worktrees") }

func (g fakeRepo) Status(dir string) (string, error) { return "", g.f.notModelled("status") }

func (g fakeRepo) RemoveWorktree(path string) error { return g.f.notModelled("worktrees") }

func (g fakeRepo) AddWorktree(path, branch string) error { return g.f.notModelled("worktrees") }

func (g fakeRepo) AddWorktreeOnNewBranch(path, branch, from string) error {
	return g.f.notModelled("worktrees")
}

func (g fakeRepo) AddDetachedWorktree(path, rev string) error { return g.f.notModelled("worktrees") }

func (g fakeRepo) CheckoutDetached(dir, rev string) error { return g.f.notModelled("checkout") }

func (g fakeRepo) PruneWorktrees() error { return g.f.notModelled("worktrees") }

func (g fakeRepo) CreateBranch(branch, commit string) error {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return err
	}
	ref := "refs/heads/" + branch
	if _, ok := r.refs[ref]; ok {
		return fmt.Errorf("fatal: a branch named '%s' already exists", branch)
	}
	id, err := g.resolve(r, commit)
	if err != nil {
		return err
	}
	if err := g.checkRef(r, ref, ""); err != nil {
		return err
	}
	r.refs[ref] = id
	return nil
}

func (g fakeRepo) HasFile(rev, path string) bool {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return false
	}
	c, err := g.resolve(r, rev)
	if err != nil {
		return false
	}
	tree := r.treeOf(c)
	_, ok := tree[path]
	return ok || holds(fakeTree{}, tree, path, false)
}

func (g fakeRepo) CurrentBranch(dir string) (string, error) {
	return "", g.f.notModelled("the current branch")
}

func (g fakeRepo) LastCommit(dir string) (id, subject string, err error) {
	return "", "", g.f.notModelled("the last commit")
}
