package factory

import (
	"context"
	"os"
	"path/filepath"

	"github.com/tomoram/code-central/factory/git"
)

// cloneRepository clones url to path through g, naming its remote remote;
// out is git's output when it fails, and nothing is left at path then. A
// stop (ctx) kills it. The clone is made in a folder beside path, the
// Factory's own, and moved into place once whole, so a clone cut off
// part way is never at path to be adopted.
func cloneRepository(ctx context.Context, g git.Git, remote, url, path string) (string, error) {
	partial := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".cloning")
	if err := os.RemoveAll(partial); err != nil {
		return "", err
	}
	out, err := g.Clone(ctx, remote, url, partial)
	if err == nil {
		err = os.Rename(partial, path)
	}
	if err != nil {
		os.RemoveAll(partial)
	}
	return out, err
}
