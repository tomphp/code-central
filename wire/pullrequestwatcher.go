package wire

import (
	"context"
	"encoding/json"
	"github.com/tomoram/code-central/cli"
	"os"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/pullrequestwatcher"
)

// The Pull Request Watcher (Spec code-central-383) is a hidden subcommand:
// the service starts it through its own executable, and nobody types it.
func init() {
	cli.Register(pullrequestwatcher.Command, cli.Command{
		Hidden: true,
		Usage: pullrequestwatcher.Command + " --project <key>  open and watch the Project's pull requests until stdin closes; " +
			pullrequestwatcher.Command + " --check --code-host github|azure_repos --repository <owner/name, or organization/project/repository> [--repositories <JSON list of {name, code_host, repository}>] [--gh-path <path>] [--pat-env <variable>]  check the Pull Request Watcher can work the repository's pull requests, or each listed Repository's, answering JSON",
		Run: runPullRequestWatcher,
	})
}

// runPullRequestWatcher runs the Pull Request Watcher. Its watch mode polls
// for one Project until its stdin, a pipe from the service, closes. Its
// check mode prints one check result as JSON, a refusal's reason and fix
// included, and exits 0; the service reads it. It never opens a pull
// request.
func runPullRequestWatcher(ctx context.Context, e cli.Env, args []string) error {
	fs := cli.FlagSet(pullrequestwatcher.Command)
	check := fs.Bool("check", false, "check access once and exit")
	project := fs.String("project", "", "the Project to watch")
	var in api.PullRequestCheckInput
	codeHost := fs.String("code-host", string(api.CodeHostGitHub), "the Code Host")
	fs.StringVar(&in.Repository, "repository", "", "the repository on the Code Host")
	fs.StringVar(&in.GHPath, "gh-path", "", "the gh path Option")
	fs.StringVar(&in.PATEnv, "pat-env", "", "the environment variable holding the Azure Repos PAT")
	repositories := fs.String("repositories", "", "the Repositories to check, as JSON")
	pos, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	in.CodeHost = api.CodeHost(*codeHost)
	if *repositories != "" {
		if err := json.Unmarshal([]byte(*repositories), &in.Repositories); err != nil {
			return cli.UsageError("--repositories isn't a JSON list of Repositories: %v", err)
		}
	}
	switch {
	case len(pos) > 0:
		return cli.UsageError("unexpected argument %q", pos[0])
	case !*check && *project == "":
		return cli.UsageError("the Pull Request Watcher needs --project, or --check")
	case !*check:
		return pullrequestwatcher.Watch(ctx, pullrequestwatcher.Config{Client: cli.NewClient(e), Project: *project, Env: gh.Env{Environ: os.Environ()},
			Clock: clock.System{}, Stdin: e.Stdin, Log: e.Stderr})
	case in.Repository == "" && len(in.Repositories) == 0:
		return cli.UsageError("--check needs --repository")
	}
	r := pullrequestwatcher.Check(ctx, in, gh.Env{Environ: os.Environ()})
	return json.NewEncoder(e.Stdout).Encode(r)
}
