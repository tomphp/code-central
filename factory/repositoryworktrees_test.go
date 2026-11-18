package factory_test

import (
	"context"
	"github.com/tomoram/code-central/domain"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
)

// Worktrees in every Repository (Spec code-central-848, ADR 0048): at
// each Claim's start a Ticket gets a worktree in every Repository, on the
// same branch, and its session starts in the Home worktree with the
// others added as directories.

// wantAddDirs fails the test unless the run c was given exactly dirs,
// in order, with --add-dir.
func wantAddDirs(t *testing.T, c factorytest.Call, dirs ...string) {
	t.Helper()
	if got := c.Values("--add-dir"); !slices.Equal(got, dirs) {
		t.Errorf("--add-dir %q, want %q", got, dirs)
	}
}

// wantOnBranch fails the test unless each of worktrees is on branch, or
// detached for "".
func wantOnBranch(t *testing.T, h *factorytest.Harness, branch string, worktrees ...string) {
	t.Helper()
	for _, wt := range worktrees {
		if b := h.GitIn(wt, "branch", "--show-current"); b != branch {
			t.Errorf("%s is on branch %q, want %q", wt, b, branch)
		}
	}
}

func TestAClaimGetsAWorktreeOnTheSameBranchInEveryRepositoryAndTheSessionSeesThemAll(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	id := inAPI(h, "Add an endpoint")

	h.Start()
	tk := h.WantState(id, domain.InReview)

	wantOnBranch(t, h, id, h.Worktree(id), api.Worktree(id))
	if parent, base := h.GitIn(api.Path, "rev-parse", id+"^"), h.GitIn(api.Path, "rev-parse", "main"); parent != base {
		t.Errorf("api's branch %s starts at %s, want api's Base tip %s", id, parent, base)
	}
	if got := h.GitIn(api.Path, "log", "-1", "--format=%s", id); got != "Change endpoint.go" {
		t.Errorf("api's branch %s ends with %q, want the session's commit", id, got)
	}

	c := h.WantCalls(1)[0]
	if c.Dir != h.Worktree(id) {
		t.Errorf("claude ran in %s, want the Home worktree %s", c.Dir, h.Worktree(id))
	}
	wantAddDirs(t, c, api.Worktree(id))
	wantList := "\n\nRepositories:\n" +
		"- acme, the Home Repository: worktree " + h.Worktree(id) + ", Base main, branch " + id + "\n" +
		"- api: worktree " + api.Worktree(id) + ", Base main, branch " + id
	if !strings.HasPrefix(c.Prompt, "/implement "+id) || !strings.HasSuffix(c.Prompt, wantList) {
		t.Errorf("prompt %q, want /implement %s ending with the Repositories list %q", c.Prompt, id, wantList)
	}
	for _, want := range []string{
		"Commit each Repository's changes in its own worktree, on branch " + id,
		"finish with `repositories`",
	} {
		if !strings.Contains(c.SystemPrompt, want) {
			t.Errorf("system prompt doesn't say %q:\n%s", want, c.SystemPrompt)
		}
	}
	claim := tk.Claims[len(tk.Claims)-1]
	if want := "claude --resume " + c.Session + " --add-dir " + api.Worktree(id); claim.WorkingDir != h.Worktree(id) || claim.ResumeCommand != want {
		t.Errorf("claim's working_dir %q and Resume Command %q, want %q and %q", claim.WorkingDir, claim.ResumeCommand, h.Worktree(id), want)
	}
}

func TestAWorktreeIsNamedAfterItsRepositoryBesideAClonesLocalPath(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.MakeRepository("api")
	checkout := filepath.Join(filepath.Dir(filepath.Dir(h.Repo)), "src", "api-checkout")
	h.GitIn(filepath.Dir(h.Repo), "clone", "-q", api.CloneURL, checkout)
	if _, err := h.Human().AddRepository(context.Background(), h.Project, core.AddRepositoryInput{Name: "api", CloneURL: api.CloneURL, LocalPath: checkout}); err != nil {
		t.Fatal(err)
	}
	id := h.ReadyTicket("Add an endpoint")

	h.Start()
	h.WantState(id, domain.InReview)

	wt := filepath.Join(filepath.Dir(checkout), "api.worktrees", id)
	wantAddDirs(t, h.WantCalls(1)[0], wt)
	wantOnBranch(t, h, id, wt)
}

func TestTheHomeRepositoryAloneGetsNoAddedDirectoriesAndNoRepositoriesList(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Add a button")
	r := h.Start()
	tk := h.WantState(id, domain.InReview)
	h.SendChat("Hi")
	cv := h.WaitForReply()

	c := callFor(t, h, id)
	if c.Has("--add-dir") || c.Prompt != "/implement "+id || strings.Contains(c.SystemPrompt, "several Repositories") {
		t.Errorf("args %q, prompt %q: want no --add-dir, no Repositories list and nothing of several Repositories", c.Args, c.Prompt)
	}
	if got, want := tk.Claims[0].ResumeCommand, "claude --resume "+c.Session; got != want {
		t.Errorf("Resume Command %q, want %q", got, want)
	}
	r.WantOutput("; resume: claude --resume " + c.Session + "\n")
	chat := chatCalls(h)[0]
	if chat.Has("--add-dir") || strings.Contains(chat.SystemPrompt, "several Repositories") {
		t.Errorf("Chat args %q: want no --add-dir and nothing of several Repositories", chat.Args)
	}
	if want := "cd " + chatWorktree(h) + " && claude --resume " + cv.SessionID; cv.ResumeCommand != want {
		t.Errorf("Chat Resume Command %q, want %q", cv.ResumeCommand, want)
	}

	// Done, its unused branch is kept, as a Ticket branch always was.
	h.Accept(id)
	h.WantState(id, domain.Done)
	r.Poll()
	r.WantOutput(id + ": Done: removed its worktree " + h.Worktree(id))
	if b := h.Git("branch", "--list", id); b == "" {
		t.Errorf("the branch %s was deleted, want it kept with the Home Repository alone", id)
	}
}

func TestAStageGetsADetachedWorktreeAtEachRepositorysBaseTip(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	id := h.StageTicket("spec", "Spec: Add an endpoint")

	h.Start()
	h.WantState(id, domain.InReview)

	wantOnBranch(t, h, "", h.Worktree(id), api.Worktree(id))
	for _, w := range []struct{ wt, clone string }{{h.Worktree(id), h.Repo}, {api.Worktree(id), api.Path}} {
		if got, tip := h.GitIn(w.wt, "rev-parse", "HEAD"), h.GitIn(w.clone, "rev-parse", "main"); got != tip {
			t.Errorf("%s is at %s, want its Base's tip %s", w.wt, got, tip)
		}
	}
	if b := h.GitIn(api.Path, "branch", "--list", id); b != "" {
		t.Errorf("a Stage made branch %q in api", b)
	}
	c := h.WantCalls(1)[0]
	wantAddDirs(t, c, api.Worktree(id))
	if want := "\n- api: worktree " + api.Worktree(id) + ", Base main, detached at its tip"; !strings.HasSuffix(c.Prompt, want) {
		t.Errorf("prompt %q, want it to end with %q", c.Prompt, want)
	}
	if want := "a detached, read-only worktree in each at its Base's tip"; !strings.Contains(c.SystemPrompt, want) {
		t.Errorf("system prompt doesn't say %q:\n%s", want, c.SystemPrompt)
	}
}

func TestARepositoryAddedMidSessionGetsItsWorktreeAtTheTicketsNextClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	id, first, r := answeredTicket(t, h)
	web := h.AddRepository("web")

	r.Poll()
	h.WantState(id, domain.InReview)
	resumed := h.WantCalls(2)[1]
	if s, _ := resumed.Flag("--resume"); s != first.Session {
		t.Fatalf("args %q, want the session %s resumed", resumed.Args, first.Session)
	}
	// The Claim's own, without web's.
	wantAddDirs(t, resumed, api.Worktree(id))
	if _, err := os.Stat(web.Worktree(id)); !os.IsNotExist(err) {
		t.Errorf("web has a worktree for %s mid-Claim (%v), want none until the next Claim", id, err)
	}

	if _, err := h.Human().Reject(context.Background(), h.Project, core.RejectInput{Ticket: core.Ref(id), Comment: "Add web's side too."}); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	h.WantState(id, domain.InReview)
	wantAddDirs(t, h.WantCalls(3)[2], api.Worktree(id), web.Worktree(id))
	wantOnBranch(t, h, id, web.Worktree(id))
}

func TestAChatSeesEveryRepositoryThroughAChatWorktreeInEachAtItsBaseTip(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	r := h.Start()

	h.SendChat("What does api hold?")
	cv := h.WaitForReply()

	c := chatCalls(h)[0]
	session, _ := c.Flag("--session-id")
	if c.Dir != chatWorktree(h) {
		t.Errorf("ran in %s, want the Home Repository's Chat worktree %s", c.Dir, chatWorktree(h))
	}
	wantAddDirs(t, c, api.Worktree("chat"))
	wantOnBranch(t, h, "", api.Worktree("chat"))
	if head, tip := h.GitIn(api.Worktree("chat"), "rev-parse", "HEAD"), h.GitIn(api.Path, "rev-parse", "main"); head != tip {
		t.Errorf("api's Chat worktree is at %s, want api's Base tip %s", head, tip)
	}
	if want := "cd " + chatWorktree(h) + " && claude --resume " + session + " --add-dir " + api.Worktree("chat"); cv.ResumeCommand != want {
		t.Errorf("Resume Command %q, want %q", cv.ResumeCommand, want)
	}
	if want := "; api: " + api.Worktree("chat") + "."; !strings.Contains(c.SystemPrompt, want) {
		t.Errorf("system prompt doesn't say %q:\n%s", want, c.SystemPrompt)
	}

	h.SendChat("And its tests?")
	h.WaitForReply()
	wantAddDirs(t, chatCalls(h)[1], api.Worktree("chat"))

	r.Poll()
	r.Poll()
	if _, err := os.Stat(filepath.Join(api.Worktree("chat"), "README.md")); err != nil {
		t.Errorf("api's Chat worktree is gone: %v", err)
	}
	if out := r.Stdout(); strings.Contains(out, "chat: get_ticket") || strings.Contains(out, "removed its worktree") {
		t.Errorf("stdout %q, want nothing said of a Chat worktree as a Ticket's", out)
	}
}

func TestADirtyChatWorktreeInAnotherRepositoryFailsTheFirstTurnNamingIt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	h.Start()
	h.SendChat("First")
	h.WaitForReply()
	if err := os.WriteFile(filepath.Join(api.Worktree("chat"), "notes.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Human().StartConversation(context.Background(), h.Project, core.StartConversationInput{}); err != nil {
		t.Fatal(err)
	}

	h.SendChat("Second conversation")
	r := reply(h.WaitForReply())

	if r.Outcome != core.ChatFailed || !strings.HasPrefix(r.Error, "In Repository api, the Chat worktree at "+api.Worktree("chat")) || !strings.Contains(r.Error, "notes.txt") {
		t.Errorf("reply ended %q (%q), want failed naming Repository api, its Chat worktree and the file", r.Outcome, r.Error)
	}
	if n := len(chatCalls(h)); n != 1 {
		t.Errorf("%d Chat calls, want no turn started for the dirty worktree", n)
	}
}

func TestADoneTicketsWorktreesAndTheBranchesItNeverUsedAreSweptInEveryRepository(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	id := h.ReadyTicket("Change nothing in api")
	r := h.Start()
	h.WantState(id, domain.InReview)
	// api's Base moves on after the branch was cut from it.
	api.CommitOn("main", "later.go", "package api\n")
	h.Accept(id)
	h.WantState(id, domain.Done)

	r.Poll()

	for _, w := range []struct{ wt, clone string }{{h.Worktree(id), h.Repo}, {api.Worktree(id), api.Path}} {
		if _, err := os.Stat(w.wt); !os.IsNotExist(err) {
			t.Errorf("%s is still there (%v), want it removed", w.wt, err)
		}
		if b := h.GitIn(w.clone, "branch", "--list", id); b != "" {
			t.Errorf("%s still has the unused branch %q", w.clone, b)
		}
	}
	r.WantOutput(id + ": deleted its unused branch " + id + "\n")
	r.WantOutput(id + ": deleted its unused branch " + id + " in Repository api\n")
}

func TestABranchWithWorkOfItsOwnIsKeptInEveryRepositoryEvenOnceMerged(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	id := h.ReadyTicket("Add an endpoint")
	h.Script(id, append(factorytest.CommitIn("api", "endpoint.go", "package api\n"),
		factorytest.WriteFile("client.go", "package acme\n"), factorytest.Git("add", "client.go"), factorytest.Git("commit", "-q", "-m", "Call it"),
		factorytest.FinishIn("Done.", true, "api"), factorytest.Result("success"))...)
	r := h.Start()
	h.WantState(id, domain.InReview)
	heads := map[string]string{}
	for _, clone := range []string{h.Repo, api.Path} {
		heads[clone] = h.GitIn(clone, "rev-parse", id)
		// The human merges it into the Base by hand.
		h.GitIn(clone, "merge", "-q", "--no-ff", "-m", "Merge "+id, id)
	}
	h.Accept(id)
	r.Poll()
	h.WantState(id, domain.Done)

	r.Poll()

	if _, err := os.Stat(api.Worktree(id)); !os.IsNotExist(err) {
		t.Errorf("api's worktree is still there (%v), want it removed", err)
	}
	for clone, head := range heads {
		if got := h.GitIn(clone, "rev-parse", id); got != head {
			t.Errorf("%s's branch %s is at %q, want it kept at its work, %s", clone, id, got, head)
		}
	}
	if strings.Contains(r.Stdout(), "deleted its unused branch") {
		t.Errorf("stdout %q, want no branch deleted", r.Stdout())
	}
}

func TestASpecLandedTicketsBranchIsCutFromEachRepositorysOwnSpecBranch(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	h.MakeRequirement()
	r := h.Start()
	spec := "spec/" + h.SpecID()
	// api's Spec Branch already holds an earlier Ticket's work.
	api.CommitOn(spec, "earlier.go", "package api\n")
	apiSpecTip := h.GitIn(api.Path, "rev-parse", spec)
	id := h.ReadyTicket("Add a feature")
	h.Script(id, factorytest.Wait())

	r.Poll()

	if got, base := h.Git("rev-parse", spec), h.Git("rev-parse", "main"); got != base {
		t.Errorf("the Home Repository's Spec Branch is at %s, want made at its Base's tip %s", got, base)
	}
	if got := h.GitIn(h.Worktree(id), "rev-parse", "HEAD"); got != h.Git("rev-parse", spec) {
		t.Errorf("the Home worktree is at %s, want its Spec Branch's tip", got)
	}
	if got := h.GitIn(api.Worktree(id), "rev-parse", "HEAD"); got != apiSpecTip {
		t.Errorf("api's worktree is at %s, want api's Spec Branch's tip %s", got, apiSpecTip)
	}
}

func TestASpecBranchInEachRepositorysBaseIsDeletedOnceEveryTicketIsDone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	h.MakeRequirement()
	cancelDrafts(t, h)
	id := h.ReadyTicket("Change nothing")
	r := h.Start()
	h.WantState(id, domain.InReview)
	if h.GitIn(api.Path, "branch", "--list", "spec/*") == "" {
		t.Fatal("the Claim made no Spec Branch in api")
	}
	h.Accept(id)
	h.WantState(id, domain.Done)

	r.Poll()

	r.WantOutput(h.SpecID() + ": deleted spec/" + h.SpecID() + ", which is in main")
	r.WantOutput(h.SpecID() + ": deleted spec/" + h.SpecID() + " in Repository api, which is in main")
	for _, clone := range []string{h.Repo, api.Path} {
		if out := h.GitIn(clone, "branch", "--list", "spec/*"); out != "" {
			t.Errorf("%s still has the Spec Branch %q", clone, out)
		}
	}
}

func TestEachRepositorysBranchIsCutFromItsOwnBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	develop := "develop"
	if _, err := h.Human().EditRepository(context.Background(), h.Project, core.EditRepositoryInput{Name: "api", Base: &develop}); err != nil {
		t.Fatal(err)
	}
	r := h.Start()
	api.CommitOn(develop, "develop.go", "package api\n")
	developTip := h.GitIn(api.Path, "rev-parse", develop)
	id := h.ReadyTicket("Add an endpoint")
	h.Script(id, factorytest.Wait())

	r.Poll()

	if got := h.GitIn(api.Worktree(id), "rev-parse", "HEAD"); got != developTip {
		t.Errorf("api's branch %s starts at %s, want api's Base develop's tip %s", id, got, developTip)
	}
	if got, main := h.GitIn(h.Worktree(id), "rev-parse", "HEAD"), h.Git("rev-parse", "main"); got != main {
		t.Errorf("the Home branch %s starts at %s, want the Project's Base main's tip %s", id, got, main)
	}
	if want := "\n- api: worktree " + api.Worktree(id) + ", Base develop, branch " + id; !strings.HasSuffix(callFor(t, h, id).Prompt, want) {
		t.Errorf("prompt %q, want it to end with %q", callFor(t, h, id).Prompt, want)
	}
}

func TestAPrototypeWorksOnItsPrototypeBranchInEveryRepository(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	id := h.LabelledTicket("prototype", "Prototype: the theme picker")
	h.Script(id, factorytest.Wait())

	h.Start()

	wantOnBranch(t, h, "prototype/"+id, h.Worktree(id), api.Worktree(id))
	if want := "Commit each Repository's changes in its own worktree, on branch prototype/" + id; !strings.Contains(callFor(t, h, id).SystemPrompt, want) {
		t.Errorf("system prompt doesn't say %q", want)
	}
}

func TestAStrayDirectoryAtAnotherRepositorysWorktreePathFailsTheClaimNamingIt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	r := h.Start()
	id := h.ReadyTicket("Add an endpoint")
	if err := os.MkdirAll(api.Worktree(id), 0o755); err != nil {
		t.Fatal(err)
	}

	r.Poll()
	tk := h.WantState(id, domain.Failed)

	if n := len(h.Calls()); n != 0 {
		t.Errorf("claude ran %d times, want none", n)
	}
	f := tk.FailureReports[len(tk.FailureReports)-1]
	if !strings.HasPrefix(f.WhatBroke, "In Repository api, something that isn't a git worktree is at this Ticket's worktree path, "+api.Worktree(id)) || !strings.Contains(f.Why, filepath.Dir(api.Worktree(id))+"/<ticket-id>") {
		t.Errorf("failure report %+v doesn't name Repository api and its worktree folder", f)
	}
}

func TestABranchConflictingInAnotherRepositoryMakesAConflictClaimListingItsFiles(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	r := h.Start()
	id := h.ReadyTicket("Change api's readme")
	api.CommitOn(id, "README.md", "theirs\n")
	api.CommitOn("main", "README.md", "ours\n")
	h.Script(id, factorytest.Wait())

	r.Poll()

	c := callFor(t, h, id)
	requireConflictClaim(t, c, "README.md")
	if want := "\n\napi, base main:\n- README.md\n"; !strings.Contains(c.Prompt, want) {
		t.Errorf("first prompt %q doesn't list api's conflicting files as %q", c.Prompt, want)
	}
	if strings.Contains(c.Prompt, "acme, base") {
		t.Errorf("first prompt %q lists the Home Repository, which doesn't conflict", c.Prompt)
	}
	if want := "\n- api: worktree " + api.Worktree(id) + ", Base main, branch " + id; !strings.HasSuffix(c.Prompt, want) {
		t.Errorf("first prompt %q doesn't end with the Repositories list", c.Prompt)
	}
	for _, want := range []string{"finish with `repositories`", "for the Home Repository and `repositories` for each other one you committed in"} {
		if !strings.Contains(c.SystemPrompt, want) {
			t.Errorf("system prompt doesn't say %q:\n%s", want, c.SystemPrompt)
		}
	}
}

func TestADirtyStageWorktreeInAnotherRepositoryFailsTheClaimNamingIt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	r := h.Start()
	id := h.StageTicket("spec", "Spec: Add an endpoint")
	h.GitIn(api.Path, "worktree", "add", "-q", "--detach", api.Worktree(id), "main")
	if err := os.WriteFile(filepath.Join(api.Worktree(id), "notes.md"), []byte("written by a Stage"), 0o644); err != nil {
		t.Fatal(err)
	}

	r.Poll()
	tk := h.WantState(id, domain.Failed)

	if n := len(h.Calls()); n != 0 {
		t.Errorf("claude ran %d times with a dirty Stage worktree, want none", n)
	}
	f := tk.FailureReports[len(tk.FailureReports)-1]
	if !strings.HasPrefix(f.WhatBroke, "In Repository api, the Stage's worktree is dirty") || !strings.Contains(f.WhatBroke, "notes.md") || !strings.Contains(f.WhereWorkStands, api.Worktree(id)) {
		t.Errorf("failure report %+v doesn't name Repository api, the file and its worktree", f)
	}
}
