package wire

import (
	"context"
	"encoding/json"
	"github.com/tomoram/code-central/cli"
	"os"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/issuewatcher"
)

// The Issue Watcher (Spec code-central-279) is a hidden subcommand: the
// service starts it through its own executable, and nobody types it.
func init() {
	cli.Register(issuewatcher.Command, cli.Command{
		Hidden: true,
		Usage: issuewatcher.Command + " --project <key>  watch the Project's GitHub issues until stdin closes; " +
			issuewatcher.Command + " --check --repositories <owner/name,...> [--gh-path <path>]  check the Issue Watcher can work the repositories' issues, answering JSON",
		Run: runIssueWatcher,
	})
}

// runIssueWatcher runs the Issue Watcher. Its watch mode polls for one
// Project until its stdin, a pipe from the service, closes, writing as
// the integration Actor. Its check mode prints one Issues check result as
// JSON, a refusal's reason and fix included, and exits 0; the service
// reads it.
func runIssueWatcher(ctx context.Context, e cli.Env, args []string) error {
	fs := cli.FlagSet(issuewatcher.Command)
	check := fs.Bool("check", false, "check access once and exit")
	project := fs.String("project", "", "the Project to watch")
	var in api.IssuesCheckInput
	repositories := fs.String("repositories", "", "GitHub repositories, owner/name, comma-separated")
	fs.StringVar(&in.GHPath, "gh-path", "", "the gh path Option")
	pos, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	switch {
	case len(pos) > 0:
		return cli.UsageError("unexpected argument %q", pos[0])
	case !*check && *project == "":
		return cli.UsageError("the Issue Watcher needs --project, or --check")
	case !*check:
		c := cli.NewClient(e)
		c.Actor = api.Integration
		return issuewatcher.Watch(ctx, issuewatcher.Config{Client: c, Project: *project, Env: gh.Env{Environ: os.Environ()},
			Clock: clock.System{}, Stdin: e.Stdin, Log: e.Stderr})
	case *repositories == "":
		return cli.UsageError("--check needs --repositories")
	}
	in.Repositories = strings.Split(*repositories, ",")
	r := issuewatcher.Check(ctx, in, gh.Env{Environ: os.Environ()})
	return json.NewEncoder(e.Stdout).Encode(r)
}
