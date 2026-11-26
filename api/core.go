package api

import (
	"time"

	"github.com/tomoram/code-central/domain"
)

// LandingView is a Project's Landing Method with its Push option and the
// remote it uses.
type LandingView struct {
	Method domain.LandingMethod `json:"method"`
	Push   bool                 `json:"push"`
	Remote string               `json:"remote"`
	// LandPerSpec is the Merge option to land a Spec's work together,
	// shown whatever the method.
	LandPerSpec bool `json:"land_per_spec"`
	// Rebase is the Merge and Pull Request option to replay a Ticket's
	// commits onto its target instead of merging its branch, shown
	// whatever the method.
	Rebase bool `json:"rebase"`
	// CodeHost is the Code Host the reported Push remote URL names,
	// github or azure_repos, empty while none is reported or it is
	// unsupported. Repository is the host's name for the repository.
	CodeHost   CodeHost `json:"code_host"`
	Repository string   `json:"repository"`
	// PATEnv is the environment variable holding the Azure Repos PAT,
	// ADO_PAT unless the Project names another.
	PATEnv string `json:"pat_env"`
	// UsesAzureReposPAT is whether the PAT variable is used: any Azure
	// Repos Repository is under Pull Request.
	UsesAzureReposPAT bool `json:"uses_azure_repos_pat"`
	// Base is the branch the Project's work is cut from and lands on: the
	// stored one, else the reported fallback, else main.
	Base string `json:"base"`
	// Methods is the Landing capabilities of each Landing Method, as if
	// it were the Default Landing: what the Options form offers while the
	// human drafts it.
	Methods LandingMethodsCapabilities `json:"methods"`
	// RunsPullRequestWatcher is whether the Pull Request Watcher runs, as
	// saved: any Repository is under Pull Request.
	RunsPullRequestWatcher bool `json:"runs_pull_request_watcher"`
	// RepositoryLanding is the Default Landing named as a Repository's
	// Landing names it.
	RepositoryLanding RepositoryLanding `json:"repository_landing"`
	// PushesNothing is whether Push is on in some Landing while every
	// Repository is local-only, so it pushes nothing.
	PushesNothing bool `json:"pushes_nothing"`
	// Watcher is the Pull Request Watcher's status, not_reported unless the
	// method is Pull Request.
	Watcher WatcherView `json:"watcher"`
	// OpenPullRequests are the pull requests left open on the Code Host
	// after a switch away from Pull Request, one per Ticket's, until the
	// Ticket leaves Accepted. Always empty under Pull Request.
	OpenPullRequests []OpenPullRequest `json:"open_pull_requests"`
}

// OpenPullRequest is a pull request left open for an Accepted Ticket.
type OpenPullRequest struct {
	Ticket string `json:"ticket"`
	Link   string `json:"link"`
}

// ProjectView is a Project as every read returns it.
type ProjectView struct {
	Key         string      `json:"key"`
	DisplayName string      `json:"display_name"`
	Created     time.Time   `json:"created"`
	Mode        domain.Mode `json:"mode"`
	Archived    bool        `json:"archived"`
	// ArchivedAt is when the human archived the Project, while it is.
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	// Landing is how the Project's accepted work lands, always filled in:
	// merge, push off, origin by default.
	Landing LandingView `json:"landing"`
	// CI is the Project's CI settings, always filled in: CI Tracking off
	// by default.
	CI CIView `json:"ci"`
	// GitHubIssues is the Project's GitHub Issues settings, always filled
	// in: GitHub Issues off by default.
	GitHubIssues GitHubIssuesView `json:"github_issues"`
	// AzureBoards is the Project's Azure Boards settings, always filled
	// in: Azure Boards off by default.
	AzureBoards AzureBoardsView `json:"azure_boards"`
	// Models is the Project's Models rows, always filled in: every row at
	// Default to begin with.
	Models ModelsView `json:"models"`
	// ReportedModels is what the Factory last reported its claude
	// accepts, always filled in: no aliases or model IDs, and the seeded
	// effort levels, until a Factory reports.
	ReportedModels ReportedModelsView `json:"reported_models"`
	// StageGuides is the Project's Stage Guides settings, always filled
	// in: Subagent Exploration on by default.
	StageGuides StageGuidesView `json:"stage_guides"`
	// AutoAccept is the Project's Auto-accept setting, one entry per
	// Auto-accept kind, always filled in: every kind off by default.
	AutoAccept AutoAcceptView `json:"auto_accept"`
	// FocusKinds is the Project's Focus Kinds, always filled in: every
	// Needs Me reason on by default.
	FocusKinds FocusKindsView `json:"focus_kinds"`
	// FactoryStatus is whether the Project's Factories are online, from
	// their Heartbeats, always filled in: offline, with no Factories,
	// until one sends a Heartbeat.
	FactoryStatus FactoryStatusView `json:"factory_status"`
	// CheckCommand is the Project's Check Command, the default for each
	// Repository without its own; empty for none.
	CheckCommand string `json:"check_command"`
	// Repositories are the Project's Repositories in landing order, always
	// filled in: the Home Repository alone until the human adds another.
	Repositories []RepositoryView `json:"repositories"`
}

// SpecView is a Spec as every read returns it, and as its writes return
// it with full.
type SpecView struct {
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Labels  []string          `json:"labels"`
	Tags    map[string]string `json:"tags"`
	Created time.Time         `json:"created"`
	Updated time.Time         `json:"updated"`
	Body    string            `json:"body"`
	Edits   []EditView        `json:"edits"`
	// Link is set on a linked Spec (ticket 42).
	Link *LinkView `json:"link,omitempty"`
}

// SpecRef names a Ticket's Spec.
type SpecRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Branch is the Spec's Spec Branch: its branch Tag's value, else
	// spec/<id>.
	Branch string `json:"branch"`
}

// MoveView is one State move. An imported Ticket's one move has no From.
type MoveView struct {
	From  domain.State `json:"from,omitempty"`
	To    domain.State `json:"to"`
	Actor string       `json:"actor"`
	Time  time.Time    `json:"time"`
	Claim int          `json:"claim,omitempty"`
	Via   string       `json:"via,omitempty"`
	// Commit is the commit that put the work into the base, on a land.
	Commit string `json:"commit,omitempty"`
	// Cause is why an auto-accept made the Move (project, ticket-label or
	// spec-label); left out on every other Move.
	Cause domain.AutoAcceptCause `json:"cause,omitempty"`
}

// StopView is one Stop of a Claim; Resumed is nil until it is Resumed.
type StopView struct {
	Stopped time.Time  `json:"stopped"`
	Resumed *time.Time `json:"resumed,omitempty"`
}

// ClaimView is one Claim as reads show it: never with its Claim ID.
type ClaimView struct {
	Number    int    `json:"number"`
	SessionID string `json:"session_id,omitempty"`
	// WorkingDir and ResumeCommand are as the holder last set them, empty
	// when unset: never a filled-in default.
	WorkingDir    string `json:"working_dir,omitempty"`
	ResumeCommand string `json:"resume_command,omitempty"`
	// Model and Effort are what the Claim's latest turn ran with, empty
	// meaning Claude Code's default.
	Model   string    `json:"model,omitempty"`
	Effort  string    `json:"effort,omitempty"`
	Started time.Time `json:"started"`
	// FactoryClaim is whether a Factory made the Claim; only a Factory
	// Claim can be Stopped.
	FactoryClaim bool `json:"factory_claim,omitempty"`
	// Stopped is whether the Claim is live with its latest Stop not yet
	// Resumed.
	Stopped bool       `json:"stopped,omitempty"`
	Stops   []StopView `json:"stops,omitempty"`
	// LastActivity is the latest write the Claim's agent made on the Ticket.
	LastActivity time.Time  `json:"last_activity"`
	Ended        *time.Time `json:"ended,omitempty"`
	EndedBy      string     `json:"ended_by,omitempty"`
	// LatestEvent is the Claim's latest Session Event, shown short; left
	// out when it has none. Computed, never stored.
	LatestEvent *LatestEvent `json:"latest_event,omitempty"`
}

// TicketView is the full view of one Ticket, as its writes return it with
// full. A read (get_ticket, claim) may leave parts out: each part left out
// is nil, so it has no key, and LeftOut counts what it had.
type TicketView struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	State        domain.State      `json:"state"`
	Spec         SpecRef           `json:"spec"`
	Labels       []string          `json:"labels"`
	Tags         map[string]string `json:"tags"`
	BlockedBy    []string          `json:"blocked_by"`
	Blockers     []BlockerView     `json:"blockers"`
	Replacements []string          `json:"replacements"`
	Created      time.Time         `json:"created"`
	Updated      time.Time         `json:"updated"`
	Description  string            `json:"description"`
	// Branch and Commit are the work's branch and head commit in the Home
	// Repository, as the last finish naming them gave them. Work with a
	// branch is Accepted, then lands.
	Branch string `json:"branch,omitempty"`
	Commit string `json:"commit,omitempty"`
	// Repositories are the Ticket's Changed Repositories, in landing
	// order: live while it is In Progress, as the Factory reports them,
	// and settled by each finish. Left out while the Project has the Home
	// Repository alone, whose work Branch and Commit give.
	Repositories []ChangedRepositoryView `json:"repositories,omitempty"`
	// PartlyLanded says the Ticket is Accepted and has landed in some of
	// its Changed Repositories but not all. Computed on read.
	PartlyLanded bool `json:"partly_landed,omitempty"`
	// Recommendation is a spec Stage's Spec recommendation, as its last
	// finish gave it: implement (Implement) or split (Split into
	// Tickets); empty on any other Ticket.
	Recommendation domain.Recommendation `json:"recommendation,omitempty"`
	// PushedCommit is the latest commit the Factory pushed for the Ticket
	// in the Home Repository while CI Tracking was on, and CICommit the
	// pushed commit there the Home Repository's CI result comes from:
	// under the CI gate one that passed, if any did, else the latest
	// (code-central-430). CIResult is the Ticket's CI result, combined
	// across its watched Repositories: running, else failed, else
	// cancelled, else passed; each Repository's own is in Repositories.
	// All are left out while it has none. CIRuns are every CI Run tracked
	// against it, the Home Repository's first, each Repository's latest
	// pushed commit's first. Computed on read.
	PushedCommit *PushedCommitView `json:"pushed_commit,omitempty"`
	// PullRequest is the pull request the Pull Request Watcher reported
	// for the Ticket's pushed work; left out until it has one.
	PullRequest *PullRequestView `json:"pull_request,omitempty"`
	CIResult    CIResult         `json:"ci_result,omitempty"`
	CICommit    string           `json:"ci_commit,omitempty"`
	CIRuns      []CIRunView      `json:"ci_runs,omitempty"`
	// LandAnyway says the human accepted the failed CI result of the
	// latest pushed commit in the Home Repository, so the Factory lands it
	// there (code-central-261), on this Ticket or on another sharing the
	// commit (code-central-432).
	LandAnyway bool `json:"land_anyway,omitempty"`
	// CIAutoFix is what Auto-fix does about the Ticket's CI failure in the
	// Home Repository: the auto fix Ticket fixing it, the one covering it,
	// or the chain it stopped at the limit; left out while it does
	// nothing. Each Repository's own is in Repositories. Computed on read.
	CIAutoFix *CIAutoFixView `json:"ci_auto_fix,omitempty"`
	// SpecLanded says Land per Spec covers the Ticket, and WaitingForSpec
	// that it is Accepted and waits for its Spec to land (code-central-371).
	// Computed on read.
	SpecMerge  *SpecMergeView `json:"spec_merge,omitempty"`
	SpecLanded bool           `json:"spec_landed,omitempty"`
	// CIItem is the CI failed, CI cancelled or CI unavailable item that
	// lists this Ticket, with every other Ticket waiting on its pushed
	// commit (code-central-432); left out while none does. A Ticket listed
	// in several Repositories has the first in landing order whose reason
	// is its own here, and CIItems lists each one, but only while the
	// Project has more than one Repository.
	CIItem         *CIItemView  `json:"ci_item,omitempty"`
	CIItems        []CIItemView `json:"ci_items,omitempty"`
	WaitingForSpec bool         `json:"waiting_for_spec,omitempty"`
	// CIFailureOptions are the failure options, each an operation's name,
	// the human may choose on the Ticket's CI item while it is CI failed
	// or CI cancelled, acting on every Ticket it lists; left out
	// otherwise, and empty when none applies. Computed on read.
	CIFailureOptions []CIFailureOption `json:"ci_failure_options,omitzero"`
	// ToMergeOptions are the options, each an operation's name, the human
	// may choose on the Ticket while it is Accepted under a Landing Method
	// they merge (Branch or Pull Request), as send_back and new_fix_ticket
	// would accept them; left out otherwise, and empty when none applies.
	// Computed on read.
	ToMergeOptions []ToMergeOption     `json:"to_merge_options,omitzero"`
	Claims         []ClaimView         `json:"claims,omitzero"`
	Rounds         []RoundView         `json:"rounds,omitzero"`
	FailureReports []FailureReportView `json:"failure_reports,omitzero"`
	Comments       []CommentView       `json:"comments,omitzero"`
	Moves          []MoveView          `json:"moves,omitzero"`
	Edits          []EditView          `json:"edits,omitzero"`
	// LeftOut counts each part a read left out that has anything, by
	// part name.
	LeftOut   map[string]int `json:"left_out,omitempty"`
	Blocked   bool           `json:"blocked"`
	Claimable bool           `json:"claimable"`
	// Editable says the UI offers Edit on the Ticket, and Retroable its
	// Retro button, as ticketEditable and ticketRetroable decide them.
	// Every read sets them. Computed on read.
	Editable  bool `json:"editable"`
	Retroable bool `json:"retroable"`
	// Stoppable says the human may stop the Ticket's live Claim, and
	// Resumable that they may resume it, as stop and resume would accept
	// them. Computed on read.
	Stoppable bool `json:"stoppable,omitempty"`
	Resumable bool `json:"resumable,omitempty"`
	// SilentSince is set while the Ticket is a Silent Claim: In Progress
	// with no activity for 30 minutes. It is the time of the last
	// activity, so the quiet time is now minus it. Computed, never stored.
	SilentSince *time.Time `json:"silent_since,omitempty"`
	// NeedsMeReason says why the Ticket waits on the human, if it does.
	NeedsMeReason NeedsMeReason `json:"needs_me_reason,omitempty"`
	// Link is set on a linked Ticket (ticket 42).
	Link *LinkView `json:"link,omitempty"`
	// AwaitingTriage is set on an Incoming Issue that came in while New
	// issues need me was on and that the human hasn't triaged yet.
	AwaitingTriage bool `json:"awaiting_triage,omitempty"`
	// SpecMergeTicket marks a Spec merge Ticket, one open_spec_merge
	// opened; left out on any other Ticket.
	SpecMergeTicket bool `json:"spec_merge_ticket,omitempty"`
	// AutoFix is an auto fix Ticket's Auto-fix mark; left out on any
	// other Ticket, a manual fix Ticket too.
	AutoFix *AutoFixMarkView `json:"auto_fix,omitempty"`
	// TriageActions are the triage actions, each an operation's name, the
	// human may take on an Incoming Issue; left out on any other Ticket.
	// Computed on read.
	TriageActions []TriageAction `json:"triage_actions,omitzero"`
	// TriageNotes say what each of TriageActions does to the linked issue
	// or work item, as the UI shows it before the human confirms; left out
	// on any other Ticket. Computed on read.
	TriageNotes map[TriageAction]string `json:"triage_notes,omitzero"`
	// ModelChoice is the model and effort the Ticket's next turn runs
	// with, each with its source; only the UI's read carries it.
	ModelChoice *ModelChoice `json:"model_choice,omitempty"`
	// Retros are the Retros looking back on this Ticket, oldest first;
	// left out when there are none. Only get_ticket sets it.
	Retros []RetroView `json:"retros,omitempty"`
	// AutoAcceptKind is the Ticket's Auto-accept kind (grilling, spec,
	// tickets, implementation or merge); left out when it has none. Only
	// the UI's read carries it.
	AutoAcceptKind domain.AutoAcceptKind `json:"auto_accept_kind,omitempty"`
}

// TicketRow is one find_tickets result.
type TicketRow struct {
	ID      string       `json:"id"`
	Title   string       `json:"title"`
	State   domain.State `json:"state"`
	Spec    string       `json:"spec"`
	Labels  []string     `json:"labels"`
	Blocked bool         `json:"blocked"`
	Created time.Time    `json:"created"`
	Updated time.Time    `json:"updated"`
	Link    *LinkView    `json:"link,omitempty"`
	// NodeType is the Ticket's type on a Map (see NodeTypeOf); left out
	// for a Ticket on any other Spec.
	NodeType NodeType `json:"node_type,omitempty"`
	// BlockedBy is the Ticket's Blockers, including any on another Spec: on
	// a find_tickets row only for a Ticket on a Map (the Map view's edges),
	// on a get_spec row for every Spec kind except Long-lived. Left out
	// otherwise.
	BlockedBy []string `json:"blocked_by,omitempty"`
	// LatestEvent is the latest Session Event of the Ticket's live Claim,
	// shown short; left out when there is none.
	LatestEvent *LatestEvent `json:"latest_event,omitempty"`
	// CIResult is the Ticket's CI result, for its row's badge; left out
	// for a Ticket never pushed while CI Tracking was on.
	CIResult CIResult `json:"ci_result,omitempty"`
	// PullRequest is the Ticket's pull request, for its row's badge; left
	// out until the Pull Request Watcher reports one.
	PullRequest *PullRequestView `json:"pull_request,omitempty"`
	// WaitingForSpec says the Ticket is Accepted and waits for its Spec to
	// land, for its badge (code-central-371).
	WaitingForSpec bool `json:"waiting_for_spec,omitempty"`
	// NeedsMeReason says why the Ticket waits on the human, if it does:
	// the reason get_ticket gives, a CI item's for a Ticket it lists.
	NeedsMeReason NeedsMeReason `json:"needs_me_reason,omitempty"`
	// Repositories are the Names of the Ticket's Changed Repositories, in
	// landing order, for its card's chips; left out while the Project has
	// the Home Repository alone.
	Repositories []string `json:"repositories,omitempty"`
	// PartlyLanded says the Ticket is Accepted and has landed in some of
	// its Changed Repositories but not all, for its card's badge.
	PartlyLanded bool `json:"partly_landed,omitempty"`
}

// CreateProjectInput is create_project's input.
type CreateProjectInput struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name,omitempty"`
	// Mode is committed (the default) or local.
	Mode domain.Mode `json:"mode,omitempty"`
}

// CreateSpecInput is create_spec's input.
type CreateSpecInput struct {
	Title string `json:"title" desc:"The Spec's title." required:"true"`
	Body  string `json:"body" desc:"The Spec in Markdown."`
	// Full asks for the Spec in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Spec instead of the acknowledgement."`
}

// CreateTicketInput is create_ticket's input. State defaults to Draft.
type CreateTicketInput struct {
	Spec        Ref          `json:"spec" desc:"The Spec it belongs to." required:"true"`
	Title       string       `json:"title" desc:"The Ticket's title." required:"true"`
	Description string       `json:"description,omitempty" desc:"What to build, in Markdown."`
	State       domain.State `json:"state,omitempty" desc:"Draft (the default) or Ready." enum:"Draft,Ready"`
	Labels      []string     `json:"labels,omitempty" desc:"Labels to add."`
	BlockedBy   []Ref        `json:"blocked_by,omitempty" desc:"Tickets it waits on until they are Done. IDs: <key>-<n> or <n>."`
	// Tags are the Ticket's Tags from the start, checked as set_tags checks them.
	Tags map[string]string `json:"tags,omitempty" desc:"Tags to set on it, as {key: value}; checked as set_tags checks them, so effort must be a valid level."`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// FindTicketsInput is find_tickets' filters; every filter given must match.
type FindTicketsInput struct {
	State  []domain.State `json:"state,omitempty" desc:"Only Tickets in one of these States."`
	Title  string         `json:"title,omitempty" desc:"Only Tickets whose title contains this text."`
	Labels []string       `json:"labels,omitempty" query:"label" desc:"Only Tickets with all of these Labels."`
	// Tags must all be present with exactly these values, or with any
	// value for an empty one. Over HTTP each is a tag=<key>=<value>
	// parameter, which DecodeQuery reads.
	Tags map[string]string `json:"tags,omitempty" query:"tag" desc:"Only Tickets with all of these Tags, as {key: value}; an empty value matches any value."`
	Spec Ref               `json:"spec,omitempty" desc:"Only Tickets in this Spec."`
	// Claimable, when given, keeps only Tickets that are (or aren't) claimable.
	Claimable *bool `json:"claimable,omitempty" desc:"Only Tickets that are (true) or aren't (false) claimable."`
	// Full asks for full Ticket views, as get_ticket returns them, in
	// place of rows; FindTicketViews answers it.
	Full bool `json:"full,omitempty" desc:"Return each Ticket as get_ticket does, in place of the row."`
	// Parts are the parts the views carry, as get_ticket's: the default
	// parts, brief, when none are named. Only the HTTP API (as repeated
	// part parameters) and the CLI name them; an agent's MCP read takes
	// the default.
	Parts []string `json:"-" query:"part"`
	// WithModelChoice adds each Ticket's Model Choice and Auto-accept
	// kind, as get_ticket's
	// does for the UI's read over HTTP.
	WithModelChoice bool `json:"-"`
}
