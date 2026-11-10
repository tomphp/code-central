package store_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/home"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/store/storetest"
)

func openFile(t *testing.T, dir string) *store.FileStore {
	t.Helper()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestFileStoreContract(t *testing.T) {
	storetest.Run(t, storetest.Factory{
		New: func(t *testing.T) store.Store { return openFile(t, t.TempDir()) },
		Reopen: func(t *testing.T, s store.Store) store.Store {
			fsx := s.(*store.FileStore)
			if err := fsx.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			return openFile(t, fsx.Dir())
		},
	})
}

var t0 = time.Date(2026, 11, 7, 9, 30, 0, 0, time.UTC)

func mustUpdate(t *testing.T, s store.Store, fn func(store.Writer) error) {
	t.Helper()
	if err := s.Update(fn); err != nil {
		t.Fatalf("update: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNewStoreGetsTheCurrentFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fresh")
	openFile(t, dir)
	got, err := os.ReadFile(filepath.Join(dir, "store.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fmt.Sprintf("format: %d\n", store.Format) {
		t.Fatalf("store.yaml = %q", got)
	}
}

func TestUnknownFormatRefusesToOpen(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "store.yaml"), fmt.Sprintf("format: %d\n", store.Format+1))
	_, err := store.Open(dir)
	if err == nil {
		t.Fatal("opened a store with an unknown format")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("format %d", store.Format+1)) || !strings.Contains(err.Error(), "store.yaml") {
		t.Fatalf("error doesn't name the format: %v", err)
	}
}

func TestSecondOpenOfSameStoreIsRefused(t *testing.T) {
	dir := t.TempDir()
	first := openFile(t, dir)
	if _, err := store.Open(dir); !errors.Is(err, store.ErrLocked) {
		t.Fatalf("second open: err = %v, want ErrLocked", err)
	}
	first.Close()
	openFile(t, dir) // the lock is released on Close
}

// On a case-insensitive file system (Windows, and macOS by default) the
// path in another case is the same store, so it is locked; on a
// case-sensitive one (Linux) it is another folder, so another store.
func TestTheSameStoreInAnotherCaseIsRefused(t *testing.T) {
	root := t.TempDir()
	dir, other := filepath.Join(root, "Store"), filepath.Join(root, "STORE")
	openFile(t, dir)
	if _, err := os.Stat(other); errors.Is(err, fs.ErrNotExist) {
		openFile(t, other)
		return
	}
	if _, err := store.Open(other); !errors.Is(err, store.ErrLocked) {
		t.Fatalf("open in another case: err = %v, want ErrLocked", err)
	}
}

func TestFilesAreWrittenInFixedFieldOrder(t *testing.T) {
	dir := t.TempDir()
	s := openFile(t, dir)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutProject(store.Project{Key: "acme", DisplayName: "Acme Co", Created: t0, Mode: "committed"})
	})
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutSpec("acme", store.Spec{Number: 1, Title: "Login", Labels: []string{"auth"}, Created: t0, Updated: t0, Body: "# Login\n\nText.\n",
			Edits: []store.Edit{{Number: 1, Actor: "agent", Time: t0, Previous: map[string]string{"title": "Sign in", "body": "Old.\nText.\n"}}}})
	})
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutTicket("acme", store.Ticket{
			Number: 2, Title: "Form", State: store.Ready, Spec: 1, Labels: []string{"ui"},
			Created: t0, Updated: t0.Add(90 * time.Second), Description: "Build it.\nWell.\n",
			Branch: "acme-2", Commit: "abc123",
			Moves:  []store.Move{{From: store.Accepted, To: store.Done, Actor: "agent", Time: t0, Via: "merge", Commit: "def456"}},
			Claims: []store.Claim{{Number: 1, ID: "ABC", SessionID: "s1", WorkingDir: "/work/acme 2", ResumeCommand: "claude --resume s1 && echo 'back'", Model: "opus", Effort: "high", Started: t0, Ended: t0.Add(time.Minute), EndedBy: "finish"}},
			Rounds: []store.Round{{Number: 1, Claim: 1, SessionID: "s1", Posted: t0, Questions: []store.Question{
				{Number: 1, Title: "Which DB?", Body: "Postgres\nor SQLite?\n", Recommended: "SQLite", Status: store.QuestionAnswered, Answer: "SQLite\nfor now.\n", Closed: t0.Add(30 * time.Second)},
			}}},
			FailureReports: []store.FailureReport{{Number: 1, Claim: 1, SessionID: "s1", WorkingDir: "/work/acme 2", ResumeCommand: "claude --resume s1 && echo 'back'", Actor: "agent", Time: t0,
				WhatBroke: "It broke.\nBadly.\n", Why: "Because.", WhatToFix: "Fix it.", WhereWorkStands: "Pushed."}},
			Comments: []store.Comment{{Number: 1, Actor: "agent", Time: t0.Add(time.Minute), Claim: 1, Body: "Done.\nCheck it.\n"}},
			Edits:    []store.Edit{{Number: 1, Actor: "agent", Time: t0, Previous: map[string]string{"description": "Old.\n"}}},
		})
	})

	want := map[string]string{
		"projects/acme/project.yaml": `key: acme
display_name: Acme Co
created: 2026-11-07T09:30:00Z
mode: committed
`,
		"projects/acme/specs/1.md": `---
id: 1
title: Login
labels:
  - auth
tags: {}
created: 2026-11-07T09:30:00Z
updated: 2026-11-07T09:30:00Z
edits:
  - number: 1
    actor: agent
    time: 2026-11-07T09:30:00Z
    previous:
      body: |
        Old.
        Text.
      title: Sign in
---
# Login

Text.
`,
		"projects/acme/tickets/2.yaml": `title: Form
state: Ready
spec: 1
labels:
  - ui
tags: {}
blocked_by: []
replacements: []
created: 2026-11-07T09:30:00Z
updated: 2026-11-07T09:31:30Z
description: |
  Build it.
  Well.
branch: acme-2
commit: abc123
claims:
  - number: 1
    id: ABC
    session_id: s1
    working_dir: /work/acme 2
    resume_command: claude --resume s1 && echo 'back'
    model: opus
    effort: high
    started: 2026-11-07T09:30:00Z
    ended: 2026-11-07T09:31:00Z
    ended_by: finish
rounds:
  - number: 1
    claim: 1
    session_id: s1
    posted: 2026-11-07T09:30:00Z
    questions:
      - number: 1
        title: Which DB?
        body: |
          Postgres
          or SQLite?
        recommended: |-
          SQLite
        status: answered
        answer: |
          SQLite
          for now.
        closed: 2026-11-07T09:30:30Z
failure_reports:
  - number: 1
    claim: 1
    session_id: s1
    working_dir: /work/acme 2
    resume_command: claude --resume s1 && echo 'back'
    actor: agent
    time: 2026-11-07T09:30:00Z
    what_broke: |
      It broke.
      Badly.
    why: |-
      Because.
    what_to_fix: |-
      Fix it.
    where_work_stands: |-
      Pushed.
comments:
  - number: 1
    actor: agent
    time: 2026-11-07T09:31:00Z
    claim: 1
    body: |
      Done.
      Check it.
moves:
  - from: Accepted
    to: Done
    actor: agent
    time: 2026-11-07T09:30:00Z
    via: merge
    commit: def456
edits:
  - number: 1
    actor: agent
    time: 2026-11-07T09:30:00Z
    previous:
      description: |
        Old.
`,
	}
	for rel, w := range want {
		got, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != w {
			t.Errorf("%s:\n--- got\n%s--- want\n%s", rel, got, w)
		}
	}
}

func TestEachWriteLeavesOnlyItsOneFile(t *testing.T) {
	dir := t.TempDir()
	s := openFile(t, dir)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutProject(store.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed"})
	})
	before := listFiles(t, dir)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutTicket("acme", store.Ticket{Number: 1, Title: "T", State: store.Draft, Spec: 1, Created: t0, Updated: t0})
	})
	after := listFiles(t, dir)
	var added []string
	for _, f := range after {
		if !slices.Contains(before, f) {
			added = append(added, f)
		}
	}
	if !slices.Equal(added, []string{"projects/acme/tickets/1.yaml"}) {
		t.Fatalf("files added by one write: %v", added)
	}
}

func TestOptionsAreOneFileAtTheStoreRoot(t *testing.T) {
	dir := t.TempDir()
	s := openFile(t, dir)
	before := listFiles(t, dir)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutOptions(store.Options{FocusClear: store.FocusAllClear})
	})
	var added []string
	for _, f := range listFiles(t, dir) {
		if !slices.Contains(before, f) {
			added = append(added, f)
		}
	}
	if !slices.Equal(added, []string{"options.yaml"}) {
		t.Fatalf("files added by an options write: %v", added)
	}
	if got := readFile(t, filepath.Join(dir, "options.yaml")); got != "focus_clear: all_clear\n" {
		t.Fatalf("options.yaml = %q", got)
	}
}

func TestAnOptionsFileKeepsTheGHPathOnlyWhenSet(t *testing.T) {
	dir := t.TempDir()
	s := openFile(t, dir)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutOptions(store.Options{FocusClear: store.FocusAllClear, GHPath: "/opt/tools/gh"})
	})
	if got := readFile(t, filepath.Join(dir, "options.yaml")); got != "focus_clear: all_clear\ngh_path: /opt/tools/gh\n" {
		t.Fatalf("options.yaml = %q", got)
	}
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutOptions(store.Options{FocusClear: store.FocusAllClear})
	})
	if got := readFile(t, filepath.Join(dir, "options.yaml")); got != "focus_clear: all_clear\n" {
		t.Fatalf("options.yaml = %q, want no gh_path once it is cleared", got)
	}
}

func TestAnOptionsFileWithNoKeysReadsAsTheDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "store.yaml"), fmt.Sprintf("format: %d\n", store.Format))
	writeFile(t, filepath.Join(dir, "options.yaml"), "{}\n")
	s := openFile(t, dir)
	s.View(func(r store.Reader) error {
		if o := r.Options(); o.FocusClear != store.FocusActivityDashboard {
			t.Fatalf("options = %+v, want the defaults", o)
		}
		return nil
	})
}

func TestAnOptionsFileKeepsAnExplicitAllClear(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "store.yaml"), fmt.Sprintf("format: %d\n", store.Format))
	writeFile(t, filepath.Join(dir, "options.yaml"), "focus_clear: all_clear\n")
	s := openFile(t, dir)
	s.View(func(r store.Reader) error {
		if o := r.Options(); o.FocusClear != store.FocusAllClear {
			t.Fatalf("options = %+v, want the All clear the file holds", o)
		}
		return nil
	})
}

func TestAnOptionsFileOutsideItsSetNamesTheValuesDefaultFirst(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "store.yaml"), fmt.Sprintf("format: %d\n", store.Format))
	writeFile(t, filepath.Join(dir, "options.yaml"), "focus_clear: dashboard\n")
	if _, err := store.Open(dir); err == nil || !strings.Contains(err.Error(), "activity_dashboard or all_clear") {
		t.Fatalf("err = %v, want the values listed default first", err)
	}
}

func TestADefaultLandingMethodLeavesProjectYAMLUnchanged(t *testing.T) {
	dir := t.TempDir()
	s := openFile(t, dir)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutProject(store.Project{Key: "acme", DisplayName: "Acme", Created: time.Date(2026, 11, 7, 9, 30, 0, 0, time.UTC), Mode: store.ModeCommitted})
	})
	const plain = "key: acme\ndisplay_name: Acme\ncreated: 2026-11-07T09:30:00Z\nmode: committed\n"
	if got := readFile(t, filepath.Join(dir, "projects/acme/project.yaml")); got != plain {
		t.Fatalf("project.yaml = %q", got)
	}
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutProject(store.Project{Key: "acme", DisplayName: "Acme", Created: time.Date(2026, 11, 7, 9, 30, 0, 0, time.UTC), Mode: store.ModeCommitted,
			Landing: store.Landing{Method: store.LandingBranch, Push: true, Remote: "origin"}})
	})
	if got, want := readFile(t, filepath.Join(dir, "projects/acme/project.yaml")), plain+"landing:\n  method: branch\n  push: true\n  remote: origin\n"; got != want {
		t.Fatalf("project.yaml = %q, want %q", got, want)
	}
}

func TestDefaultCISettingsLeaveProjectYAMLUnchanged(t *testing.T) {
	dir := t.TempDir()
	s := openFile(t, dir)
	created := time.Date(2026, 11, 7, 9, 30, 0, 0, time.UTC)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutProject(store.Project{Key: "acme", DisplayName: "Acme", Created: created, Mode: store.ModeCommitted, CI: store.DefaultCI()})
	})
	const plain = "key: acme\ndisplay_name: Acme\ncreated: 2026-11-07T09:30:00Z\nmode: committed\n"
	if got := readFile(t, filepath.Join(dir, "projects/acme/project.yaml")); got != plain {
		t.Fatalf("project.yaml = %q", got)
	}
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutProject(store.Project{Key: "acme", DisplayName: "Acme", Created: created, Mode: store.ModeCommitted,
			CI: store.CI{Tracking: true, Repository: "acme/web", IgnoredWorkflows: []string{"nightly.yml"}}})
	})
	want := plain + "ci:\n  tracking: true\n  provider: github_actions\n  repository: acme/web\n  ignored_workflows:\n    - nightly.yml\n"
	if got := readFile(t, filepath.Join(dir, "projects/acme/project.yaml")); got != want {
		t.Fatalf("project.yaml = %q, want %q", got, want)
	}
}

func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

func TestInvalidFilesStopStartupNamingFileAndField(t *testing.T) {
	project := "key: acme\ndisplay_name: acme\ncreated: 2026-11-07T09:30:00Z\nmode: committed\n"
	ticket := "title: T\nstate: Draft\nspec: 1\nlabels: []\ntags: {}\nblocked_by: []\nreplacements: []\ncreated: 2026-11-07T09:30:00Z\nupdated: 2026-11-07T09:30:00Z\ndescription: \"\"\nmoves: []\n"
	cases := []struct {
		name, file, content, field string
	}{
		{"unknown ticket field", "projects/acme/tickets/2.yaml", ticket + "colour: red\n", "colour"},
		{"bad state", "projects/acme/tickets/2.yaml", strings.Replace(ticket, "state: Draft", "state: Doing", 1), "state"},
		{"bad timestamp", "projects/acme/tickets/2.yaml", strings.Replace(ticket, "updated: 2026-11-07T09:30:00Z", "updated: yesterday", 1), "updated"},
		{"missing title", "projects/acme/tickets/2.yaml", strings.Replace(ticket, "title: T\n", "", 1), "title"},
		{"bad archived time", "projects/acme/project.yaml", project + "archived: true\n", "archived"},
		{"bad mode", "projects/acme/project.yaml", strings.Replace(project, "mode: committed", "mode: shared", 1), "mode"},
		{"unknown project field", "projects/acme/project.yaml", project + "path: /src\n", "path"},
		{"landing method outside its set", "projects/acme/project.yaml", project + "landing:\n  method: squash\n  push: false\n  remote: origin\n", "landing.method"},
		{"ci provider outside its set", "projects/acme/project.yaml", project + "ci:\n  tracking: true\n  provider: circleci\n", "ci.provider"},
		{"ci rule state outside draft and ready", "projects/acme/project.yaml", project + "ci:\n  tracking: false\n  provider: github_actions\n  rules:\n    - spec: 4\n      workflow: ci.yml\n      branch: main\n      state: Done\n      created: 2026-11-07T09:30:00Z\n", "ci.rules[0].state"},
		{"ci rule without created", "projects/acme/project.yaml", project + "ci:\n  tracking: false\n  provider: github_actions\n  rules:\n    - spec: 4\n      workflow: ci.yml\n      branch: main\n      state: Ready\n      created: \"\"\n", "ci.rules[0].created"},
		{"unknown ci field", "projects/acme/project.yaml", project + "ci:\n  tracking: true\n  provider: github_actions\n  token: x\n", "ci.token"},
		{"unknown landing field", "projects/acme/project.yaml", project + "landing:\n  method: merge\n  push: false\n  remote: origin\n  force: true\n", "landing.force"},
		{"unknown models field", "projects/acme/project.yaml", project + "models:\n  default:\n    model: opus\n    temperature: 1\n", "models.default.temperature"},
		{"models kind outside the kinds of work", "projects/acme/project.yaml", project + "models:\n  kinds:\n    - kind: review\n      model: opus\n", "models.kinds[0].kind"},
		{"models kind twice", "projects/acme/project.yaml", project + "models:\n  kinds:\n    - kind: spec\n      model: opus\n    - kind: spec\n      effort: low\n", "models.kinds[1].kind"},
		{"models row setting nothing", "projects/acme/project.yaml", project + "models:\n  kinds:\n    - kind: spec\n", "models.kinds[0]"},
		{"blank model", "projects/acme/project.yaml", project + "models:\n  default:\n    model: \" \"\n", "models.default.model"},
		{"multi-line effort", "projects/acme/project.yaml", project + "models:\n  kinds:\n    - kind: tickets\n      effort: \"high\\nmax\"\n", "models.kinds[0].effort"},
		{"spec without frontmatter", "projects/acme/specs/3.md", "# just markdown\n", "frontmatter"},
		{"spec id mismatch", "projects/acme/specs/3.md", "---\nid: 4\ntitle: S\nlabels: []\ntags: {}\ncreated: 2026-11-07T09:30:00Z\nupdated: 2026-11-07T09:30:00Z\n---\nbody\n", "id"},
		{"unknown previous field", "projects/acme/tickets/2.yaml", ticket + "edits:\n  - number: 1\n    actor: agent\n    time: 2026-11-07T09:30:00Z\n    previous:\n      colour: red\n", "edits[0].previous.colour"},
		{"bad comment time", "projects/acme/tickets/2.yaml", ticket + "comments:\n  - number: 1\n    actor: agent\n    time: soon\n    body: hi\n", "comments[0].time"},
		{"bad question status", "projects/acme/tickets/2.yaml", ticket + "rounds:\n  - number: 1\n    claim: 1\n    posted: 2026-11-07T09:30:00Z\n    questions:\n      - number: 1\n        title: Q\n        body: B\n        status: maybe\n", "rounds[0].questions[0].status"},
		{"bad external state", "projects/acme/tickets/2.yaml", ticket + "link:\n  integration: github\n  id: \"12\"\n  external_state: gone\n", "link.external_state"},
		{"link without id", "projects/acme/tickets/2.yaml", ticket + "link:\n  integration: github\n  external_state: open\n", "link.id"},
		{"badly named ticket file", "projects/acme/tickets/two.yaml", ticket, "file name"},
		{"unknown option", "options.yaml", "focus_clear: all_clear\ntheme: dark\n", "theme"},
		{"focus_clear outside its set", "options.yaml", "focus_clear: dashboard\n", "focus_clear"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "store.yaml"), fmt.Sprintf("format: %d\n", store.Format))
			writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"), project)
			writeFile(t, filepath.Join(dir, c.file), c.content)
			_, err := store.Open(dir)
			var le *store.LoadError
			if !errors.As(err, &le) {
				t.Fatalf("err = %v, want a LoadError", err)
			}
			if le.File != c.file || le.Field != c.field {
				t.Fatalf("LoadError names %q field %q, want %q field %q (%v)", le.File, le.Field, c.file, c.field, err)
			}
			if !strings.Contains(err.Error(), c.file) || !strings.Contains(err.Error(), c.field) {
				t.Fatalf("message doesn't name file and field: %v", err)
			}
		})
	}
}

func TestDefaultDir(t *testing.T) {
	// A Windows account: no HOME, but a profile folder.
	env := func(m map[string]string) home.Lookup {
		return home.Lookup{
			Getenv:      func(k string) string { return m[k] },
			UserHomeDir: func() (string, error) { return "/profile", nil },
		}
	}
	cases := []struct {
		name string
		flag string
		env  map[string]string
		want string
	}{
		{"flag wins", "/flag", map[string]string{"CODE_CENTRAL_STORE": "/env", "XDG_DATA_HOME": "/xdg", "HOME": "/home/me"}, "/flag"},
		{"env override", "", map[string]string{"CODE_CENTRAL_STORE": "/env", "XDG_DATA_HOME": "/xdg", "HOME": "/home/me"}, "/env"},
		{"xdg", "", map[string]string{"XDG_DATA_HOME": "/xdg", "HOME": "/home/me"}, filepath.Join("/xdg", "code-central")},
		{"home fallback", "", map[string]string{"HOME": "/home/me"}, filepath.Join("/home/me", ".local", "share", "code-central")},
		{"profile fallback", "", nil, filepath.Join("/profile", ".local", "share", "code-central")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := store.Dir(c.flag, env(c.env))
			if err != nil || got != c.want {
				t.Fatalf("Dir = %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

// A Project's usage/ folder (ADR 0016) is the usage recorder's, not the
// loader's: its JSON Lines files, even unreadable ones, never affect
// startup.
func TestUsageFoldersDoNotAffectLoading(t *testing.T) {
	dir := t.TempDir()
	s := openFile(t, dir)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutProject(store.Project{Key: "acme", DisplayName: "Acme", Created: t0, Mode: store.ModeCommitted})
	})
	s.Close()
	writeFile(t, filepath.Join(dir, "projects/acme/usage/2026-11-07.jsonl"), `{"type":"mcp_call","op":"claim"}`+"\nnot json\n")

	s = openFile(t, dir)
	var keys []string
	s.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			keys = append(keys, p.Key)
		}
		return nil
	})
	if !slices.Equal(keys, []string{"acme"}) {
		t.Fatalf("projects %v, want acme", keys)
	}
}
