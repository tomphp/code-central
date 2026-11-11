package cli

import (
	"context"
	"encoding/json"
	"os"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/store"
)

// The CI Watcher (Spec code-central-247) is a hidden subcommand: the
// service starts it through its own executable, and nobody types it.
func init() {
	commands[ciwatcher.Command] = command{
		hidden: true,
		usage: ciwatcher.Command + " --project <key>  watch the Project's CI until stdin closes; " +
			ciwatcher.Command + " --check --provider github_actions --repository <owner/name> [--gh-path <path>] | --provider azure_pipelines --project-url <url> [--pat-env <name>]  check the CI Watcher can watch what it is given, answering JSON",
		run: runCIWatcher,
	}
}

// runCIWatcher runs the CI Watcher. Its watch mode polls for one Project
// until its stdin, a pipe from the service, closes. Its check mode prints
// one CI check result as JSON, a refusal's reason and fix included, and
// exits 0; the service reads it.
func runCIWatcher(ctx context.Context, e Env, args []string) error {
	fs := flagSet(ciwatcher.Command)
	check := fs.Bool("check", false, "check access once and exit")
	project := fs.String("project", "", "the Project to watch")
	var in core.CICheckInput
	provider := fs.String("provider", string(store.CIGitHubActions), "CI Provider")
	fs.StringVar(&in.Repository, "repository", "", "GitHub repository, owner/name")
	fs.StringVar(&in.ProjectURL, "project-url", "", "Azure DevOps project URL")
	fs.StringVar(&in.PATEnv, "pat-env", store.DefaultPATEnv, "the environment variable holding the Azure DevOps PAT")
	fs.StringVar(&in.GHPath, "gh-path", "", "the gh path Option")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	switch {
	case len(pos) > 0:
		return usagef("unexpected argument %q", pos[0])
	case !*check && *project == "":
		return usagef("the CI Watcher needs --project, or --check")
	case !*check:
		return ciwatcher.Watch(ctx, ciwatcher.Config{Client: newClient(e), Project: *project, Env: gh.Env{Environ: os.Environ()},
			Clock: clock.System{}, Stdin: e.Stdin, Log: e.Stderr})
	}
	in.Provider = store.CIProvider(*provider)
	switch {
	case in.Provider == store.CIAzurePipelines && in.ProjectURL == "":
		return usagef("--check needs --project-url")
	case in.Provider != store.CIAzurePipelines && in.Repository == "":
		return usagef("--check needs --repository")
	}
	r := ciwatcher.Check(ctx, in, gh.Env{Environ: os.Environ()})
	return json.NewEncoder(e.Stdout).Encode(r)
}
