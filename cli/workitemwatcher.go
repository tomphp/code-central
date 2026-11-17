package cli

import (
	"context"
	"encoding/json"
	"os"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/workitemwatcher"
)

// The Work Item Watcher (Spec code-central-894) is a hidden subcommand:
// the service starts it through its own executable, and nobody types it.
func init() {
	commands[workitemwatcher.Command] = command{
		hidden: true,
		usage: workitemwatcher.Command + " --project <key>  watch the Project's Azure Boards work items until stdin closes; " +
			workitemwatcher.Command + " --check --mode saved_query --query-url <url> | --mode wiql --project-url <url> --where <clause> [--pat-env <name>]  check the Work Item Watcher can work the query's work items, answering JSON",
		run: runWorkItemWatcher,
	}
}

// runWorkItemWatcher runs the Work Item Watcher's watch mode: it polls
// for one Project until its stdin, a pipe from the service, closes,
// writing as the integration Actor.
func runWorkItemWatcher(ctx context.Context, e Env, args []string) error {
	fs := flagSet(workitemwatcher.Command)
	check := fs.Bool("check", false, "check access once and exit")
	project := fs.String("project", "", "the Project to watch")
	var in core.AzureBoardsCheckInput
	mode := fs.String("mode", string(store.AzureBoardsSavedQuery), "how the query is picked: saved_query or wiql")
	fs.StringVar(&in.QueryURL, "query-url", "", "the Saved query's URL")
	fs.StringVar(&in.ProjectURL, "project-url", "", "the Azure DevOps project URL a WIQL filter runs in")
	fs.StringVar(&in.Where, "where", "", "the WIQL filter's WHERE clause")
	fs.StringVar(&in.PATEnv, "pat-env", store.DefaultPATEnv, "the environment variable holding the Azure DevOps PAT")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	switch {
	case len(pos) > 0:
		return usagef("unexpected argument %q", pos[0])
	case !*check && *project == "":
		return usagef("the Work Item Watcher needs --project, or --check")
	case *check:
		in.Mode = store.AzureBoardsMode(*mode)
		switch {
		case !in.Mode.Valid():
			return usagef("--mode must be %s or %s", store.AzureBoardsSavedQuery, store.AzureBoardsWIQL)
		case in.Mode == store.AzureBoardsWIQL && (in.ProjectURL == "" || in.Where == ""):
			return usagef("--check with --mode wiql needs --project-url and --where")
		case in.Mode == store.AzureBoardsSavedQuery && in.QueryURL == "":
			return usagef("--check needs --query-url")
		}
		r := workitemwatcher.Check(ctx, in, gh.Env{Environ: os.Environ()})
		return json.NewEncoder(e.Stdout).Encode(r)
	}
	c := newClient(e)
	c.Actor = core.Integration
	return workitemwatcher.Watch(ctx, workitemwatcher.Config{Client: c, Project: *project, Env: gh.Env{Environ: os.Environ()},
		Clock: clock.System{}, Stdin: e.Stdin, Log: e.Stderr})
}
