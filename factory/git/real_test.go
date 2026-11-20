package git_test

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/git"
	"github.com/tomoram/code-central/factory/git/gittest"
)

func TestRealGitContract(t *testing.T) {
	binary, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, func(t *testing.T) gittest.Setup {
		r := realRepository{t: t, binary: binary, dir: t.TempDir(), environ: realEnviron(t)}
		r.git(nil, "", "init", "-q", "-b", "main", ".")
		if err := os.WriteFile(filepath.Join(r.dir, "README.md"), []byte("Hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		r.git(nil, "", "add", "README.md")
		r.git(nil, "", "commit", "-q", "-m", "First")
		return gittest.Setup{Git: git.Real{Binary: binary, Environ: r.environ}, Dir: r.dir, Commit: r.commit}
	})
}

// realEnviron is git's environment for a contract case: no system or
// global config, no background maintenance, and a fixed author,
// committer and date, so that equal commits have equal IDs.
func realEnviron(t *testing.T) []string {
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+global,
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=maintenance.auto",
		"GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=gc.auto",
		"GIT_CONFIG_VALUE_1=0",
		"GIT_AUTHOR_NAME=Factory Test",
		"GIT_AUTHOR_EMAIL=factory@example.com",
		"GIT_AUTHOR_DATE=@1795000000 +0000",
		"GIT_COMMITTER_NAME=Factory Test",
		"GIT_COMMITTER_EMAIL=factory@example.com",
		"GIT_COMMITTER_DATE=@1795000000 +0000",
	)
}

// realRepository makes a contract case's commits in the repository at dir
// with git's plumbing, through an index of their own.
type realRepository struct {
	t       *testing.T
	binary  string
	dir     string
	environ []string
}

func (r realRepository) commit(c git.Change) string {
	r.t.Helper()
	index := []string{"GIT_INDEX_FILE=" + filepath.Join(r.t.TempDir(), "index")}
	if len(c.Parents) > 0 {
		r.git(index, "", "read-tree", c.Parents[0])
	}
	for _, path := range slices.Sorted(maps.Keys(c.Files)) {
		blob := r.git(nil, c.Files[path], "hash-object", "-w", "--stdin")
		r.git(index, "", "update-index", "--add", "--cacheinfo", "100644,"+blob+","+path)
	}
	for _, path := range c.Removed {
		r.git(index, "", "update-index", "--force-remove", path)
	}
	tree := r.git(index, "", "write-tree")
	args := []string{"commit-tree", tree, "-m", c.Message}
	for _, p := range c.Parents {
		args = append(args, "-p", p)
	}
	var author []string
	if c.Author != "" {
		name, email, ok := strings.Cut(strings.TrimSuffix(c.Author, ">"), " <")
		if !ok {
			r.t.Fatalf("author %q isn't Name <email>", c.Author)
		}
		author = []string{"GIT_AUTHOR_NAME=" + name, "GIT_AUTHOR_EMAIL=" + email}
	}
	return r.git(author, "", args...)
}

// git runs git in the repository with env added to its environment and
// input on its standard input, and returns its trimmed output, failing
// the test when git fails.
func (r realRepository) git(env []string, input string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command(r.binary, args...)
	cmd.Dir = r.dir
	cmd.Env = append(slices.Clone(r.environ), env...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
