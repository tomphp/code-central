package core_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/ops"
)

// finishedWork is Project acme with Spec acme-1, Done Ticket acme-2 and
// Cancelled Ticket acme-3: nothing left to do.
func (f fixture) finishedWork(t *testing.T) {
	t.Helper()
	f.work(t, domain.Draft, domain.Draft)
	f.move(t, "2", domain.Done)
	f.move(t, "3", domain.Cancelled)
}

func (f fixture) archive(t *testing.T, key string) api.ProjectView {
	t.Helper()
	p, err := f.core.ArchiveProject(human, key)
	if err != nil {
		t.Fatalf("archive_project: %v", err)
	}
	return p
}

func TestArchiveIsRefusedWhileAnyTicketIsUnfinished(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Draft, domain.Ready, domain.Draft)
	f.move(t, "2", domain.Done)
	_, err := f.core.ArchiveProject(human, "acme")
	wantCode(t, err, api.ActionNotAllowed, "")
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
	for _, a := range []api.Actor{api.Agent, api.Integration} {
		_, err := f.core.ArchiveProject(api.WithActor(ctx, a), "acme")
		wantCode(t, err, api.ActionNotAllowed, "")
	}
	_, err := f.core.ArchiveProject(human, "nope")
	wantCode(t, err, api.UnknownProject, "")
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
	wantCode(t, err, api.ActionNotAllowed, "") // the agent can't bring it back
	p, err = f.core.UnarchiveProject(human, "acme")
	if err != nil || p.Archived || p.ArchivedAt != nil {
		t.Fatalf("unarchive_project: %+v, %v", p, err)
	}
	// Unarchiving a Project that isn't archived changes nothing.
	if p, err = f.core.UnarchiveProject(human, "acme"); err != nil || p.Archived {
		t.Fatalf("unarchive twice: %+v, %v", p, err)
	}
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "More"}) // it takes work again
}

func wantArchived(t *testing.T, err error) {
	t.Helper()
	var e *api.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want the Archived Project refusal", err)
	}
	if e.Code != api.ActionNotAllowed || !slices.Equal(e.AllowedActions, []string{"unarchive_project"}) ||
		!strings.Contains(e.Message, "archived") {
		t.Fatalf("got %+v, want action_not_allowed listing unarchive_project", e)
	}
}

// writesOnAcme makes every write operation on Project acme once, with
// input that would succeed (or fail only on State) were acme not
// archived. Keyed by operation name.
var writesOnAcme = map[string]func(f fixture) error{
	"create_spec": func(f fixture) error {
		_, err := f.core.CreateSpec(ctx, "acme", api.CreateSpecInput{Title: "More", Body: "b"})
		return err
	},
	"create_ticket": func(f fixture) error {
		_, err := f.core.CreateTicket(ctx, "acme", api.CreateTicketInput{Spec: "1", Title: "More"})
		return err
	},
	"edit_spec": func(f fixture) error {
		title := "New"
		_, err := f.core.EditSpec(ctx, "acme", api.EditSpecInput{Spec: "1", Title: &title})
		return err
	},
	"edit_ticket": func(f fixture) error {
		title := "New"
		_, err := f.core.EditTicket(ctx, "acme", api.EditTicketInput{Ticket: "2", Title: &title})
		return err
	},
	"add_labels": func(f fixture) error {
		_, err := f.core.AddLabels(ctx, "acme", api.LabelsInput{Target: "2", Labels: []string{"ui"}})
		return err
	},
	"remove_labels": func(f fixture) error {
		_, err := f.core.RemoveLabels(ctx, "acme", api.LabelsInput{Target: "2", Labels: []string{"ui"}})
		return err
	},
	"set_tags": func(f fixture) error {
		_, err := f.core.SetTags(ctx, "acme", api.SetTagsInput{Target: "1", Tags: map[string]string{"k": "v"}})
		return err
	},
	"remove_tags": func(f fixture) error {
		_, err := f.core.RemoveTags(ctx, "acme", api.RemoveTagsInput{Target: "1", Keys: []string{"k"}})
		return err
	},
	"comment": func(f fixture) error {
		_, err := f.core.Comment(ctx, "acme", api.CommentInput{Ticket: "2", Body: "hi"})
		return err
	},
	"approve": func(f fixture) error {
		_, err := f.core.Approve(ctx, "acme", api.TicketInput{Ticket: "2"})
		return err
	},
	"defer": func(f fixture) error {
		_, err := f.core.Defer(ctx, "acme", api.DeferInput{Ticket: "2", Reason: "later"})
		return err
	},
	"claim": func(f fixture) error {
		_, err := f.core.Claim(ctx, "acme", api.ClaimInput{Ticket: "2"})
		return err
	},
	"finish": func(f fixture) error {
		_, err := f.core.Finish(ctx, "acme", api.FinishInput{Ticket: "2", ClaimID: "x", Summary: "s"})
		return err
	},
	"accept": func(f fixture) error {
		_, err := f.core.Accept(human, "acme", api.TicketInput{Ticket: "2"})
		return err
	},
	"implement": func(f fixture) error {
		_, err := f.core.Implement(human, "acme", api.TicketInput{Ticket: "2"})
		return err
	},
	"reject": func(f fixture) error {
		_, err := f.core.Reject(human, "acme", api.RejectInput{Ticket: "2", Comment: "no"})
		return err
	},
	"set_session": func(f fixture) error {
		_, err := f.core.SetSession(ctx, "acme", api.SetSessionInput{Ticket: "2", ClaimID: "x", SessionID: "s"})
		return err
	},
	"release": func(f fixture) error {
		_, err := f.core.Release(human, "acme", api.ReleaseInput{Ticket: "2"})
		return err
	},
	"stop": func(f fixture) error {
		_, err := f.core.Stop(human, "acme", api.StopInput{Ticket: "2"})
		return err
	},
	"resume": func(f fixture) error {
		_, err := f.core.Resume(human, "acme", api.ResumeInput{Ticket: "2"})
		return err
	},
	"add_blocker": func(f fixture) error {
		_, err := f.core.AddBlocker(ctx, "acme", api.BlockerInput{Ticket: "3", Blocker: "2"})
		return err
	},
	"remove_blocker": func(f fixture) error {
		_, err := f.core.RemoveBlocker(ctx, "acme", api.BlockerInput{Ticket: "3", Blocker: "2"})
		return err
	},
	"cancel": func(f fixture) error {
		_, err := f.core.Cancel(human, "acme", api.CancelInput{Ticket: "2"})
		return err
	},
	"report_failure": func(f fixture) error {
		_, err := f.core.ReportFailure(ctx, "acme", api.ReportFailureInput{Ticket: "2", ClaimID: "x", WhatBroke: "a", Why: "b", WhatToFix: "c", WhereWorkStands: "d"})
		return err
	},
	"give_up": func(f fixture) error {
		_, err := f.core.GiveUp(ctx, "acme", api.GiveUpInput{Ticket: "2", ClaimID: "x", Note: "n"})
		return err
	},
	"send_back": func(f fixture) error {
		_, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "2"})
		return err
	},
	"to_draft": func(f fixture) error {
		_, err := f.core.ToDraft(human, "acme", api.TicketInput{Ticket: "2"})
		return err
	},
	"set_project_mode": func(f fixture) error {
		_, err := f.core.SetProjectMode(human, "acme", api.SetProjectModeInput{Mode: domain.ModeLocal})
		return err
	},
	"post_round": func(f fixture) error {
		_, err := f.core.PostRound(ctx, "acme", api.PostRoundInput{Ticket: "2", ClaimID: "x", Questions: []api.QuestionInput{{Title: "q", Body: "b"}}})
		return err
	},
	"pick_up_answers": func(f fixture) error {
		_, err := f.core.PickUpAnswers(ctx, "acme", api.PickUpAnswersInput{Ticket: "2", ClaimID: "x"})
		return err
	},
	"answer": func(f fixture) error {
		_, err := f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "a"})
		return err
	},
	"accept_recommended": func(f fixture) error {
		_, err := f.core.AcceptRecommended(human, "acme", api.AcceptRecommendedInput{Ticket: "2", Round: 1})
		return err
	},
	"dismiss_question": func(f fixture) error {
		_, err := f.core.DismissQuestion(human, "acme", api.DismissQuestionInput{Ticket: "2", Question: 1})
		return err
	},
	"move": func(f fixture) error {
		_, err := f.core.Move(human, "acme", api.MoveInput{Ticket: "2", ToState: domain.Ready})
		return err
	},
	"rename_project": func(f fixture) error {
		_, err := f.core.RenameProject(human, "acme", api.RenameProjectInput{DisplayName: "Acme"})
		return err
	},
	"set_landing_method": func(f fixture) error {
		_, err := f.core.SetLandingMethod(human, "acme", api.SetLandingMethodInput{Method: domain.LandingBranch})
		return err
	},
	"set_ci_tracking": func(f fixture) error {
		_, err := f.core.SetCITracking(human, "acme", api.SetCITrackingInput{Tracking: true, Repository: "acme/web"})
		return err
	},
	"set_auto_fix": func(f fixture) error {
		_, err := f.core.SetAutoFix(human, "acme", api.SetAutoFixInput{FixInstructions: "Fix it.", Limit: 2})
		return err
	},
	"set_repository_ci": func(f fixture) error {
		_, err := f.core.SetRepositoryCI(human, "acme", api.SetRepositoryCIInput{Name: "web", RepositoryCI: api.RepositoryCI{NotWatched: true}})
		return err
	},
	"set_models": func(f fixture) error {
		_, err := f.core.SetModels(human, "acme", api.SetModelsInput{Default: api.ModelRowView{Model: "opus"}})
		return err
	},
	"set_model_assignment": func(f fixture) error {
		_, err := f.core.SetModelAssignment(human, "acme", api.SetModelAssignmentInput{On: true})
		return err
	},
	"set_subagent_exploration": func(f fixture) error {
		_, err := f.core.SetSubagentExploration(human, "acme", api.SetSubagentExplorationInput{On: false})
		return err
	},
	"set_check_command": func(f fixture) error {
		_, err := f.core.SetCheckCommand(human, "acme", api.SetCheckCommandInput{CheckCommand: "make check"})
		return err
	},
	"set_auto_accept": func(f fixture) error {
		_, err := f.core.SetAutoAccept(human, "acme", api.SetAutoAcceptInput{Kinds: map[domain.AutoAcceptKind]bool{domain.AutoAcceptImplementation: true}})
		return err
	},
	"set_focus_kinds": func(f fixture) error {
		_, err := f.core.SetFocusKinds(human, "acme", api.SetFocusKindsInput{Off: []api.NeedsMeReason{api.ReasonAnswered}})
		return err
	},
	"add_repository": func(f fixture) error {
		_, err := f.core.AddRepository(human, "acme", api.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
		return err
	},
	"edit_repository": func(f fixture) error {
		base := "develop"
		_, err := f.core.EditRepository(human, "acme", api.EditRepositoryInput{Name: "web", Base: &base})
		return err
	},
	"remove_repository": func(f fixture) error {
		_, err := f.core.RemoveRepository(human, "acme", api.RemoveRepositoryInput{Name: "web"})
		return err
	},
	"set_landing_order": func(f fixture) error {
		_, err := f.core.SetLandingOrder(human, "acme", api.SetLandingOrderInput{Names: []string{"acme"}})
		return err
	},
	"report_models": func(f fixture) error {
		_, err := f.core.ReportModels(ctx, "acme", api.ReportModelsInput{EffortLevels: []string{"low"}})
		return err
	},
	"report_heartbeat": func(f fixture) error {
		_, err := f.core.ReportHeartbeat(ctx, "acme", api.ReportHeartbeatInput{Factory: "f", Host: "h", State: api.HeartbeatOnline, IntervalSeconds: 10})
		return err
	},
	"report_base": func(f fixture) error {
		_, err := f.core.ReportBase(ctx, "acme", api.ReportBaseInput{State: api.BaseCurrent, Local: "a", Remote: "a"})
		return err
	},
	"report_repositories": func(f fixture) error {
		_, err := f.core.ReportRepositories(ctx, "acme", api.ReportRepositoriesInput{Repositories: []api.RepositoryReport{{Name: "acme", Path: "/src/acme", State: api.CloneReady}}})
		return err
	},
	"report_changed_repositories": func(f fixture) error {
		_, err := f.core.ReportChangedRepositories(ctx, "acme", api.ReportChangedRepositoriesInput{Ticket: "acme-2", ClaimID: "claim-1", Repositories: []api.RepositoryWork{{Name: "acme", Branch: "acme-2", Commit: "abc"}}})
		return err
	},
	"fail_finish": func(f fixture) error {
		_, err := f.core.FailFinish(ctx, "acme", api.FailFinishInput{Ticket: "acme-2", WhatBroke: "a", Why: "b", WhatToFix: "c", WhereWorkStands: "d"})
		return err
	},
	"start_conversation": func(f fixture) error {
		_, err := f.core.Factories().StartConversation(human, "acme", api.StartConversationInput{})
		return err
	},
	"send_chat_message": func(f fixture) error {
		_, err := f.core.Factories().SendChatMessage(human, "acme", api.SendChatMessageInput{Text: "What is acme?"})
		return err
	},
	"stop_chat_reply": func(f fixture) error {
		_, err := f.core.Factories().StopChatReply(human, "acme", api.StopChatReplyInput{})
		return err
	},
	"set_github_issues": func(f fixture) error {
		_, err := f.core.SetGitHubIssues(human, "acme", api.SetGitHubIssuesInput{On: true, Repositories: []string{"acme/web"}})
		return err
	},
	"set_azure_boards": func(f fixture) error {
		_, err := f.core.SetAzureBoards(human, "acme", api.SetAzureBoardsInput{On: true, QueryURL: "https://dev.azure.com/contoso/web/_queries/query/6b4a0c1e-9f1d-4a51-8f43-2a7d3c1b9e10"})
		return err
	},
	"report_issue_watcher_status": func(f fixture) error {
		_, err := f.core.ReportIssueWatcherStatus(integration, "acme", api.ReportWatcherStatusInput{State: api.WatcherWatching})
		return err
	},
	"report_work_item_watcher_status": func(f fixture) error {
		_, err := f.core.ReportWorkItemWatcherStatus(integration, "acme", api.ReportWatcherStatusInput{State: api.WatcherWatching})
		return err
	},
	"report_push_remote": func(f fixture) error {
		_, err := f.core.ReportPushRemote(ctx, "acme", api.ReportPushRemoteInput{URL: "git@github.com:acme/web.git"})
		return err
	},
	"add_ci_rule": func(f fixture) error {
		_, err := f.core.AddCIRule(human, "acme", api.CIRuleInput{Workflow: "ci.yml", Branch: "main"})
		return err
	},
	"edit_ci_rule": func(f fixture) error {
		_, err := f.core.EditCIRule(human, "acme", api.CIRuleInput{Spec: "1", Workflow: "ci.yml", Branch: "main"})
		return err
	},
	"remove_ci_rule": func(f fixture) error {
		_, err := f.core.RemoveCIRule(human, "acme", api.RemoveCIRuleInput{Spec: "1"})
		return err
	},
	"report_ci_watcher_status": func(f fixture) error {
		_, err := f.core.ReportCIWatcherStatus(ctx, "acme", api.ReportWatcherStatusInput{State: api.WatcherWatching})
		return err
	},
	"report_pull_request_watcher_status": func(f fixture) error {
		_, err := f.core.ReportPullRequestWatcherStatus(ctx, "acme", api.ReportWatcherStatusInput{State: api.WatcherWatching})
		return err
	},
	"report_pushed_commit": func(f fixture) error {
		_, err := f.core.ReportPushedCommit(ctx, "acme", api.ReportPushedCommitInput{Ticket: "acme-2", Commit: "abc123"})
		return err
	},
	"report_pull_request": func(f fixture) error {
		_, err := f.core.ReportPullRequest(ctx, "acme", api.ReportPullRequestInput{Branch: "acme-2", Number: 1, Link: "https://github.com/a/b/pull/1", State: domain.PROpen})
		return err
	},
	"report_spec_merge": func(f fixture) error {
		_, err := f.core.ReportSpecMerge(ctx, "acme", api.ReportSpecMergeInput{Ticket: "acme-2", Commit: "abc123"})
		return err
	},
	"open_spec_merge": func(f fixture) error {
		_, err := f.core.OpenSpecMerge(ctx, "acme", api.OpenSpecMergeInput{Spec: "acme-1", Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222"})
		return err
	},
	"report_model_choice": func(f fixture) error {
		_, err := f.core.ReportModelChoice(ctx, "acme", api.ReportModelChoiceInput{Ticket: "acme-2", ClaimID: "claim-1", Model: "opus"})
		return err
	},
	"keep_as_draft": func(f fixture) error {
		_, err := f.core.KeepAsDraft(human, "acme", api.TicketInput{Ticket: "acme-2"})
		return err
	},
	"land_anyway": func(f fixture) error {
		_, err := f.core.LandAnyway(human, "acme", api.CIResponseInput{Ticket: "acme-2"})
		return err
	},
	"rerun_ci": func(f fixture) error {
		_, err := f.core.RerunCI(human, "acme", api.CIResponseInput{Ticket: "acme-2"})
		return err
	},
	"new_fix_ticket": func(f fixture) error {
		_, err := f.core.NewFixTicket(human, "acme", api.NewFixTicketInput{Ticket: "acme-2", Note: "Fix CI."})
		return err
	},
	"report_ci_runs": func(f fixture) error {
		_, err := f.core.ReportCIRuns(ctx, "acme", api.ReportCIRunsInput{Runs: []api.CIRunView{{ID: 1, Attempt: 1, Workflow: "ci.yml", Commit: "abc123", Status: domain.CIQueued, Ticket: "acme-2"}}})
		return err
	},
	"archive_project": func(f fixture) error {
		_, err := f.core.ArchiveProject(human, "acme")
		return err
	},
	"submit_requirement": func(f fixture) error {
		_, err := f.core.SubmitRequirement(ctx, "acme", api.SubmitRequirementInput{Text: "Dark mode"})
		return err
	},
	"submit_prototype": func(f fixture) error {
		_, err := f.core.SubmitPrototype(ctx, "acme", api.SubmitPrototypeInput{Spec: "acme-1", Text: "Theme picker"})
		return err
	},
	"submit_small_change": func(f fixture) error {
		_, err := f.core.SubmitSmallChange(ctx, "acme", api.SubmitSmallChangeInput{Text: "Fix the typo"})
		return err
	},
	"new_ticket": func(f fixture) error {
		_, err := f.core.NewTicket(human, "acme", api.NewTicketInput{Spec: "acme-1", Text: "Rename the toggle"})
		return err
	},
	"submit_retro": func(f fixture) error {
		_, err := f.core.SubmitRetro(ctx, "acme", api.SubmitRetroInput{Target: "acme-2"})
		return err
	},
	"submit_map": func(f fixture) error {
		_, err := f.core.SubmitMap(ctx, "acme", api.SubmitMapInput{Text: "Offline mode"})
		return err
	},
	"land": func(f fixture) error {
		_, err := f.core.Land(ctx, "acme", api.LandInput{Ticket: "2", Commit: "abc123"})
		return err
	},
	"import": func(f fixture) error {
		_, err := f.core.Import(ctx, "acme", api.ImportInput{Payload: api.ImportPayload{
			Specs:   []api.ImportSpec{{Source: "feat", Title: "Feat"}},
			Tickets: []api.ImportTicket{{Source: "feat/01.md", Spec: "feat", Title: "One", State: domain.Draft}},
		}})
		return err
	},
	// The Integration contract (ticket 42).
	"create_linked_spec": func(f fixture) error {
		_, err := f.core.CreateLinkedSpec(integration, "acme", api.CreateLinkedSpecInput{Title: "Ext", Link: api.LinkInput{Integration: "gh", ID: "1"}})
		return err
	},
	"create_linked_ticket": func(f fixture) error {
		_, err := f.core.CreateLinkedTicket(integration, "acme", api.CreateLinkedTicketInput{Spec: "1", Title: "Ext", Link: api.LinkInput{Integration: "gh", ID: "2"}})
		return err
	},
	"update_linked_spec": func(f fixture) error {
		title := "New"
		_, err := f.core.UpdateLinkedSpec(integration, "acme", api.UpdateLinkedSpecInput{Spec: "1", Title: &title})
		return err
	},
	"update_linked_ticket": func(f fixture) error {
		title := "New"
		_, err := f.core.UpdateLinkedTicket(integration, "acme", api.UpdateLinkedTicketInput{Ticket: "2", Title: &title})
		return err
	},
	"set_external_state": func(f fixture) error {
		_, err := f.core.SetExternalState(integration, "acme", api.SetExternalStateInput{Target: "2", ExternalState: "closed"})
		return err
	},
	"report_issue_written": func(f fixture) error {
		_, err := f.core.ReportIssueWritten(integration, "acme", api.ReportIssueWrittenInput{Target: "2", Key: "wont-fix"})
		return err
	},
	"to_requirement": func(f fixture) error {
		_, err := f.core.ToRequirement(human, "acme", api.ToRequirementInput{Ticket: "acme-2"})
		return err
	},
	"wont_fix": func(f fixture) error {
		_, err := f.core.WontFix(human, "acme", api.WontFixInput{Ticket: "acme-2"})
		return err
	},
	"unlink": func(f fixture) error {
		_, err := f.core.Unlink(human, "acme", api.UnlinkInput{Target: "2"})
		return err
	},
	"import (dry run)": func(f fixture) error {
		_, err := f.core.Import(ctx, "acme", api.ImportInput{DryRun: true, Payload: api.ImportPayload{
			Specs:   []api.ImportSpec{{Source: "feat", Title: "Feat"}},
			Tickets: []api.ImportTicket{{Source: "feat/01.md", Spec: "feat", Title: "One", State: domain.Draft}},
		}})
		return err
	},
}

func TestAnArchivedProjectRefusesEveryWrite(t *testing.T) {
	// Writes that aren't on one existing Project can't be refused by one.
	notOnAProject := []string{"create_project", "unarchive_project", "set_options"}
	for _, op := range ops.All() {
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
			if p, _ := f.core.GetProject(ctx, "acme"); !p.Archived || p.DisplayName != "acme" || p.Mode != domain.ModeCommitted {
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
	if v, err := f.core.GetTicket(ctx, "acme", whole("acme-2")); err != nil || v.State != domain.Done {
		t.Fatalf("get_ticket: %+v, %v", v, err)
	}
	if rows, err := f.core.FindTickets(ctx, "acme", api.FindTicketsInput{State: []domain.State{domain.Done, domain.Cancelled}}); err != nil || len(rows) != 2 {
		t.Fatalf("find_tickets: %+v, %v", rows, err)
	}
	if s, err := f.core.GetSpec(ctx, "acme", api.GetSpecInput{Spec: "1"}); err != nil || s.ID != "acme-1" {
		t.Fatalf("get_spec: %+v, %v", s, err)
	}
	if _, err := f.core.FindSpecs(ctx, "acme", api.FindSpecsInput{Finished: true}); err != nil {
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
	f.work(t, domain.Draft, domain.Ready, domain.Draft)
	f.move(t, "2", domain.Done)
	rows, err := f.core.ListProjects(ctx)
	if err != nil || len(rows) != 1 || rows[0].Unfinished != 2 {
		t.Fatalf("list_projects: %+v, %v", rows, err)
	}
}
