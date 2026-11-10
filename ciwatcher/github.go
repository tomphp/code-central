// GitHub Actions, the only CI Provider: the CI Watcher reaches GitHub
// through gh, the GitHub CLI, and its own login, so Code Central stores
// no GitHub token.

package ciwatcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tomoram/code-central/core"
)

// GitHubActions is the GitHub Actions CI Provider for one repository.
type GitHubActions struct {
	// Repository is owner/name.
	Repository string
	// GHPath is the gh path Option: gh itself, or the folder it is in;
	// "" finds it on PATH, then in the install places.
	GHPath string
	Env    Env
}

// DefaultInstallPlaces are where gh's installers put it: Homebrew on
// Apple silicon and Intel Macs, Homebrew on Linux, and the Windows
// installer under %ProgramFiles%.
func DefaultInstallPlaces(getenv func(string) string) []string {
	places := []string{"/opt/homebrew/bin", "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin"}
	if pf := getenv("ProgramFiles"); pf != "" {
		places = append(places, filepath.Join(pf, "GitHub CLI"))
	}
	return places
}

// ghMissing is the problem of a gh found nowhere it was looked for.
func ghMissing(reason string) core.CICheckResult {
	return core.CICheckResult{Problem: core.CIGHMissing, Reason: reason,
		Fix: "Install gh, the GitHub CLI (https://cli.github.com), or set the gh path Option to where it is."}
}

// FindGH finds gh: the gh path Option if set, else on PATH, else in the
// install places. Not finding it is the gh missing problem.
func (g *GitHubActions) FindGH() (string, *core.CICheckResult) {
	if g.GHPath != "" {
		if gh, ok := isProgram(g.GHPath); ok {
			return gh, nil
		}
		if gh, ok := ghIn(g.GHPath); ok {
			return gh, nil
		}
		r := ghMissing(fmt.Sprintf("The gh path Option is %s, but gh isn't there.", g.GHPath))
		r.Fix = "Set the gh path Option to where gh is, or clear it to find gh on the service's PATH."
		return "", &r
	}
	for _, dir := range filepath.SplitList(g.Env.getenv("PATH")) {
		if gh, ok := ghIn(dir); ok {
			return gh, nil
		}
	}
	places := g.Env.InstallPlaces
	if places == nil {
		places = DefaultInstallPlaces(g.Env.getenv)
	}
	for _, dir := range places {
		if gh, ok := ghIn(dir); ok {
			return gh, nil
		}
	}
	r := ghMissing(fmt.Sprintf("gh, the GitHub CLI, isn't on the service's PATH or in %s.", strings.Join(places, ", ")))
	return "", &r
}

// ghIn is gh in the folder dir, if it is there.
func ghIn(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	return isProgram(filepath.Join(dir, "gh"+exeSuffix))
}

// isProgram reports whether path is a program that can be run.
func isProgram(path string) (string, bool) {
	fi, err := os.Stat(path)
	return path, err == nil && !fi.IsDir() && executable(fi)
}

// CheckAccess finds gh, checks its login with gh auth status, and reads
// the repository.
func (g *GitHubActions) CheckAccess(ctx context.Context) core.CICheckResult {
	gh, missing := g.FindGH()
	if missing != nil {
		return *missing
	}
	if out, err := g.run(ctx, gh, "auth", "status", "--hostname", "github.com"); err != nil {
		if r, ran := didntRun(gh, err); !ran {
			return r
		}
		return core.CICheckResult{Problem: core.CILoggedOut, Reason: "gh isn't logged in to GitHub: " + out,
			Fix: "Run `gh auth login` as the user the service runs as, then turn CI Tracking on again."}
	}
	if out, err := g.run(ctx, gh, "api", "repos/"+g.Repository, "--jq", ".full_name"); err != nil {
		if r, ran := didntRun(gh, err); !ran {
			return r
		}
		return core.CICheckResult{Problem: core.CIRepositoryUnreadable, Reason: fmt.Sprintf("gh can't read the repository %s: %s", g.Repository, out),
			Fix: "Check the repository is the owner/name GitHub shows, and that the account gh is logged in as can see it."}
	}
	return core.CICheckResult{}
}

// run runs gh with args, never prompting, and returns its first line of
// output: stderr's when it fails, as gh writes its errors there.
func (g *GitHubActions) run(ctx context.Context, gh string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, gh, args...)
	cmd.Env = append(append([]string{}, g.Env.Environ...), "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	out := stdout.String()
	if err != nil && strings.TrimSpace(stderr.String()) != "" {
		out = stderr.String()
	}
	return firstLine(out), err
}

// didntRun is the check failing because gh didn't run at all, rather
// than running and saying no; ran is true when it did run.
func didntRun(gh string, err error) (r core.CICheckResult, ran bool) {
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.Exited() {
		return r, true
	}
	return core.CICheckResult{Problem: core.CICheckFailed, Reason: fmt.Sprintf("gh at %s didn't run: %v", gh, err),
		Fix: "Check gh runs for the user the service runs as, or set the gh path Option to one that does."}, false
}

// firstLine is s's first line with text on it, trimmed.
func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}
