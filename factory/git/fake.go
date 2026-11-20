package git

import (
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
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Fake is git in memory, for tests: one world of repositories, each known
// by its folder, whose commits and trees are real data with
// content-addressed IDs. Its worktrees are real folders: checking out
// writes a commit's files there, and status and CommitAll read them back.
// It merges by the whole-file rule (Fake.merge), and fails the test it
// was made with on whatever it doesn't model,
// rather than quietly doing something real git wouldn't.
type Fake struct {
	tb    testing.TB
	mu    sync.Mutex
	repos map[string]*fakeRepository
	// version is what Version gives; "" is fakeVersion.
	version string
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
	// commits and trees are its objects by ID, and refs each ref's
	// commit by its full name.
	commits map[string]fakeCommit
	trees   map[string]fakeTree
	refs    map[string]string
	// worktrees are its worktrees, the one in its own folder first; a
	// bare repository's is its folder, with nothing checked out, and its
	// HEAD.
	worktrees []*fakeWorktree
	// bare is whether it has no folder checked out; remotes are its
	// remotes' URLs by name; broken is whether it can't be reached as a
	// remote, and refusing whether it refuses pushes as one.
	bare     bool
	remotes  map[string]string
	broken   bool
	refusing bool
}

// newFakeRepository is a repository with no objects or refs in the folder
// dir, resolved, with head its HEAD.
func newFakeRepository(dir, head string) *fakeRepository {
	return &fakeRepository{commits: map[string]fakeCommit{}, trees: map[string]fakeTree{}, refs: map[string]string{}, worktrees: []*fakeWorktree{{dir: dir, head: head}}}
}

// fakeWorktree is a worktree of a fakeRepository: a real folder, holding
// the files of what is checked out there and whatever else is written
// to it.
type fakeWorktree struct {
	// dir is its folder, with its symbolic links resolved, as git
	// reports it.
	dir string
	// head is the full name of the branch checked out there, or the
	// commit when it is detached.
	head string
	// staged is its index when a Script's add or rm (Fake.Run) has
	// staged a change there; nil is the tree of what is checked out.
	staged fakeTree
}

// branch is the name of the branch checked out in w, or "" when it is
// detached.
func (w *fakeWorktree) branch() string {
	if b, ok := strings.CutPrefix(w.head, "refs/heads/"); ok {
		return b
	}
	return ""
}

// at is the commit checked out in w.
func (r *fakeRepository) at(w *fakeWorktree) string {
	if w.branch() != "" {
		return r.refs[w.head]
	}
	return w.head
}

// setAt moves the branch checked out in w to commit, or, when it is
// detached, w itself.
func (r *fakeRepository) setAt(w *fakeWorktree, commit string) {
	if w.branch() != "" {
		r.refs[w.head] = commit
	} else {
		w.head = commit
	}
}

// main is r's worktree in its own folder: git's main worktree, which is
// no Home worktree.
func (r *fakeRepository) main() *fakeWorktree { return r.worktrees[0] }

// worktree is r's worktree in the folder dir, resolved, or nil.
func (r *fakeRepository) worktree(dir string) *fakeWorktree {
	for _, w := range r.worktrees {
		if w.dir == dir {
			return w
		}
	}
	return nil
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
// commit. It fails the test when dir is, is in or holds a worktree
// already.
func (f *Fake) Init(dir string, files map[string]string, msg string) string {
	f.tb.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.tb.Fatal(err)
	}
	dir = realPath(dir)
	if w := f.overlapping(dir); w != nil {
		f.tb.Fatalf("fake git: %s is, is in or holds the worktree %s already", dir, w.dir)
	}
	r := newFakeRepository(dir, "refs/heads/main")
	w := r.main()
	tree := fakeTree(maps.Clone(files))
	c := r.writeCommit(fakeCommit{tree: r.writeTree(tree), author: fakeAuthor, message: msg})
	r.refs[w.head] = c
	if err := checkOut(dir, fakeTree{}, tree); err != nil {
		f.tb.Fatal(err)
	}
	f.repos[dir] = r
	return c
}

// CommitAll commits what the worktree dir's folder holds, untracked files
// included, onto what is checked out there, with the message msg, as `git
// add -A` then `git commit` do, and returns the commit's ID. It fails, as
// git does, when there is nothing to commit or msg is empty.
func (f *Fake) CommitAll(dir, msg string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := fakeRepo{f: f, dir: filepath.Clean(dir)}
	r, err := g.repository()
	if err != nil {
		return "", err
	}
	w, err := g.worktree(r, dir)
	if err != nil {
		return "", err
	}
	folder, err := f.folderTree(w.dir)
	if err != nil {
		return "", err
	}
	parent := r.at(w)
	switch {
	case maps.Equal(folder, r.treeOf(parent)):
		return "", errors.New("nothing to commit, working tree clean")
	case strings.TrimSpace(msg) == "":
		return "", errors.New("Aborting commit due to empty commit message.")
	}
	c := r.writeCommit(fakeCommit{tree: r.writeTree(folder), parents: []string{parent}, author: fakeAuthor, message: msg})
	r.setAt(w, c)
	w.staged = nil
	return c, nil
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

// notModelled fails the test on something the Fake doesn't model, and is
// the error to return for it. It fails it with Errorf, not Fatalf: the
// Factory calls git from goroutines of its own.
func (f *Fake) notModelled(what string, args ...any) error {
	err := fmt.Errorf("%w: "+what, append([]any{errNotModelled}, args...)...)
	f.tb.Errorf("%v", err)
	return err
}

// errNotModelled is what every error of notModelled wraps.
var errNotModelled = errors.New("fake git: not modelled")

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

// moveTo moves what is checked out in the worktree w to commit, as the git
// command does: it writes into its folder only the files that differ
// between the two commits, and refuses, with git's account of why, when
// one of them has a change of its own or is untracked where commit adds
// it. A tracked file that is missing isn't a change. A move with changes
// staged isn't modelled.
func (g fakeRepo) moveTo(r *fakeRepository, w *fakeWorktree, commit, command string) (string, error) {
	if w.staged != nil {
		return "", g.f.notModelled("a %s in %s, with changes staged", command, w.dir)
	}
	was, now := r.treeOf(r.at(w)), r.treeOf(commit)
	var changed, untracked []string
	for _, p := range touched(was, now) {
		content, err := os.ReadFile(filepath.Join(w.dir, filepath.FromSlash(p)))
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
	if err := checkOut(w.dir, was, now); err != nil {
		return "", err
	}
	r.setAt(w, commit)
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

// checkOut writes the files that differ from was to now into the folder
// dir, removing those now hasn't, with any folder that leaves empty.
func checkOut(dir string, was, now fakeTree) error {
	for p := range was {
		if _, ok := now[p]; ok {
			continue
		}
		name := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		for d := filepath.Dir(name); d != dir && os.Remove(d) == nil; d = filepath.Dir(d) {
		}
	}
	for p, content := range now {
		if old, ok := was[p]; ok && old == content {
			continue
		}
		name := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// folderTree is what the worktree folder dir holds, as `git add -A` would
// stage it. What git would treat in ways the Fake doesn't model, an
// ignore file, a nested repository, a link, an executable or a path
// status would quote, isn't modelled.
func (f *Fake) folderTree(dir string) (fakeTree, error) {
	tree := fakeTree{}
	err := filepath.WalkDir(dir, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}
		p := filepath.ToSlash(rel)
		switch {
		case name == dir:
			return nil
		case p == ".git":
			return skip(d)
		case d.Name() == ".git":
			return f.notModelled("the repository %s, inside the worktree %s", name, dir)
		case d.Name() == ".gitignore":
			return f.notModelled("the ignore file %s", name)
		case strings.Trim(p, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._/-") != "":
			return f.notModelled("the path %q, which status quotes", p)
		case d.IsDir():
			return nil
		case !d.Type().IsRegular():
			return f.notModelled("%s, which isn't a file or a folder", name)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&0o111 != 0 && runtime.GOOS != "windows" {
			return f.notModelled("the executable %s", name)
		}
		content, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		tree[p] = string(content)
		return nil
	})
	return tree, err
}

// skip skips what WalkDir is at, a folder and what is in it or a file.
func skip(d fs.DirEntry) error {
	if d.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

// realPath is p with its symbolic links resolved, as git reports a
// worktree's path, as far as they exist: a path that doesn't is its
// folder's real path and its name.
func realPath(p string) string {
	p = filepath.Clean(p)
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(realPath(parent), filepath.Base(p))
}

// within is whether the path p is the folder dir or in it.
func within(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
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

// repository is the repository whose folder, or one of whose worktrees'
// folders, is dir. A folder inside one isn't modelled.
func (g fakeRepo) repository() (*fakeRepository, error) {
	dir := realPath(g.dir)
	r, w := g.f.holding(dir)
	switch {
	case r == nil:
		return nil, fmt.Errorf("fatal: not a git repository: %s", g.dir)
	case w.dir != dir:
		return nil, g.f.notModelled("the repository opened in %s, inside the worktree %s", g.dir, w.dir)
	}
	return r, nil
}

// holding is the repository with a worktree whose folder is dir, resolved,
// or holds it, with that worktree, or nil when there is none.
func (f *Fake) holding(dir string) (*fakeRepository, *fakeWorktree) {
	for _, r := range f.repos {
		for _, w := range r.worktrees {
			if within(dir, w.dir) {
				return r, w
			}
		}
	}
	return nil, nil
}

// overlapping is a worktree, of any repository, whose folder is dir,
// resolved, holds it or is in it, or nil when there is none.
func (f *Fake) overlapping(dir string) *fakeWorktree {
	for _, r := range f.repos {
		for _, w := range r.worktrees {
			if within(dir, w.dir) || within(w.dir, dir) {
				return w
			}
		}
	}
	return nil
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
	w, err := g.worktree(r, dir)
	if err != nil {
		return "", err
	}
	to, err := g.resolve(r, commit)
	if err != nil {
		return gitFailed("merge: %s - not something we can merge", commit)
	}
	at := r.at(w)
	switch {
	case r.ancestors(at)[to]:
		return "", nil
	case !r.ancestors(to)[at]:
		return gitFailed("fatal: Not possible to fast-forward, aborting.")
	}
	return g.moveTo(r, w, to, "merge")
}

func (g fakeRepo) Reset(dir, commit, old string) (string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	w, err := g.worktree(r, dir)
	if err != nil {
		return "", err
	}
	to, err := g.resolve(r, commit)
	if err != nil {
		return gitFailed("fatal: ambiguous argument '%s': unknown revision or path not in the working tree.", commit)
	}
	was := r.at(w)
	if out, err := g.moveTo(r, w, to, "reset"); err != nil {
		return out, err
	}
	// As Real's does, a reset of a branch that moved since the caller
	// looked puts it back.
	if was != old {
		if out, err := g.moveTo(r, w, was, "reset"); err != nil {
			return out, fmt.Errorf("the branch moved to %s meanwhile, and can't be put back: %w", was, err)
		}
		return "", fmt.Errorf("the branch moved to %s meanwhile", was)
	}
	return "", nil
}

// worktree is r's worktree in the folder dir. A folder that isn't one
// isn't modelled; one whose folder is gone fails as git would in it.
func (g fakeRepo) worktree(r *fakeRepository, dir string) (*fakeWorktree, error) {
	if err := g.notBare(r, "a checkout"); err != nil {
		return nil, err
	}
	w := r.worktree(realPath(dir))
	if w == nil {
		return nil, g.f.notModelled("the folder %s, which isn't a worktree of the repository", dir)
	}
	if _, err := os.Stat(w.dir); err != nil {
		return nil, err
	}
	if _, err := g.gone(r, w); err != nil {
		return nil, err
	}
	return w, nil
}

// gone is whether the worktree w's folder is gone. A worktree whose
// folder is there but whose .git file isn't, which git would prune,
// isn't modelled.
func (g fakeRepo) gone(r *fakeRepository, w *fakeWorktree) (bool, error) {
	if _, err := os.Lstat(w.dir); errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if _, err := os.Lstat(filepath.Join(w.dir, ".git")); w != r.main() && errors.Is(err, fs.ErrNotExist) {
		return false, g.f.notModelled("the worktree %s, whose .git file is gone", w.dir)
	}
	return false, nil
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

func (g fakeRepo) Worktrees() ([]Worktree, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return nil, err
	}
	if err := g.notBare(r, "worktrees"); err != nil {
		return nil, err
	}
	var list []Worktree
	for _, w := range r.worktrees {
		list = append(list, Worktree{Path: w.dir, Branch: w.branch()})
	}
	return list, nil
}

// Status is what status --porcelain reports of the folder dir against
// what is checked out there: each changed or deleted file, sorted, then
// each untracked one, sorted.
func (g fakeRepo) Status(dir string) (string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	w, err := g.worktree(r, dir)
	if err != nil {
		return "", err
	}
	return g.status(r, w)
}

// status is Status of the worktree w: each changed path's two columns,
// what is staged against what is checked out, then the folder against
// what is staged, as porcelain gives them, then what is untracked.
func (g fakeRepo) status(r *fakeRepository, w *fakeWorktree) (string, error) {
	folder, err := g.f.folderTree(w.dir)
	if err != nil {
		return "", err
	}
	tree, index := r.treeOf(r.at(w)), r.index(w)
	var changed, untracked []string
	for _, p := range slices.Compact(slices.Sorted(slices.Values(append(touched(tree, index), touched(index, folder)...)))) {
		_, tracked := tree[p]
		_, staged := index[p]
		_, present := folder[p]
		if !staged && present {
			untracked = append(untracked, "?? "+p)
			if !tracked {
				continue
			}
		}
		indexColumn, folderColumn := ' ', ' '
		switch {
		case !tracked:
			indexColumn = 'A'
		case !staged:
			indexColumn = 'D'
		case tree[p] != index[p]:
			indexColumn = 'M'
		}
		switch {
		case staged && !present:
			folderColumn = 'D'
		case staged && folder[p] != index[p]:
			folderColumn = 'M'
		}
		changed = append(changed, string([]rune{indexColumn, folderColumn, ' '})+p)
	}
	return strings.TrimSpace(strings.Join(append(changed, untracked...), "\n")), nil
}

// index is the worktree w's index: what is staged there, or else the
// tree of what is checked out.
func (r *fakeRepository) index(w *fakeWorktree) fakeTree {
	if w.staged != nil {
		return w.staged
	}
	return r.treeOf(r.at(w))
}

// RemoveWorktree removes the worktree at path and its folder, as git does:
// not the repository's own, nor one with changes or untracked files. One
// whose folder is gone is forgotten.
func (g fakeRepo) RemoveWorktree(path string) error {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return err
	}
	w := r.worktree(realPath(path))
	switch {
	case w == nil:
		return fmt.Errorf("fatal: '%s' is not a working tree", path)
	case w == r.main():
		return fmt.Errorf("fatal: '%s' is a main working tree", path)
	}
	switch gone, err := g.gone(r, w); {
	case err != nil:
		return err
	case gone:
		r.forget(w)
		return nil
	}
	status, err := g.status(r, w)
	switch {
	case err != nil:
		return err
	case status != "":
		return fmt.Errorf("fatal: '%s' contains modified or untracked files, use --force to delete it", path)
	}
	if err := os.RemoveAll(w.dir); err != nil {
		return err
	}
	r.forget(w)
	return nil
}

// forget drops the worktree w from r.
func (r *fakeRepository) forget(w *fakeWorktree) {
	r.worktrees = slices.DeleteFunc(r.worktrees, func(o *fakeWorktree) bool { return o == w })
}

// AddWorktree adds a worktree at path with branch checked out there, as
// git does, unless it is checked out in another. A rev that isn't a
// branch, which git would check out detached, isn't modelled.
func (g fakeRepo) AddWorktree(path, branch string) error {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return err
	}
	if err := g.notBare(r, "a worktree"); err != nil {
		return err
	}
	ref := "refs/heads/" + branch
	if _, ok := r.refs[ref]; !ok {
		_, err := g.resolve(r, branch)
		switch {
		case err == nil:
			return g.f.notModelled("a worktree on %s, which isn't a branch", branch)
		case errors.Is(err, errNotModelled):
			return err
		}
		return fmt.Errorf("fatal: invalid reference: %s", branch)
	}
	if w := r.usedBy(ref); w != nil {
		return fmt.Errorf("fatal: '%s' is already used by worktree at '%s'", branch, w.dir)
	}
	return g.addWorktree(r, path, ref)
}

// usedBy is the worktree of r with ref, a branch's full name, checked
// out, or nil.
func (r *fakeRepository) usedBy(ref string) *fakeWorktree {
	for _, w := range r.worktrees {
		if w.head == ref {
			return w
		}
	}
	return nil
}

// AddWorktreeOnNewBranch creates the branch, then adds its worktree, as
// git does: a branch that exists already stops it, but a path that can't
// be its worktree leaves the branch made.
func (g fakeRepo) AddWorktreeOnNewBranch(path, branch, from string) error {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return err
	}
	if err := g.notBare(r, "a worktree"); err != nil {
		return err
	}
	ref := "refs/heads/" + branch
	if _, ok := r.refs[ref]; ok {
		return fmt.Errorf("fatal: a branch named '%s' already exists", branch)
	}
	commit, err := g.resolve(r, from)
	if err != nil {
		return err
	}
	if err := g.checkRef(r, ref, ""); err != nil {
		return err
	}
	r.refs[ref] = commit
	return g.addWorktree(r, path, ref)
}

func (g fakeRepo) AddDetachedWorktree(path, rev string) error {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return err
	}
	if err := g.notBare(r, "a worktree"); err != nil {
		return err
	}
	commit, err := g.resolve(r, rev)
	if err != nil {
		return err
	}
	return g.addWorktree(r, path, commit)
}

// notBare is nil, or, in a bare repository, which has nothing checked
// out, the failure of what, which isn't modelled there.
func (g fakeRepo) notBare(r *fakeRepository, what string) error {
	if r.bare {
		return g.f.notModelled("%s in the bare repository %s", what, g.dir)
	}
	return nil
}

// addWorktree adds a worktree with head checked out at path, a folder
// that is empty or doesn't exist. A relative path, or one inside or
// around another worktree's folder, isn't modelled.
func (g fakeRepo) addWorktree(r *fakeRepository, path, head string) error {
	if !filepath.IsAbs(path) {
		return g.f.notModelled("a worktree at the relative path %s", path)
	}
	if entries, err := os.ReadDir(path); err == nil && len(entries) > 0 || err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("fatal: '%s' already exists", path)
	}
	dir := realPath(path)
	switch w := g.f.overlapping(dir); {
	case w == nil:
	case w.dir == dir:
		return fmt.Errorf("fatal: '%s' is a missing but already registered worktree;\nuse 'add -f' to override, or 'prune' or 'remove' to clear", path)
	default:
		return g.f.notModelled("a worktree at %s, with the worktree %s inside or around it", path, w.dir)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	w := &fakeWorktree{dir: realPath(path), head: head}
	// As git's, it holds a .git file, for the worktree that loses it.
	if err := os.WriteFile(filepath.Join(w.dir, ".git"), []byte("gitdir: fake\n"), 0o644); err != nil {
		return err
	}
	if err := checkOut(w.dir, fakeTree{}, r.treeOf(r.at(w))); err != nil {
		return err
	}
	r.worktrees = append(r.worktrees, w)
	slices.SortFunc(r.worktrees[1:], func(a, b *fakeWorktree) int { return strings.Compare(a.dir, b.dir) })
	return nil
}

// CheckoutDetached detaches the worktree dir, then checks out rev there,
// as checkout --detach does.
func (g fakeRepo) CheckoutDetached(dir, rev string) error {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return err
	}
	w, err := g.worktree(r, dir)
	if err != nil {
		return err
	}
	commit, err := g.resolve(r, rev)
	if err != nil {
		return err
	}
	if out, err := g.switchTo(r, w, commit, ""); err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}

// PruneWorktrees forgets each worktree whose folder is gone.
func (g fakeRepo) PruneWorktrees() error {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return err
	}
	var gone []*fakeWorktree
	for _, w := range r.worktrees[1:] {
		isGone, err := g.gone(r, w)
		if err != nil {
			return err
		}
		if isGone {
			gone = append(gone, w)
		}
	}
	for _, w := range gone {
		r.forget(w)
	}
	return nil
}

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
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	w, err := g.worktree(r, dir)
	if err != nil {
		return "", err
	}
	if w.branch() == "" {
		return "", errors.New("git symbolic-ref: HEAD names no branch")
	}
	return w.branch(), nil
}

// LastCommit is the abbreviated ID and the subject of what is checked out
// in the worktree dir: its message's first paragraph on one line, as %s
// is.
func (g fakeRepo) LastCommit(dir string) (id, subject string, err error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", "", err
	}
	w, err := g.worktree(r, dir)
	if err != nil {
		return "", "", err
	}
	commit := r.at(w)
	return commit[:7], subjectOf(r.commits[commit].message), nil
}

// subjectOf is message's first paragraph on one line, as %s gives it.
func subjectOf(message string) string {
	paragraph, _, _ := strings.Cut(strings.TrimLeft(message, "\n"), "\n\n")
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, " ")
}

// Subject is the subject of the commit rev in the repository at dir, as
// `git log --format=%s` gives it, for a test to check. It fails the test
// when dir holds no repository or rev is no commit there.
func (f *Fake) Subject(dir, rev string) string {
	f.tb.Helper()
	var s string
	f.reading(dir, rev, func(r *fakeRepository, commit string) { s = subjectOf(r.commits[commit].message) })
	return s
}

// File is the content of the file path at the commit rev in the
// repository at dir, and whether one is committed there, for a test to
// check. It fails the test when dir holds no repository or rev is no
// commit there.
func (f *Fake) File(dir, rev, path string) (content string, ok bool) {
	f.tb.Helper()
	f.reading(dir, rev, func(r *fakeRepository, commit string) { content, ok = r.treeOf(commit)[path] })
	return content, ok
}

// Ref is the commit rev names in the repository at dir, and whether it
// names one, for a test to check. It fails the test when dir holds no
// repository.
func (f *Fake) Ref(dir, rev string) (commit string, ok bool) {
	f.tb.Helper()
	g := fakeRepo{f: f, dir: filepath.Clean(dir)}
	r, unlock, err := g.lock()
	if err == nil {
		var missing error
		commit, missing = g.resolve(r, rev)
		ok = missing == nil
	}
	unlock()
	if err != nil {
		f.tb.Fatal(err)
	}
	return commit, ok
}

// reading calls read with the repository at dir and rev resolved there,
// failing the test when either can't be.
func (f *Fake) reading(dir, rev string, read func(r *fakeRepository, commit string)) {
	f.tb.Helper()
	g := fakeRepo{f: f, dir: filepath.Clean(dir)}
	r, unlock, err := g.lock()
	if err == nil {
		var commit string
		if commit, err = g.resolve(r, rev); err == nil {
			read(r, commit)
		}
	}
	unlock()
	if err != nil {
		f.tb.Fatal(err)
	}
}
