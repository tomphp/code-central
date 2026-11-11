package cli

import (
	"context"
	"encoding/json"
	"os"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/pullrequestwatcher"
)

// The Pull Request Watcher (Spec code-central-383) is a hidden subcommand:
// the service starts it through its own executable, and nobody types it.
func init() {
	commands[pullrequestwatcher.Command] = command{
		hidden: true,
		usage: pullrequestwatcher.Command + " --project <key>  open and watch the Project's pull requests until stdin closes; " +
			pullrequestwatcher.Command + " --check --code-host github --repository <owner/name> [--gh-path <path>]  check the Pull Request Watcher can work the repository's pull requests, answering JSON",
		run: runPullRequestWatcher,
	}
}

// runPullRequestWatcher runs the Pull Request Watcher. Its watch mode polls
// for one Project until its stdin, a pipe from the service, closes. Its
// check mode prints one check result as JSON, a refusal's reason and fix
// included, and exits 0; the service reads it. It never opens a pull
// request.
func runPullRequestWatcher(ctx context.Context, e Env, args []string) error {
	fs := flagSet(pullrequestwatcher.Command)
	check := fs.Bool("check", false, "check access once and exit")
	project := fs.String("project", "", "the Project to watch")
	var in core.PullRequestCheckInput
	fs.StringVar(&in.CodeHost, "code-host", core.CodeHostGitHub, "the Code Host")
	fs.StringVar(&in.Repository, "repository", "", "the repository on the Code Host")
	fs.StringVar(&in.GHPath, "gh-path", "", "the gh path Option")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	switch {
	case len(pos) > 0:
		return usagef("unexpected argument %q", pos[0])
	case !*check && *project == "":
		return usagef("the Pull Request Watcher needs --project, or --check")
	case !*check:
		return pullrequestwatcher.Watch(ctx, pullrequestwatcher.Config{Client: newClient(e), Project: *project, Env: gh.Env{Environ: os.Environ()},
			Clock: clock.System{}, Stdin: e.Stdin, Log: e.Stderr})
	case in.Repository == "":
		return usagef("--check needs --repository")
	}
	r := pullrequestwatcher.Check(ctx, in, gh.Env{Environ: os.Environ()})
	return json.NewEncoder(e.Stdout).Encode(r)
}
