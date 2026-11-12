// Package store is the persistence interface core writes through.
//
// Two implementations exist: Open (YAML files on disk, loaded whole into
// memory at startup) and NewMemory (an in-memory fake). Both pass the
// contract suite in store/storetest. Every read comes from memory; every
// Update writes at most one record, and memory changes only after that
// record is durable.
package store

import (
	"errors"
	"maps"
	"slices"
	"time"
)

// State is a Ticket's single position in the workflow.
type State string

// The ten States of the one fixed workflow.
const (
	Draft       State = "Draft"
	Ready       State = "Ready"
	InProgress  State = "In Progress"
	NeedsAnswer State = "Needs Answer"
	Answered    State = "Answered"
	Failed      State = "Failed"
	InReview    State = "In Review"
	Accepted    State = "Accepted"
	Done        State = "Done"
	Cancelled   State = "Cancelled"
)

// States lists every State in workflow order.
var States = []State{Draft, Ready, InProgress, NeedsAnswer, Answered, Failed, InReview, Accepted, Done, Cancelled}

// Valid reports whether s is one of the ten States.
func (s State) Valid() bool { return slices.Contains(States, s) }

// Mode is a Project's mode: where init puts its files, committed to the
// repository or kept local to one checkout (`init --local`).
type Mode string

// The two modes.
const (
	ModeCommitted Mode = "committed"
	ModeLocal     Mode = "local"
)

// Valid reports whether m is one of the two modes.
func (m Mode) Valid() bool { return m == ModeCommitted || m == ModeLocal }

// Project is one tracked repository.
type Project struct {
	Key         string
	DisplayName string
	Created     time.Time
	Mode        Mode
	// Archived is when the human archived the Project; zero while it isn't.
	Archived time.Time
	// Landing is how the Project's accepted work lands. A Project stored
	// without one reads as DefaultLanding.
	Landing Landing
	// CI is the Project's CI settings. A Project stored without them reads
	// as DefaultCI: CI Tracking off.
	CI CI
	// GitHubIssues is the Project's GitHub Issues settings. A Project
	// stored without them reads as the zero GitHubIssues: off.
	GitHubIssues GitHubIssues
	// Models is the Project's Models rows. A Project stored without them
	// sets no model or effort.
	Models Models
	// ReportedModels is what the Factory last reported its claude accepts.
	// A Project stored without them has had no report.
	ReportedModels ReportedModels
	// StageGuides is the Project's Stage Guides settings. A Project stored
	// without them reads as the zero StageGuides: every optional line on.
	StageGuides StageGuides
	// CIResponses are the human's responses to pushed commits' CI
	// results, one per commit (code-central-432). A Project stored
	// without them has none.
	CIResponses []CIResponse
	// CIFixesSpec is the number of the Project's CI Fixes Spec, the
	// Long-lived Spec its New fix Tickets open on; 0 until the first one.
	CIFixesSpec int
}

// StageGuides is a Project's Stage Guides settings (Spec
// code-central-441), each the opposite of a named optional guide line, so
// the zero value keeps every line in and a Project stored without the
// record needs no migration.
type StageGuides struct {
	// SubagentExplorationOff leaves the implementation guide's subagent
	// line out.
	SubagentExplorationOff bool
}

// CIResponse is the human's response to one pushed commit's CI result,
// stored once per commit on the Project, whatever Tickets wait on it
// (code-central-432): Land anyway, and a Re-run request with the failed
// run attempts it asked for, as a PushedCommit carries them.
type CIResponse struct {
	Commit string
	// LandAnyway is when the human accepted the commit's failed CI
	// result; zero until then.
	LandAnyway time.Time
	// Rerun is when the human last asked for the commit's failed jobs to
	// be re-run; zero until then. Reruns are the failed run attempts it
	// asked for.
	Rerun  time.Time
	Reruns []CIRunAttempt
	// FixTicket is the number of the fix Ticket the human opened for the
	// commit's failure with New fix Ticket; 0 until then.
	FixTicket int
}

// LandingMethod is how accepted work reaches a Project's base.
type LandingMethod string

// The Landing Methods.
const (
	// LandingMerge: the Factory merges the branch into the base.
	LandingMerge LandingMethod = "merge"
	// LandingBranch: the Factory leaves the branch for the human to merge.
	LandingBranch LandingMethod = "branch"
	// LandingPullRequest: the Factory pushes the branch and the Pull
	// Request Watcher opens a pull request on the Code Host. Push is
	// implied.
	LandingPullRequest LandingMethod = "pull_request"
)

// LandingMethods lists every LandingMethod, the default first.
var LandingMethods = []LandingMethod{LandingMerge, LandingBranch, LandingPullRequest}

// Valid reports whether m is one of the LandingMethods.
func (m LandingMethod) Valid() bool { return slices.Contains(LandingMethods, m) }

// landingMethodNames is the LandingMethods as strings, for messages and
// the schema.
func landingMethodNames() []string {
	var out []string
	for _, m := range LandingMethods {
		out = append(out, string(m))
	}
	return out
}

// Landing is a Project's Landing Method with its Push option: whether
// the Factory pushes the work to Remote.
type Landing struct {
	Method LandingMethod
	Push   bool
	Remote string
	// LandPerSpec is the Merge option that lands a Spec's work together.
	// It reads as on for a Project that never stored it, and is kept
	// whatever the method.
	LandPerSpec bool
	// PATEnv is the Pull Request setting naming the environment variable
	// that holds the Azure Repos PAT. Empty reads as DefaultPATEnv, and
	// it is kept whatever the method.
	PATEnv string
	// Base is the branch the Project's work is cut from and lands on,
	// under every Landing Method. Empty reads as the Project's stored
	// PushRemoteBase, else DefaultBase; see Project.Base.
	Base string
}

// DefaultBase is the base of a Project that names none and never had one
// reported.
const DefaultBase = "main"

// Base is the branch p's work is cut from and lands on: the stored
// Landing Base, else the frozen PushRemoteBase, else DefaultBase.
func (p Project) Base() string {
	switch {
	case p.Landing.Base != "":
		return p.Landing.Base
	case p.CI.PushRemoteBase != "":
		return p.CI.PushRemoteBase
	}
	return DefaultBase
}

// DefaultLandingRemote is the remote Push uses when none is named.
const DefaultLandingRemote = "origin"

// DefaultLanding is the Landing of a Project that has never had one set:
// Merge, Push off, remote origin.
func DefaultLanding() Landing {
	return Landing{Method: LandingMerge, Remote: DefaultLandingRemote, LandPerSpec: true}
}

// orDefault fills in what l leaves empty from DefaultLanding.
func (l Landing) orDefault() Landing {
	if l.Method == "" {
		// A Landing with no method was never set: Land per Spec is on.
		l.Method = LandingMerge
		l.LandPerSpec = true
	}
	if l.Remote == "" {
		l.Remote = DefaultLandingRemote
	}
	return l
}

// CIProvider is the CI system a Project's CI Tracking watches.
type CIProvider string

// The CI Providers.
const (
	// CIGitHubActions is GitHub Actions, reached through the gh CLI.
	CIGitHubActions CIProvider = "github_actions"
	// CIAzurePipelines is Azure Pipelines, reached over Azure DevOps'
	// REST API with a PAT from the service's environment.
	CIAzurePipelines CIProvider = "azure_pipelines"
)

// DefaultPATEnv is the PAT environment variable a Project that names none
// reads; the store keeps the name empty then, and Project reads fill it in.
const DefaultPATEnv = "ADO_PAT"

// CIProviders lists every CIProvider, the default first.
var CIProviders = []CIProvider{CIGitHubActions, CIAzurePipelines}

// Valid reports whether p is one of the CIProviders.
func (p CIProvider) Valid() bool { return slices.Contains(CIProviders, p) }

// ciProviderNames is the CIProviders as strings, for messages and the
// schema.
func ciProviderNames() []string {
	var out []string
	for _, p := range CIProviders {
		out = append(out, string(p))
	}
	return out
}

// CI is a Project's CI settings: whether CI Tracking is on, the CI
// Provider and the repository it watches, the Ignored workflows (each a
// workflow file's base name, such as ci.yml, or a pipeline's name), the
// URL of the Push remote the Factory last reported, and the CI Rules.
// Repository is the GitHub repository,
// ProjectURL the Azure DevOps project and PATEnv the name of the
// environment variable holding its PAT (never the PAT); each provider
// keeps its own.
type CI struct {
	Tracking         bool
	Provider         CIProvider
	Repository       string
	ProjectURL       string
	PATEnv           string
	IgnoredWorkflows []string
	PushRemoteURL    string
	// PushRemoteBase is the base branch a Factory reported before the base
	// became a Project setting. It is frozen: only Project.Base reads it,
	// as the fallback for a Landing with no Base.
	PushRemoteBase string
	Rules          []CIRule
}

// AnyWorkflow is the workflow of a CI Rule that matches every workflow
// but the Ignored ones.
const AnyWorkflow = "any"

// CIRule is "when Workflow fails on Branch, open a Ticket to fix it": a
// workflow file's base name or AnyWorkflow, an exact branch name, and
// the State its Tickets open in, Draft or Ready. Its Tickets go on its
// own Spec, whose number names the rule.
type CIRule struct {
	Spec     int
	Workflow string
	Branch   string
	State    State
	Created  time.Time
}

// Equal reports whether r and s are the same rule.
func (r CIRule) Equal(s CIRule) bool {
	return r.Spec == s.Spec && r.Workflow == s.Workflow && r.Branch == s.Branch && r.State == s.State && r.Created.Equal(s.Created)
}

// DefaultCI is the CI of a Project that has never had any set: CI
// Tracking off, on GitHub Actions, with nothing named.
func DefaultCI() CI { return CI{Provider: CIGitHubActions} }

// Equal reports whether c and d are the same settings.
func (c CI) Equal(d CI) bool {
	return c.Tracking == d.Tracking && c.Provider == d.Provider && c.Repository == d.Repository && c.ProjectURL == d.ProjectURL && c.PATEnv == d.PATEnv &&
		slices.Equal(c.IgnoredWorkflows, d.IgnoredWorkflows) && c.PushRemoteURL == d.PushRemoteURL && c.PushRemoteBase == d.PushRemoteBase &&
		slices.EqualFunc(c.Rules, d.Rules, CIRule.Equal)
}

// orDefault fills in what c leaves empty from DefaultCI.
func (c CI) orDefault() CI {
	if c.Provider == "" {
		c.Provider = CIGitHubActions
	}
	return c
}

// GitHubIssues is a Project's GitHub Issues settings (Spec
// code-central-279): whether it is on, the repository (owner/name) its
// issues come from, the filter label (empty for every issue), whether New
// issues need me, and the number of its Issues Spec, 0 until GitHub
// Issues is first turned on.
type GitHubIssues struct {
	On              bool
	Repository      string
	Label           string
	NewIssuesNeedMe bool
	Spec            int
}

// Models is a Project's Models rows (Spec code-central-301): the model
// and effort for the Project default and for each kind of work, as plain
// strings, each empty when the row leaves it unset.
type Models struct {
	Default ModelRow
	// Kinds holds only the rows that set something, one per kind of work
	// at most, in ModelKinds order.
	Kinds []KindModels
}

// ModelRow is one Models row's model and effort, each empty when unset.
type ModelRow struct {
	Model  string
	Effort string
}

// KindModels is the Models row for one kind of work, a Stage Guide name.
type KindModels struct {
	Kind string
	ModelRow
}

// ModelKinds are the kinds of work a Models row may name: core's
// GuideNames, which core's tests keep in step.
var ModelKinds = []string{"grilling", "spec", "tickets", "map-chart", "map-grilling", "map-research", "map-task", "prototype", "implementation"}

// IsZero reports whether m sets nothing.
func (m Models) IsZero() bool { return m.Default == (ModelRow{}) && len(m.Kinds) == 0 }

// Equal reports whether m and n set the same rows.
func (m Models) Equal(n Models) bool {
	return m.Default == n.Default && slices.Equal(m.Kinds, n.Kinds)
}

// Kind is the row for the kind of work named kind, empty when it sets
// nothing.
func (m Models) Kind(kind string) ModelRow {
	for _, k := range m.Kinds {
		if k.Kind == kind {
			return k.ModelRow
		}
	}
	return ModelRow{}
}

// ReportedModels are the effort levels, model aliases and model IDs a
// Project's claude was reported to accept, with the Factory's own
// --model and --effort flags (Spec code-central-301), all plain strings.
type ReportedModels struct {
	EffortLevels []string
	Aliases      []string
	ModelIDs     []string
	// ModelFlag and EffortFlag are the Factory's flags, empty when unset.
	ModelFlag  string
	EffortFlag string
	// Reported is when the Factory last reported, zero before any report.
	Reported time.Time
}

// IsZero reports whether r holds nothing.
func (r ReportedModels) IsZero() bool {
	return len(r.EffortLevels) == 0 && len(r.Aliases) == 0 && len(r.ModelIDs) == 0 && r.ModelFlag == "" && r.EffortFlag == "" && r.Reported.IsZero()
}

// Spec is a description of work, stored as Markdown with YAML frontmatter.
type Spec struct {
	Number  int
	Title   string
	Labels  []string
	Tags    map[string]string
	Link    Link
	Created time.Time
	Updated time.Time
	Body    string
	Edits   []Edit
}

// Comment is one numbered comment on a Ticket. Required notes on actions
// (a finish summary, a reject comment...) are stored as comments too.
type Comment struct {
	Number int
	Actor  string
	Time   time.Time
	Claim  int // Claim number the note was made under, 0 when none
	Body   string
}

// Edit records one edit that changed a Spec's or Ticket's text. Previous
// maps each changed field (title, body, description) to its old value.
type Edit struct {
	Number   int
	Actor    string
	Time     time.Time
	Previous map[string]string
}

// Move records one State change on a Ticket.
type Move struct {
	From  State
	To    State
	Actor string
	Time  time.Time
	Claim int // Claim number, 0 when none
	Via   string
	// Commit is the commit that put the work into the base, on a land.
	Commit string
}

// Claim is one agent's hold on a Ticket, from the claim until a move out
// of In Progress, Needs Answer or Answered ends it for good.
type Claim struct {
	Number    int
	ID        string // the Claim ID; never returned by a read
	SessionID string
	// WorkingDir and ResumeCommand are free-form, never parsed: how the
	// human gets back into the Claim's session.
	WorkingDir    string
	ResumeCommand string
	// Model and Effort are what the Claim's latest turn ran with, empty
	// meaning Claude Code's default.
	Model   string
	Effort  string
	Started time.Time
	Ended   time.Time // zero while the Claim is live
	EndedBy string    // the action that ended it
}

// FailureReport is one numbered, permanent report of why an agent with
// no way forward stopped. Posting one ends the Claim it was made on.
type FailureReport struct {
	Number          int
	Claim           int
	SessionID       string // the Claim's session ID when the report was posted
	WorkingDir      string // the Claim's working directory then
	ResumeCommand   string // the Claim's Resume Command then
	Actor           string
	Time            time.Time
	WhatBroke       string
	Why             string
	WhatToFix       string
	WhereWorkStands string
}

// Live reports whether the Claim has not ended.
func (c Claim) Live() bool { return c.Ended.IsZero() }

// QuestionStatus is where one Question stands.
type QuestionStatus string

// The three Question statuses.
const (
	QuestionOpen      QuestionStatus = "open"
	QuestionAnswered  QuestionStatus = "answered"
	QuestionDismissed QuestionStatus = "dismissed"
)

// Question is one query in a Round. Questions are numbered across the
// Ticket, so a number names one Question on its Ticket.
type Question struct {
	Number      int
	Title       string
	Body        string
	Recommended string
	Status      QuestionStatus
	Answer      string
	Closed      time.Time // when last answered or dismissed; zero while open
}

// Round is the Questions an agent posted together, in one write.
type Round struct {
	Number    int
	Claim     int // the Claim number it was posted under
	SessionID string
	Posted    time.Time
	Questions []Question
}

// Ticket is a unit of work within a Spec.
type Ticket struct {
	Number       int
	Title        string
	State        State
	Spec         int
	Labels       []string
	Tags         map[string]string
	BlockedBy    []int
	Replacements []int
	Link         Link
	// AwaitingTriage is set by Code Central on an Incoming Issue created
	// while New issues need me is on (Spec code-central-279), and cleared
	// when the human triages it; never set again once cleared.
	AwaitingTriage bool
	Created        time.Time
	Updated        time.Time
	Description    string
	// Branch and Commit are what the last finish naming them gave: the
	// work's branch and its head commit. Free-form, never parsed.
	Branch string
	Commit string
	// PushedCommits are the commits the Factory pushed for the Ticket
	// while CI Tracking was on, oldest first: the latest is the one its CI
	// result comes from, and the older ones stay as its history.
	PushedCommits []PushedCommit
	// SpecMerge is the Spec Branch commit the Factory merged the Ticket's
	// work into under Land per Spec, and when Code Central heard of it;
	// nil until then.
	SpecMerge *SpecMerge
	// PullRequest is the pull request the Pull Request Watcher reported
	// for the Ticket's pushed work (code-central-388); nil until then.
	PullRequest *PullRequest
	Claims      []Claim
	Rounds      []Round
	// FailureReports are numbered and never edited.
	FailureReports []FailureReport
	Comments       []Comment
	Moves          []Move
	Edits          []Edit
}

// SpecMerge is the Spec Branch commit a Ticket's work was merged into, and
// when Code Central heard of it.
type SpecMerge struct {
	Commit string
	Merged time.Time
}

// PullRequestState is where a pull request stands on its Code Host.
type PullRequestState string

// The pull request states.
const (
	PROpen        PullRequestState = "open"
	PRConflicting PullRequestState = "conflicting"
	PRClosed      PullRequestState = "closed"
	PRMerged      PullRequestState = "merged"
)

// PullRequestStates lists every PullRequestState.
var PullRequestStates = []PullRequestState{PROpen, PRConflicting, PRClosed, PRMerged}

// PullRequest is a pull request on a Code Host: its number and link, its
// state, and once merged the merge commit the host reports.
type PullRequest struct {
	Number      int
	Link        string
	State       PullRequestState
	MergeCommit string
}

// PushedCommit is one commit the Factory pushed for a Ticket, and when
// Code Central heard of it.
type PushedCommit struct {
	Commit string
	// Branch is the branch the commit was pushed as; empty for a push
	// reported without one.
	Branch string
	Pushed time.Time
	// LandAnyway is when the human accepted this commit's failed CI
	// result, so the Factory lands it (code-central-261); zero until then.
	// Since code-central-432 the response is stored once per commit on
	// the Project (CIResponse); a value stored here before is still read.
	LandAnyway time.Time
	// Rerun is when the human last asked for this commit's failed jobs to
	// be re-run (code-central-265); zero until then. Reruns are the failed
	// run attempts it asked for: each waits until the CI Watcher reports
	// a later attempt of its run. Stored on the Project since
	// code-central-432, like LandAnyway.
	Rerun  time.Time
	Reruns []CIRunAttempt
}

// CIRunAttempt names one attempt of one CI Run.
type CIRunAttempt struct {
	ID      int64
	Attempt int
}

// CIRunStatus is where a CI Run stands, as Code Central has it.
type CIRunStatus string

// The CI Run statuses.
const (
	CIQueued     CIRunStatus = "queued"
	CIInProgress CIRunStatus = "in_progress"
	CICompleted  CIRunStatus = "completed"
)

// CIRunStatuses lists every CIRunStatus.
var CIRunStatuses = []CIRunStatus{CIQueued, CIInProgress, CICompleted}

// Valid reports whether s is one of the CIRunStatuses.
func (s CIRunStatus) Valid() bool { return slices.Contains(CIRunStatuses, s) }

// CIConclusion is how a finished CI Run or job ended, as Code Central has
// it; empty while it hasn't.
type CIConclusion string

// The CI conclusions.
const (
	CISuccess   CIConclusion = "success"
	CIFailure   CIConclusion = "failure"
	CICancelled CIConclusion = "cancelled"
	CITimedOut  CIConclusion = "timed_out"
	CISkipped   CIConclusion = "skipped"
	CINeutral   CIConclusion = "neutral"
	// CIPartiallySucceeded is a run that passed with failures its
	// pipeline let through; it counts as passed.
	CIPartiallySucceeded CIConclusion = "partially_succeeded"
)

// CIConclusions lists every CIConclusion.
var CIConclusions = []CIConclusion{CISuccess, CIFailure, CICancelled, CITimedOut, CISkipped, CINeutral, CIPartiallySucceeded}

// Valid reports whether c is one of the CIConclusions, or empty.
func (c CIConclusion) Valid() bool { return c == "" || slices.Contains(CIConclusions, c) }

// Failed reports whether c fails a CI result: failure or timed out.
// A cancelled run isn't a failure; see Cancelled.
func (c CIConclusion) Failed() bool { return c == CIFailure || c == CITimedOut }

// Cancelled reports whether c is a cancelled run, which makes a CI
// result cancelled rather than failed.
func (c CIConclusion) Cancelled() bool { return c == CICancelled }

// CIRun is one attempt of one workflow run the CI Provider reported,
// keyed by the provider's run ID and the attempt. It is tracked against
// a Ticket or a CI Rule (the Spec number naming it). A run of an Ignored
// workflow is kept, marked Ignored.
type CIRun struct {
	ID      int64
	Attempt int
	// Workflow is the workflow file's base name, such as ci.yml, and Name
	// the name it shows.
	Workflow   string
	Name       string
	Branch     string
	Commit     string
	Status     CIRunStatus
	Conclusion CIConclusion
	// Started and Finished are zero until the run starts and finishes.
	Started  time.Time
	Finished time.Time
	URL      string
	Jobs     []CIJob
	// Ticket and Rule are what it is tracked against: a Ticket's number,
	// or a CI Rule's Spec number; the other is 0.
	Ticket  int
	Rule    int
	Ignored bool
}

// CIJob is one job of a CI Run: its conclusion is empty until it ends.
type CIJob struct {
	Name       string
	Conclusion CIConclusion
	URL        string
}

func (r CIRun) clone() CIRun {
	r.Jobs = slices.Clone(r.Jobs)
	return r
}

// CIRuns is a Project's CI Runs, written whole as one record, apart from
// its Tickets so frequent run updates don't rewrite Ticket files.
type CIRuns struct {
	Runs []CIRun
}

func (rs CIRuns) clone() CIRuns {
	out := CIRuns{Runs: make([]CIRun, 0, len(rs.Runs))}
	for _, r := range rs.Runs {
		out.Runs = append(out.Runs, r.clone())
	}
	return out
}

// FocusClear is what Focus Mode shows when nothing needs the human.
type FocusClear string

// The two things Focus Mode can show when nothing needs the human.
const (
	FocusAllClear          FocusClear = "all_clear"
	FocusActivityDashboard FocusClear = "activity_dashboard"
)

// FocusClears lists every FocusClear, the default first.
var FocusClears = []FocusClear{FocusActivityDashboard, FocusAllClear}

// Valid reports whether f is one of the FocusClears.
func (f FocusClear) Valid() bool { return slices.Contains(FocusClears, f) }

// Options are the human's global preferences, set from the UI's Options
// view. They are the store's one record that belongs to no Project.
type Options struct {
	FocusClear FocusClear
	// GHPath is where gh, the GitHub CLI, is: the gh path Option. Empty
	// finds it on the service's PATH, then in the usual install places.
	GHPath string
}

// DefaultOptions are the Options of a store that has never had any
// written.
func DefaultOptions() Options { return Options{FocusClear: FocusClears[0]} }

// FocusClearNames is the FocusClears as strings, for messages and the
// schema.
func FocusClearNames() []string {
	var out []string
	for _, f := range FocusClears {
		out = append(out, string(f))
	}
	return out
}

// Reader reads from memory. Records returned are copies.
type Reader interface {
	Project(key string) (Project, bool)
	Projects() []Project
	Spec(project string, n int) (Spec, bool)
	Specs(project string) []Spec
	Ticket(project string, n int) (Ticket, bool)
	// Tickets returns the project's Tickets in number order.
	Tickets(project string) []Ticket
	// NextNumber is the highest Spec or Ticket number in the project plus
	// one. It is derived, never stored, so numbers are never reused.
	NextNumber(project string) int
	// Options is always a whole value: the defaults until any are written.
	Options() Options
	// CIRuns is the project's CI Runs, in the order they were written.
	CIRuns(project string) []CIRun
}

// Writer is a Reader that can also write one record.
type Writer interface {
	Reader
	PutProject(Project) error
	PutSpec(project string, s Spec) error
	PutTicket(project string, t Ticket) error
	// PutImport is the one bulk write: every Spec and Ticket in im is new,
	// but for the existing Tickets it rewrites, and they all land together
	// or none does. It counts as the update's one write.
	PutImport(project string, im Import) error
	// PutOptions replaces the Options whole. It touches no Project.
	PutOptions(Options) error
	// PutCIRuns replaces the project's CI Runs whole.
	PutCIRuns(project string, runs []CIRun) error
}

// Import is a batch of new Specs and Tickets written all or nothing.
type Import struct {
	Specs   []Spec
	Tickets []Ticket
	// Rewritten are existing Tickets written along with the new records,
	// such as the Incoming Issue To Requirement Cancels.
	Rewritten []Ticket
}

func (im Import) clone() Import {
	out := Import{}
	for _, s := range im.Specs {
		out.Specs = append(out.Specs, s.clone())
	}
	for _, t := range im.Tickets {
		out.Tickets = append(out.Tickets, t.clone())
	}
	for _, t := range im.Rewritten {
		out.Rewritten = append(out.Rewritten, t.clone())
	}
	return out
}

// Store is the persistence interface. View and Update share one
// store-wide read-write mutex, so a check and the write it guards are
// atomic.
type Store interface {
	View(fn func(Reader) error) error
	// Update runs fn under the write lock. fn may write at most one
	// record; a second write fails with ErrSecondWrite.
	Update(fn func(Writer) error) error
}

var (
	// ErrSecondWrite means an Update tried to write more than one record.
	ErrSecondWrite = errors.New("store: an update may write only one record")
	// ErrUnknownProject means a Spec or Ticket was written to a project
	// that does not exist.
	ErrUnknownProject = errors.New("store: unknown project")
	// ErrExists means an Import named a Spec or Ticket number already in use.
	ErrExists = errors.New("store: an import may only add new records")
	// ErrNotExists means an Import rewrote a Ticket that doesn't exist.
	ErrNotExists = errors.New("store: an import may only rewrite existing Tickets")
	// ErrInvalidOption means an Option written was outside its set.
	ErrInvalidOption = errors.New("store: an Option's value is outside its set")
)

func (s Spec) clone() Spec {
	s.Labels = slices.Clone(s.Labels)
	s.Tags = maps.Clone(s.Tags)
	s.Edits = cloneEdits(s.Edits)
	s.Link.Written = slices.Clone(s.Link.Written)
	return s
}

func (t Ticket) clone() Ticket {
	t.Labels = slices.Clone(t.Labels)
	t.Tags = maps.Clone(t.Tags)
	t.BlockedBy = slices.Clone(t.BlockedBy)
	t.Replacements = slices.Clone(t.Replacements)
	t.PushedCommits = slices.Clone(t.PushedCommits)
	if t.SpecMerge != nil {
		sm := *t.SpecMerge
		t.SpecMerge = &sm
	}
	if t.PullRequest != nil {
		pr := *t.PullRequest
		t.PullRequest = &pr
	}
	t.Claims = slices.Clone(t.Claims)
	t.Rounds = slices.Clone(t.Rounds)
	for i := range t.Rounds {
		t.Rounds[i].Questions = slices.Clone(t.Rounds[i].Questions)
	}
	t.FailureReports = slices.Clone(t.FailureReports)
	t.Comments = slices.Clone(t.Comments)
	t.Moves = slices.Clone(t.Moves)
	t.Edits = cloneEdits(t.Edits)
	t.Link.Written = slices.Clone(t.Link.Written)
	return t
}

func cloneEdits(es []Edit) []Edit {
	es = slices.Clone(es)
	for i := range es {
		es[i].Previous = maps.Clone(es[i].Previous)
	}
	return es
}
