package cli

import (
	"context"
	"flag"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/tomoram/code-central/api"
)

// The repository command (Spec code-central-848) manages a Project's
// Repositories. Like init it sits outside the operation table: it calls
// the human's add_repository, edit_repository, remove_repository and
// set_landing_order through the client, as the human, so agents still get
// no command for them.
func init() {
	commands["repository"] = command{
		usage: "repository list | add <name> [--clone-url <url>] [--base <branch>] [--local-path <path>] [--landing <landing>] [--push-remote <remote>] [--check-command <command>] | edit <name> [--clone-url <url>] [--base <branch>] [--local-path <path>] [--landing <landing>] [--push-remote <remote>] [--check-command <command>] | remove <name> | move <name> --before|--after <name>  [--project <key>]  manage the Project's Repositories, listed in landing order; each needs a Clone URL, a local path or both, and one with no Clone URL is local-only, never cloned or pushed; an empty --clone-url or --local-path clears it; --landing is default (the Project's Default Landing), merge, merge+push, branch, branch+push or pull-request, an empty --push-remote uses the Project's, and an empty --check-command goes back to the Project's Check Command; edit takes the Home Repository's --landing, --push-remote and --check-command",
		run:   runRepository,
	}
}

func runRepository(ctx context.Context, e Env, args []string) error {
	if len(args) == 0 {
		return usagef("name a subcommand: list, add, edit, remove or move")
	}
	sub, args := args[0], args[1:]
	fs := flagSet("repository " + sub)
	project := addScope(fs)
	cloneURL := fs.String("clone-url", "", "")
	base := fs.String("base", "", "")
	localPath := fs.String("local-path", "", "")
	landing := fs.String("landing", "", "")
	pushRemote := fs.String("push-remote", "", "")
	checkCommand := fs.String("check-command", "", "")
	before := fs.String("before", "", "")
	after := fs.String("after", "", "")
	allowed := map[string][]string{
		"list":   {"project"},
		"add":    {"project", "clone-url", "base", "local-path", "landing", "push-remote", "check-command"},
		"edit":   {"project", "clone-url", "base", "local-path", "landing", "push-remote", "check-command"},
		"remove": {"project"},
		"move":   {"project", "before", "after"},
	}[sub]
	if allowed == nil {
		return usagef("unknown subcommand %q: use list, add, edit, remove or move", sub)
	}
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	for name := range set {
		if !slices.Contains(allowed, name) {
			return usagef("repository %s takes no --%s", sub, name)
		}
	}
	wantArgs := 1
	if sub == "list" {
		wantArgs = 0
	}
	if len(pos) != wantArgs {
		if wantArgs == 0 {
			return usagef("unexpected argument %q", pos[0])
		}
		return usagef("repository %s takes one Repository Name", sub)
	}
	var l api.RepositoryLanding
	if set["landing"] {
		if l, err = parseLanding(*landing); err != nil {
			return err
		}
	}
	sc, err := resolveScope(e, *project)
	if err != nil {
		return err
	}
	sc.client.Actor = api.Human
	var w api.Written[api.ProjectView]
	switch sub {
	case "list":
		p, err := sc.client.GetProject(ctx, sc.project)
		if err != nil {
			return err
		}
		return printRepositories(e, p)
	case "add":
		if !set["clone-url"] && !set["local-path"] {
			return usagef("repository add needs --clone-url or --local-path")
		}
		w, err = sc.client.AddRepository(ctx, sc.project, api.AddRepositoryInput{Name: pos[0], CloneURL: *cloneURL, Base: *base, LocalPath: *localPath,
			Landing: l, PushRemote: *pushRemote, CheckCommand: *checkCommand, Full: true})
	case "edit":
		in := api.EditRepositoryInput{Name: pos[0], Full: true}
		if set["clone-url"] {
			in.CloneURL = cloneURL
		}
		if set["base"] {
			in.Base = base
		}
		if set["local-path"] {
			in.LocalPath = localPath
		}
		if set["landing"] {
			in.Landing = &l
		}
		if set["push-remote"] {
			in.PushRemote = pushRemote
		}
		if set["check-command"] {
			in.CheckCommand = checkCommand
		}
		if len(set) == 0 || len(set) == 1 && set["project"] {
			return usagef("repository edit needs --clone-url, --base, --local-path, --landing, --push-remote or --check-command")
		}
		w, err = sc.client.EditRepository(ctx, sc.project, in)
	case "remove":
		w, err = sc.client.RemoveRepository(ctx, sc.project, api.RemoveRepositoryInput{Name: pos[0], Full: true})
	case "move":
		if set["before"] == set["after"] {
			return usagef("repository move needs one of --before or --after")
		}
		anchor, offset := *before, 0
		if set["after"] {
			anchor, offset = *after, 1
		}
		var p api.ProjectView
		if p, err = sc.client.GetProject(ctx, sc.project); err != nil {
			return err
		}
		var names []string
		if names, err = moved(p, pos[0], anchor, offset); err != nil {
			return err
		}
		w, err = sc.client.SetLandingOrder(ctx, sc.project, api.SetLandingOrderInput{Names: names, Full: true})
	}
	if err != nil {
		return err
	}
	return printRepositories(e, *w.Record)
}

// moved is p's landing order with name moved to anchor's place, offset 0
// for before it and 1 for after.
func moved(p api.ProjectView, name, anchor string, offset int) ([]string, error) {
	var names []string
	for _, r := range p.Repositories {
		names = append(names, r.Name)
	}
	for _, n := range []string{name, anchor} {
		if !slices.Contains(names, n) {
			return nil, &api.Error{Code: api.NotFound, Field: "name", Message: fmt.Sprintf("Project %s has no Repository %q", p.Key, n),
				Fix: "name one of its Repositories: " + strings.Join(names, ", ")}
		}
	}
	if name == anchor {
		return names, nil
	}
	names = slices.DeleteFunc(names, func(n string) bool { return n == name })
	i := slices.Index(names, anchor) + offset
	return slices.Insert(names, i, name), nil
}

// cliLandings are the --landing values, in the order usage lists them,
// with the Landing each names and how a list shows it.
var cliLandings = []struct {
	flag    string
	landing api.RepositoryLanding
	shown   string
}{
	{"default", api.RepositoryLandingDefault, ""},
	{"merge", api.RepositoryLandingMerge, "Merge"},
	{"merge+push", api.RepositoryLandingMergePush, "Merge + Push"},
	{"branch", api.RepositoryLandingBranch, "Branch"},
	{"branch+push", api.RepositoryLandingBranchPush, "Branch + Push"},
	{"pull-request", api.RepositoryLandingPullRequest, "Pull Request"},
}

// parseLanding is the Landing a --landing value names.
func parseLanding(s string) (api.RepositoryLanding, error) {
	var names []string
	for _, l := range cliLandings {
		if l.flag == s {
			return l.landing, nil
		}
		names = append(names, l.flag)
	}
	return "", usagef("--landing %q isn't a Landing: use %s", s, strings.Join(names, ", "))
}

// landingShown is how a list shows r's effective Landing: its name,
// marked own or default.
func landingShown(r api.RepositoryView) string {
	name := string(r.Landing)
	for _, l := range cliLandings {
		if l.landing == r.Landing {
			name = l.shown
		}
	}
	if r.OwnLanding {
		return name + " (own)"
	}
	return name + " (default)"
}

// printRepositories prints p's Repositories in landing order, one a line.
func printRepositories(e Env, p api.ProjectView) error {
	w := tabwriter.NewWriter(e.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCLONE URL\tBASE\tLOCAL PATH\tLANDING\tPUSH REMOTE\tCHECK COMMAND")
	for _, r := range p.Repositories {
		name, url, path := r.Name, r.CloneURL, r.LocalPath
		if r.Home {
			name += " (home)"
		}
		switch {
		case r.LocalOnly:
			url = "(none: local only)"
		case url == "" && r.Home:
			url = "(not reported yet)"
		}
		if path == "" {
			path = "-"
		}
		landing := landingShown(r)
		if r.LocalOnlyUnderPullRequest {
			landing += ": nothing lands, as it is local-only"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", name, url, r.Base, path, landing, r.PushRemote, checkCommandShown(r))
	}
	return w.Flush()
}

// checkCommandShown is how a list shows r's effective Check Command,
// marked own or default, and nothing when none applies.
func checkCommandShown(r api.RepositoryView) string {
	switch {
	case r.CheckCommand == "":
		return ""
	case r.OwnCheckCommand:
		return r.CheckCommand + " (own)"
	}
	return r.CheckCommand + " (default)"
}
