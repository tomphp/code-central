// Package bind is the service's side of the operations ops defines (ADR
// 0063): it binds each Definition to the Core call that serves it, and
// serves the Core to the doors as an ops.Service. ops holds what every
// door and client knows of an operation and links no Core; only server
// composes the two, so a client that knows the operations links no Core.
package bind

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/core/factories"
	"github.com/tomoram/code-central/ops"
)

// A binding is one operation and the Core call that serves it, with its
// types erased.
type binding struct {
	op     ops.Operation
	invoke func(ctx context.Context, c *core.Core, key string, in any) (any, error)
}

// bind binds d to call, usually the Core method as a method expression,
// such as (*core.Core).CreateSpec. The call gets the input the door
// decoded, a *In, by value.
func bind[In, Out any](d *ops.Definition[In, Out], call func(c *core.Core, ctx context.Context, key string, in In) (Out, error)) binding {
	return binding{op: d, invoke: func(ctx context.Context, c *core.Core, key string, in any) (any, error) {
		return call(c, ctx, key, *in.(*In))
	}}
}

// bindFactories binds d to call, a method of the Factories area.
func bindFactories[In, Out any](d *ops.Definition[In, Out], call func(f *factories.Factories, ctx context.Context, key string, in In) (Out, error)) binding {
	return bind(d, func(c *core.Core, ctx context.Context, key string, in In) (Out, error) {
		return call(c.Factories(), ctx, key, in)
	})
}

// Bound is the operation each binding serves, in the list's order, a
// repeat included: the cross-door suite checks that every Definition is
// in it exactly once.
func Bound() []ops.Operation {
	out := make([]ops.Operation, len(all))
	for i, b := range all {
		out[i] = b.op
	}
	return out
}

// bindings are the bindings by the operation they serve.
var bindings = func() map[ops.Operation]binding {
	m := make(map[ops.Operation]binding, len(all))
	for _, b := range all {
		m[b.op] = b
	}
	return m
}()

// all is the list, in ops.All's order. Add a new operation's binding here
// once its Definition is in ops.
var all = []binding{
	bind(ops.CreateProject, func(c *core.Core, ctx context.Context, _ string, in api.CreateProjectInput) (api.ProjectView, error) {
		return c.CreateProject(ctx, in)
	}),
	bind(ops.GetProject, func(c *core.Core, ctx context.Context, key string, _ struct{}) (api.ProjectView, error) {
		return c.GetProject(ctx, key)
	}),
	bind(ops.CreateSpec, (*core.Core).CreateSpec),
	bind(ops.CreateTicket, (*core.Core).CreateTicket),
	bind(ops.GetTicket, (*core.Core).GetTicket),
	bind(ops.FindTickets, func(c *core.Core, ctx context.Context, key string, in api.FindTicketsInput) (any, error) {
		if !in.Full {
			if len(in.Parts) > 0 {
				return nil, &api.Error{Code: api.Invalid, Field: "part", Message: "part names the parts of full Tickets, so it needs full=true"}
			}
			return c.FindTickets(ctx, key, in)
		}
		return c.FindTicketViews(ctx, key, in)
	}),
	// Defining work.
	bind(ops.EditSpec, (*core.Core).EditSpec),
	bind(ops.EditTicket, (*core.Core).EditTicket),
	bind(ops.AddLabels, (*core.Core).AddLabels),
	bind(ops.RemoveLabels, (*core.Core).RemoveLabels),
	bind(ops.SetTags, (*core.Core).SetTags),
	bind(ops.RemoveTags, (*core.Core).RemoveTags),
	bind(ops.Comment, (*core.Core).Comment),
	bind(ops.Approve, (*core.Core).Approve),
	bind(ops.Defer, (*core.Core).Defer),

	// The delivery loop (ticket 25).
	bind(ops.Claim, (*core.Core).Claim),
	bind(ops.Finish, (*core.Core).Finish),
	bind(ops.Accept, (*core.Core).Accept),
	bind(ops.Implement, (*core.Core).Implement),
	bind(ops.Reject, (*core.Core).Reject),

	// Silent Claims and sessions (ticket 31).
	bind(ops.SetSession, (*core.Core).SetSession),
	bind(ops.Release, (*core.Core).Release),
	// Blockers, Replacements and finding work (ticket 28).
	bind(ops.AddBlocker, (*core.Core).AddBlocker),
	bind(ops.RemoveBlocker, (*core.Core).RemoveBlocker),
	bind(ops.Cancel, (*core.Core).Cancel),
	bind(ops.GetSpec, (*core.Core).GetSpec),
	bind(ops.FindSpecs, (*core.Core).FindSpecs),

	// Failure Reports and giving up (ticket 30).
	bind(ops.ReportFailure, (*core.Core).ReportFailure),
	bind(ops.GiveUp, (*core.Core).GiveUp),
	// The agent sends Accepted work back on a conflict (ticket 72).
	bind(ops.SendBack, (*core.Core).SendBack),
	bind(ops.ToDraft, (*core.Core).ToDraft),

	// init's teaching files and modes (ticket 41).
	bind(ops.SetProjectMode, (*core.Core).SetProjectMode),

	// The published workflow (ticket 26).
	bind(ops.GetWorkflow, func(c *core.Core, ctx context.Context, _ string, _ struct{}) (api.WorkflowView, error) {
		return c.GetWorkflow(ctx)
	}),

	// Rounds and Questions (ticket 29).
	bind(ops.PostRound, (*core.Core).PostRound),
	bind(ops.PickUpAnswers, (*core.Core).PickUpAnswers),
	bind(ops.Answer, (*core.Core).Answer),
	bind(ops.AcceptRecommended, (*core.Core).AcceptRecommended),
	bind(ops.DismissQuestion, (*core.Core).DismissQuestion),
	// The roll-up of a Spec's Questions is HTTP-only, for the UI.
	bind(ops.GetSpecQuestions, func(c *core.Core, ctx context.Context, key string, in api.SpecInput) (api.SpecQuestions, error) {
		return c.GetSpecQuestions(ctx, key, in.Spec)
	}),

	// Needs Me, moving by hand and Projects (ticket 32).
	bind(ops.NeedsMe, func(c *core.Core, ctx context.Context, _ string, _ struct{}) (api.NeedsMeView, error) {
		return c.NeedsMe(ctx)
	}),
	bind(ops.ListProjects, func(c *core.Core, ctx context.Context, _ string, _ struct{}) ([]api.ProjectRow, error) {
		return c.ListProjects(ctx)
	}),
	bind(ops.Move, (*core.Core).Move),
	bind(ops.RenameProject, (*core.Core).RenameProject),
	// The Landing Method (code-central-173).
	bind(ops.SetLandingMethod, (*core.Core).SetLandingMethod),
	// Subagent Exploration (code-central-441).
	bind(ops.SetSubagentExploration, (*core.Core).SetSubagentExploration),
	bind(ops.SetModelAssignment, (*core.Core).SetModelAssignment),
	// Auto-accept (code-central-536): the human's alone, over HTTP only.
	bind(ops.SetAutoAccept, (*core.Core).SetAutoAccept),
	// Focus Kinds (Spec code-central-550).
	bind(ops.SetFocusKinds, (*core.Core).SetFocusKinds),
	// The Check Command (Spec code-central-1229): the human's alone, over
	// HTTP only.
	bind(ops.SetCheckCommand, (*core.Core).SetCheckCommand),
	// Repositories (Spec code-central-848): the human's alone, over HTTP
	// and the CLI's repository command, never MCP.
	bind(ops.AddRepository, (*core.Core).AddRepository),
	bind(ops.EditRepository, (*core.Core).EditRepository),
	bind(ops.RemoveRepository, (*core.Core).RemoveRepository),
	bind(ops.SetLandingOrder, (*core.Core).SetLandingOrder),

	// The Integration contract (ticket 42).
	bind(ops.CreateLinkedSpec, (*core.Core).CreateLinkedSpec),
	bind(ops.CreateLinkedTicket, (*core.Core).CreateLinkedTicket),
	bind(ops.UpdateLinkedSpec, (*core.Core).UpdateLinkedSpec),
	bind(ops.UpdateLinkedTicket, (*core.Core).UpdateLinkedTicket),
	bind(ops.SetExternalState, (*core.Core).SetExternalState),
	bind(ops.FindLinked, (*core.Core).FindLinked),
	bind(ops.Unlink, (*core.Core).Unlink),

	// Scratch Import (ticket 40).
	bind(ops.GetImportGuide, func(c *core.Core, ctx context.Context, key string, _ struct{}) (api.ImportGuideView, error) {
		return c.GetImportGuide(ctx, key)
	}),
	bind(ops.Import, (*core.Core).Import),

	// Finished work and Archived Projects (ticket 39).
	bind(ops.ArchiveProject, func(c *core.Core, ctx context.Context, key string, _ struct{}) (api.ProjectView, error) {
		return c.ArchiveProject(ctx, key)
	}),
	bind(ops.UnarchiveProject, func(c *core.Core, ctx context.Context, key string, _ struct{}) (api.ProjectView, error) {
		return c.UnarchiveProject(ctx, key)
	}),

	// Requirements and the local Factory (Spec 66).
	bind(ops.SubmitRequirement, (*core.Core).SubmitRequirement),
	bind(ops.Land, (*core.Core).Land),
	bind(ops.GetStageGuide, (*core.Core).GetStageGuide),

	// Maps and Prototypes (Spec code-central-213).
	bind(ops.SubmitMap, (*core.Core).SubmitMap),
	bind(ops.SubmitPrototype, (*core.Core).SubmitPrototype),
	bind(ops.SubmitSmallChange, (*core.Core).SubmitSmallChange),
	// New Ticket (Spec code-central-1067).
	bind(ops.NewTicket, (*core.Core).NewTicket),
	// Retros (Spec code-central-646).
	bind(ops.SubmitRetro, (*core.Core).SubmitRetro),

	// Options, the human's global preferences (Spec code-central-160).
	// HTTP only: no MCP tool and no CLI command.
	bind(ops.GetOptions, func(c *core.Core, ctx context.Context, _ string, _ struct{}) (api.OptionsView, error) {
		return c.GetOptions(ctx)
	}),
	bind(ops.SetOptions, func(c *core.Core, ctx context.Context, _ string, in api.SetOptionsInput) (api.OptionsView, error) {
		return c.SetOptions(ctx, in)
	}),

	// The Storage tab (Spec code-central-1032). HTTP only.
	bind(ops.GetStorage, func(c *core.Core, ctx context.Context, _ string, in api.GetStorageInput) (api.StorageView, error) {
		return c.GetStorage(ctx, in)
	}),

	// CI Tracking (Spec code-central-247).
	bind(ops.SetCITracking, (*core.Core).SetCITracking),
	bind(ops.SetAutoFix, (*core.Core).SetAutoFix),
	// A Repository's CI entry (Spec code-central-848).
	bind(ops.SetRepositoryCI, (*core.Core).SetRepositoryCI),
	bind(ops.ReportPushRemote, (*core.Core).ReportPushRemote),
	bind(ops.AddCIRule, (*core.Core).AddCIRule),
	bind(ops.EditCIRule, (*core.Core).EditCIRule),
	bind(ops.RemoveCIRule, (*core.Core).RemoveCIRule),
	// The CI Watcher's own (code-central-253).
	bind(ops.GetCIWatch, func(c *core.Core, ctx context.Context, key string, _ ops.WatchRead) (api.CIWatch, error) {
		return c.GetCIWatch(ctx, key)
	}),
	bind(ops.ReportCIWatcherStatus, (*core.Core).ReportCIWatcherStatus),
	// Tracking a pushed Ticket's CI Runs (code-central-256).
	bind(ops.ReportPushedCommit, (*core.Core).ReportPushedCommit),
	// The Pull Request Watcher's own (code-central-388).
	bind(ops.GetPullRequestWatch, func(c *core.Core, ctx context.Context, key string, _ ops.WatchRead) (api.PullRequestWatch, error) {
		return c.GetPullRequestWatch(ctx, key)
	}),
	bind(ops.ReportPullRequest, (*core.Core).ReportPullRequest),
	bind(ops.ReportPullRequestWatcherStatus, (*core.Core).ReportPullRequestWatcherStatus),
	bind(ops.ReportCIRuns, (*core.Core).ReportCIRuns),
	// Land per Spec (code-central-371).
	bind(ops.ReportSpecMerge, (*core.Core).ReportSpecMerge),
	// Spec merge Tickets (code-central-542).
	bind(ops.OpenSpecMerge, (*core.Core).OpenSpecMerge),
	// CI failed and its failure options (code-central-261).
	bind(ops.LandAnyway, (*core.Core).LandAnyway),
	// Re-run failed jobs (code-central-265).
	bind(ops.RerunCI, (*core.Core).RerunCI),
	// Send back to fix and New fix Ticket (code-central-264).
	bind(ops.NewFixTicket, (*core.Core).NewFixTicket),

	// GitHub Issues (Spec code-central-279).
	bind(ops.SetGitHubIssues, (*core.Core).SetGitHubIssues),
	// The Issue Watcher's own (code-central-285).
	bind(ops.GetIssueWatch, func(c *core.Core, ctx context.Context, key string, _ api.WatchInput) (api.IssueWatch, error) {
		return c.GetIssueWatch(ctx, key)
	}),
	bind(ops.ReportIssueWatcherStatus, (*core.Core).ReportIssueWatcherStatus),
	// New issue and the triage actions (code-central-287).
	bind(ops.KeepAsDraft, (*core.Core).KeepAsDraft),
	// Write-backs and Won't fix (code-central-289).
	bind(ops.ReportIssueWritten, (*core.Core).ReportIssueWritten),
	bind(ops.WontFix, (*core.Core).WontFix),
	// To Requirement and the outbound Link (code-central-290).
	bind(ops.ToRequirement, (*core.Core).ToRequirement),
	// Azure Boards (Spec code-central-894).
	bind(ops.SetAzureBoards, (*core.Core).SetAzureBoards),
	// The Work Item Watcher's own; it reports each comment written with
	// report_issue_written.
	bind(ops.GetWorkItemWatch, func(c *core.Core, ctx context.Context, key string, _ api.WatchInput) (api.WorkItemWatch, error) {
		return c.GetWorkItemWatch(ctx, key)
	}),
	bind(ops.ReportWorkItemWatcherStatus, (*core.Core).ReportWorkItemWatcherStatus),
	// The Model Choice (Spec code-central-301).
	bind(ops.GetModelChoice, (*core.Core).GetModelChoice),
	bind(ops.ReportModelChoice, (*core.Core).ReportModelChoice),
	// The Models section of Project Options (Spec code-central-301).
	bind(ops.SetModels, (*core.Core).SetModels),
	// The Reported Models (code-central-311).
	bind(ops.ReportModels, (*core.Core).ReportModels),
	bind(ops.ReportHeartbeat, (*core.Core).ReportHeartbeat),
	// The Factory's base under Pull Request (code-central-394).
	bind(ops.ReportBase, (*core.Core).ReportBase),
	// The Factory's clones of the Repositories (Spec code-central-848).
	bind(ops.ReportRepositories, (*core.Core).ReportRepositories),
	// A Ticket's Changed Repositories, live, and the Factory's finish
	// check (Spec code-central-848).
	bind(ops.ReportChangedRepositories, (*core.Core).ReportChangedRepositories),
	bind(ops.FailFinish, (*core.Core).FailFinish),

	// Chat (Spec code-central-633): the human's Conversation and the
	// Factory's long-poll for its turns. Chat writes change only memory.
	bindFactories(ops.GetChat, (*factories.Factories).GetChat),
	bindFactories(ops.StartConversation, (*factories.Factories).StartConversation),
	bindFactories(ops.SendChatMessage, (*factories.Factories).SendChatMessage),
	bindFactories(ops.StopChatReply, (*factories.Factories).StopChatReply),
	bindFactories(ops.NextChatWork, (*factories.Factories).NextChatWork),

	// The Factory's wake (Spec code-central-743): a long-poll that ends
	// when a write its poll acts on reaches the Project.
	bindFactories(ops.NextFactoryWake, (*factories.Factories).NextFactoryWake),
}
