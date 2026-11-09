// Command contextreport reports what agents load into context, from
// Claude Code's session logs: for each kind of session (each Stage,
// implementation, and the review and other subagents), the size of the
// tool results its sessions read, per tool and per Code Central operation;
// the largest results; the Specs and Tickets read more than once; and
// signs that a cut went too far. With -save it writes the figures to a
// baseline file, and with -baseline it prints each figure's change
// against one. It is a developer tool for the Makefile, not a shipped
// command, so it lives under internal/ rather than cmd/.
//
// Run it from the repository, through make:
//
//	make context-report ARGS="-baseline internal/contextreport/baseline.json -since 2026-11-10T00:00:00Z"
//
// docs/context-report.md explains each figure.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "contextreport:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("contextreport", flag.ContinueOnError)
	projects := flags.String("projects", "", "Claude Code's projects folder (default ~/.claude/projects)")
	repo := flags.String("repo", "", "the main checkout whose sessions to read (default this repository's)")
	since := flags.String("since", "", "only sessions started at or after this RFC 3339 time")
	until := flags.String("until", "", "only sessions started before this RFC 3339 time")
	top := flags.Int("top", 20, "how many of the largest results, re-read IDs and wider reads to list")
	save := flags.String("save", "", "write the figures to this file, as a baseline")
	baseline := flags.String("baseline", "", "print each figure's change against this baseline")
	if err := flags.Parse(args); err != nil {
		return err
	}

	var w Window
	var err error
	if w.Since, err = parseTime(*since); err != nil {
		return err
	}
	if w.Until, err = parseTime(*until); err != nil {
		return err
	}
	if *projects == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		*projects = filepath.Join(home, ".claude", "projects")
	}
	if *repo == "" {
		if *repo, err = mainCheckout(); err != nil {
			return err
		}
	}

	sessions, err := Load(*projects, *repo, w)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return fmt.Errorf("no Factory sessions of %s in %s", *repo, *projects)
	}
	r := Build(sessions, *top)

	var base *Report
	if *baseline != "" {
		raw, err := os.ReadFile(*baseline)
		if err != nil {
			return err
		}
		base = &Report{}
		if err := json.Unmarshal(raw, base); err != nil {
			return fmt.Errorf("%s: %w", *baseline, err)
		}
	}
	if *save != "" {
		raw, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*save, append(raw, '\n'), 0o644); err != nil {
			return err
		}
	}
	return write(out, r, base)
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

// mainCheckout is the main checkout of the repository in the current
// folder, even from a worktree: the folder holding the shared .git.
func mainCheckout() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", errors.New("not in a git repository; pass -repo")
	}
	return filepath.Dir(strings.TrimSpace(string(out))), nil
}
