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
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 7\n")
	_, err := store.Open(dir)
	if err == nil {
		t.Fatal("opened a store with an unknown format")
	}
	if !strings.Contains(err.Error(), "format 7") || !strings.Contains(err.Error(), "store.yaml") {
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
			Claims: []store.Claim{{Number: 1, ID: "ABC", SessionID: "s1", WorkingDir: "/work/acme 2", ResumeCommand: "claude --resume s1 && echo 'back'", Started: t0, Ended: t0.Add(time.Minute), EndedBy: "finish"}},
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
		{"spec without frontmatter", "projects/acme/specs/3.md", "# just markdown\n", "frontmatter"},
		{"spec id mismatch", "projects/acme/specs/3.md", "---\nid: 4\ntitle: S\nlabels: []\ntags: {}\ncreated: 2026-11-07T09:30:00Z\nupdated: 2026-11-07T09:30:00Z\n---\nbody\n", "id"},
		{"unknown previous field", "projects/acme/tickets/2.yaml", ticket + "edits:\n  - number: 1\n    actor: agent\n    time: 2026-11-07T09:30:00Z\n    previous:\n      colour: red\n", "edits[0].previous.colour"},
		{"bad comment time", "projects/acme/tickets/2.yaml", ticket + "comments:\n  - number: 1\n    actor: agent\n    time: soon\n    body: hi\n", "comments[0].time"},
		{"bad question status", "projects/acme/tickets/2.yaml", ticket + "rounds:\n  - number: 1\n    claim: 1\n    posted: 2026-11-07T09:30:00Z\n    questions:\n      - number: 1\n        title: Q\n        body: B\n        status: maybe\n", "rounds[0].questions[0].status"},
		{"bad external state", "projects/acme/tickets/2.yaml", ticket + "link:\n  integration: github\n  id: \"12\"\n  external_state: gone\n", "link.external_state"},
		{"link without id", "projects/acme/tickets/2.yaml", ticket + "link:\n  integration: github\n  external_state: open\n", "link.id"},
		{"badly named ticket file", "projects/acme/tickets/two.yaml", ticket, "file name"},
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
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		name string
		flag string
		env  map[string]string
		want string
	}{
		{"flag wins", "/flag", map[string]string{"CODE_CENTRAL_STORE": "/env", "XDG_DATA_HOME": "/xdg", "HOME": "/home/me"}, "/flag"},
		{"env override", "", map[string]string{"CODE_CENTRAL_STORE": "/env", "XDG_DATA_HOME": "/xdg", "HOME": "/home/me"}, "/env"},
		{"xdg", "", map[string]string{"XDG_DATA_HOME": "/xdg", "HOME": "/home/me"}, "/xdg/code-central"},
		{"home fallback", "", map[string]string{"HOME": "/home/me"}, "/home/me/.local/share/code-central"},
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
