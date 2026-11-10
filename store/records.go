// The files a record is stored in: their YAML shapes, decoding them
// strictly as load does, and encoding a record as write does.

package store

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

type storeFile struct {
	Format int `yaml:"format"`
}

// optionsFile is options.yaml, the human's global Options. The file and
// every key are optional: what's left out takes its default.
type optionsFile struct {
	FocusClear string `yaml:"focus_clear,omitempty"`
	GHPath     string `yaml:"gh_path,omitempty"`
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
}

type ciFile struct {
	Tracking         bool         `yaml:"tracking"`
	Provider         string       `yaml:"provider"`
	Repository       string       `yaml:"repository,omitempty"`
	IgnoredWorkflows []string     `yaml:"ignored_workflows,omitempty"`
	PushRemoteURL    string       `yaml:"push_remote_url,omitempty"`
	Rules            []ciRuleFile `yaml:"rules,omitempty"`
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
	Title          string              `yaml:"title"`
	State          string              `yaml:"state"`
	Spec           int                 `yaml:"spec"`
	Labels         []string            `yaml:"labels"`
	Tags           map[string]string   `yaml:"tags"`
	BlockedBy      []int               `yaml:"blocked_by"`
	Replacements   []int               `yaml:"replacements"`
	Link           *linkFile           `yaml:"link,omitempty"`
	Created        string              `yaml:"created"`
	Updated        string              `yaml:"updated"`
	Description    string              `yaml:"description"`
	Branch         string              `yaml:"branch,omitempty"`
	Commit         string              `yaml:"commit,omitempty"`
	PushedCommits  []pushedCommitFile  `yaml:"pushed_commits,omitempty"`
	Claims         []claimFile         `yaml:"claims"`
	Rounds         []roundFile         `yaml:"rounds"`
	FailureReports []failureReportFile `yaml:"failure_reports"`
	Comments       []commentFile       `yaml:"comments"`
	Moves          []moveFile          `yaml:"moves"`
	Edits          []editFile          `yaml:"edits"`
}

type pushedCommitFile struct {
	Commit string `yaml:"commit"`
	Pushed string `yaml:"pushed"`
}

// ciRunsFile is projects/<key>/ci-runs.yaml, the Project's CI Runs.
type ciRunsFile struct {
	Runs []ciRunFile `yaml:"runs"`
}

type ciRunFile struct {
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
}

// The fields an edit's previous values may name.
var (
	// external_state and detached record a Link's changes; spec is a
	// linked Ticket's parent, which only its Integration changes.
	specEditFields   = []string{"title", "body", "external_state", "detached"}
	ticketEditFields = []string{"title", "description", "spec", "external_state", "detached"}
)

func decodeSpec(data []byte, n int) (Spec, *LoadError) {
	front, body, ok := splitFrontmatter(data)
	if !ok {
		return Spec{}, &LoadError{Field: "frontmatter", Msg: "a Spec must start with YAML frontmatter between --- lines"}
	}
	var f specFront
	if field, err := decodeStrict(front, &f); err != nil {
		return Spec{}, &LoadError{Field: field, Msg: err.Error()}
	}
	if f.ID != n {
		return Spec{}, &LoadError{Field: "id", Msg: fmt.Sprintf("id %d does not match the file name", f.ID)}
	}
	if f.Title == "" {
		return Spec{}, &LoadError{Field: "title", Msg: "required"}
	}
	sp := Spec{Number: n, Title: f.Title, Labels: f.Labels, Tags: f.Tags, Body: string(body)}
	var lerr *LoadError
	if sp.Link, lerr = decodeLink(f.Link); lerr != nil {
		return Spec{}, lerr
	}
	var err error
	if sp.Created, err = parseTime(f.Created); err != nil {
		return Spec{}, &LoadError{Field: "created", Msg: err.Error()}
	}
	if sp.Updated, err = parseTime(f.Updated); err != nil {
		return Spec{}, &LoadError{Field: "updated", Msg: err.Error()}
	}
	if sp.Edits, err = decodeEdits(f.Edits, specEditFields...); err != nil {
		return Spec{}, err.(*LoadError)
	}
	return sp, nil
}

func decodeTicket(data []byte, n int) (Ticket, *LoadError) {
	var f ticketFile
	if field, err := decodeStrict(data, &f); err != nil {
		return Ticket{}, &LoadError{Field: field, Msg: err.Error()}
	}
	t := Ticket{
		Number: n, Title: f.Title, State: State(f.State), Spec: f.Spec, Labels: f.Labels, Tags: f.Tags,
		BlockedBy: f.BlockedBy, Replacements: f.Replacements, Description: f.Description,
		Branch: f.Branch, Commit: f.Commit,
	}
	if t.Title == "" {
		return Ticket{}, &LoadError{Field: "title", Msg: "required"}
	}
	if !t.State.Valid() {
		return Ticket{}, &LoadError{Field: "state", Msg: fmt.Sprintf("unknown State %q", f.State)}
	}
	if t.Spec < 1 {
		return Ticket{}, &LoadError{Field: "spec", Msg: "required"}
	}
	var lerr *LoadError
	if t.Link, lerr = decodeLink(f.Link); lerr != nil {
		return Ticket{}, lerr
	}
	var err error
	if t.Created, err = parseTime(f.Created); err != nil {
		return Ticket{}, &LoadError{Field: "created", Msg: err.Error()}
	}
	if t.Updated, err = parseTime(f.Updated); err != nil {
		return Ticket{}, &LoadError{Field: "updated", Msg: err.Error()}
	}
	for i, pc := range f.PushedCommits {
		p := PushedCommit{Commit: pc.Commit}
		if p.Commit == "" {
			return Ticket{}, &LoadError{Field: fmt.Sprintf("pushed_commits[%d].commit", i), Msg: "required"}
		}
		if p.Pushed, err = parseTime(pc.Pushed); err != nil {
			return Ticket{}, &LoadError{Field: fmt.Sprintf("pushed_commits[%d].pushed", i), Msg: err.Error()}
		}
		t.PushedCommits = append(t.PushedCommits, p)
	}
	for i, c := range f.Claims {
		cl := Claim{Number: c.Number, ID: c.ID, SessionID: c.SessionID, WorkingDir: c.WorkingDir, ResumeCommand: c.ResumeCommand, EndedBy: c.EndedBy}
		field := fmt.Sprintf("claims[%d]", i)
		if cl.ID == "" {
			return Ticket{}, &LoadError{Field: field + ".id", Msg: "required"}
		}
		if cl.Started, err = parseTime(c.Started); err != nil {
			return Ticket{}, &LoadError{Field: field + ".started", Msg: err.Error()}
		}
		if c.Ended != "" {
			if cl.Ended, err = parseTime(c.Ended); err != nil {
				return Ticket{}, &LoadError{Field: field + ".ended", Msg: err.Error()}
			}
		}
		t.Claims = append(t.Claims, cl)
	}
	if t.Rounds, err = decodeRounds(f.Rounds); err != nil {
		return Ticket{}, err.(*LoadError)
	}
	for i, r := range f.FailureReports {
		fr := FailureReport{
			Number: r.Number, Claim: r.Claim, SessionID: r.SessionID, WorkingDir: r.WorkingDir, ResumeCommand: r.ResumeCommand, Actor: r.Actor,
			WhatBroke: r.WhatBroke, Why: r.Why, WhatToFix: r.WhatToFix, WhereWorkStands: r.WhereWorkStands,
		}
		if fr.Time, err = parseTime(r.Time); err != nil {
			return Ticket{}, &LoadError{Field: fmt.Sprintf("failure_reports[%d].time", i), Msg: err.Error()}
		}
		t.FailureReports = append(t.FailureReports, fr)
	}
	for i, c := range f.Comments {
		cm := Comment{Number: c.Number, Actor: c.Actor, Claim: c.Claim, Body: c.Body}
		if cm.Time, err = parseTime(c.Time); err != nil {
			return Ticket{}, &LoadError{Field: fmt.Sprintf("comments[%d].time", i), Msg: err.Error()}
		}
		t.Comments = append(t.Comments, cm)
	}
	for i, m := range f.Moves {
		mv := Move{From: State(m.From), To: State(m.To), Actor: m.Actor, Claim: m.Claim, Via: m.Via, Commit: m.Commit}
		field := fmt.Sprintf("moves[%d]", i)
		if m.From != "" && !mv.From.Valid() {
			return Ticket{}, &LoadError{Field: field + ".from", Msg: fmt.Sprintf("unknown State %q", m.From)}
		}
		if !mv.To.Valid() {
			return Ticket{}, &LoadError{Field: field + ".to", Msg: fmt.Sprintf("unknown State %q", m.To)}
		}
		if mv.Time, err = parseTime(m.Time); err != nil {
			return Ticket{}, &LoadError{Field: field + ".time", Msg: err.Error()}
		}
		t.Moves = append(t.Moves, mv)
	}
	if t.Edits, err = decodeEdits(f.Edits, ticketEditFields...); err != nil {
		return Ticket{}, err.(*LoadError)
	}
	return t, nil
}

// decodeCIRuns decodes ci-runs.yaml, checking each run's status and
// conclusions.
func decodeCIRuns(data []byte) ([]CIRun, *LoadError) {
	var f ciRunsFile
	if field, err := decodeStrict(data, &f); err != nil {
		return nil, &LoadError{Field: field, Msg: err.Error()}
	}
	var out []CIRun
	for i, rf := range f.Runs {
		field := fmt.Sprintf("runs[%d]", i)
		r := CIRun{ID: rf.ID, Attempt: rf.Attempt, Workflow: rf.Workflow, Name: rf.Name, Branch: rf.Branch, Commit: rf.Commit,
			Status: CIRunStatus(rf.Status), Conclusion: CIConclusion(rf.Conclusion), URL: rf.URL, Ticket: rf.Ticket, Rule: rf.Rule, Ignored: rf.Ignored}
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
			job := CIJob{Name: jf.Name, Conclusion: CIConclusion(jf.Conclusion), URL: jf.URL}
			if !job.Conclusion.Valid() {
				return nil, &LoadError{Field: fmt.Sprintf("%s.jobs[%d].conclusion", field, j), Msg: fmt.Sprintf("unknown CI conclusion %q", jf.Conclusion)}
			}
			r.Jobs = append(r.Jobs, job)
		}
		out = append(out, r)
	}
	return out, nil
}

func decodeRounds(rs []roundFile) ([]Round, error) {
	var out []Round
	for i, r := range rs {
		field := fmt.Sprintf("rounds[%d]", i)
		rd := Round{Number: r.Number, Claim: r.Claim, SessionID: r.SessionID}
		var err error
		if rd.Posted, err = parseTime(r.Posted); err != nil {
			return nil, &LoadError{Field: field + ".posted", Msg: err.Error()}
		}
		for j, q := range r.Questions {
			qf := fmt.Sprintf("%s.questions[%d]", field, j)
			qu := Question{Number: q.Number, Title: q.Title, Body: q.Body, Recommended: q.Recommended, Status: QuestionStatus(q.Status), Answer: q.Answer}
			switch qu.Status {
			case QuestionOpen, QuestionAnswered, QuestionDismissed:
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
func decodeEdits(fs []editFile, fields ...string) ([]Edit, error) {
	var out []Edit
	for i, e := range fs {
		ed := Edit{Number: e.Number, Actor: e.Actor, Previous: e.Previous}
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

func encodeEdits(es []Edit) []editFile {
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
	case Project:
		rel = filepath.Join("projects", r.Key, "project.yaml")
		data, err = encodeYAML(projectFile{Key: r.Key, DisplayName: r.DisplayName, Created: formatTime(r.Created), Mode: string(r.Mode), Archived: formatOptionalTime(r.Archived), Landing: encodeLanding(r.Landing), CI: encodeCI(r.CI)})
	case Options:
		rel = "options.yaml"
		data, err = encodeYAML(optionsFile{FocusClear: string(r.FocusClear), GHPath: r.GHPath})
	case Spec:
		rel = filepath.Join("projects", project, "specs", strconv.Itoa(r.Number)+".md")
		var front []byte
		front, err = encodeYAML(specFront{ID: r.Number, Title: r.Title, Labels: r.Labels, Tags: r.Tags, Link: encodeLink(r.Link), Created: formatTime(r.Created), Updated: formatTime(r.Updated), Edits: encodeEdits(r.Edits)})
		data = append(append(append([]byte("---\n"), front...), "---\n"...), r.Body...)
	case Ticket:
		rel = filepath.Join("projects", project, "tickets", strconv.Itoa(r.Number)+".yaml")
		f := ticketFile{
			Title: r.Title, State: string(r.State), Spec: r.Spec, Labels: r.Labels, Tags: r.Tags,
			BlockedBy: r.BlockedBy, Replacements: r.Replacements, Link: encodeLink(r.Link),
			Created: formatTime(r.Created), Updated: formatTime(r.Updated), Description: r.Description,
			Branch: r.Branch, Commit: r.Commit,
			Claims: []claimFile{}, Rounds: []roundFile{}, FailureReports: []failureReportFile{},
			Comments: []commentFile{}, Moves: []moveFile{}, Edits: encodeEdits(r.Edits),
		}
		for _, pc := range r.PushedCommits {
			f.PushedCommits = append(f.PushedCommits, pushedCommitFile{Commit: pc.Commit, Pushed: formatTime(pc.Pushed)})
		}
		for _, c := range r.Claims {
			cf := claimFile{Number: c.Number, ID: c.ID, SessionID: c.SessionID, WorkingDir: c.WorkingDir, ResumeCommand: c.ResumeCommand, Started: formatTime(c.Started), EndedBy: c.EndedBy}
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
			f.Moves = append(f.Moves, moveFile{From: string(m.From), To: string(m.To), Actor: m.Actor, Time: formatTime(m.Time), Claim: m.Claim, Via: m.Via, Commit: m.Commit})
		}
		data, err = encodeYAML(f)
	case CIRuns:
		rel = filepath.Join("projects", project, ciRunsName)
		f := ciRunsFile{Runs: []ciRunFile{}}
		for _, run := range r.Runs {
			rf := ciRunFile{ID: run.ID, Attempt: run.Attempt, Workflow: run.Workflow, Name: run.Name, Branch: run.Branch, Commit: run.Commit,
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
func encodeLanding(l Landing) *landingFile {
	if l = l.orDefault(); l == DefaultLanding() {
		return nil
	}
	return &landingFile{Method: string(l.Method), Push: l.Push, Remote: l.Remote}
}

// encodeCI leaves the default CI out of project.yaml, so a Project that
// never had CI settings keeps the file it always had.
func encodeCI(c CI) *ciFile {
	if c = c.orDefault(); c.Equal(DefaultCI()) {
		return nil
	}
	f := &ciFile{Tracking: c.Tracking, Provider: string(c.Provider), Repository: c.Repository,
		IgnoredWorkflows: c.IgnoredWorkflows, PushRemoteURL: c.PushRemoteURL}
	for _, r := range c.Rules {
		f.Rules = append(f.Rules, ciRuleFile{Spec: r.Spec, Workflow: r.Workflow, Branch: r.Branch, State: string(r.State), Created: formatTime(r.Created)})
	}
	return f
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
