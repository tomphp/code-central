// The files a record is stored in: their YAML shapes, decoding them
// strictly as load does, and encoding a record as write does.

package store

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/tomoram/code-central/domain"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type storeFile struct {
	Format int `yaml:"format"`
}

// optionsFile is options.yaml, the human's global Options. The file and
// every key are optional: what's left out takes its default.
type optionsFile struct {
	FocusClear string `yaml:"focus_clear,omitempty"`
	FocusPopup string `yaml:"focus_popup,omitempty"`
	// LimitsPopup is left out while it is the default.
	LimitsPopup string `yaml:"limits_popup,omitempty"`
	GHPath      string `yaml:"gh_path,omitempty"`
}

type projectFile struct {
	Key         string `yaml:"key"`
	DisplayName string `yaml:"display_name"`
	Created     string `yaml:"created"`
	Mode        string `yaml:"mode"`
	Archived    string `yaml:"archived,omitempty"`
	// Landing is left out while it is the default.
	Landing *landingFile `yaml:"landing,omitempty"`
	// CI is left out while it is the default.
	CI *ciFile `yaml:"ci,omitempty"`
	// GitHubIssues is left out while it is off with nothing set.
	GitHubIssues *githubIssuesFile `yaml:"github_issues,omitempty"`
	// AzureBoards is left out while it is off with nothing set.
	AzureBoards *azureBoardsFile `yaml:"azure_boards,omitempty"`
	// Models is left out while it sets nothing.
	Models *modelsFile `yaml:"models,omitempty"`
	// ReportedModels is left out while it holds nothing.
	ReportedModels *reportedModelsFile `yaml:"reported_models,omitempty"`
	// StageGuides is left out while every optional line is on.
	StageGuides *stageGuidesFile `yaml:"stage_guides,omitempty"`
	// FocusKindsOff is left out while every Needs Me reason is on.
	FocusKindsOff []string `yaml:"focus_kinds_off,omitempty"`
	// CIResponses is left out while there are none.
	CIResponses []ciResponseFile `yaml:"ci_responses,omitempty"`
	// CIFixesSpec is the CI Fixes Spec's number, left out until it exists.
	CIFixesSpec int `yaml:"ci_fixes_spec,omitempty"`
	// SmallChangesSpec is the Small Changes Spec's number, left out until it exists.
	SmallChangesSpec int `yaml:"small_changes_spec,omitempty"`
	// RetrosSpec is the Retros Spec's number, left out until it exists.
	RetrosSpec int `yaml:"retros_spec,omitempty"`
	// AutoAccept is the Auto-accept kinds that are on, left out while
	// every kind is off.
	AutoAccept []string `yaml:"auto_accept,omitempty"`
	// Repositories is left out while the Home Repository is the only one.
	Repositories []repositoryFile `yaml:"repositories,omitempty"`
}

// repositoryFile is one of a Project's Repositories, in landing order.
// The Home Repository's holds only its name, Own Landing and Push remote.
type repositoryFile struct {
	Name      string `yaml:"name"`
	Home      bool   `yaml:"home,omitempty"`
	CloneURL  string `yaml:"clone_url,omitempty"`
	Base      string `yaml:"base,omitempty"`
	LocalPath string `yaml:"local_path,omitempty"`
	// Landing is the Own Landing, left out while it follows the Default
	// Landing.
	Landing *ownLandingFile `yaml:"landing,omitempty"`
	// PushRemote is left out while it uses the Project's.
	PushRemote string `yaml:"push_remote,omitempty"`
}

// ownLandingFile is a Repository's Own Landing.
type ownLandingFile struct {
	Method string `yaml:"method"`
	Push   bool   `yaml:"push,omitempty"`
}

// stageGuidesFile is a Project's Stage Guides settings.
type stageGuidesFile struct {
	SubagentExplorationOff bool `yaml:"subagent_exploration_off,omitempty"`
	ModelAssignmentOn      bool `yaml:"model_assignment_on,omitempty"`
}

// ciResponseFile is the human's response to one pushed commit's CI
// result.
type ciResponseFile struct {
	// Repository is left out for the Home Repository.
	Repository string           `yaml:"repository,omitempty"`
	Commit     string           `yaml:"commit"`
	LandAnyway string           `yaml:"land_anyway,omitempty"`
	Rerun      string           `yaml:"rerun,omitempty"`
	Reruns     []runAttemptFile `yaml:"reruns,omitempty"`
	FixTicket  int              `yaml:"fix_ticket,omitempty"`
}

// reportedModelsFile is what the Factory reported its claude accepts.
type reportedModelsFile struct {
	EffortLevels []string `yaml:"effort_levels,omitempty"`
	Aliases      []string `yaml:"aliases,omitempty"`
	ModelIDs     []string `yaml:"model_ids,omitempty"`
	ModelFlag    string   `yaml:"model_flag,omitempty"`
	EffortFlag   string   `yaml:"effort_flag,omitempty"`
	Reported     string   `yaml:"reported,omitempty"`
}

// modelsFile is a Project's Models rows; a row that sets nothing is left
// out.
type modelsFile struct {
	Default *modelRowFile   `yaml:"default,omitempty"`
	Chat    *modelRowFile   `yaml:"chat,omitempty"`
	Kinds   []kindModelFile `yaml:"kinds,omitempty"`
}

type modelRowFile struct {
	Model  string `yaml:"model,omitempty"`
	Effort string `yaml:"effort,omitempty"`
}

type kindModelFile struct {
	Kind   string `yaml:"kind"`
	Model  string `yaml:"model,omitempty"`
	Effort string `yaml:"effort,omitempty"`
}

type githubIssuesFile struct {
	On              bool     `yaml:"on"`
	Repositories    []string `yaml:"repositories,omitempty"`
	Label           string   `yaml:"label,omitempty"`
	NewIssuesNeedMe bool     `yaml:"new_issues_need_me,omitempty"`
	Spec            int      `yaml:"spec,omitempty"`
}

type azureBoardsFile struct {
	On              bool   `yaml:"on"`
	Mode            string `yaml:"mode,omitempty"`
	QueryURL        string `yaml:"query_url,omitempty"`
	ProjectURL      string `yaml:"project_url,omitempty"`
	Where           string `yaml:"where,omitempty"`
	PATEnv          string `yaml:"pat_env,omitempty"`
	NewIssuesNeedMe bool   `yaml:"new_issues_need_me,omitempty"`
	HideCodeCentral bool   `yaml:"hide_code_central,omitempty"`
	Spec            int    `yaml:"spec,omitempty"`
}

type ciFile struct {
	Tracking bool `yaml:"tracking"`
	// Provider, Repository and ProjectURL are the Home Repository's CI
	// before it became a CI entry like the others' (Spec
	// code-central-890). They are read as the Home's entry, so a file
	// that still holds them loads, and never written.
	Provider         string       `yaml:"provider,omitempty"`
	Repository       string       `yaml:"repository,omitempty"`
	ProjectURL       string       `yaml:"project_url,omitempty"`
	PATEnv           string       `yaml:"pat_env,omitempty"`
	IgnoredWorkflows []string     `yaml:"ignored_workflows,omitempty"`
	PushRemoteURL    string       `yaml:"push_remote_url,omitempty"`
	NoPushRemote     bool         `yaml:"no_push_remote,omitempty"`
	PushRemoteBase   string       `yaml:"push_remote_base,omitempty"`
	Rules            []ciRuleFile `yaml:"rules,omitempty"`
	// UnblockOnPush is the removed Unblock on push option (ADR 0023,
	// superseded by ADR 0032). It is read so a file that still holds it loads, and
	// never kept or written.
	UnblockOnPush bool `yaml:"unblock_on_push,omitempty"`
	// Repositories is left out while no Repository has an entry.
	Repositories []ciRepositoryFile `yaml:"repositories,omitempty"`
	// AutoFix is left out while it is never set.
	AutoFix *autoFixFile `yaml:"auto_fix,omitempty"`
}

// autoFixFile is the Auto-fix setting; instructions and limit are left
// out while never set.
type autoFixFile struct {
	On           bool   `yaml:"on,omitempty"`
	Instructions string `yaml:"instructions,omitempty"`
	Limit        int    `yaml:"limit,omitempty"`
}

// ciRepositoryFile is the CI entry of one Repository, the Home's
// included.
type ciRepositoryFile struct {
	Name       string `yaml:"name"`
	NotWatched bool   `yaml:"not_watched,omitempty"`
	Provider   string `yaml:"provider,omitempty"`
	Repository string `yaml:"repository,omitempty"`
	ProjectURL string `yaml:"project_url,omitempty"`
}

type ciRuleFile struct {
	Spec     int    `yaml:"spec"`
	Workflow string `yaml:"workflow"`
	Branch   string `yaml:"branch"`
	State    string `yaml:"state"`
	Created  string `yaml:"created"`
}

type landingFile struct {
	Method string `yaml:"method"`
	Push   bool   `yaml:"push"`
	Remote string `yaml:"remote"`
	// LandPerSpec is a pointer so a file without it, which reads as on,
	// differs from false.
	LandPerSpec *bool `yaml:"land_per_spec,omitempty"`
	// Rebase is left out while it is off.
	Rebase bool `yaml:"rebase,omitempty"`
	// PATEnv is left out while it is unset.
	PATEnv string `yaml:"pat_env,omitempty"`
	// Base is left out while it is unset.
	Base string `yaml:"base,omitempty"`
}

type specFront struct {
	ID      int               `yaml:"id"`
	Title   string            `yaml:"title"`
	Labels  []string          `yaml:"labels"`
	Tags    map[string]string `yaml:"tags"`
	Link    *linkFile         `yaml:"link,omitempty"`
	Created string            `yaml:"created"`
	Updated string            `yaml:"updated"`
	Edits   []editFile        `yaml:"edits"`
}

type ticketFile struct {
	Title           string              `yaml:"title"`
	State           string              `yaml:"state"`
	Spec            int                 `yaml:"spec"`
	Labels          []string            `yaml:"labels"`
	Tags            map[string]string   `yaml:"tags"`
	BlockedBy       []int               `yaml:"blocked_by"`
	Replacements    []int               `yaml:"replacements"`
	Link            *linkFile           `yaml:"link,omitempty"`
	AwaitingTriage  bool                `yaml:"awaiting_triage,omitempty"`
	SpecMergeTicket bool                `yaml:"spec_merge_ticket,omitempty"`
	AutoFix         *autoFixMarkFile    `yaml:"auto_fix,omitempty"`
	Created         string              `yaml:"created"`
	Updated         string              `yaml:"updated"`
	Description     string              `yaml:"description"`
	Branch          string              `yaml:"branch,omitempty"`
	Commit          string              `yaml:"commit,omitempty"`
	Repositories    []changedRepoFile   `yaml:"repositories,omitempty"`
	Recommendation  string              `yaml:"recommendation,omitempty"`
	PushedCommits   []pushedCommitFile  `yaml:"pushed_commits,omitempty"`
	SpecMerge       *specMergeFile      `yaml:"spec_merge,omitempty"`
	PullRequest     *pullRequestFile    `yaml:"pull_request,omitempty"`
	PullRequests    []pullRequestFile   `yaml:"pull_requests,omitempty"`
	Claims          []claimFile         `yaml:"claims"`
	Rounds          []roundFile         `yaml:"rounds"`
	FailureReports  []failureReportFile `yaml:"failure_reports"`
	Comments        []commentFile       `yaml:"comments"`
	Moves           []moveFile          `yaml:"moves"`
	Edits           []editFile          `yaml:"edits"`
}

// autoFixMarkFile is an auto fix Ticket's Auto-fix mark.
type autoFixMarkFile struct {
	Attempt    int    `yaml:"attempt"`
	Repository string `yaml:"repository,omitempty"`
	Commit     string `yaml:"commit"`
	Covers     []int  `yaml:"covers"`
	Previous   int    `yaml:"previous,omitempty"`
}

// changedRepoFile is one of a Ticket's Changed Repositories.
type changedRepoFile struct {
	Name   string      `yaml:"name"`
	Branch string      `yaml:"branch,omitempty"`
	Commit string      `yaml:"commit,omitempty"`
	Landed *landedFile `yaml:"landed,omitempty"`
}

type landedFile struct {
	Commit string `yaml:"commit"`
	Time   string `yaml:"time"`
}

type specMergeFile struct {
	Repository string `yaml:"repository,omitempty"`
	Commit     string `yaml:"commit"`
	Merged     string `yaml:"merged"`
}

type pullRequestFile struct {
	Repository  string `yaml:"repository,omitempty"`
	Number      int    `yaml:"number"`
	Link        string `yaml:"link"`
	State       string `yaml:"state"`
	MergeCommit string `yaml:"merge_commit,omitempty"`
	Close       bool   `yaml:"close,omitempty"`
}

type pushedCommitFile struct {
	Repository string `yaml:"repository,omitempty"`
	Commit     string `yaml:"commit"`
	Branch     string `yaml:"branch,omitempty"`
	Pushed     string `yaml:"pushed"`
}

type runAttemptFile struct {
	ID      int64 `yaml:"id"`
	Attempt int   `yaml:"attempt"`
}

// ciRunsFile is projects/<key>/ci-runs.yaml, the Project's CI Runs.
type ciRunsFile struct {
	Runs []ciRunFile `yaml:"runs"`
}

type ciRunFile struct {
	// Repository is left out for the Home Repository.
	Repository string      `yaml:"repository,omitempty"`
	ID         int64       `yaml:"id"`
	Attempt    int         `yaml:"attempt"`
	Workflow   string      `yaml:"workflow"`
	Name       string      `yaml:"name"`
	Branch     string      `yaml:"branch"`
	Commit     string      `yaml:"commit"`
	Status     string      `yaml:"status"`
	Conclusion string      `yaml:"conclusion,omitempty"`
	Started    string      `yaml:"started,omitempty"`
	Finished   string      `yaml:"finished,omitempty"`
	URL        string      `yaml:"url"`
	Jobs       []ciJobFile `yaml:"jobs,omitempty"`
	Ticket     int         `yaml:"ticket,omitempty"`
	Rule       int         `yaml:"rule,omitempty"`
	Ignored    bool        `yaml:"ignored,omitempty"`
}

type ciJobFile struct {
	Name       string `yaml:"name"`
	Conclusion string `yaml:"conclusion,omitempty"`
	URL        string `yaml:"url"`
}

type editFile struct {
	Number   int               `yaml:"number"`
	Actor    string            `yaml:"actor"`
	Time     string            `yaml:"time"`
	Previous map[string]string `yaml:"previous"`
}

type claimFile struct {
	Number        int    `yaml:"number"`
	ID            string `yaml:"id"`
	SessionID     string `yaml:"session_id,omitempty"`
	WorkingDir    string `yaml:"working_dir,omitempty"`
	ResumeCommand string `yaml:"resume_command,omitempty"`
	Model         string `yaml:"model,omitempty"`
	Effort        string `yaml:"effort,omitempty"`
	Started       string `yaml:"started"`
	Ended         string `yaml:"ended,omitempty"`
	EndedBy       string `yaml:"ended_by,omitempty"`
}

type roundFile struct {
	Number    int            `yaml:"number"`
	Claim     int            `yaml:"claim"`
	SessionID string         `yaml:"session_id,omitempty"`
	Posted    string         `yaml:"posted"`
	Questions []questionFile `yaml:"questions"`
}

type questionFile struct {
	Number      int    `yaml:"number"`
	Title       string `yaml:"title"`
	Body        string `yaml:"body"`
	Recommended string `yaml:"recommended,omitempty"`
	Status      string `yaml:"status"`
	Answer      string `yaml:"answer,omitempty"`
	Closed      string `yaml:"closed,omitempty"`
}

type failureReportFile struct {
	Number          int    `yaml:"number"`
	Claim           int    `yaml:"claim"`
	SessionID       string `yaml:"session_id,omitempty"`
	WorkingDir      string `yaml:"working_dir,omitempty"`
	ResumeCommand   string `yaml:"resume_command,omitempty"`
	Actor           string `yaml:"actor"`
	Time            string `yaml:"time"`
	WhatBroke       string `yaml:"what_broke"`
	Why             string `yaml:"why"`
	WhatToFix       string `yaml:"what_to_fix"`
	WhereWorkStands string `yaml:"where_work_stands"`
}

type commentFile struct {
	Number int    `yaml:"number"`
	Actor  string `yaml:"actor"`
	Time   string `yaml:"time"`
	Claim  int    `yaml:"claim,omitempty"`
	Body   string `yaml:"body"`
}

type moveFile struct {
	From   string `yaml:"from,omitempty"` // empty only on an imported Ticket's one move
	To     string `yaml:"to"`
	Actor  string `yaml:"actor"`
	Time   string `yaml:"time"`
	Claim  int    `yaml:"claim,omitempty"`
	Via    string `yaml:"via,omitempty"`
	Commit string `yaml:"commit,omitempty"` // a land's commit
	Cause  string `yaml:"cause,omitempty"`  // an auto-accept's cause
}

// The fields an edit's previous values may name.
var (
	// external_state and detached record a Link's changes; spec is a
	// linked Ticket's parent, which only its Integration changes.
	specEditFields   = []string{"title", "body", "external_state", "detached"}
	ticketEditFields = []string{"title", "description", "spec", "external_state", "detached"}
)

func decodeSpec(data []byte, n int) (domain.Spec, *LoadError) {
	front, body, ok := splitFrontmatter(data)
	if !ok {
		return domain.Spec{}, &LoadError{Field: "frontmatter", Msg: "a Spec must start with YAML frontmatter between --- lines"}
	}
	var f specFront
	if field, err := decodeStrict(front, &f); err != nil {
		return domain.Spec{}, &LoadError{Field: field, Msg: err.Error()}
	}
	if f.ID != n {
		return domain.Spec{}, &LoadError{Field: "id", Msg: fmt.Sprintf("id %d does not match the file name", f.ID)}
	}
	if f.Title == "" {
		return domain.Spec{}, &LoadError{Field: "title", Msg: "required"}
	}
	sp := domain.Spec{Number: n, Title: f.Title, Labels: f.Labels, Tags: f.Tags, Body: string(body)}
	var lerr *LoadError
	if sp.Link, lerr = decodeLink(f.Link); lerr != nil {
		return domain.Spec{}, lerr
	}
	var err error
	if sp.Created, err = parseTime(f.Created); err != nil {
		return domain.Spec{}, &LoadError{Field: "created", Msg: err.Error()}
	}
	if sp.Updated, err = parseTime(f.Updated); err != nil {
		return domain.Spec{}, &LoadError{Field: "updated", Msg: err.Error()}
	}
	if sp.Edits, err = decodeEdits(f.Edits, specEditFields...); err != nil {
		return domain.Spec{}, err.(*LoadError)
	}
	return sp, nil
}

func decodeTicket(data []byte, n int) (domain.Ticket, *LoadError) {
	var f ticketFile
	if field, err := decodeStrict(data, &f); err != nil {
		return domain.Ticket{}, &LoadError{Field: field, Msg: err.Error()}
	}
	t := domain.Ticket{
		Number: n, Title: f.Title, State: domain.State(f.State), Spec: f.Spec, Labels: f.Labels, Tags: f.Tags,
		BlockedBy: f.BlockedBy, Replacements: f.Replacements, AwaitingTriage: f.AwaitingTriage, SpecMergeTicket: f.SpecMergeTicket, Description: f.Description,
		Branch: f.Branch, Commit: f.Commit, Recommendation: domain.Recommendation(f.Recommendation),
	}
	if t.Title == "" {
		return domain.Ticket{}, &LoadError{Field: "title", Msg: "required"}
	}
	if !t.State.Valid() {
		return domain.Ticket{}, &LoadError{Field: "state", Msg: fmt.Sprintf("unknown State %q", f.State)}
	}
	if t.Spec < 1 {
		return domain.Ticket{}, &LoadError{Field: "spec", Msg: "required"}
	}
	if t.Recommendation != "" && !t.Recommendation.Valid() {
		return domain.Ticket{}, &LoadError{Field: "recommendation", Msg: fmt.Sprintf("unknown Spec recommendation %q", f.Recommendation)}
	}
	var lerr *LoadError
	if t.Link, lerr = decodeLink(f.Link); lerr != nil {
		return domain.Ticket{}, lerr
	}
	var err error
	if t.Created, err = parseTime(f.Created); err != nil {
		return domain.Ticket{}, &LoadError{Field: "created", Msg: err.Error()}
	}
	if t.Updated, err = parseTime(f.Updated); err != nil {
		return domain.Ticket{}, &LoadError{Field: "updated", Msg: err.Error()}
	}
	if t.Repositories, lerr = decodeChangedRepositories(f.Repositories); lerr != nil {
		return domain.Ticket{}, lerr
	}
	if a := f.AutoFix; a != nil {
		if a.Attempt < 1 {
			return domain.Ticket{}, &LoadError{Field: "auto_fix.attempt", Msg: fmt.Sprintf("an Auto-fix attempt is 1 or more, not %d", a.Attempt)}
		}
		if lerr := checkRepositoryName("auto_fix.repository", a.Repository); lerr != nil {
			return domain.Ticket{}, lerr
		}
		if a.Commit == "" {
			return domain.Ticket{}, &LoadError{Field: "auto_fix.commit", Msg: "required"}
		}
		t.AutoFix = &domain.AutoFixMark{Attempt: a.Attempt, Repository: a.Repository, Commit: a.Commit, Covers: a.Covers, Previous: a.Previous}
	}
	for i, pc := range f.PushedCommits {
		p := domain.PushedCommit{Repository: pc.Repository, Commit: pc.Commit, Branch: pc.Branch}
		if lerr := checkRepositoryName(fmt.Sprintf("pushed_commits[%d].repository", i), p.Repository); lerr != nil {
			return domain.Ticket{}, lerr
		}
		if p.Commit == "" {
			return domain.Ticket{}, &LoadError{Field: fmt.Sprintf("pushed_commits[%d].commit", i), Msg: "required"}
		}
		if p.Pushed, err = parseTime(pc.Pushed); err != nil {
			return domain.Ticket{}, &LoadError{Field: fmt.Sprintf("pushed_commits[%d].pushed", i), Msg: err.Error()}
		}
		t.PushedCommits = append(t.PushedCommits, p)
	}
	// The Home Repository's pull request is pull_request, as it always
	// was, and the other Repositories' are pull_requests.
	if f.PullRequest != nil {
		if lerr := decodePullRequest(&t, "pull_request", *f.PullRequest); lerr != nil {
			return domain.Ticket{}, lerr
		}
	}
	for i, pf := range f.PullRequests {
		if lerr := decodePullRequest(&t, fmt.Sprintf("pull_requests[%d]", i), pf); lerr != nil {
			return domain.Ticket{}, lerr
		}
	}
	if f.SpecMerge != nil {
		sm := &domain.SpecMerge{Repository: f.SpecMerge.Repository, Commit: f.SpecMerge.Commit}
		if lerr := checkRepositoryName("spec_merge.repository", sm.Repository); lerr != nil {
			return domain.Ticket{}, lerr
		}
		if sm.Commit == "" {
			return domain.Ticket{}, &LoadError{Field: "spec_merge.commit", Msg: "required"}
		}
		if sm.Merged, err = parseTime(f.SpecMerge.Merged); err != nil {
			return domain.Ticket{}, &LoadError{Field: "spec_merge.merged", Msg: err.Error()}
		}
		t.SpecMerge = sm
	}
	for i, c := range f.Claims {
		cl := domain.Claim{Number: c.Number, ID: c.ID, SessionID: c.SessionID, WorkingDir: c.WorkingDir, ResumeCommand: c.ResumeCommand, Model: c.Model, Effort: c.Effort, EndedBy: c.EndedBy}
		field := fmt.Sprintf("claims[%d]", i)
		if cl.ID == "" {
			return domain.Ticket{}, &LoadError{Field: field + ".id", Msg: "required"}
		}
		if cl.Started, err = parseTime(c.Started); err != nil {
			return domain.Ticket{}, &LoadError{Field: field + ".started", Msg: err.Error()}
		}
		if c.Ended != "" {
			if cl.Ended, err = parseTime(c.Ended); err != nil {
				return domain.Ticket{}, &LoadError{Field: field + ".ended", Msg: err.Error()}
			}
		}
		t.Claims = append(t.Claims, cl)
	}
	if t.Rounds, err = decodeRounds(f.Rounds); err != nil {
		return domain.Ticket{}, err.(*LoadError)
	}
	for i, r := range f.FailureReports {
		fr := domain.FailureReport{
			Number: r.Number, Claim: r.Claim, SessionID: r.SessionID, WorkingDir: r.WorkingDir, ResumeCommand: r.ResumeCommand, Actor: r.Actor,
			WhatBroke: r.WhatBroke, Why: r.Why, WhatToFix: r.WhatToFix, WhereWorkStands: r.WhereWorkStands,
		}
		if fr.Time, err = parseTime(r.Time); err != nil {
			return domain.Ticket{}, &LoadError{Field: fmt.Sprintf("failure_reports[%d].time", i), Msg: err.Error()}
		}
		t.FailureReports = append(t.FailureReports, fr)
	}
	for i, c := range f.Comments {
		cm := domain.Comment{Number: c.Number, Actor: c.Actor, Claim: c.Claim, Body: c.Body}
		if cm.Time, err = parseTime(c.Time); err != nil {
			return domain.Ticket{}, &LoadError{Field: fmt.Sprintf("comments[%d].time", i), Msg: err.Error()}
		}
		t.Comments = append(t.Comments, cm)
	}
	for i, m := range f.Moves {
		mv := domain.Move{From: domain.State(m.From), To: domain.State(m.To), Actor: m.Actor, Claim: m.Claim, Via: m.Via, Commit: m.Commit, Cause: domain.AutoAcceptCause(m.Cause)}
		if mv.Cause != "" && !mv.Cause.Valid() {
			return domain.Ticket{}, &LoadError{Field: fmt.Sprintf("moves[%d].cause", i), Msg: fmt.Sprintf("unknown auto-accept cause %q", m.Cause)}
		}
		field := fmt.Sprintf("moves[%d]", i)
		if m.From != "" && !mv.From.Valid() {
			return domain.Ticket{}, &LoadError{Field: field + ".from", Msg: fmt.Sprintf("unknown State %q", m.From)}
		}
		if !mv.To.Valid() {
			return domain.Ticket{}, &LoadError{Field: field + ".to", Msg: fmt.Sprintf("unknown State %q", m.To)}
		}
		if mv.Time, err = parseTime(m.Time); err != nil {
			return domain.Ticket{}, &LoadError{Field: field + ".time", Msg: err.Error()}
		}
		t.Moves = append(t.Moves, mv)
	}
	if t.Edits, err = decodeEdits(f.Edits, ticketEditFields...); err != nil {
		return domain.Ticket{}, err.(*LoadError)
	}
	return t, nil
}

// decodeCIRuns decodes ci-runs.yaml, checking each run's status and
// conclusions.
func decodeCIRuns(data []byte) ([]domain.CIRun, *LoadError) {
	var f ciRunsFile
	if field, err := decodeStrict(data, &f); err != nil {
		return nil, &LoadError{Field: field, Msg: err.Error()}
	}
	var out []domain.CIRun
	for i, rf := range f.Runs {
		field := fmt.Sprintf("runs[%d]", i)
		r := domain.CIRun{Repository: rf.Repository, ID: rf.ID, Attempt: rf.Attempt, Workflow: rf.Workflow, Name: rf.Name, Branch: rf.Branch, Commit: rf.Commit,
			Status: domain.CIRunStatus(rf.Status), Conclusion: domain.CIConclusion(rf.Conclusion), URL: rf.URL, Ticket: rf.Ticket, Rule: rf.Rule, Ignored: rf.Ignored}
		if !r.Status.Valid() {
			return nil, &LoadError{Field: field + ".status", Msg: fmt.Sprintf("unknown CI Run status %q", rf.Status)}
		}
		if !r.Conclusion.Valid() {
			return nil, &LoadError{Field: field + ".conclusion", Msg: fmt.Sprintf("unknown CI conclusion %q", rf.Conclusion)}
		}
		var err error
		if rf.Started != "" {
			if r.Started, err = parseTime(rf.Started); err != nil {
				return nil, &LoadError{Field: field + ".started", Msg: err.Error()}
			}
		}
		if rf.Finished != "" {
			if r.Finished, err = parseTime(rf.Finished); err != nil {
				return nil, &LoadError{Field: field + ".finished", Msg: err.Error()}
			}
		}
		for j, jf := range rf.Jobs {
			job := domain.CIJob{Name: jf.Name, Conclusion: domain.CIConclusion(jf.Conclusion), URL: jf.URL}
			if !job.Conclusion.Valid() {
				return nil, &LoadError{Field: fmt.Sprintf("%s.jobs[%d].conclusion", field, j), Msg: fmt.Sprintf("unknown CI conclusion %q", jf.Conclusion)}
			}
			r.Jobs = append(r.Jobs, job)
		}
		out = append(out, r)
	}
	return out, nil
}

// decodeChangedRepositories is a Ticket's Changed Repositories, each
// named by a valid Repository Name, once.
func decodeChangedRepositories(fs []changedRepoFile) ([]domain.ChangedRepository, *LoadError) {
	var out []domain.ChangedRepository
	for i, f := range fs {
		field := fmt.Sprintf("repositories[%d]", i)
		if f.Name == "" {
			return nil, &LoadError{Field: field + ".name", Msg: "required"}
		}
		if lerr := checkRepositoryName(field+".name", f.Name); lerr != nil {
			return nil, lerr
		}
		if slices.ContainsFunc(out, func(r domain.ChangedRepository) bool { return r.Name == f.Name }) {
			return nil, &LoadError{Field: field + ".name", Msg: fmt.Sprintf("%q is listed twice", f.Name)}
		}
		r := domain.ChangedRepository{Name: f.Name, Branch: f.Branch, Commit: f.Commit}
		if f.Landed != nil {
			l := &domain.Landed{Commit: f.Landed.Commit}
			if l.Commit == "" {
				return nil, &LoadError{Field: field + ".landed.commit", Msg: "required"}
			}
			var err error
			if l.Time, err = parseTime(f.Landed.Time); err != nil {
				return nil, &LoadError{Field: field + ".landed.time", Msg: err.Error()}
			}
			r.Landed = l
		}
		out = append(out, r)
	}
	return out, nil
}

// decodePullRequest adds f, a pull request stored at field, to t's,
// refusing a bad state and a second pull request in one Repository.
func decodePullRequest(t *domain.Ticket, field string, f pullRequestFile) *LoadError {
	pr := domain.PullRequest{Repository: f.Repository, Number: f.Number, Link: f.Link, State: domain.PullRequestState(f.State), MergeCommit: f.MergeCommit, Close: f.Close}
	if lerr := checkRepositoryName(field+".repository", pr.Repository); lerr != nil {
		return lerr
	}
	if !slices.Contains(domain.PullRequestStates, pr.State) {
		return &LoadError{Field: field + ".state", Msg: fmt.Sprintf("%q is not one of %v", pr.State, domain.PullRequestStates)}
	}
	if slices.ContainsFunc(t.PullRequests, func(o domain.PullRequest) bool { return o.Repository == pr.Repository }) {
		return &LoadError{Field: field + ".repository", Msg: fmt.Sprintf("a second pull request in %q", pr.Repository)}
	}
	t.PullRequests = append(t.PullRequests, pr)
	return nil
}

// checkRepositoryName refuses a Repository Name that breaks the rules;
// an empty one, the Home Repository's, passes.
func checkRepositoryName(field, name string) *LoadError {
	if name != "" && !domain.ValidRepositoryName(name) {
		return &LoadError{Field: field, Msg: fmt.Sprintf("%q is not a Repository Name", name)}
	}
	return nil
}

func decodeRounds(rs []roundFile) ([]domain.Round, error) {
	var out []domain.Round
	for i, r := range rs {
		field := fmt.Sprintf("rounds[%d]", i)
		rd := domain.Round{Number: r.Number, Claim: r.Claim, SessionID: r.SessionID}
		var err error
		if rd.Posted, err = parseTime(r.Posted); err != nil {
			return nil, &LoadError{Field: field + ".posted", Msg: err.Error()}
		}
		for j, q := range r.Questions {
			qf := fmt.Sprintf("%s.questions[%d]", field, j)
			qu := domain.Question{Number: q.Number, Title: q.Title, Body: q.Body, Recommended: q.Recommended, Status: domain.QuestionStatus(q.Status), Answer: q.Answer}
			switch qu.Status {
			case domain.QuestionOpen, domain.QuestionAnswered, domain.QuestionDismissed:
			default:
				return nil, &LoadError{Field: qf + ".status", Msg: fmt.Sprintf("unknown status %q (open, answered or dismissed)", q.Status)}
			}
			if q.Closed != "" {
				if qu.Closed, err = parseTime(q.Closed); err != nil {
					return nil, &LoadError{Field: qf + ".closed", Msg: err.Error()}
				}
			}
			rd.Questions = append(rd.Questions, qu)
		}
		out = append(out, rd)
	}
	return out, nil
}

// decodeEdits checks an edits list, whose previous values may name only
// the given fields.
func decodeEdits(fs []editFile, fields ...string) ([]domain.Edit, error) {
	var out []domain.Edit
	for i, e := range fs {
		ed := domain.Edit{Number: e.Number, Actor: e.Actor, Previous: e.Previous}
		var err error
		if ed.Time, err = parseTime(e.Time); err != nil {
			return nil, &LoadError{Field: fmt.Sprintf("edits[%d].time", i), Msg: err.Error()}
		}
		for k := range e.Previous {
			if !slices.Contains(fields, k) {
				return nil, &LoadError{Field: fmt.Sprintf("edits[%d].previous.%s", i, k), Msg: "unknown field"}
			}
		}
		out = append(out, ed)
	}
	return out, nil
}

func encodeEdits(es []domain.Edit) []editFile {
	out := []editFile{}
	for _, e := range es {
		out = append(out, editFile{Number: e.Number, Actor: e.Actor, Time: formatTime(e.Time), Previous: e.Previous})
	}
	return out
}

// encode turns one record into its file: a path relative to the store
// directory, and the file's contents.
func encode(record any, project string) (string, []byte, error) {
	var rel string
	var data []byte
	var err error
	switch r := record.(type) {
	case domain.Project:
		rel = filepath.Join("projects", r.Key, "project.yaml")
		data, err = encodeYAML(projectFile{Key: r.Key, DisplayName: r.DisplayName, Created: formatTime(r.Created), Mode: string(r.Mode), Archived: formatOptionalTime(r.Archived), Landing: encodeLanding(r.Landing), CI: encodeCI(r.CI),
			GitHubIssues: encodeGitHubIssues(r.GitHubIssues), AzureBoards: encodeAzureBoards(r.AzureBoards), Models: encodeModels(r.Models),
			ReportedModels: encodeReportedModels(r.ReportedModels), StageGuides: encodeStageGuides(r.StageGuides), FocusKindsOff: r.FocusKindsOff, CIResponses: encodeCIResponses(r.CIResponses), CIFixesSpec: r.CIFixesSpec, SmallChangesSpec: r.SmallChangesSpec, RetrosSpec: r.RetrosSpec, AutoAccept: encodeAutoAccept(r.AutoAccept),
			Repositories: encodeRepositories(r.Repositories)})
	case domain.Options:
		rel = "options.yaml"
		f := optionsFile{FocusClear: string(r.FocusClear), FocusPopup: string(r.FocusPopup), GHPath: r.GHPath}
		if r.LimitsPopup != domain.DefaultOptions().LimitsPopup {
			f.LimitsPopup = string(r.LimitsPopup)
		}
		data, err = encodeYAML(f)
	case domain.Spec:
		rel = filepath.Join("projects", project, "specs", strconv.Itoa(r.Number)+".md")
		var front []byte
		front, err = encodeYAML(specFront{ID: r.Number, Title: r.Title, Labels: r.Labels, Tags: r.Tags, Link: encodeLink(r.Link), Created: formatTime(r.Created), Updated: formatTime(r.Updated), Edits: encodeEdits(r.Edits)})
		data = append(append(append([]byte("---\n"), front...), "---\n"...), r.Body...)
	case domain.Ticket:
		rel = filepath.Join("projects", project, "tickets", strconv.Itoa(r.Number)+".yaml")
		f := ticketFile{
			Title: r.Title, State: string(r.State), Spec: r.Spec, Labels: r.Labels, Tags: r.Tags,
			BlockedBy: r.BlockedBy, Replacements: r.Replacements, Link: encodeLink(r.Link), AwaitingTriage: r.AwaitingTriage,
			SpecMergeTicket: r.SpecMergeTicket, Created: formatTime(r.Created), Updated: formatTime(r.Updated), Description: r.Description,
			Branch: r.Branch, Commit: r.Commit, Recommendation: string(r.Recommendation),
			Claims: []claimFile{}, Rounds: []roundFile{}, FailureReports: []failureReportFile{},
			Comments: []commentFile{}, Moves: []moveFile{}, Edits: encodeEdits(r.Edits),
		}
		if a := r.AutoFix; a != nil {
			f.AutoFix = &autoFixMarkFile{Attempt: a.Attempt, Repository: a.Repository, Commit: a.Commit, Covers: a.Covers, Previous: a.Previous}
			if f.AutoFix.Covers == nil {
				f.AutoFix.Covers = []int{}
			}
		}
		for _, cr := range r.Repositories {
			cf := changedRepoFile{Name: cr.Name, Branch: cr.Branch, Commit: cr.Commit}
			if cr.Landed != nil {
				cf.Landed = &landedFile{Commit: cr.Landed.Commit, Time: formatTime(cr.Landed.Time)}
			}
			f.Repositories = append(f.Repositories, cf)
		}
		for _, pc := range r.PushedCommits {
			f.PushedCommits = append(f.PushedCommits, pushedCommitFile{Repository: pc.Repository, Commit: pc.Commit, Branch: pc.Branch, Pushed: formatTime(pc.Pushed)})
		}
		for _, pr := range r.PullRequests {
			pf := pullRequestFile{Repository: pr.Repository, Number: pr.Number, Link: pr.Link, State: string(pr.State), MergeCommit: pr.MergeCommit, Close: pr.Close}
			if pr.Repository == "" && f.PullRequest == nil {
				f.PullRequest = &pf
			} else {
				f.PullRequests = append(f.PullRequests, pf)
			}
		}
		if r.SpecMerge != nil {
			f.SpecMerge = &specMergeFile{Repository: r.SpecMerge.Repository, Commit: r.SpecMerge.Commit, Merged: formatTime(r.SpecMerge.Merged)}
		}
		for _, c := range r.Claims {
			cf := claimFile{Number: c.Number, ID: c.ID, SessionID: c.SessionID, WorkingDir: c.WorkingDir, ResumeCommand: c.ResumeCommand, Model: c.Model, Effort: c.Effort, Started: formatTime(c.Started), EndedBy: c.EndedBy}
			if !c.Ended.IsZero() {
				cf.Ended = formatTime(c.Ended)
			}
			f.Claims = append(f.Claims, cf)
		}
		for _, rd := range r.Rounds {
			rf := roundFile{Number: rd.Number, Claim: rd.Claim, SessionID: rd.SessionID, Posted: formatTime(rd.Posted), Questions: []questionFile{}}
			for _, q := range rd.Questions {
				qf := questionFile{Number: q.Number, Title: q.Title, Body: q.Body, Recommended: q.Recommended, Status: string(q.Status), Answer: q.Answer}
				if !q.Closed.IsZero() {
					qf.Closed = formatTime(q.Closed)
				}
				rf.Questions = append(rf.Questions, qf)
			}
			f.Rounds = append(f.Rounds, rf)
		}
		for _, fr := range r.FailureReports {
			f.FailureReports = append(f.FailureReports, failureReportFile{
				Number: fr.Number, Claim: fr.Claim, SessionID: fr.SessionID, WorkingDir: fr.WorkingDir, ResumeCommand: fr.ResumeCommand,
				Actor: fr.Actor, Time: formatTime(fr.Time),
				WhatBroke: fr.WhatBroke, Why: fr.Why, WhatToFix: fr.WhatToFix, WhereWorkStands: fr.WhereWorkStands,
			})
		}
		for _, c := range r.Comments {
			f.Comments = append(f.Comments, commentFile{Number: c.Number, Actor: c.Actor, Time: formatTime(c.Time), Claim: c.Claim, Body: c.Body})
		}
		for _, m := range r.Moves {
			f.Moves = append(f.Moves, moveFile{From: string(m.From), To: string(m.To), Actor: m.Actor, Time: formatTime(m.Time), Claim: m.Claim, Via: m.Via, Commit: m.Commit, Cause: string(m.Cause)})
		}
		data, err = encodeYAML(f)
	case domain.CIRuns:
		rel = filepath.Join("projects", project, ciRunsName)
		f := ciRunsFile{Runs: []ciRunFile{}}
		for _, run := range r.Runs {
			rf := ciRunFile{Repository: run.Repository, ID: run.ID, Attempt: run.Attempt, Workflow: run.Workflow, Name: run.Name, Branch: run.Branch, Commit: run.Commit,
				Status: string(run.Status), Conclusion: string(run.Conclusion), Started: formatOptionalTime(run.Started), Finished: formatOptionalTime(run.Finished),
				URL: run.URL, Ticket: run.Ticket, Rule: run.Rule, Ignored: run.Ignored}
			for _, j := range run.Jobs {
				rf.Jobs = append(rf.Jobs, ciJobFile{Name: j.Name, Conclusion: string(j.Conclusion), URL: j.URL})
			}
			f.Runs = append(f.Runs, rf)
		}
		data, err = encodeYAML(f)
	default:
		return "", nil, fmt.Errorf("store: cannot write %T", record)
	}
	return rel, data, err
}

func splitFrontmatter(data []byte) (front, body []byte, ok bool) {
	rest, found := bytes.CutPrefix(data, []byte("---\n"))
	if !found {
		return nil, nil, false
	}
	if bytes.HasPrefix(rest, []byte("---\n")) {
		return nil, rest[4:], true
	}
	i := bytes.Index(rest, []byte("\n---\n"))
	if i < 0 {
		return nil, nil, false
	}
	return rest[:i+1], rest[i+5:], true
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// encodeLanding leaves the default Landing out of project.yaml, so a
// Project that never had one set keeps the file it always had.
func encodeLanding(l domain.Landing) *landingFile {
	if l = landingOrDefault(l); l == domain.DefaultLanding() {
		return nil
	}
	f := &landingFile{Method: string(l.Method), Push: l.Push, Remote: l.Remote, Rebase: l.Rebase, PATEnv: l.PATEnv, Base: l.Base}
	if !l.LandPerSpec {
		f.LandPerSpec = &l.LandPerSpec
	}
	return f
}

// encodeCI leaves the default CI out of project.yaml, so a Project that
// never had CI settings keeps the file it always had.
func encodeCI(c domain.CI) *ciFile {
	if c.Equal(domain.DefaultCI()) {
		return nil
	}
	f := &ciFile{Tracking: c.Tracking, PATEnv: c.PATEnv,
		IgnoredWorkflows: c.IgnoredWorkflows, PushRemoteURL: c.PushRemoteURL, NoPushRemote: c.NoPushRemote, PushRemoteBase: c.PushRemoteBase}
	for _, r := range c.Rules {
		f.Rules = append(f.Rules, ciRuleFile{Spec: r.Spec, Workflow: r.Workflow, Branch: r.Branch, State: string(r.State), Created: formatTime(r.Created)})
	}
	if c.AutoFix != (domain.AutoFix{}) {
		f.AutoFix = &autoFixFile{On: c.AutoFix.On, Instructions: c.AutoFix.Instructions, Limit: c.AutoFix.Limit}
	}
	for _, r := range c.Repositories {
		f.Repositories = append(f.Repositories, ciRepositoryFile{Name: r.Name, NotWatched: r.NotWatched, Provider: string(r.Provider), Repository: r.Repository, ProjectURL: r.ProjectURL})
	}
	return f
}

// decodeCIRepositories checks a Project's CI entries as load does: each
// names a Repository once, with a CI Provider unless it is Not watched.
// It returns the field, relative to ci.repositories, with the error.
func decodeCIRepositories(fs []ciRepositoryFile) ([]domain.CIRepository, string, error) {
	var out []domain.CIRepository
	for i, f := range fs {
		r := domain.CIRepository{Name: f.Name, NotWatched: f.NotWatched, Provider: domain.CIProvider(f.Provider), Repository: f.Repository, ProjectURL: f.ProjectURL}
		switch {
		case !domain.ValidRepositoryName(r.Name):
			return nil, fmt.Sprintf("[%d].name", i), fmt.Errorf("%q isn't a Repository Name", r.Name)
		case slices.ContainsFunc(out, func(o domain.CIRepository) bool { return o.Name == r.Name }):
			return nil, fmt.Sprintf("[%d].name", i), fmt.Errorf("%s has two CI entries", r.Name)
		case r.Provider != "" && !r.Provider.Valid(), r.Provider == "" && !r.NotWatched:
			return nil, fmt.Sprintf("[%d].provider", i), fmt.Errorf("unknown CI provider %q (%s)", f.Provider, strings.Join(domain.CIProviderNames(), " or "))
		}
		out = append(out, r)
	}
	return out, "", nil
}

// withLegacyHomeCI is es with the Home Repository's CI entry, named key,
// read from f's older provider, repository and project URL (Spec
// code-central-890) when they name what to watch and es has no entry for
// the Home. Without them the Home's entry is filled in from its Clone
// URL, as any Repository's is.
func withLegacyHomeCI(key string, es []domain.CIRepository, f ciFile) []domain.CIRepository {
	e := domain.CIRepository{Name: key, Provider: domain.CIProvider(f.Provider), Repository: f.Repository, ProjectURL: f.ProjectURL}
	if e.Provider == "" {
		e.Provider = domain.CIGitHubActions
	}
	named := e.Repository != ""
	if e.Provider == domain.CIAzurePipelines {
		named = e.ProjectURL != ""
	}
	if !named || slices.ContainsFunc(es, func(o domain.CIRepository) bool { return o.Name == key }) {
		return es
	}
	return append([]domain.CIRepository{e}, es...)
}

// encodeGitHubIssues leaves GitHub Issues out of project.yaml while it is
// off with nothing set, so a Project that never had it keeps the file it
// always had.
func encodeGitHubIssues(g domain.GitHubIssues) *githubIssuesFile {
	if !g.On && len(g.Repositories) == 0 && g.Label == "" && !g.NewIssuesNeedMe && g.Spec == 0 {
		return nil
	}
	return &githubIssuesFile{On: g.On, Repositories: g.Repositories, Label: g.Label, NewIssuesNeedMe: g.NewIssuesNeedMe, Spec: g.Spec}
}

// encodeAzureBoards leaves Azure Boards out of project.yaml while it is
// off with nothing set, so a Project that never had it keeps the file it
// always had.
func encodeAzureBoards(a domain.AzureBoards) *azureBoardsFile {
	if a == (domain.AzureBoards{}) {
		return nil
	}
	return &azureBoardsFile{On: a.On, Mode: string(a.Mode), QueryURL: a.QueryURL, ProjectURL: a.ProjectURL, Where: a.Where,
		PATEnv: a.PATEnv, NewIssuesNeedMe: a.NewIssuesNeedMe, HideCodeCentral: a.HideCodeCentral, Spec: a.Spec}
}

// encodeAutoAccept is the on kinds as project.yaml holds them.
func encodeAutoAccept(kinds []domain.AutoAcceptKind) []string {
	var out []string
	for _, k := range kinds {
		out = append(out, string(k))
	}
	return out
}

// encodeRepositories is the Repositories as project.yaml holds them, the
// Home Repository's with its name, Own Landing and Push remote alone.
func encodeRepositories(rs []domain.Repository) []repositoryFile {
	var out []repositoryFile
	for _, r := range rs {
		f := repositoryFile{Name: r.Name, Home: r.Home, PushRemote: r.PushRemote}
		if !r.Home {
			f.CloneURL, f.Base, f.LocalPath = r.CloneURL, r.Base, r.LocalPath
		}
		if o := r.OwnLanding.Normalized(); o.Set() {
			f.Landing = &ownLandingFile{Method: string(o.Method), Push: o.Push}
		}
		out = append(out, f)
	}
	return out
}

// decodeRepositories checks a Project's Repositories as load does: each
// Name is unique, the Home Repository, named key, is listed once with
// nothing more than an Own Landing and a Push remote, every other keeps
// the Repository Name rules and has a Clone URL or a local path and a
// Base, and each Own Landing names a Landing Method, with Pull Request's
// Push normalised away. The Home Repository's name is the Project Key,
// which keeps the key's own rules. It returns the offending field,
// relative to repositories, with the error.
func decodeRepositories(key string, fs []repositoryFile) ([]domain.Repository, string, error) {
	if len(fs) == 0 {
		return nil, "", nil
	}
	var out []domain.Repository
	homes := 0
	for i, f := range fs {
		field := fmt.Sprintf("[%d]", i)
		if !f.Home && !domain.ValidRepositoryName(f.Name) {
			return nil, field + ".name", fmt.Errorf("%q isn't a Repository Name: lowercase letters, digits, -, _ and ., starting with a letter or digit, at most %d characters", f.Name, domain.MaxRepositoryName)
		}
		if slices.ContainsFunc(out, func(r domain.Repository) bool { return r.Name == f.Name }) {
			return nil, field + ".name", fmt.Errorf("%q a second time", f.Name)
		}
		var own domain.OwnLanding
		if l := f.Landing; l != nil {
			if own = (domain.OwnLanding{Method: domain.LandingMethod(l.Method), Push: l.Push}).Normalized(); !own.Method.Valid() {
				return nil, field + ".landing.method", fmt.Errorf("unknown landing method %q (%s)", l.Method, strings.Join(domain.LandingMethodNames(), " or "))
			}
		}
		if f.Home {
			homes++
			if f.Name != key {
				return nil, field + ".name", fmt.Errorf("the Home Repository is named by the Project Key %q, not %q", key, f.Name)
			}
			if f.CloneURL != "" || f.Base != "" || f.LocalPath != "" {
				return nil, field, errors.New("the Home Repository holds only its name, Own Landing and Push remote: its Clone URL is reported and its Base is the Project's")
			}
			out = append(out, domain.Repository{Name: f.Name, Home: true, OwnLanding: own, PushRemote: f.PushRemote})
			continue
		}
		if f.CloneURL == "" && f.LocalPath == "" {
			return nil, field + ".clone_url", errors.New("missing, and so is local_path: a Repository needs one or both")
		}
		if f.Base == "" {
			return nil, field + ".base", errors.New("missing")
		}
		out = append(out, domain.Repository{Name: f.Name, CloneURL: f.CloneURL, Base: f.Base, LocalPath: f.LocalPath, OwnLanding: own, PushRemote: f.PushRemote})
	}
	if homes != 1 {
		return nil, "", fmt.Errorf("lists the Home Repository %d times, want once", homes)
	}
	return out, "", nil
}

// encodeStageGuides leaves the Stage Guides settings out of project.yaml
// while every optional line is on.
func encodeStageGuides(g domain.StageGuides) *stageGuidesFile {
	if g == (domain.StageGuides{}) {
		return nil
	}
	return &stageGuidesFile{SubagentExplorationOff: g.SubagentExplorationOff, ModelAssignmentOn: g.ModelAssignmentOn}
}

// decodeFocusKindsOff checks off as load does: each a Needs Me reason,
// once. It returns them in NeedsMeReasons order, with the offending
// field, relative to focus_kinds_off, with the error.
func decodeFocusKindsOff(off []string) ([]string, string, error) {
	var out []string
	for i, r := range off {
		field := fmt.Sprintf("[%d]", i)
		if !slices.Contains(domain.NeedsMeReasons, r) {
			return nil, field, fmt.Errorf("unknown Needs Me reason %q (%s)", r, strings.Join(domain.NeedsMeReasons, ", "))
		}
		if slices.Contains(out, r) {
			return nil, field, fmt.Errorf("%q a second time", r)
		}
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b string) int {
		return slices.Index(domain.NeedsMeReasons, a) - slices.Index(domain.NeedsMeReasons, b)
	})
	return out, "", nil
}

// encodeModels leaves Models out of project.yaml while it sets nothing,
// and each row that sets nothing out of Models.
func encodeModels(m domain.Models) *modelsFile {
	if m.IsZero() {
		return nil
	}
	f := &modelsFile{}
	if m.Default != (domain.ModelRow{}) {
		f.Default = &modelRowFile{Model: m.Default.Model, Effort: m.Default.Effort}
	}
	if m.Chat != (domain.ModelRow{}) {
		f.Chat = &modelRowFile{Model: m.Chat.Model, Effort: m.Chat.Effort}
	}
	for _, k := range m.Kinds {
		f.Kinds = append(f.Kinds, kindModelFile{Kind: k.Kind, Model: k.Model, Effort: k.Effort})
	}
	return f
}

// decodeModels checks f's rows as load does: each names a kind of work
// once, sets something, and holds single-line values. It returns the
// offending field, relative to models, with the error.
func decodeModels(f modelsFile) (domain.Models, string, error) {
	var m domain.Models
	row := func(field, model, effort string) (domain.ModelRow, string, error) {
		r := domain.ModelRow{Model: model, Effort: effort}
		switch {
		case r == domain.ModelRow{}:
			return r, field, errors.New("a Models row sets a model or an effort; leave out one that sets neither")
		case !singleLine(model):
			return r, field + ".model", errors.New("a model is one line, not blank")
		case !singleLine(effort):
			return r, field + ".effort", errors.New("an effort is one line, not blank")
		}
		return r, "", nil
	}
	if d := f.Default; d != nil {
		r, field, err := row("default", d.Model, d.Effort)
		if err != nil {
			return domain.Models{}, field, err
		}
		m.Default = r
	}
	if c := f.Chat; c != nil {
		r, field, err := row("chat", c.Model, c.Effort)
		if err != nil {
			return domain.Models{}, field, err
		}
		m.Chat = r
	}
	for i, k := range f.Kinds {
		field := fmt.Sprintf("kinds[%d]", i)
		if !slices.Contains(domain.ModelKinds, k.Kind) {
			return domain.Models{}, field + ".kind", fmt.Errorf("unknown kind of work %q (%s)", k.Kind, strings.Join(domain.ModelKinds, ", "))
		}
		if m.Kind(k.Kind) != (domain.ModelRow{}) {
			return domain.Models{}, field + ".kind", fmt.Errorf("a second row for %q", k.Kind)
		}
		r, bad, err := row(field, k.Model, k.Effort)
		if err != nil {
			return domain.Models{}, bad, err
		}
		m.Kinds = append(m.Kinds, domain.KindModels{Kind: k.Kind, ModelRow: r})
	}
	slices.SortStableFunc(m.Kinds, func(a, b domain.KindModels) int {
		return slices.Index(domain.ModelKinds, a.Kind) - slices.Index(domain.ModelKinds, b.Kind)
	})
	return m, "", nil
}

// encodeCIResponses is p's CI responses as project.yaml holds them.
func encodeCIResponses(rs []domain.CIResponse) []ciResponseFile {
	var out []ciResponseFile
	for _, r := range rs {
		f := ciResponseFile{Repository: r.Repository, Commit: r.Commit, LandAnyway: formatOptionalTime(r.LandAnyway), Rerun: formatOptionalTime(r.Rerun), FixTicket: r.FixTicket}
		for _, ra := range r.Reruns {
			f.Reruns = append(f.Reruns, runAttemptFile{ID: ra.ID, Attempt: ra.Attempt})
		}
		out = append(out, f)
	}
	return out
}

// decodeCIResponses checks fs as load does, returning the field, relative
// to ci_responses, with the error.
func decodeCIResponses(fs []ciResponseFile) ([]domain.CIResponse, string, error) {
	var out []domain.CIResponse
	for i, f := range fs {
		r := domain.CIResponse{Repository: f.Repository, Commit: f.Commit, FixTicket: f.FixTicket}
		if r.Commit == "" {
			return nil, fmt.Sprintf("[%d].commit", i), errors.New("required")
		}
		var err error
		if f.LandAnyway != "" {
			if r.LandAnyway, err = parseTime(f.LandAnyway); err != nil {
				return nil, fmt.Sprintf("[%d].land_anyway", i), err
			}
		}
		if f.Rerun != "" {
			if r.Rerun, err = parseTime(f.Rerun); err != nil {
				return nil, fmt.Sprintf("[%d].rerun", i), err
			}
		}
		for _, ra := range f.Reruns {
			r.Reruns = append(r.Reruns, domain.CIRunAttempt{ID: ra.ID, Attempt: ra.Attempt})
		}
		out = append(out, r)
	}
	return out, "", nil
}

// encodeReportedModels leaves the Reported Models out of project.yaml
// while they hold nothing.
func encodeReportedModels(r domain.ReportedModels) *reportedModelsFile {
	if r.IsZero() {
		return nil
	}
	return &reportedModelsFile{EffortLevels: r.EffortLevels, Aliases: r.Aliases, ModelIDs: r.ModelIDs,
		ModelFlag: r.ModelFlag, EffortFlag: r.EffortFlag, Reported: formatOptionalTime(r.Reported)}
}

// decodeReportedModels checks f as load does: every value is one line,
// not blank, and reported a timestamp. It returns the offending field,
// relative to reported_models, with the error.
func decodeReportedModels(f reportedModelsFile) (domain.ReportedModels, string, error) {
	r := domain.ReportedModels{EffortLevels: f.EffortLevels, Aliases: f.Aliases, ModelIDs: f.ModelIDs, ModelFlag: f.ModelFlag, EffortFlag: f.EffortFlag}
	lists := []struct {
		field  string
		values []string
	}{{"effort_levels", f.EffortLevels}, {"aliases", f.Aliases}, {"model_ids", f.ModelIDs}}
	for _, l := range lists {
		for i, v := range l.values {
			if v == "" || !singleLine(v) {
				return domain.ReportedModels{}, fmt.Sprintf("%s[%d]", l.field, i), errors.New("each is one line, not blank")
			}
		}
	}
	switch {
	case !singleLine(f.ModelFlag):
		return domain.ReportedModels{}, "model_flag", errors.New("a flag is empty, or one line that isn't blank")
	case !singleLine(f.EffortFlag):
		return domain.ReportedModels{}, "effort_flag", errors.New("a flag is empty, or one line that isn't blank")
	}
	if f.Reported != "" {
		t, err := parseTime(f.Reported)
		if err != nil {
			return domain.ReportedModels{}, "reported", err
		}
		r.Reported = t
	}
	return r, "", nil
}

// singleLine reports whether an optional value is empty, or one line
// that isn't blank.
func singleLine(v string) bool {
	return v == "" || (strings.TrimSpace(v) != "" && !strings.ContainsAny(v, "\r\n"))
}

// formatOptionalTime leaves a zero time out of the file.
func formatOptionalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return formatTime(t)
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("required")
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("not an RFC 3339 timestamp: %q", s)
	}
	return t.UTC(), nil
}
