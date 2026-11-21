package factory_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory/factorytest"
)

// The Spec merge Ticket under Rebase (Spec code-central-1063, section
// "Conflicts and prompts"): a Spec Branch that conflicts with the base
// opens "Rebase the Spec Branch of <spec id> onto the base", whose agent
// rebases its branch, cut from the Spec Branch, onto the base. Accepting
// it moves the Spec Branch to that rebased result, which doesn't descend
// from the old tip, after replaying onto it any commits the Spec Branch
// gained while the Ticket was open; then the Spec lands.

// rebaseSpecMergeScript is the Spec merge Ticket's session under Rebase:
// it rebases its branch onto the base, keeping the branch's side of
// README, and finishes on the branch.
func rebaseSpecMergeScript(fix string) []factorytest.Step {
	return []factorytest.Step{
		factorytest.Git("rebase", "-q", "-X", "theirs", "main"),
		factorytest.CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", "Rebased onto the base.", "--branch", fix),
	}
}

// specMergeOpened is a Requirement Spec under Rebase whose first Ticket,
// a, changed README on the Spec Branch, after the base changed README
// too and the Factory opened the Spec merge Ticket fix, whose session ran
// script. It returns the Factory, with the Spec held from landing by
// release until it is called.
func specMergeOpened(t *testing.T, h *factorytest.Harness, fix string, script ...factorytest.Step) (r *factorytest.Running, a string, release func()) {
	t.Helper()
	h.MakeRequirement()
	setRebase(h, true)
	// a changes README, then adds one.txt, so a rebase that keeps the
	// base's README still keeps a commit of a's.
	a = h.ReadyTicket("First")
	h.CommitOn(a, "README", "spec side\n")
	h.Finished(a, a, h.CommitOn(a, "one.txt", "1\n"))
	h.Accept(a)
	release = holdTicketsStage(t, h)
	h.Script(fix, script...)
	r = h.Start()
	r.WantOutput(a + ": replayed onto spec/" + h.SpecID())
	h.CommitOn("main", "README", "base side\n")
	release()
	r.Poll()
	r.WantOutput(h.SpecID() + ": spec/" + h.SpecID() + " conflicts with main in README: opened " + fix)
	return r, a, release
}

func TestUnderRebaseASpecBranchConflictOpensTheRebaseTicketAndAcceptingItReplacesTheSpecBranch(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	fix := "acme-5"
	r, a, _ := specMergeOpened(t, h, fix, rebaseSpecMergeScript(fix)...)
	spec := "spec/" + h.SpecID()

	tk := h.WantState(fix, domain.InReview)
	if tk.Title != api.SpecMergeRebaseTitle(h.SpecID()) || !tk.SpecMergeTicket || !strings.Contains(tk.Description, "Rebase this branch onto the base") {
		t.Fatalf("%s is %+v, want it marked, titled %q and saying to rebase", fix, tk, api.SpecMergeRebaseTitle(h.SpecID()))
	}
	oldTip, rebased, baseTip := h.Git("rev-parse", spec), h.Git("rev-parse", fix), h.Git("rev-parse", "main")
	if descends(h, h.Repo, rebased, oldTip) || !descends(h, h.Repo, rebased, baseTip) {
		t.Fatalf("the session's branch %s should descend from the base %s and not from the Spec Branch's old tip %s", rebased, baseTip, oldTip)
	}

	h.Accept(fix)
	r.Poll()

	r.WantOutput(fix + ": moved " + spec + " to its rebased result at " + rebased[:12])
	h.WantState(a, domain.Done)
	h.WantState(fix, domain.Done)
	// The Spec Branch is the rebased result: linear on the base, the
	// Ticket's one commit on it, with no merge; the base got one `Merge
	// spec` commit whose second parent it is.
	if got := h.Git("rev-parse", spec); got != rebased {
		t.Errorf("%s is at %s, want the rebased result %s", spec, got, rebased)
	}
	subs, merges := specLine(h, h, baseTip)
	if strings.Join(subs, ",") != "Change README,Change one.txt" || len(merges) != 0 {
		t.Errorf("%s holds %v with merges %v on the base, want a's two commits and no merge", spec, subs, merges)
	}
	if ms := mergeSpecs(h, baseTip); len(ms) != 1 || h.Git("rev-parse", ms[0]+"^2") != rebased || h.Git("rev-parse", ms[0]+"^1") != baseTip {
		t.Errorf("main gained the Merge spec commits %v, want one merging %s into %s", ms, rebased, baseTip)
	}
}

// descends reports whether commit descends from ancestor in dir, or is it.
func descends(h *factorytest.Harness, dir, commit, ancestor string) bool {
	return h.GitIn(dir, "rev-list", "-n", "1", ancestor, "^"+commit) == ""
}

// acceptedOnSpecBranch makes an Accepted Ticket whose branch, cut from
// the Spec Branch's tip, changes file, and returns it with its commit.
func acceptedOnSpecBranch(h *factorytest.Harness, title, file, content string) (id, commit string) {
	id = h.ReadyTicket(title)
	h.Git("branch", id, "spec/"+h.SpecID())
	commit = h.CommitOn(id, file, content)
	h.Finished(id, id, commit)
	h.Accept(id)
	return id, commit
}

func TestUnderRebaseTicketsAcceptedWhileTheSpecMergeTicketWasOpenAreReplayedOntoItsResult(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	fix := "acme-5"
	r, a, _ := specMergeOpened(t, h, fix, rebaseSpecMergeScript(fix)...)
	spec := "spec/" + h.SpecID()
	h.WantState(fix, domain.InReview)
	rebased, baseTip := h.Git("rev-parse", fix), h.Git("rev-parse", "main")

	// Another Ticket is accepted while the Spec merge Ticket waits: the
	// Spec Branch goes on from the fork point.
	b, _ := acceptedOnSpecBranch(h, "Second", "two.txt", "2\n")
	r.Poll()
	r.WantOutput(b + ": replayed onto " + spec)
	goneOn := h.Git("rev-parse", spec)

	h.Accept(fix)
	r.Poll()

	r.WantOutput(fix + ": moved " + spec + " to its rebased result at ")
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	h.WantState(fix, domain.Done)
	// The Spec Branch is the rebased result with b's commit replayed on
	// it, once, and nothing of the old tip.
	tip := h.Git("rev-parse", spec)
	if h.Git("rev-parse", tip+"^") != rebased || descends(h, h.Repo, tip, goneOn) {
		t.Errorf("%s is at %s, want b's commit on the rebased result %s, not on the old tip %s", spec, tip, rebased, goneOn)
	}
	subs, merges := specLine(h, h, baseTip)
	if strings.Join(subs, ",") != "Change README,Change one.txt,Change two.txt" || len(merges) != 0 {
		t.Errorf("%s holds %v with merges %v on the base, want a's then b's commits, each once, and no merge", spec, subs, merges)
	}
	if ms := mergeSpecs(h, baseTip); len(ms) != 1 || h.Git("rev-parse", ms[0]+"^2") != tip {
		t.Errorf("main gained the Merge spec commits %v, want one merging %s", ms, tip)
	}
}

func TestUnderRebaseAConflictReplayingTheSpecBranchOntoTheSpecMergeTicketSendsItBackToRebase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	fix := "acme-5"
	// The session keeps the base's side of README; a Ticket accepted
	// meanwhile changes the Spec Branch's side, so it doesn't replay.
	r, a, _ := specMergeOpened(t, h, fix,
		factorytest.Git("rebase", "-q", "-X", "ours", "main"),
		factorytest.CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", "Rebased onto the base.", "--branch", fix))
	spec := "spec/" + h.SpecID()
	h.WantState(fix, domain.InReview)
	fork := h.Git("rev-parse", spec)
	b, _ := acceptedOnSpecBranch(h, "Second", "README", "b side\n")
	r.Poll()
	r.WantOutput(b + ": replayed onto " + spec)
	goneOn := h.Git("rev-parse", spec)
	// Sent back, its next session brings the Spec Branch's commits since
	// the fork point onto the branch, resolving README the base's way
	// again, which isn't how b's commit has it.
	h.Script(fix,
		factorytest.Git("cherry-pick", "-X", "ours", "--empty=keep", fork+".."+goneOn),
		factorytest.CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", "Brought the Spec Branch's commits over.", "--branch", fix))

	h.Accept(fix)
	r.Poll()

	tk := h.Ticket(fix)
	if !movedTo(tk, domain.Accepted, domain.Ready) {
		t.Fatalf("%s was not sent back: %+v", fix, tk.Moves)
	}
	if !commented(tk, "replaying "+spec+"'s commits since "+fork[:12], "- README", "Rebase "+spec+"'s commits since "+fork[:12]+" onto the branch, resolve the conflicts, and finish again.") ||
		commented(tk, "Rebase the branch onto "+spec) {
		t.Errorf("comments %+v, want the send-back note to say to rebase the Spec Branch's commits since %s onto the branch", tk.Comments, fork[:12])
	}
	if got := h.Git("rev-parse", spec); got != goneOn {
		t.Errorf("%s moved to %s, want it left at %s", spec, got, goneOn)
	}
	h.WantState(a, domain.Accepted)
	h.WantState(b, domain.Accepted)

	// Finished again with those commits on the branch, resolved its own
	// way, accepting it moves the Spec Branch to the branch as it is: the
	// Factory doesn't replay them a second time.
	h.WantState(fix, domain.InReview)
	resolved := h.Git("rev-parse", fix)
	if descends(h, h.Repo, resolved, goneOn) || h.Git("show", resolved+":README") != "base side" {
		t.Fatalf("the second session left %s at %s with README %q, want b's commit cherry-picked keeping the base's side", fix, resolved, h.Git("show", resolved+":README"))
	}
	// A Spec merge counts only after the Ticket's last move back to Ready.
	h.Server.Clock.Advance(time.Second)
	h.Accept(fix)
	r.Poll()

	r.WantOutput(fix + ": moved " + spec + " to its rebased result at " + resolved[:12])
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	h.WantState(fix, domain.Done)
	if h.Git("show", "main:README") != "base side" {
		t.Errorf("main's README is %q, want the base's side, as the Spec merge Ticket resolved it", h.Git("show", "main:README"))
	}
}

func TestUnderRebaseTheSpecMergeTicketStartsAsAConflictClaimAskedToRebase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	fix := "acme-5"
	specMergeOpened(t, h, fix, factorytest.Wait())

	c := callFor(t, h, fix)
	requireConflictClaim(t, c, "README")
	if !strings.Contains(c.Prompt, "Rebase the branch onto main and resolve the conflicts") || strings.Contains(c.Prompt, "Merge main into the branch") {
		t.Errorf("first prompt %q, want it to say to rebase onto main", c.Prompt)
	}
}

func TestAnUnmarkedTicketWithTheRebaseTitleIsRecognisedAsASpecMergeTicket(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.MakeRequirement()
	setRebase(h, true)
	a, _ := accepted(h, "First", "README", "spec side\n")
	holdTicketsStage(t, h)
	legacy := "acme-5"
	h.Script(legacy, factorytest.Wait())
	r := h.Start()
	r.WantOutput(a + ": replayed onto spec/" + h.SpecID())
	h.CommitOn("main", "README", "base side\n")
	// Opened before the mark existed: unmarked, with no branch yet, so
	// the Spec Branch is what is checked against the base.
	if got := h.ReadyTicket(api.SpecMergeRebaseTitle(h.SpecID())); got != legacy {
		t.Fatalf("the unmarked Ticket is %s, want %s", got, legacy)
	}
	r.Poll()

	c := callFor(t, h, legacy)
	requireConflictClaim(t, c, "README")
	if !strings.Contains(c.Prompt, "Rebase the branch onto main and resolve the conflicts") {
		t.Errorf("first prompt %q, want it to say to rebase onto main", c.Prompt)
	}
}

func TestUnderRebaseAMergeTheSpecBranchGainedWhileRebaseWasOffIsRemadeOnTheRebasedResult(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	fix := "acme-5"
	r, a, _ := specMergeOpened(t, h, fix, rebaseSpecMergeScript(fix)...)
	spec := "spec/" + h.SpecID()
	h.WantState(fix, domain.InReview)
	rebased := h.Git("rev-parse", fix)
	// Rebase is turned off while the Spec merge Ticket waits, so a Ticket
	// accepted meanwhile is merged into the Spec Branch, then on again.
	setRebase(h, false)
	b, bCommit := acceptedOnSpecBranch(h, "Second", "two.txt", "2\n")
	r.Poll()
	r.WantOutput(b + ": merged into " + spec)
	setRebase(h, true)

	h.Accept(fix)
	r.Poll()

	r.WantOutput(fix + ": moved " + spec + " to its rebased result at ")
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	h.WantState(fix, domain.Done)
	// The merge is remade on the rebased result, merging the same work.
	tip := h.Git("rev-parse", spec)
	if h.Git("rev-parse", tip+"^1") != rebased || h.Git("rev-parse", tip+"^2") != bCommit || subject(h, h.Repo, tip) != "Merge "+b+": Second" {
		t.Errorf("%s is at %s %q with parents %s, want b's merge remade on the rebased result %s with b's commit %s as its second parent", spec, tip, subject(h, h.Repo, tip), h.Git("log", "-1", "--format=%P", tip), rebased, bCommit)
	}
	if h.Git("show", tip+":two.txt") != "2" {
		t.Errorf("two.txt isn't on %s: the remade merge lost b's work", spec)
	}
}

// specMergeOpenedInTwo is a Requirement Spec with the Repository api
// under Rebase, whose Ticket wrote client.go in the Home Repository and
// endpoint.go and other.go in api, accepted and replayed onto the Spec
// Branch in both, after the base changed client.go and endpoint.go, so
// the Spec Branch conflicts in both and the Factory opened the Spec
// merge Ticket fix, whose session ran script. It returns the Factory,
// api and the Ticket.
func specMergeOpenedInTwo(t *testing.T, h *factorytest.Harness, fix string, script ...factorytest.Step) (r *factorytest.Running, apiRepo *factorytest.Repository, id string) {
	t.Helper()
	h.MakeRequirement()
	apiRepo = h.AddRepository("api")
	id = h.ReadyTicket("Add an endpoint")
	// other.go too, so a rebase that keeps the base's endpoint.go still
	// keeps a commit of the Ticket's in api.
	steps := factorytest.CommitIn("api", "endpoint.go", "package api\n")
	steps = append(steps, factorytest.CommitIn("api", "other.go", "package api\n")...)
	steps = append(steps, factorytest.WriteFile("client.go", "package acme\n"), factorytest.Git("add", "client.go"), factorytest.Git("commit", "-q", "-m", "Call it"),
		factorytest.FinishIn("Done.", true, "api"), factorytest.Result("success"))
	h.Script(id, steps...)
	setRebase(h, true)
	release := holdTicketsStage(t, h)
	h.Script(fix, script...)
	r = h.Start()
	h.WantState(id, domain.InReview)
	h.Accept(id)
	r.Poll()
	r.WantOutput(id + ": replayed onto spec/" + h.SpecID())
	h.CommitOn("main", "client.go", "package base\n")
	apiRepo.CommitOn("main", "endpoint.go", "package other\n")
	release()
	r.Poll()
	r.WantOutput(h.SpecID() + ": spec/" + h.SpecID() + " conflicts with the base in " + h.Project + ", api: opened " + fix + " to rebase it onto the base")
	return r, apiRepo, id
}

// rebaseInTwo is a Spec merge Ticket's session that rebases its branch
// onto the base in the Home Repository, keeping the branch's side, and in
// api, keeping the side apiSide names, and finishes with both.
func rebaseInTwo(apiSide string) []factorytest.Step {
	return []factorytest.Step{
		factorytest.InRepository("api", factorytest.Git("rebase", "-q", "-X", apiSide, "main")),
		factorytest.Git("rebase", "-q", "-X", "theirs", "main"),
		factorytest.FinishIn("Rebased onto the base.", true, "api"),
		factorytest.Result("success"),
	}
}

func TestUnderRebaseAcceptingTheSpecMergeTicketMovesTheSpecBranchInEachRepository(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	fix := "acme-5"
	r, apiRepo, id := specMergeOpenedInTwo(t, h, fix, rebaseInTwo("theirs")...)
	spec := "spec/" + h.SpecID()
	h.WantState(fix, domain.InReview)
	homeRebased, apiRebased := h.Git("rev-parse", fix), h.GitIn(apiRepo.Path, "rev-parse", fix)
	// A Ticket accepted meanwhile moves the Spec Branch on in the Home
	// Repository alone: api's stays at the fork point.
	b, _ := acceptedOnSpecBranch(h, "Second", "two.txt", "2\n")
	r.Poll()
	r.WantOutput(b + ": replayed onto " + spec)

	h.Accept(fix)
	r.Poll()

	r.WantOutput(fix + ": moved " + spec + " to its rebased result at ")
	r.WantOutput(fix + ": moved " + spec + " to its rebased result at " + apiRebased[:12] + " in Repository api")
	h.WantState(id, domain.Done)
	h.WantState(b, domain.Done)
	h.WantState(fix, domain.Done)
	if tip := h.Git("rev-parse", spec); h.Git("rev-parse", tip+"^") != homeRebased || subject(h, h.Repo, tip) != "Change two.txt" {
		t.Errorf("the Home Repository's %s is at %s %q, want b's commit replayed on the rebased result %s", spec, tip, subject(h, h.Repo, tip), homeRebased)
	}
	if tip := h.GitIn(apiRepo.Path, "rev-parse", spec); tip != apiRebased {
		t.Errorf("api's %s is at %s, want the rebased result %s", spec, tip, apiRebased)
	}
	if h.Git("show", "main:client.go") != "package acme" || h.GitIn(apiRepo.Path, "show", "main:endpoint.go") != "package api" {
		t.Error("a base doesn't hold the Spec Branch's side: the Spec didn't land its rebased result in each")
	}
}

func TestUnderRebaseASpecMergeTicketConflictingInOneRepositoryMovesTheSpecBranchInNeither(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	fix := "acme-5"
	// In api the session keeps the base's side of endpoint.go.
	r, apiRepo, id := specMergeOpenedInTwo(t, h, fix, rebaseInTwo("ours")...)
	spec := "spec/" + h.SpecID()
	h.WantState(fix, domain.InReview)
	apiHead := h.GitIn(apiRepo.Path, "rev-parse", fix)
	homeFork, apiFork := h.Git("rev-parse", spec), h.GitIn(apiRepo.Path, "rev-parse", spec)
	// A Ticket accepted meanwhile changes endpoint.go on api's Spec Branch,
	// which doesn't replay onto the base's side, and two.txt in the Home
	// Repository, which does.
	b := h.ReadyTicket("Second")
	h.Git("branch", b, spec)
	h.GitIn(apiRepo.Path, "branch", b, spec)
	homeCommit := h.CommitOn(b, "two.txt", "2\n")
	apiCommit := apiRepo.CommitOn(b, "endpoint.go", "package b\n")
	h.Finished(b, b, homeCommit, api.RepositoryWork{Name: "api", Branch: b, Commit: apiCommit})
	h.Accept(b)
	r.Poll()
	r.WantOutput(b + ": replayed onto " + spec + " at ")
	homeTip, apiTip := h.Git("rev-parse", spec), h.GitIn(apiRepo.Path, "rev-parse", spec)

	h.Accept(fix)
	r.Poll()

	tk := h.Ticket(fix)
	if !movedTo(tk, domain.Accepted, domain.Ready) {
		t.Fatalf("%s was not sent back: %+v", fix, tk.Moves)
	}
	if h.Git("rev-parse", spec) != homeTip || h.GitIn(apiRepo.Path, "rev-parse", spec) != apiTip {
		t.Error("a Spec Branch moved: a Spec merge Ticket conflicting in any Repository moves the Spec Branch in none")
	}
	if !commented(tk, "replaying the Spec Branch's later commits onto it conflicts in these Repositories, so it moved the Spec Branch in none of them",
		"- api: replaying "+spec+"'s commits since "+apiFork[:12]+" onto "+apiHead[:12]+" at "+apiTip[:12]+" conflicts in these files:\n  - endpoint.go",
		"In each, rebase its Spec Branch's commits since the fork point named above onto the branch, resolve the conflicts, and finish again. The Factory then takes the branch as holding them") ||
		commented(tk, "- "+h.Project+":") || commented(tk, homeFork[:12]) {
		t.Fatalf("comments %+v, want the send-back note to name api alone, with its fork point and file", tk.Comments)
	}
	h.WantState(id, domain.Accepted)
	h.WantState(b, domain.Accepted)
}
