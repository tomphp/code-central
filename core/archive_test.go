package core_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// finishedWork is Project acme with Spec acme-1, Done Ticket acme-2 and
// Cancelled Ticket acme-3: nothing left to do.
func (f fixture) finishedWork(t *testing.T) {
	t.Helper()
	f.work(t, store.Draft, store.Draft)
	f.move(t, "2", store.Done)
	f.move(t, "3", store.Cancelled)
}

func (f fixture) archive(t *testing.T, key string) core.ProjectView {
	t.Helper()
	p, err := f.core.ArchiveProject(human, key)
	if err != nil {
		t.Fatalf("archive_project: %v", err)
	}
	return p
}

func TestArchiveIsRefusedWhileAnyTicketIsUnfinished(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Draft, store.Ready, store.Draft)
	f.move(t, "2", store.Done)
	_, err := f.core.ArchiveProject(human, "acme")
	wantCode(t, err, core.ActionNotAllowed, "")
	msg := err.Error()
	if !strings.Contains(msg, "acme-3 (Ready)") || !strings.Contains(msg, "acme-4 (Draft)") || strings.Contains(msg, "acme-2") {
		t.Fatalf("the refusal should name the unfinished Tickets: %s", msg)
	}
	if p, _ := f.core.GetProject(ctx, "acme"); p.Archived {
		t.Fatal("a refused archive changed the Project")
	}
}

func TestArchiveIsTheHumans(t *testing.T) {
	f := newFixture(t)
	f.finishedWork(t)
	for _, a := range []core.Actor{core.Agent, core.Integration} {
		_, err := f.core.ArchiveProject(core.WithActor(ctx, a), "acme")
		wantCode(t, err, core.ActionNotAllowed, "")
	}
	_, err := f.core.ArchiveProject(human, "nope")
	wantCode(t, err, core.UnknownProject, "")
}

func TestArchiveAndUnarchive(t *testing.T) {
	f := newFixture(t)
	f.finishedWork(t)
	f.project(t, "empty") // no Tickets at all: nothing unfinished
	f.clock.Advance(time.Hour)

	p := f.archive(t, "acme")
	if !p.Archived || p.ArchivedAt == nil || !p.ArchivedAt.Equal(f.clock.Now()) {
		t.Fatalf("archive_project: %+v", p)
	}
	f.archive(t, "empty")
	got, _ := f.core.GetProject(ctx, "acme")
	if !got.Archived || !got.ArchivedAt.Equal(f.clock.Now()) {
		t.Fatalf("get_project after archive: %+v", got)
	}
	// Archiving twice is a write on an Archived Project.
	_, err := f.core.ArchiveProject(human, "acme")
	wantArchived(t, err)

	_, err = f.core.UnarchiveProject(ctx, "acme")
	wantCode(t, err, core.ActionNotAllowed, "") // the agent can't bring it back
	p, err = f.core.UnarchiveProject(human, "acme")
	if err != nil || p.Archived || p.ArchivedAt != nil {
		t.Fatalf("unarchive_project: %+v, %v", p, err)
	}
	// Unarchiving a Project that isn't archived changes nothing.
	if p, err = f.core.UnarchiveProject(human, "acme"); err != nil || p.Archived {
		t.Fatalf("unarchive twice: %+v, %v", p, err)
	}
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "More"}) // it takes work again
}

func wantArchived(t *testing.T, err error) {
	t.Helper()
	var e *core.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want the Archived Project refusal", err)
	}
	if e.Code != core.ActionNotAllowed || !slices.Equal(e.AllowedActions, []string{"unarchive_project"}) ||
		!strings.Contains(e.Message, "archived") {
		t.Fatalf("got %+v, want action_not_allowed listing unarchive_project", e)
	}
}

// writesOnAcme makes every write operation on Project acme once, with
// input that would succeed (or fail only on State) were acme not
// archived. Keyed by operation name.
var writesOnAcme = map[string]func(f fixture) error{
	"create_spec": func(f fixture) error {
		_, err := f.core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "More", Body: "b"})
		return err
	},
	"create_ticket": func(f fixture) error {
		_, err := f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "More"})
		return err
	},
	"edit_spec": func(f fixture) error {
		title := "New"
		_, err := f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Title: &title})
		return err
	},
	"edit_ticket": func(f fixture) error {
		title := "New"
		_, err := f.core.EditTicket(ctx, "acme", core.EditTicketInput{Ticket: "2", Title: &title})
		return err
	},
	"add_labels": func(f fixture) error {
		_, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "2", Labels: []string{"ui"}})
		return err
	},
	"remove_labels": func(f fixture) error {
		_, err := f.core.RemoveLabels(ctx, "acme", core.LabelsInput{Target: "2", Labels: []string{"ui"}})
		return err
	},
	"set_tags": func(f fixture) error {
		_, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "1", Tags: map[string]string{"k": "v"}})
		return err
	},
	"remove_tags": func(f fixture) error {
		_, err := f.core.RemoveTags(ctx, "acme", core.RemoveTagsInput{Target: "1", Keys: []string{"k"}})
		return err
	},
	"comment": func(f fixture) error {
		_, err := f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "hi"})
		return err
	},
	"approve": func(f fixture) error {
		_, err := f.core.Approve(ctx, "acme", core.TicketInput{Ticket: "2"})
		return err
	},
	"defer": func(f fixture) error {
		_, err := f.core.Defer(ctx, "acme", core.DeferInput{Ticket: "2", Reason: "later"})
		return err
	},
	"claim": func(f fixture) error {
		_, err := f.core.Claim(ctx, "acme", core.ClaimInput{Ticket: "2"})
		return err
	},
	"finish": func(f fixture) error {
		_, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "x", Summary: "s"})
		return err
	},
	"accept": func(f fixture) error {
		_, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"})
		return err
	},
	"reject": func(f fixture) error {
		_, err := f.core.Reject(human, "acme", core.RejectInput{Ticket: "2", Comment: "no"})
		return err
	},
	"set_session": func(f fixture) error {
		_, err := f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "2", ClaimID: "x", SessionID: "s"})
		return err
	},
	"release": func(f fixture) error {
		_, err := f.core.Release(human, "acme", core.ReleaseInput{Ticket: "2"})
		return err
	},
	"add_blocker": func(f fixture) error {
		_, err := f.core.AddBlocker(ctx, "acme", core.BlockerInput{Ticket: "3", Blocker: "2"})
		return err
	},
	"remove_blocker": func(f fixture) error {
		_, err := f.core.RemoveBlocker(ctx, "acme", core.BlockerInput{Ticket: "3", Blocker: "2"})
		return err
	},
	"cancel": func(f fixture) error {
		_, err := f.core.Cancel(human, "acme", core.CancelInput{Ticket: "2"})
		return err
	},
	"report_failure": func(f fixture) error {
		_, err := f.core.ReportFailure(ctx, "acme", core.ReportFailureInput{Ticket: "2", ClaimID: "x", WhatBroke: "a", Why: "b", WhatToFix: "c", WhereWorkStands: "d"})
		return err
	},
	"give_up": func(f fixture) error {
		_, err := f.core.GiveUp(ctx, "acme", core.GiveUpInput{Ticket: "2", ClaimID: "x", Note: "n"})
		return err
	},
	"send_back": func(f fixture) error {
		_, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2"})
		return err
	},
	"to_draft": func(f fixture) error {
		_, err := f.core.ToDraft(human, "acme", core.TicketInput{Ticket: "2"})
		return err
	},
	"set_project_mode": func(f fixture) error {
		_, err := f.core.SetProjectMode(human, "acme", core.SetProjectModeInput{Mode: core.ModeLocal})
		return err
	},
	"post_round": func(f fixture) error {
		_, err := f.core.PostRound(ctx, "acme", core.PostRoundInput{Ticket: "2", ClaimID: "x", Questions: []core.QuestionInput{{Title: "q", Body: "b"}}})
		return err
	},
	"pick_up_answers": func(f fixture) error {
		_, err := f.core.PickUpAnswers(ctx, "acme", core.PickUpAnswersInput{Ticket: "2", ClaimID: "x"})
		return err
	},
	"answer": func(f fixture) error {
		_, err := f.core.Answer(human, "acme", core.AnswerInput{Ticket: "2", Question: 1, Text: "a"})
		return err
	},
	"accept_recommended": func(f fixture) error {
		_, err := f.core.AcceptRecommended(human, "acme", core.AcceptRecommendedInput{Ticket: "2", Round: 1})
		return err
	},
	"dismiss_question": func(f fixture) error {
		_, err := f.core.DismissQuestion(human, "acme", core.DismissQuestionInput{Ticket: "2", Question: 1})
		return err
	},
	"move": func(f fixture) error {
		_, err := f.core.Move(human, "acme", core.MoveInput{Ticket: "2", ToState: store.Ready})
		return err
	},
	"rename_project": func(f fixture) error {
		_, err := f.core.RenameProject(human, "acme", core.RenameProjectInput{DisplayName: "Acme"})
		return err
	},
	"set_landing_method": func(f fixture) error {
		_, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingBranch})
		return err
	},
	"set_ci_tracking": func(f fixture) error {
		_, err := f.core.SetCITracking(human, "acme", core.SetCITrackingInput{Tracking: true, Repository: "acme/web"})
		return err
	},
	"set_github_issues": func(f fixture) error {
		_, err := f.core.SetGitHubIssues(human, "acme", core.SetGitHubIssuesInput{On: true, Repository: "acme/web"})
		return err
	},
	"report_issue_watcher_status": func(f fixture) error {
		_, err := f.core.ReportIssueWatcherStatus(integration, "acme", core.ReportWatcherStatusInput{State: core.WatcherWatching})
		return err
	},
	"report_push_remote": func(f fixture) error {
		_, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/web.git"})
		return err
	},
	"add_ci_rule": func(f fixture) error {
		_, err := f.core.AddCIRule(human, "acme", core.CIRuleInput{Workflow: "ci.yml", Branch: "main"})
		return err
	},
	"edit_ci_rule": func(f fixture) error {
		_, err := f.core.EditCIRule(human, "acme", core.CIRuleInput{Spec: "1", Workflow: "ci.yml", Branch: "main"})
		return err
	},
	"remove_ci_rule": func(f fixture) error {
		_, err := f.core.RemoveCIRule(human, "acme", core.RemoveCIRuleInput{Spec: "1"})
		return err
	},
	"report_ci_watcher_status": func(f fixture) error {
		_, err := f.core.ReportCIWatcherStatus(ctx, "acme", core.ReportWatcherStatusInput{State: core.WatcherWatching})
		return err
	},
	"report_pushed_commit": func(f fixture) error {
		_, err := f.core.ReportPushedCommit(ctx, "acme", core.ReportPushedCommitInput{Ticket: "acme-2", Commit: "abc123"})
		return err
	},
	"land_anyway": func(f fixture) error {
		_, err := f.core.LandAnyway(human, "acme", core.TicketInput{Ticket: "acme-2"})
		return err
	},
	"rerun_ci": func(f fixture) error {
		_, err := f.core.RerunCI(human, "acme", core.TicketInput{Ticket: "acme-2"})
		return err
	},
	"new_fix_ticket": func(f fixture) error {
		_, err := f.core.NewFixTicket(human, "acme", core.NewFixTicketInput{Ticket: "acme-2", Note: "Fix CI."})
		return err
	},
	"report_ci_runs": func(f fixture) error {
		_, err := f.core.ReportCIRuns(ctx, "acme", core.ReportCIRunsInput{Runs: []core.CIRunView{{ID: 1, Attempt: 1, Workflow: "ci.yml", Commit: "abc123", Status: store.CIQueued, Ticket: "acme-2"}}})
		return err
	},
	"archive_project": func(f fixture) error {
		_, err := f.core.ArchiveProject(human, "acme")
		return err
	},
	"submit_requirement": func(f fixture) error {
		_, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: "Dark mode"})
		return err
	},
	"submit_prototype": func(f fixture) error {
		_, err := f.core.SubmitPrototype(ctx, "acme", core.SubmitPrototypeInput{Spec: "acme-1", Text: "Theme picker"})
		return err
	},
	"submit_map": func(f fixture) error {
		_, err := f.core.SubmitMap(ctx, "acme", core.SubmitMapInput{Text: "Offline mode"})
		return err
	},
	"land": func(f fixture) error {
		_, err := f.core.Land(ctx, "acme", core.LandInput{Ticket: "2", Commit: "abc123"})
		return err
	},
	"import": func(f fixture) error {
		_, err := f.core.Import(ctx, "acme", core.ImportInput{Payload: core.ImportPayload{
			Specs:   []core.ImportSpec{{Source: "feat", Title: "Feat"}},
			Tickets: []core.ImportTicket{{Source: "feat/01.md", Spec: "feat", Title: "One", State: store.Draft}},
		}})
		return err
	},
	// The Integration contract (ticket 42).
	"create_linked_spec": func(f fixture) error {
		_, err := f.core.CreateLinkedSpec(integration, "acme", core.CreateLinkedSpecInput{Title: "Ext", Link: core.LinkInput{Integration: "gh", ID: "1"}})
		return err
	},
	"create_linked_ticket": func(f fixture) error {
		_, err := f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "1", Title: "Ext", Link: core.LinkInput{Integration: "gh", ID: "2"}})
		return err
	},
	"update_linked_spec": func(f fixture) error {
		title := "New"
		_, err := f.core.UpdateLinkedSpec(integration, "acme", core.UpdateLinkedSpecInput{Spec: "1", Title: &title})
		return err
	},
	"update_linked_ticket": func(f fixture) error {
		title := "New"
		_, err := f.core.UpdateLinkedTicket(integration, "acme", core.UpdateLinkedTicketInput{Ticket: "2", Title: &title})
		return err
	},
	"set_external_state": func(f fixture) error {
		_, err := f.core.SetExternalState(integration, "acme", core.SetExternalStateInput{Target: "2", ExternalState: "closed"})
		return err
	},
	"unlink": func(f fixture) error {
		_, err := f.core.Unlink(human, "acme", core.UnlinkInput{Target: "2"})
		return err
	},
	"import (dry run)": func(f fixture) error {
		_, err := f.core.Import(ctx, "acme", core.ImportInput{DryRun: true, Payload: core.ImportPayload{
			Specs:   []core.ImportSpec{{Source: "feat", Title: "Feat"}},
			Tickets: []core.ImportTicket{{Source: "feat/01.md", Spec: "feat", Title: "One", State: store.Draft}},
		}})
		return err
	},
}

func TestAnArchivedProjectRefusesEveryWrite(t *testing.T) {
	// Writes that aren't on one existing Project can't be refused by one.
	notOnAProject := []string{"create_project", "unarchive_project", "set_options"}
	for _, op := range core.Operations {
		if _, ok := writesOnAcme[op.Name]; op.Write && !ok && !slices.Contains(notOnAProject, op.Name) {
			t.Errorf("write %s has no case in writesOnAcme", op.Name)
		}
	}
	for name, write := range writesOnAcme {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.finishedWork(t)
			f.archive(t, "acme")
			wantArchived(t, write(f))
			if p, _ := f.core.GetProject(ctx, "acme"); !p.Archived || p.DisplayName != "acme" || p.Mode != core.ModeCommitted {
				t.Fatalf("the refused %s changed the Project: %+v", name, p)
			}
		})
	}
}

func TestAnArchivedProjectStaysReadable(t *testing.T) {
	f := newFixture(t)
	f.finishedWork(t)
	f.project(t, "other")
	f.archive(t, "acme")
	if v, err := f.core.GetTicket(ctx, "acme", whole("acme-2")); err != nil || v.State != store.Done {
		t.Fatalf("get_ticket: %+v, %v", v, err)
	}
	if rows, err := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{State: []store.State{store.Done, store.Cancelled}}); err != nil || len(rows) != 2 {
		t.Fatalf("find_tickets: %+v, %v", rows, err)
	}
	if s, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "1"}); err != nil || s.ID != "acme-1" {
		t.Fatalf("get_spec: %+v, %v", s, err)
	}
	if _, err := f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{Finished: true}); err != nil {
		t.Fatalf("find_specs: %v", err)
	}
	rows, err := f.core.ListProjects(ctx)
	if err != nil || len(rows) != 2 || !rows[0].Archived || rows[1].Archived {
		t.Fatalf("list_projects: %+v, %v", rows, err)
	}
	// Other Projects take writes as before.
	f.spec(t, "other", "Fine")
}

func TestListProjectsCountsUnfinishedTickets(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Draft, store.Ready, store.Draft)
	f.move(t, "2", store.Done)
	rows, err := f.core.ListProjects(ctx)
	if err != nil || len(rows) != 1 || rows[0].Unfinished != 2 {
		t.Fatalf("list_projects: %+v, %v", rows, err)
	}
}
