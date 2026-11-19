// Package domain is Code Central's domain model: the records the store
// keeps, their enumerations, defaults and invariants. It imports nothing
// of Code Central, so any client may import it; the store's file shapes
// and persistence live in store.
package domain

import (
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

// Modes are the two modes.
var Modes = []Mode{ModeCommitted, ModeLocal}

// Valid reports whether m is one of the two modes.
func (m Mode) Valid() bool { return slices.Contains(Modes, m) }

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
	// AzureBoards is the Project's Azure Boards settings. A Project stored
	// without them reads as the zero AzureBoards: off.
	AzureBoards AzureBoards
	// Models is the Project's Models rows. A Project stored without them
	// sets no model or effort.
	Models Models
	// ReportedModels is what the Factory last reported its claude accepts.
	// A Project stored without them has had no report.
	ReportedModels ReportedModels
	// StageGuides is the Project's Stage Guides settings. A Project stored
	// without them reads as the zero StageGuides: every optional line on.
	StageGuides StageGuides
	// FocusKindsOff are the Needs Me reasons the Project's Focus Kinds
	// turn off (Spec code-central-550), in NeedsMeReasons order with no
	// duplicates. A Project stored without them has every reason on.
	FocusKindsOff []string
	// CIResponses are the human's responses to pushed commits' CI
	// results, one per commit (code-central-432). A Project stored
	// without them has none.
	CIResponses []CIResponse
	// CIFixesSpec is the number of the Project's CI Fixes Spec, the
	// Long-lived Spec its New fix Tickets open on; 0 until the first one.
	CIFixesSpec int
	// SmallChangesSpec is the number of the Project's Small Changes Spec, the
	// Long-lived Spec its Small Changes open on; 0 until the first one.
	SmallChangesSpec int
	// RetrosSpec is the number of the Project's Retros Spec, the Long-lived
	// Spec its Retros open on; 0 until the first one.
	RetrosSpec int
	// AutoAccept is the Project's Auto-accept setting: the kinds it is on
	// for, in AutoAcceptKinds order (ADR 0037). It is stored as on, so a
	// Project stored without it reads every kind off (ADR 0059).
	AutoAccept []AutoAcceptKind
	// Repositories are the Project's Repositories in landing order (Spec
	// code-central-848), the Home Repository among them. A Project stored
	// without them has the Home Repository alone, as RepositoryList reads
	// it, and a list holding only the Home Repository is stored as none.
	Repositories []Repository
}

// Repository is one git repository a Project's work changes. The Home
// Repository, where the Factory runs, keeps its Name, the Project Key,
// its place in the landing order, and any Own Landing and Push remote:
// its Base is the Project's (Landing.Base) and its Clone URL the Push
// remote URL the Factory reports.
type Repository struct {
	// Name is the Repository Name, fixed once added.
	Name string
	Home bool
	// CloneURL is the remote the Repository is cloned from and pushed to.
	CloneURL string
	// Base is the branch the Repository's work is cut from and lands on.
	Base string
	// LocalPath is where the Factory finds the Repository's clone, when
	// it isn't beside the Home Repository; empty for there.
	LocalPath string
	// OwnLanding is the Repository's Own Landing, the zero OwnLanding
	// while it follows the Project's Default Landing (Spec
	// code-central-890).
	OwnLanding OwnLanding
	// PushRemote names the git remote the Factory pushes the Repository
	// to and fetches it from; empty for the Project's (Landing.Remote).
	PushRemote string
}

// OwnLanding is a Repository's own Landing Method and Push option, set
// in place of the Project's Default Landing. Its zero value, with no
// Method, is none: the Repository follows the Default Landing.
type OwnLanding struct {
	Method LandingMethod
	Push   bool
}

// Set reports whether o is an Own Landing, not the zero one.
func (o OwnLanding) Set() bool { return o.Method != "" }

// Normalized is o as it is stored: Push is implied under Pull Request,
// so it is cleared there, as for the Default Landing.
func (o OwnLanding) Normalized() OwnLanding {
	if o.Method == LandingPullRequest {
		o.Push = false
	}
	return o
}

// DefaultRepositoryBase is a Repository's Base when the human names none.
const DefaultRepositoryBase = "main"

// MaxRepositoryName is the longest a Repository Name may be.
const MaxRepositoryName = 64

// ValidRepositoryName reports whether name keeps the Repository Name
// rules: lowercase letters, digits, -, _ and ., starting with a letter or
// digit, at most MaxRepositoryName long.
func ValidRepositoryName(name string) bool {
	if name == "" || len(name) > MaxRepositoryName {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case i > 0 && (r == '-' || r == '_' || r == '.'):
		default:
			return false
		}
	}
	return true
}

// RepositoryList is the Project's Repositories in landing order: the
// Home Repository alone, named by the Project Key, while none is stored.
func (p Project) RepositoryList() []Repository {
	if len(p.Repositories) == 0 {
		return []Repository{{Name: p.Key, Home: true}}
	}
	return slices.Clone(p.Repositories)
}

// SetRepositories stores rs as the Project's Repositories, none while
// they are the Home Repository alone with nothing more than its name.
func (p *Project) SetRepositories(rs []Repository) {
	if len(rs) == 1 && rs[0] == (Repository{Name: rs[0].Name, Home: true}) {
		rs = nil
	}
	p.Repositories = rs
}

// AutoAcceptKind is an Auto-accept kind: the kind of Ticket a Project's
// Auto-accept setting is switched on for. A Ticket's kind is worked out
// on read from what it carries, never stored.
type AutoAcceptKind string

// The Auto-accept kinds, by their stable keys.
const (
	// AutoAcceptGrilling is Grilling outcomes: a grilling Stage.
	AutoAcceptGrilling AutoAcceptKind = "grilling"
	// AutoAcceptSpec is Spec recommendation: a spec Stage, accepted by
	// its Spec recommendation.
	AutoAcceptSpec AutoAcceptKind = "spec"
	// AutoAcceptTickets is Tickets Stage: a tickets Stage.
	AutoAcceptTickets AutoAcceptKind = "tickets"
	// AutoAcceptImplementation is Implementation results: every Ticket
	// that is none of the others and not a Prototype or a Map Ticket.
	AutoAcceptImplementation AutoAcceptKind = "implementation"
	// AutoAcceptMerge is Merge results: a Spec merge Ticket, by its mark.
	AutoAcceptMerge AutoAcceptKind = "merge"
)

// AutoAcceptKinds lists every Auto-accept kind, in the order a Project
// stores and shows them.
var AutoAcceptKinds = []AutoAcceptKind{AutoAcceptGrilling, AutoAcceptSpec, AutoAcceptTickets, AutoAcceptImplementation, AutoAcceptMerge}

// Valid reports whether k is one of the Auto-accept kinds.
func (k AutoAcceptKind) Valid() bool { return slices.Contains(AutoAcceptKinds, k) }

// AutoAccepts reports whether the Project's Auto-accept setting is on
// for kind k.
func (p Project) AutoAccepts(k AutoAcceptKind) bool { return slices.Contains(p.AutoAccept, k) }

// SetAutoAccept turns the Project's Auto-accept setting on or off for
// kind k, keeping AutoAccept in AutoAcceptKinds order.
func (p *Project) SetAutoAccept(k AutoAcceptKind, on bool) {
	var out []AutoAcceptKind
	for _, each := range AutoAcceptKinds {
		if each == k && on || each != k && p.AutoAccepts(each) {
			out = append(out, each)
		}
	}
	p.AutoAccept = out
}

// AutoAcceptCause is why a Move was made by an auto-accept: which switch
// turned it on. Empty on every other Move.
type AutoAcceptCause string

// The auto-accept causes, in the order the first that applies is
// recorded.
const (
	// CauseTicketLabel is the auto-accept Label on the Ticket.
	CauseTicketLabel AutoAcceptCause = "ticket-label"
	// CauseSpecLabel is the auto-accept Label on the Ticket's Spec.
	CauseSpecLabel AutoAcceptCause = "spec-label"
	// CauseProject is the Project's Auto-accept setting for the Ticket's
	// kind.
	CauseProject AutoAcceptCause = "project"
)

// AutoAcceptCauses lists every auto-accept cause.
var AutoAcceptCauses = []AutoAcceptCause{CauseTicketLabel, CauseSpecLabel, CauseProject}

// Valid reports whether c is one of the auto-accept causes.
func (c AutoAcceptCause) Valid() bool { return slices.Contains(AutoAcceptCauses, c) }

// StageGuides is a Project's Stage Guides settings (Spec
// code-central-441), each the opposite of a named optional guide line, so
// the zero value keeps every line in and a Project stored without the
// record needs no migration.
type StageGuides struct {
	// SubagentExplorationOff leaves the implementation guide's subagent
	// line out.
	SubagentExplorationOff bool
	// ModelAssignmentOn turns Model Assignment on. It is stored as on so
	// that a Project stored without it reads off (ADR 0059).
	ModelAssignmentOn bool
}

// CIResponse is the human's response to one pushed commit's CI result,
// stored once per commit on the Project, whatever Tickets wait on it
// (code-central-432): Land anyway, and a Re-run request with the failed
// run attempts it asked for. Before code-central-585 a response could be
// stored on the pushed commit itself; format 37 moved each one here.
// Repository is the Repository Name the commit was pushed in, empty for
// the Home Repository: a response is keyed by both.
type CIResponse struct {
	Repository string
	Commit     string
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

// LandingMethodNames is the LandingMethods as strings, for messages and
// the schema.
func LandingMethodNames() []string {
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
	// Rebase is the Merge and Pull Request option that replays a Ticket's
	// commits onto its target instead of merging its branch. It is off for
	// a Project that never stored it, and is kept whatever the method.
	Rebase bool
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

// CIProviderNames is the CIProviders as strings, for messages and the
// schema.
func CIProviderNames() []string {
	var out []string
	for _, p := range CIProviders {
		out = append(out, string(p))
	}
	return out
}

// CI is a Project's CI settings: whether CI Tracking is on, the name of
// the environment variable holding the Azure DevOps PAT (never the PAT),
// the Ignored workflows (each a workflow file's base name, such as
// ci.yml, or a pipeline's name), the URL of the Home Repository's Push
// remote the Factory last reported, or that it reported none, the CI
// Rules, and each Repository's CI entry, the Home's included (Spec
// code-central-890). The PAT environment variable, the Ignored workflows
// and the CI Rules are the Project's, for every Repository.
type CI struct {
	Tracking         bool
	PATEnv           string
	IgnoredWorkflows []string
	PushRemoteURL    string
	// NoPushRemote is whether the Factory last reported that the Home
	// Repository's clone has no Push remote, which makes it local-only;
	// PushRemoteURL is empty then. A Factory that never reports none
	// leaves it false, and the Home pushed.
	NoPushRemote bool
	// PushRemoteBase is the base branch a Factory reported before the base
	// became a Project setting. It is frozen: only Project.Base reads it,
	// as the fallback for a Landing with no Base.
	PushRemoteBase string
	Rules          []CIRule
	// Repositories are the Repositories' CI entries, the Home's included,
	// each named once; a Repository without one reads as filled in from
	// its Clone URL.
	Repositories []CIRepository
	// AutoFix is the Auto-fix setting (Spec code-central-1022). It is
	// kept while CI Tracking is off, but acts only while it is on.
	AutoFix AutoFix
}

// DefaultFixInstructions is the Fix instructions a Project that never
// set its own has.
const DefaultFixInstructions = "Work out what kind of failure this is before changing anything: a bug in the code, a broken test, or a flaky test. Then fix it the way that kind needs: fix the bug in the code, fix the test so it checks the right thing, or make the flaky test stable (never just retry or skip it). Compare against the last successful run: the commits since it are the likeliest cause (`git log <last green>..<failed commit>`). Look at other CI runs, on this branch and others, to see whether the failure is new, recurring or intermittent."

// The Auto-fix limit of a Project that never set one, and the range the
// human may set.
const (
	DefaultAutoFixLimit = 3
	MinAutoFixLimit     = 1
	MaxAutoFixLimit     = 10
)

// AutoFix is a Project's Auto-fix setting: whether it is on, the Fix
// instructions and the Auto-fix limit. Instructions empty and Limit 0
// are never set, and read as the defaults.
type AutoFix struct {
	On           bool
	Instructions string
	Limit        int
}

// FixInstructions is the Fix instructions in force.
func (a AutoFix) FixInstructions() string {
	if a.Instructions == "" {
		return DefaultFixInstructions
	}
	return a.Instructions
}

// AttemptLimit is the Auto-fix limit in force.
func (a AutoFix) AttemptLimit() int {
	if a.Limit == 0 {
		return DefaultAutoFixLimit
	}
	return a.Limit
}

// CIRepository is the CI entry of one Repository: its CI Provider and
// what that watches (the GitHub repository as owner/name, or the Azure
// DevOps project's URL), or Not watched, when its CI is never tracked.
type CIRepository struct {
	Name       string
	NotWatched bool
	Provider   CIProvider
	Repository string
	ProjectURL string
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
// Tracking off, with nothing named.
func DefaultCI() CI { return CI{} }

// Equal reports whether c and d are the same settings.
func (c CI) Equal(d CI) bool {
	return c.Tracking == d.Tracking && c.PATEnv == d.PATEnv &&
		slices.Equal(c.IgnoredWorkflows, d.IgnoredWorkflows) && c.PushRemoteURL == d.PushRemoteURL && c.NoPushRemote == d.NoPushRemote && c.PushRemoteBase == d.PushRemoteBase &&
		slices.EqualFunc(c.Rules, d.Rules, CIRule.Equal) && slices.Equal(c.Repositories, d.Repositories) && c.AutoFix == d.AutoFix
}

// GitHubIssues is a Project's GitHub Issues settings (Spec
// code-central-279): whether it is on, the repositories (owner/name) its
// issues come from, the filter label (empty for every issue) and whether
// New issues need me, both applying to every repository (Spec
// code-central-848), and the number of its Issues Spec, 0 until GitHub
// Issues is first turned on.
type GitHubIssues struct {
	On              bool
	Repositories    []string
	Label           string
	NewIssuesNeedMe bool
	Spec            int
}

// AzureBoards is a Project's Azure Boards settings (Spec
// code-central-894): whether it is on, the Mode that picks its work items
// with the settings of each Mode (a Saved query's URL, or a WIQL filter's
// project URL and WHERE clause), the environment variable holding the PAT
// (empty reads as the Project's CI PAT variable, else DefaultPATEnv),
// whether New issues need me, and the number of its Azure Boards Spec, 0
// until Azure Boards is first turned on. The settings of the Mode not
// chosen are kept, so switching back restores them.
type AzureBoards struct {
	On              bool
	Mode            AzureBoardsMode
	QueryURL        string
	ProjectURL      string
	Where           string
	PATEnv          string
	NewIssuesNeedMe bool
	// HideCodeCentral keeps Code Central out of what Write-back writes to a
	// work item.
	HideCodeCentral bool
	Spec            int
}

// AzureBoardsMode is how Azure Boards picks its work items.
type AzureBoardsMode string

// The Azure Boards Modes.
const (
	// AzureBoardsSavedQuery runs a saved Azure DevOps query, named by its
	// URL.
	AzureBoardsSavedQuery AzureBoardsMode = "saved_query"
	// AzureBoardsWIQL runs a WIQL WHERE clause in the Azure DevOps project
	// its URL names.
	AzureBoardsWIQL AzureBoardsMode = "wiql"
)

// AzureBoardsModes lists every AzureBoardsMode, the default first.
var AzureBoardsModes = []AzureBoardsMode{AzureBoardsSavedQuery, AzureBoardsWIQL}

// Valid reports whether m is one of the AzureBoardsModes.
func (m AzureBoardsMode) Valid() bool { return slices.Contains(AzureBoardsModes, m) }

// AzureBoardsModeNames is the AzureBoardsModes as strings, for messages
// and the schema.
func AzureBoardsModeNames() []string {
	var out []string
	for _, m := range AzureBoardsModes {
		out = append(out, string(m))
	}
	return out
}

// Models is a Project's Models rows (Spec code-central-301): the model
// and effort for the Project default and for each kind of work, as plain
// strings, each empty when the row leaves it unset. Chat is a row of its
// own (Spec code-central-633), not a kind of work, so ModelKinds stays
// equal to the guide names.
type Models struct {
	Default ModelRow
	Chat    ModelRow
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
var ModelKinds = []string{"grilling", "spec", "tickets", "map-chart", "map-grilling", "map-research", "map-task", "prototype", "retro", "implementation"}

// NeedsMeReasons are the Needs Me reasons a Project's Focus Kinds may
// turn off, in group order: core's Needs Me reasons, which core's tests
// keep in step.
var NeedsMeReasons = []string{"needs_answer", "answered", "silent_claim", "long_claim", "failed", "in_review",
	"ci_failed", "ci_cancelled", "base_diverged", "base_missing", "repository_missing", "pull_request_closed", "to_merge", "ci_unavailable",
	"accepted_not_landed", "blocked_by_cancelled", "new_issue", "closed_externally"}

// IsZero reports whether m sets nothing.
func (m Models) IsZero() bool {
	return m.Default == (ModelRow{}) && m.Chat == (ModelRow{}) && len(m.Kinds) == 0
}

// Equal reports whether m and n set the same rows.
func (m Models) Equal(n Models) bool {
	return m.Default == n.Default && m.Chat == n.Chat && slices.Equal(m.Kinds, n.Kinds)
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
	// Cause is why an auto-accept made the Move; empty on every other
	// Move. Its Actor is still human (ADR 0037).
	Cause AutoAcceptCause
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

// QuestionStatuses are the three Question statuses.
var QuestionStatuses = []QuestionStatus{QuestionOpen, QuestionAnswered, QuestionDismissed}

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
	// SpecMergeTicket marks a Spec merge Ticket: one Code Central opened
	// with open_spec_merge to merge the base into a Spec Branch
	// (code-central-542). Only that operation sets it; nothing clears it.
	SpecMergeTicket bool
	// AutoFix marks an auto fix Ticket: one Auto-fix opened for a CI
	// failed item (Spec code-central-1022). Only Code Central sets it;
	// nothing clears it. nil on every other Ticket, a manual fix Ticket
	// too.
	AutoFix     *AutoFixMark
	Created     time.Time
	Updated     time.Time
	Description string
	// Branch and Commit are what the last finish naming them gave: the
	// work's branch and its head commit in the Home Repository.
	// Free-form, never parsed.
	Branch string
	Commit string
	// Repositories are the Ticket's Changed Repositories (Spec
	// code-central-848): reported live by the Factory while it is In
	// Progress, and settled by each finish, the Home Repository's entry
	// mirroring Branch and Commit. A Ticket stored without them, with a
	// Branch or Commit, reads as the Home Repository's (ADR 0046).
	Repositories []ChangedRepository
	// Recommendation is the Spec recommendation the last finish of a spec
	// Stage gave; empty on any other Ticket and on a spec Stage finished
	// before there was one.
	Recommendation Recommendation
	// PushedCommits are the commits the Factory pushed for the Ticket
	// while CI Tracking was on, oldest first: the latest is the one its CI
	// result comes from, and the older ones stay as its history.
	PushedCommits []PushedCommit
	// SpecMerge is the Spec Branch commit the Factory merged the Ticket's
	// work into under Land per Spec, and when Code Central heard of it;
	// nil until then.
	SpecMerge *SpecMerge
	// PullRequests are the pull requests the Pull Request Watcher
	// reported for the Ticket's pushed work (code-central-388), one per
	// Repository at most (Spec code-central-848); none until then.
	PullRequests []PullRequest
	Claims       []Claim
	Rounds       []Round
	// FailureReports are numbered and never edited.
	FailureReports []FailureReport
	Comments       []Comment
	Moves          []Move
	Edits          []Edit
}

// Recommendation is a spec Stage's Spec recommendation: how its agent
// recommends the human accepts it.
type Recommendation string

// The two Spec recommendations.
const (
	// RecommendImplement is Implement: one implementation Ticket builds
	// the whole Spec.
	RecommendImplement Recommendation = "implement"
	// RecommendSplit is Split into Tickets: the tickets Stage runs.
	RecommendSplit Recommendation = "split"
)

// Recommendations lists every Spec recommendation.
var Recommendations = []Recommendation{RecommendImplement, RecommendSplit}

// Valid reports whether r is one of the Spec recommendations.
func (r Recommendation) Valid() bool { return slices.Contains(Recommendations, r) }

// ChangedRepository is one of a Ticket's Changed Repositories: the
// Repository Name, the Ticket's branch and its head commit there, and
// once its work has landed there, the landed commit.
type ChangedRepository struct {
	Name   string
	Branch string
	Commit string
	// Landed is nil until the Ticket's work lands in the Repository.
	Landed *Landed
}

// Landed is the commit that put a Ticket's work into a Repository's
// base, and when Code Central heard of it.
type Landed struct {
	Commit string
	Time   time.Time
}

// AutoFixMark is an auto fix Ticket's mark: its Auto-fix attempt, the
// pushed commit it was opened for, the Tickets it covers and the attempt
// before it.
type AutoFixMark struct {
	// Attempt is the Auto-fix attempt, 1 for the first in its chain.
	Attempt int
	// Repository is the Repository Name the commit was pushed in, empty
	// for the Home Repository, and Commit the commit it was opened for.
	Repository string
	Commit     string
	// Covers are the Tickets the CI failed item listed when it was
	// opened.
	Covers []int
	// Previous is the previous attempt's fix Ticket, 0 for attempt 1.
	Previous int
}

// SpecMerge is the Spec Branch commit a Ticket's work was merged into, and
// when Code Central heard of it.
type SpecMerge struct {
	// Repository is the Repository Name of the Spec Branch; empty for the
	// Home Repository.
	Repository string
	Commit     string
	Merged     time.Time
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
	// Repository is the Repository Name the pull request is in; empty
	// for the Home Repository.
	Repository  string
	Number      int
	Link        string
	State       PullRequestState
	MergeCommit string
	// Close marks an open pull request for the Pull Request Watcher to
	// close on the Code Host, as sending its Ticket back while none of
	// its siblings has merged does; it holds until the watcher reports
	// the pull request closed or merged.
	Close bool
}

// PushedCommit is one commit the Factory pushed for a Ticket, and when
// Code Central heard of it.
type PushedCommit struct {
	// Repository is the Repository Name the commit was pushed in; empty
	// for the Home Repository.
	Repository string
	Commit     string
	// Branch is the branch the commit was pushed as; empty for a push
	// reported without one.
	Branch string
	Pushed time.Time
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
// keyed by the Repository it ran for, the provider's run ID and the
// attempt. It is tracked against a Ticket or a CI Rule (the Spec number
// naming it). A run of an Ignored workflow is kept, marked Ignored.
type CIRun struct {
	// Repository is the Repository Name the run is for, empty for the
	// Home Repository.
	Repository string
	ID         int64
	Attempt    int
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

// CIRuns is a Project's CI Runs, written whole as one record, apart from
// its Tickets so frequent run updates don't rewrite Ticket files.
type CIRuns struct {
	Runs []CIRun
}

// FocusClear is what Focus Mode shows when nothing needs the human.
// FocusClears has the default first.
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

// FocusPopup is whether a popup shows each new Focus Item on every view
// but Focus Mode, Options and Project Options. FocusPopups has the
// default first.
type FocusPopup string

// The two values of the Focus Pop-up Option.
const (
	FocusPopupOff FocusPopup = "off"
	FocusPopupOn  FocusPopup = "on"
)

// FocusPopups lists every FocusPopup, the default first.
var FocusPopups = []FocusPopup{FocusPopupOff, FocusPopupOn}

// Valid reports whether f is one of the FocusPopups.
func (f FocusPopup) Valid() bool { return slices.Contains(FocusPopups, f) }

// FocusPopupNames is the FocusPopups as strings, for messages and the
// schema.
func FocusPopupNames() []string {
	var out []string
	for _, f := range FocusPopups {
		out = append(out, string(f))
	}
	return out
}

// LimitsPopup is whether the Limits popup shows a Project's Plan Limits.
// LimitsPopups has the default first.
type LimitsPopup string

// The two values of the Show Limits popup Option.
const (
	LimitsPopupOn  LimitsPopup = "on"
	LimitsPopupOff LimitsPopup = "off"
)

// LimitsPopups lists every LimitsPopup, the default first.
var LimitsPopups = []LimitsPopup{LimitsPopupOn, LimitsPopupOff}

// Valid reports whether l is one of the LimitsPopups.
func (l LimitsPopup) Valid() bool { return slices.Contains(LimitsPopups, l) }

// LimitsPopupNames is the LimitsPopups as strings, for messages and the
// schema.
func LimitsPopupNames() []string {
	var out []string
	for _, l := range LimitsPopups {
		out = append(out, string(l))
	}
	return out
}

// Options are the human's global preferences, set from the UI's Options
// view. They are the store's one record that belongs to no Project.
type Options struct {
	FocusClear FocusClear
	// FocusPopup is the Focus Pop-up Option.
	FocusPopup FocusPopup
	// LimitsPopup is the Show Limits popup Option.
	LimitsPopup LimitsPopup
	// GHPath is where gh, the GitHub CLI, is: the gh path Option. Empty
	// finds it on the service's PATH, then in the usual install places.
	GHPath string
}

// DefaultOptions are the Options of a store that has never had any
// written.
func DefaultOptions() Options {
	return Options{FocusClear: FocusClears[0], FocusPopup: FocusPopups[0], LimitsPopup: LimitsPopups[0]}
}

// FocusClearNames is the FocusClears as strings, for messages and the
// schema.
func FocusClearNames() []string {
	var out []string
	for _, f := range FocusClears {
		out = append(out, string(f))
	}
	return out
}
