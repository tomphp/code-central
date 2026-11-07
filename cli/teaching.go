package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// init's teaching files, --local and the Project's mode (ticket 41).

// generatedMark starts the header of every file init writes whole. A file
// that starts with it is init's to rewrite; any other file is the human's.
const generatedMark = "<!-- Written by code-central init "

func generatedHeader() string {
	return generatedMark + Version + ". Re-running init rewrites this file. -->\n\n"
}

// The pointer files: init adds a pointer to the tracker doc unless the file
// already mentions it.
const (
	trackerDoc       = "docs/agents/issue-tracker.md"
	claudeFile       = "CLAUDE.md"
	localClaudeFile  = "CLAUDE.local.md"
	trackerPointer   = "\n## Issue tracker\n\nSpecs and Tickets live in Code Central: see `" + trackerDoc + "`.\n"
	excludeMark      = "# code-central init --local"
	sandboxesPointer = "Agents in a sandbox or container: see `code-central help sandboxes`."
)

// setup is what init has settled before it writes the repository's files.
type setup struct {
	root     string
	inGit    bool
	key      string
	project  core.ProjectView
	wantMode store.Mode // from --local or --committed; "" follows the stored mode
	wroteMCP bool
}

// setUp settles the mode (asking before switching the stored one), then
// writes the hooks, the teaching files and the pointer for that mode.
func setUp(ctx context.Context, e Env, ask func(string) (string, error), c *client.Client, s setup) error {
	mode := s.project.Mode
	if mode == "" {
		mode = core.ModeCommitted
	}
	if s.wantMode != "" && s.wantMode != mode {
		answer, err := ask(fmt.Sprintf("Project %s is set up as %s. Switch it to %s? [y/N]: ", s.key, mode, s.wantMode))
		if err != nil {
			return err
		}
		if yes(answer, false) {
			if _, err := c.SetProjectMode(ctx, s.key, core.SetProjectModeInput{Mode: s.wantMode}); err != nil {
				return err
			}
			mode = s.wantMode
		} else {
			fmt.Fprintf(e.Stdout, "Keeping Project %s %s.\n", s.key, mode)
		}
	}
	local := mode == core.ModeLocal
	at := func(rel string) string { return filepath.Join(s.root, filepath.FromSlash(rel)) }

	var wrote, kept []string
	if s.wroteMCP {
		wrote = append(wrote, mcpFile)
	}
	hooks, other := settingsFile, localSettingsFile
	if local {
		hooks, other = other, hooks
	}
	if err := writeHooks(at(hooks), true); err != nil {
		return err
	}
	if err := writeHooks(at(other), false); err != nil {
		return err
	}
	wrote = append(wrote, hooks)

	for _, f := range core.TeachingFiles() {
		ok, err := writeGenerated(e, ask, at(f.Path), f.Path, generatedHeader()+f.Body)
		if err != nil {
			return err
		}
		if ok {
			wrote = append(wrote, f.Path)
		} else {
			kept = append(kept, f.Path)
		}
	}

	pointer := claudeFile
	if local {
		pointer = localClaudeFile
	}
	added, err := addPointer(at(pointer))
	if err != nil {
		return err
	}
	if added {
		wrote = append(wrote, pointer)
	}

	if s.inGit {
		entries := []string{mcpFile, localSettingsFile, localClaudeFile}
		for _, f := range core.TeachingFiles() {
			entries = append(entries, f.Path)
		}
		if err := writeExclude(s.root, entries, local); err != nil {
			return err
		}
	}

	fmt.Fprintf(e.Stdout, "Project %s is %s. Wrote %s.\n", s.key, mode, strings.Join(wrote, ", "))
	if len(kept) > 0 {
		fmt.Fprintf(e.Stdout, "Kept your %s.\n", strings.Join(kept, ", "))
	}
	if local && s.inGit {
		fmt.Fprintln(e.Stdout, "They are listed in .git/info/exclude; nothing was committed.")
	} else {
		fmt.Fprintln(e.Stdout, "Nothing was committed.")
	}
	fmt.Fprintln(e.Stdout, sandboxesPointer)
	return nil
}

// writeGenerated writes a whole file init owns. A missing file, or one
// init wrote before (it starts with the header), is written without
// asking; any other file is shown as a diff and replaced only if the human
// agrees. It reports whether the file now holds content.
func writeGenerated(e Env, ask func(string) (string, error), path, name, content string) (bool, error) {
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return false, err
	case string(old) == content:
		return true, nil
	case !bytes.HasPrefix(old, []byte(generatedMark)):
		fmt.Fprintf(e.Stdout, "%s exists and wasn't written by init:\n%s", name, lineDiff(name, string(old), content))
		answer, err := ask(fmt.Sprintf("Replace %s? [y/N]: ", name))
		if err != nil {
			return false, err
		}
		if !yes(answer, false) {
			return false, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(content), 0o644)
}

// addPointer appends the tracker pointer to path unless it already
// mentions the tracker doc. It reports whether it wrote.
func addPointer(path string) (bool, error) {
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if bytes.Contains(old, []byte(trackerDoc)) {
		return false, nil
	}
	text := string(old)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += trackerPointer
	if len(old) == 0 {
		text = strings.TrimLeft(text, "\n")
	}
	return true, os.WriteFile(path, []byte(text), 0o644)
}

// writeExclude lists entries in the repository's info/exclude under init's
// mark when local, and takes them out again when not. Each entry appears
// once however often init runs.
func writeExclude(root string, entries []string, local bool) error {
	rel, err := gitOut(root, "rev-parse", "--git-path", "info/exclude")
	if err != nil || rel == "" {
		return fmt.Errorf("can't find .git/info/exclude: %v", err)
	}
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ours := map[string]bool{excludeMark: true}
	for _, en := range entries {
		ours["/"+en] = true
	}
	var lines []string
	present := map[string]bool{}
	for _, l := range strings.Split(strings.TrimRight(string(old), "\n"), "\n") {
		if ours[l] {
			present[l] = true
			if !local {
				continue
			}
		}
		lines = append(lines, l)
	}
	if local {
		if !present[excludeMark] {
			lines = append(lines, excludeMark)
		}
		for _, en := range entries {
			if !present["/"+en] {
				lines = append(lines, "/"+en)
			}
		}
	}
	text := strings.TrimLeft(strings.Join(lines, "\n"), "\n")
	if text != "" {
		text += "\n"
	}
	if text == string(old) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// lineDiff shows how b differs from a, one line each: "-" only in a, "+"
// only in b, " " in both. The files are small, so it shows them whole.
func lineDiff(name, a, b string) string {
	x := strings.Split(strings.TrimRight(a, "\n"), "\n")
	y := strings.Split(strings.TrimRight(b, "\n"), "\n")
	// lcs[i][j] is the longest common subsequence of x[i:] and y[j:].
	lcs := make([][]int, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s (yours)\n+++ %s (Code Central's)\n", name, name)
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			out.WriteString(" " + x[i] + "\n")
			i, j = i+1, j+1
		case i < len(x) && (j == len(y) || lcs[i+1][j] >= lcs[i][j+1]):
			out.WriteString("-" + x[i] + "\n")
			i++
		default:
			out.WriteString("+" + y[j] + "\n")
			j++
		}
	}
	return out.String()
}
