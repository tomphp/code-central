package cli

import (
	"context"
	"encoding/json"
	"os"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// The CI Watcher (Spec code-central-247) is a hidden subcommand: the
// service starts it through its own executable, and nobody types it.
func init() {
	commands[ciwatcher.Command] = command{
		hidden: true,
		usage:  ciwatcher.Command + " --check --provider github_actions --repository <owner/name> [--gh-path <path>]  check the CI Watcher can watch the repository, answering JSON",
		run:    runCIWatcher,
	}
}

// runCIWatcher runs the CI Watcher. Its check mode prints one CI check
// result as JSON, a refusal's reason and fix included, and exits 0; the
// service reads it.
func runCIWatcher(ctx context.Context, e Env, args []string) error {
	fs := flagSet(ciwatcher.Command)
	check := fs.Bool("check", false, "check access once and exit")
	var in core.CICheckInput
	provider := fs.String("provider", string(store.CIGitHubActions), "CI Provider")
	fs.StringVar(&in.Repository, "repository", "", "GitHub repository, owner/name")
	fs.StringVar(&in.GHPath, "gh-path", "", "the gh path Option")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	switch {
	case len(pos) > 0:
		return usagef("unexpected argument %q", pos[0])
	case !*check:
		return usagef("the CI Watcher runs only in its check mode, --check, so far")
	case in.Repository == "":
		return usagef("--check needs --repository")
	}
	in.Provider = store.CIProvider(*provider)
	r := ciwatcher.Check(ctx, in, ciwatcher.Env{Environ: os.Environ()})
	return json.NewEncoder(e.Stdout).Encode(r)
}
