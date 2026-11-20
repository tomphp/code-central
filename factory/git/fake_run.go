package git

import (
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Run runs git with args in the worktree dir, as a fake claude's Script
// step does there, and returns git's output, failing as git does. It
// models the commands a Script commits with: add and rm of paths, with
// -q, add's -A and rm's -r; commit with -q and -m; checkout of a branch,
// with -q, -b and --detach; and rev-parse HEAD. Any other command,
// option or path isn't modelled.
func (f *Fake) Run(dir string, args ...string) (string, error) {
	g := fakeRepo{f: f, dir: filepath.Clean(dir)}
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	w, err := g.worktree(r, dir)
	if err != nil {
		return "", err
	}
	s := scriptRun{g: g, r: r, w: w, args: args}
	if len(args) == 0 {
		return s.notModelled()
	}
	switch args[0] {
	case "add":
		return s.add()
	case "rm":
		return s.rm()
	case "commit":
		return s.commit()
	case "checkout":
		return s.checkout()
	case "rev-parse":
		if slices.Equal(args[1:], []string{"HEAD"}) {
			return r.at(w), nil
		}
	}
	return s.notModelled()
}

// scriptRun is one Run of the command args in the worktree w of r.
type scriptRun struct {
	g    fakeRepo
	r    *fakeRepository
	w    *fakeWorktree
	args []string
}

// notModelled fails the test on the command, naming it.
func (s scriptRun) notModelled() (string, error) {
	return "", s.g.f.notModelled("the command git %s", strings.Join(s.args, " "))
}

// options parses the command's arguments after its name into the options
// given, by name, with their values, and its operands: flags take no
// value and valued take the next argument. ok is false on any other
// option, and on "--" unless operands may follow it.
func (s scriptRun) options(flags, valued []string, dashes bool) (opts map[string][]string, operands []string, ok bool) {
	opts = map[string][]string{}
	args := s.args[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--" && dashes:
			return opts, append(operands, args[i+1:]...), true
		case slices.Contains(flags, a):
			opts[a] = append(opts[a], "")
		case slices.Contains(valued, a) && i+1 < len(args):
			i++
			opts[a] = append(opts[a], args[i])
		case strings.HasPrefix(a, "-"):
			return nil, nil, false
		default:
			operands = append(operands, a)
		}
	}
	return opts, operands, true
}

// pathspecs are the operands as paths in the worktree, slash-separated,
// with "" for all of it. A pathspec git would read as more than a path,
// or one outside the worktree, isn't modelled.
func (s scriptRun) pathspecs(operands []string) ([]string, bool) {
	var specs []string
	for _, o := range operands {
		p := path.Clean(filepath.ToSlash(o))
		if filepath.IsAbs(o) || strings.ContainsAny(o, `*?[]:\!`) || p == ".." || strings.HasPrefix(p, "../") {
			return nil, false
		}
		if p == "." {
			p = ""
		}
		specs = append(specs, p)
	}
	return specs, true
}

// under is whether the path p is the pathspec spec or in it.
func under(p, spec string) bool {
	return spec == "" || p == spec || strings.HasPrefix(p, spec+"/")
}

// add stages what the folder holds under each pathspec, as add does:
// each file there as it is, and each one staged but gone removed.
func (s scriptRun) add() (string, error) {
	opts, operands, ok := s.options([]string{"-q", "-A", "--all"}, nil, true)
	if !ok {
		return s.notModelled()
	}
	if len(operands) == 0 {
		if opts["-A"] == nil && opts["--all"] == nil {
			return "Nothing specified, nothing added.", nil
		}
		operands = []string{"."}
	}
	specs, ok := s.pathspecs(operands)
	if !ok {
		return s.notModelled()
	}
	folder, err := s.g.f.folderTree(s.w.dir)
	if err != nil {
		return "", err
	}
	index := maps.Clone(s.r.index(s.w))
	paths := slices.Collect(maps.Keys(folder))
	paths = slices.AppendSeq(paths, maps.Keys(index))
	for i, spec := range specs {
		// The worktree itself always matches, even empty.
		if spec != "" && !slices.ContainsFunc(paths, func(p string) bool { return under(p, spec) }) {
			return gitFailed("fatal: pathspec '%s' did not match any files", operands[i])
		}
	}
	for _, p := range paths {
		if !slices.ContainsFunc(specs, func(spec string) bool { return under(p, spec) }) {
			continue
		}
		if content, ok := folder[p]; ok {
			index[p] = content
		} else {
			delete(index, p)
		}
	}
	if p, ok := index.clash(); ok {
		return "", s.g.f.notModelled("the path %s, staged as both a file and a folder", p)
	}
	s.stage(index)
	return "", nil
}

// stage makes index the worktree's index.
func (s scriptRun) stage(index fakeTree) {
	s.w.staged = index
	if maps.Equal(index, s.r.treeOf(s.r.at(s.w))) {
		s.w.staged = nil
	}
}

// rm removes the files staged under each pathspec from the index and the
// folder, as rm does. Removing a file with changes, or one staged but not
// committed, which git refuses, isn't modelled.
func (s scriptRun) rm() (string, error) {
	opts, operands, ok := s.options([]string{"-q", "-r"}, nil, true)
	if !ok || len(operands) == 0 {
		return s.notModelled()
	}
	specs, ok := s.pathspecs(operands)
	if !ok {
		return s.notModelled()
	}
	folder, err := s.g.f.folderTree(s.w.dir)
	if err != nil {
		return "", err
	}
	tree, index := s.r.treeOf(s.r.at(s.w)), maps.Clone(s.r.index(s.w))
	gone := fakeTree{}
	for i, spec := range specs {
		_, file := index[spec]
		var found bool
		for p, content := range index {
			if !under(p, spec) {
				continue
			}
			found = true
			was, inFolder := folder[p]
			if committed, inTree := tree[p]; (inFolder && was != content) || !inTree || committed != content {
				return "", s.g.f.notModelled("the command git %s, removing %s, which has changes", strings.Join(s.args, " "), p)
			}
			gone[p] = content
		}
		switch {
		case !found:
			return gitFailed("fatal: pathspec '%s' did not match any files", operands[i])
		case !file && opts["-r"] == nil:
			return gitFailed("fatal: not removing '%s' recursively without -r", operands[i])
		}
	}
	var out strings.Builder
	for _, p := range slices.Sorted(maps.Keys(gone)) {
		delete(index, p)
		if opts["-q"] == nil {
			fmt.Fprintf(&out, "rm '%s'\n", p)
		}
	}
	if err := checkOut(s.w.dir, gone, fakeTree{}); err != nil {
		return "", err
	}
	s.stage(index)
	return strings.TrimSuffix(out.String(), "\n"), nil
}

// commit commits what is staged onto what is checked out, with the
// paragraphs of its -m options as its message.
func (s scriptRun) commit() (string, error) {
	opts, operands, ok := s.options([]string{"-q"}, []string{"-m"}, false)
	if !ok || operands != nil || opts["-m"] == nil {
		return s.notModelled()
	}
	parent, index := s.r.at(s.w), s.r.index(s.w)
	msg := cleanMessage(strings.Join(opts["-m"], "\n\n"))
	switch {
	case maps.Equal(index, s.r.treeOf(parent)):
		return gitFailed("nothing added to commit")
	case msg == "":
		return gitFailed("Aborting commit due to empty commit message.")
	}
	c := s.r.writeCommit(fakeCommit{tree: s.r.writeTree(maps.Clone(index)), parents: []string{parent}, author: fakeAuthor, message: msg})
	s.r.setAt(s.w, c)
	s.w.staged = nil
	if opts["-q"] != nil {
		return "", nil
	}
	on := s.w.branch()
	if on == "" {
		on = "detached HEAD"
	}
	return fmt.Sprintf("[%s %s] %s", on, c[:7], subjectOf(msg)), nil
}

// cleanMessage is msg as commit keeps it: without trailing spaces on its
// lines, blank lines at either end or runs of blank lines.
func cleanMessage(msg string) string {
	var lines []string
	for _, l := range strings.Split(msg, "\n") {
		l = strings.TrimRight(l, " \t")
		if l == "" && (len(lines) == 0 || lines[len(lines)-1] == "") {
			continue
		}
		lines = append(lines, l)
	}
	return strings.TrimSuffix(strings.Join(lines, "\n"), "\n")
}

// checkout checks out an existing branch, makes a branch with -b and
// checks it out, or detaches with --detach, as checkout does.
func (s scriptRun) checkout() (string, error) {
	opts, operands, ok := s.options([]string{"-q", "--detach"}, []string{"-b"}, false)
	if !ok || len(operands) > 1 || len(opts["-b"]) > 1 || opts["-b"] != nil && opts["--detach"] != nil {
		return s.notModelled()
	}
	start := s.r.at(s.w)
	if len(operands) == 1 && (opts["-b"] != nil || opts["--detach"] != nil) {
		var err error
		if start, err = s.g.resolve(s.r, operands[0]); err != nil {
			return "", err
		}
	}
	switch {
	case opts["-b"] != nil:
		branch := opts["-b"][0]
		ref := "refs/heads/" + branch
		if _, ok := s.r.refs[ref]; ok {
			return gitFailed("fatal: a branch named '%s' already exists", branch)
		}
		if err := s.g.checkRef(s.r, ref, ""); err != nil {
			return "", err
		}
		return s.g.switchTo(s.r, s.w, start, ref)
	case opts["--detach"] != nil:
		return s.g.switchTo(s.r, s.w, start, "")
	case len(operands) == 0:
		return s.notModelled()
	}
	ref := "refs/heads/" + operands[0]
	commit, ok := s.r.refs[ref]
	if !ok {
		return s.notModelled()
	}
	switch w := s.r.usedBy(ref); {
	case w == s.w:
		return fmt.Sprintf("Already on '%s'", operands[0]), nil
	case w != nil:
		return gitFailed("fatal: '%s' is already used by worktree at '%s'", operands[0], w.dir)
	}
	return s.g.switchTo(s.r, s.w, commit, ref)
}

// switchTo checks commit out in the worktree w, then puts w on ref, a
// branch's full name, made or moved there, or leaves it detached for "".
// Refused, w is as it was.
func (g fakeRepo) switchTo(r *fakeRepository, w *fakeWorktree, commit, ref string) (string, error) {
	// Detached, moveTo moves w itself, not its branch.
	was := w.head
	w.head = r.at(w)
	if out, err := g.moveTo(r, w, commit, "checkout"); err != nil {
		w.head = was
		return out, err
	}
	if ref != "" {
		r.refs[ref] = commit
		w.head = ref
	}
	return "", nil
}
