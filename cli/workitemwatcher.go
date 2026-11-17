package cli

import (
	"context"
	"os"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/workitemwatcher"
)

// The Work Item Watcher (Spec code-central-894) is a hidden subcommand:
// the service starts it through its own executable, and nobody types it.
func init() {
	commands[workitemwatcher.Command] = command{
		hidden: true,
		usage:  workitemwatcher.Command + " --project <key>  watch the Project's Azure Boards work items until stdin closes",
		run:    runWorkItemWatcher,
	}
}

// runWorkItemWatcher runs the Work Item Watcher's watch mode: it polls
// for one Project until its stdin, a pipe from the service, closes,
// writing as the integration Actor.
func runWorkItemWatcher(ctx context.Context, e Env, args []string) error {
	fs := flagSet(workitemwatcher.Command)
	project := fs.String("project", "", "the Project to watch")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	switch {
	case len(pos) > 0:
		return usagef("unexpected argument %q", pos[0])
	case *project == "":
		return usagef("the Work Item Watcher needs --project")
	}
	c := newClient(e)
	c.Actor = core.Integration
	return workitemwatcher.Watch(ctx, workitemwatcher.Config{Client: c, Project: *project, Env: gh.Env{Environ: os.Environ()},
		Clock: clock.System{}, Stdin: e.Stdin, Log: e.Stderr})
}
