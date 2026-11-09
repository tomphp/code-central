package store

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/home"
)

// ErrLocked means another process already holds the store's lock.
var ErrLocked = errors.New("store is locked by another code-central serve")

// LoadError is a file that failed strict validation at startup. File is
// relative to the store directory.
type LoadError struct {
	File  string
	Field string
	Msg   string
}

func (e *LoadError) Error() string {
	return fmt.Sprintf("%s: field %s: %s", e.File, e.Field, e.Msg)
}

// Dir resolves the store directory: the --store flag, then
// CODE_CENTRAL_STORE, then $XDG_DATA_HOME/code-central, then
// ~/.local/share/code-central, where ~ is HOME or else the profile folder
// (ADR 0010).
func Dir(flag string, h home.Lookup) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if d := h.Getenv("CODE_CENTRAL_STORE"); d != "" {
		return d, nil
	}
	d, err := h.Data("code-central")
	if nf := (*home.NotFoundError)(nil); errors.As(err, &nf) {
		nf.Vars = append([]string{"CODE_CENTRAL_STORE"}, nf.Vars...)
	}
	if err != nil {
		return "", fmt.Errorf("cannot find the store: %w", err)
	}
	return d, nil
}

// FileStore is the file-backed Store: YAML files under one directory,
// loaded whole into memory by Open and held under an exclusive lock until
// Close.
type FileStore struct {
	*index
	dir  string
	lock *storeLock
	// format is the store format this build reads and writes, and steps
	// the migrations up to it: Format and migrations, except in tests.
	format int
	steps  []MigrationStep
	// migrated is the migration this Open ran, if any.
	migrated *Migration
	// stopImportAfter, set only by tests, stops an Import dead after a
	// step, as a crash would.
	stopImportAfter func(step int) bool
}

// Dir is the store's directory.
func (s *FileStore) Dir() string { return s.dir }

// Close releases the store's lock.
func (s *FileStore) Close() error {
	if s.lock == nil {
		return nil
	}
	err := s.lock.release()
	s.lock = nil
	return err
}

// Open locks the store at dir, creating it if needed, then runs startup
// in order: roll forward any Import manifest, migrate an older format,
// and load and strictly validate every file. A file that fails
// validation stops the open with a *LoadError, and a failed migration
// with a *MigrationError.
func Open(dir string) (*FileStore, error) { return open(dir, Format, migrations) }

func open(dir string, format int, steps []MigrationStep) (*FileStore, error) {
	if err := finishMigration(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	lock, err := lockStore(dir)
	if err != nil {
		return nil, err
	}
	s := &FileStore{index: newIndex(), dir: dir, lock: lock, format: format, steps: steps}
	if err := s.recoverImports(); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.migrate(); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.load(); err != nil {
		s.Close()
		return nil, err
	}
	s.persist = s.write
	return s, nil
}

func (s *FileStore) load() error {
	if err := s.checkFormat(); err != nil {
		return err
	}
	if err := s.loadOptions(); err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, "projects"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if err := s.loadProject(e.Name()); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileStore) checkFormat() error {
	f, ok, err := s.readFormat()
	if err != nil {
		return err
	}
	if !ok {
		return atomicWrite(filepath.Join(s.dir, "store.yaml"), fmt.Appendf(nil, "format: %d\n", s.format))
	}
	if f != s.format {
		return fmt.Errorf("store.yaml: unknown format %d (this code-central understands format %d)", f, s.format)
	}
	return nil
}

type storeFile struct {
	Format int `yaml:"format"`
}

// optionsFile is options.yaml, the human's global Options. The file and
// every key are optional: what's left out takes its default.
type optionsFile struct {
	FocusClear string `yaml:"focus_clear,omitempty"`
}

type projectFile struct {
	Key         string `yaml:"key"`
	DisplayName string `yaml:"display_name"`
	Created     string `yaml:"created"`
	Mode        string `yaml:"mode"`
	Archived    string `yaml:"archived,omitempty"`
	// Landing is left out while it is the default.
	Landing *landingFile `yaml:"landing,omitempty"`
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
	Claims         []claimFile         `yaml:"claims"`
	Rounds         []roundFile         `yaml:"rounds"`
	FailureReports []failureReportFile `yaml:"failure_reports"`
	Comments       []commentFile       `yaml:"comments"`
	Moves          []moveFile          `yaml:"moves"`
	Edits          []editFile          `yaml:"edits"`
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

// loadOptions reads options.yaml, if there is one, over the defaults.
func (s *FileStore) loadOptions() error {
	const rel = "options.yaml"
	data, err := os.ReadFile(filepath.Join(s.dir, rel))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &LoadError{File: rel, Field: "file", Msg: err.Error()}
	}
	var f optionsFile
	if field, err := decodeStrict(data, &f); err != nil {
		return &LoadError{File: rel, Field: field, Msg: err.Error()}
	}
	o := DefaultOptions()
	if f.FocusClear != "" {
		o.FocusClear = FocusClear(f.FocusClear)
	}
	if !o.FocusClear.Valid() {
		return &LoadError{File: rel, Field: "focus_clear", Msg: fmt.Sprintf("unknown value %q (%s)", f.FocusClear, strings.Join(FocusClearNames(), " or "))}
	}
	s.options = o
	return nil
}

var numbered = regexp.MustCompile(`^[1-9][0-9]*$`)

func (s *FileStore) loadProject(key string) error {
	rel := filepath.ToSlash(filepath.Join("projects", key, "project.yaml"))
	data, err := os.ReadFile(filepath.Join(s.dir, rel))
	if err != nil {
		return &LoadError{File: rel, Field: "file", Msg: err.Error()}
	}
	var pf projectFile
	if field, err := decodeStrict(data, &pf); err != nil {
		return &LoadError{File: rel, Field: field, Msg: err.Error()}
	}
	if pf.Key != key {
		return &LoadError{File: rel, Field: "key", Msg: fmt.Sprintf("key %q does not match its folder %q", pf.Key, key)}
	}
	p := Project{Key: pf.Key, DisplayName: pf.DisplayName, Mode: Mode(pf.Mode)}
	if !p.Mode.Valid() {
		return &LoadError{File: rel, Field: "mode", Msg: fmt.Sprintf("unknown mode %q (committed or local)", pf.Mode)}
	}
	if p.Created, err = parseTime(pf.Created); err != nil {
		return &LoadError{File: rel, Field: "created", Msg: err.Error()}
	}
	if pf.Archived != "" {
		if p.Archived, err = parseTime(pf.Archived); err != nil {
			return &LoadError{File: rel, Field: "archived", Msg: err.Error()}
		}
	}
	if l := pf.Landing; l != nil {
		p.Landing = Landing{Method: LandingMethod(l.Method), Push: l.Push, Remote: l.Remote}
		if !p.Landing.Method.Valid() {
			return &LoadError{File: rel, Field: "landing.method", Msg: fmt.Sprintf("unknown landing method %q (%s)", l.Method, strings.Join(landingMethodNames(), " or "))}
		}
	}
	s.setProject(p)

	for _, kind := range []string{"specs", "tickets"} {
		ext := map[string]string{"specs": ".md", "tickets": ".yaml"}[kind]
		entries, err := os.ReadDir(filepath.Join(s.dir, "projects", key, kind))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var files []loadedFile
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") { // leftover temp files
				continue
			}
			rel := "projects/" + key + "/" + kind + "/" + name
			base, found := strings.CutSuffix(name, ext)
			if e.IsDir() || !found || !numbered.MatchString(base) {
				return &LoadError{File: rel, Field: "file name", Msg: "must be <number>" + ext}
			}
			n, _ := strconv.Atoi(base)
			files = append(files, loadedFile{rel: rel, n: n})
		}
		// Decoding is the bulk of startup, and files are independent, so
		// they're decoded in parallel; the first failure in directory
		// order is reported.
		parallel(len(files), func(i int) {
			f := &files[i]
			data, err := os.ReadFile(filepath.Join(s.dir, f.rel))
			if err != nil {
				f.err = &LoadError{Field: "file", Msg: err.Error()}
			} else if kind == "specs" {
				f.spec, f.err = decodeSpec(data, f.n)
			} else {
				f.ticket, f.err = decodeTicket(data, f.n)
			}
		})
		for _, f := range files {
			if f.err != nil {
				f.err.File = f.rel
				return f.err
			}
			if kind == "specs" {
				s.projects[key].specs[f.n] = f.spec
			} else {
				s.projects[key].tickets[f.n] = f.ticket
			}
		}
	}
	return nil
}

type loadedFile struct {
	rel    string
	n      int
	spec   Spec
	ticket Ticket
	err    *LoadError
}

// parallel runs fn(0) … fn(n-1) on a pool of goroutines and waits.
func parallel(n int, fn func(i int)) {
	work := make(chan int)
	var wg sync.WaitGroup
	for range min(n, max(4, 2*runtime.NumCPU())) {
		wg.Go(func() {
			for i := range work {
				fn(i)
			}
		})
	}
	for i := range n {
		work <- i
	}
	close(work)
	wg.Wait()
}

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

func (s *FileStore) write(record any, project string) error {
	if im, ok := record.(Import); ok {
		return s.writeImport(project, im)
	}
	rel, data, err := encode(record, project)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.dir, rel), data)
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
		data, err = encodeYAML(projectFile{Key: r.Key, DisplayName: r.DisplayName, Created: formatTime(r.Created), Mode: string(r.Mode), Archived: formatOptionalTime(r.Archived), Landing: encodeLanding(r.Landing)})
	case Options:
		rel = "options.yaml"
		data, err = encodeYAML(optionsFile{FocusClear: string(r.FocusClear)})
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
	default:
		return "", nil, fmt.Errorf("store: cannot write %T", record)
	}
	return rel, data, err
}

// atomicWrite writes a temp file beside path, fsyncs it, renames it into
// place and fsyncs the directory (syncDir).
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	return syncDir(dir)
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
