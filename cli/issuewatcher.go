package cli

import (
	"context"
	"encoding/json"
	"os"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/issuewatcher"
)

// The Issue Watcher (Spec code-central-279) is a hidden subcommand: the
// service starts it through its own executable, and nobody types it.
func init() {
	commands[issuewatcher.Command] = command{
		hidden: true,
		usage: issuewatcher.Command + " --project <key>  watch the Project's GitHub issues until stdin closes; " +
			issuewatcher.Command + " --check --repository <owner/name> [--gh-path <path>]  check the Issue Watcher can work the repository's issues, answering JSON",
		run: runIssueWatcher,
	}
}

// runIssueWatcher runs the Issue Watcher. Its watch mode polls for one
// Project until its stdin, a pipe from the service, closes, writing as
// the integration Actor. Its check mode prints one Issues check result as
// JSON, a refusal's reason and fix included, and exits 0; the service
// reads it.
func runIssueWatcher(ctx context.Context, e Env, args []string) error {
	fs := flagSet(issuewatcher.Command)
	check := fs.Bool("check", false, "check access once and exit")
	project := fs.String("project", "", "the Project to watch")
	var in core.IssuesCheckInput
	fs.StringVar(&in.Repository, "repository", "", "GitHub repository, owner/name")
	fs.StringVar(&in.GHPath, "gh-path", "", "the gh path Option")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	switch {
	case len(pos) > 0:
		return usagef("unexpected argument %q", pos[0])
	case !*check && *project == "":
		return usagef("the Issue Watcher needs --project, or --check")
	case !*check:
		c := newClient(e)
		c.Actor = core.Integration
		return issuewatcher.Watch(ctx, issuewatcher.Config{Client: c, Project: *project, Env: gh.Env{Environ: os.Environ()},
			Clock: clock.System{}, Stdin: e.Stdin, Log: e.Stderr})
	case in.Repository == "":
		return usagef("--check needs --repository")
	}
	r := issuewatcher.Check(ctx, in, gh.Env{Environ: os.Environ()})
	return json.NewEncoder(e.Stdout).Encode(r)
}
