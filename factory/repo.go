package factory

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

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

// cloneRepository clones url to path, naming its remote remote; out is
// git's output when it fails, and nothing is left at path then. A stop
// (ctx) kills it. The clone is made in a folder beside path, the
// Factory's own, and moved into place once whole, so a clone cut off
// part way is never at path to be adopted.
func cloneRepository(ctx context.Context, git string, environ []string, remote, url, path string) (string, error) {
	partial := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".cloning")
	if err := os.RemoveAll(partial); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, git, "clone", "--quiet", "--origin", remote, url, partial)
	// A prompt for credentials would wait forever: nobody answers it.
	cmd.Env = append(slices.Clone(environ), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err == nil {
		err = os.Rename(partial, path)
	}
	if err != nil {
		os.RemoveAll(partial)
	}
	return strings.TrimSpace(string(out)), err
}
