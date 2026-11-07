package store

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Format is the store format this build reads and writes.
const Format = 1

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
// ~/.local/share/code-central.
func Dir(flag string, getenv func(string) string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if d := getenv("CODE_CENTRAL_STORE"); d != "" {
		return d, nil
	}
	if d := getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "code-central"), nil
	}
	home := getenv("HOME")
	if home == "" {
		return "", errors.New("cannot find the store: set HOME, XDG_DATA_HOME or CODE_CENTRAL_STORE")
	}
	return filepath.Join(home, ".local", "share", "code-central"), nil
}

// FileStore is the file-backed Store: YAML files under one directory,
// loaded whole into memory by Open and held under an exclusive lock until
// Close.
type FileStore struct {
	*index
	dir  string
	lock *os.File
}

// Dir is the store's directory.
func (s *FileStore) Dir() string { return s.dir }

// Close releases the store's lock.
func (s *FileStore) Close() error {
	if s.lock == nil {
		return nil
	}
	syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN)
	err := s.lock.Close()
	s.lock = nil
	return err
}

// Open locks the store at dir, creating it if needed, checks its format,
// and loads and strictly validates every file. A file that fails
// validation stops the open with a *LoadError.
func Open(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%s: %w", dir, ErrLocked)
		}
		return nil, err
	}
	s := &FileStore{index: newIndex(), dir: dir, lock: lock}
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
	path := filepath.Join(s.dir, "store.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return atomicWrite(path, []byte(fmt.Sprintf("format: %d\n", Format)))
	}
	if err != nil {
		return err
	}
	var f struct {
		Format int `yaml:"format"`
	}
	if field, err := decodeStrict(data, &f); err != nil {
		return &LoadError{File: "store.yaml", Field: field, Msg: err.Error()}
	}
	if f.Format != Format {
		return fmt.Errorf("store.yaml: unknown format %d (this code-central understands format %d)", f.Format, Format)
	}
	return nil
}

type projectFile struct {
	Key         string `yaml:"key"`
	DisplayName string `yaml:"display_name"`
	Created     string `yaml:"created"`
	Mode        string `yaml:"mode"`
	Archived    bool   `yaml:"archived,omitempty"`
}

type specFront struct {
	ID      int               `yaml:"id"`
	Title   string            `yaml:"title"`
	Labels  []string          `yaml:"labels"`
	Tags    map[string]string `yaml:"tags"`
	Link    string            `yaml:"link,omitempty"`
	Created string            `yaml:"created"`
	Updated string            `yaml:"updated"`
	Edits   []editFile        `yaml:"edits"`
}

type ticketFile struct {
	Title        string            `yaml:"title"`
	State        string            `yaml:"state"`
	Spec         int               `yaml:"spec"`
	Labels       []string          `yaml:"labels"`
	Tags         map[string]string `yaml:"tags"`
	BlockedBy    []int             `yaml:"blocked_by"`
	Replacements []int             `yaml:"replacements"`
	Link         string            `yaml:"link,omitempty"`
	Created      string            `yaml:"created"`
	Updated      string            `yaml:"updated"`
	Description  string            `yaml:"description"`
	Claims       []claimFile       `yaml:"claims"`
	// rounds (ticket 29) go here, before failure_reports.
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
	Number    int    `yaml:"number"`
	ID        string `yaml:"id"`
	SessionID string `yaml:"session_id,omitempty"`
	Started   string `yaml:"started"`
	Ended     string `yaml:"ended,omitempty"`
	EndedBy   string `yaml:"ended_by,omitempty"`
}

type failureReportFile struct {
	Number          int    `yaml:"number"`
	Claim           int    `yaml:"claim"`
	SessionID       string `yaml:"session_id,omitempty"`
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
	From  string `yaml:"from"`
	To    string `yaml:"to"`
	Actor string `yaml:"actor"`
	Time  string `yaml:"time"`
	Claim int    `yaml:"claim,omitempty"`
	Via   string `yaml:"via,omitempty"`
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
	p := Project{Key: pf.Key, DisplayName: pf.DisplayName, Mode: pf.Mode, Archived: pf.Archived}
	if p.Created, err = parseTime(pf.Created); err != nil {
		return &LoadError{File: rel, Field: "created", Msg: err.Error()}
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
			data, err := os.ReadFile(filepath.Join(s.dir, rel))
			if err != nil {
				return &LoadError{File: rel, Field: "file", Msg: err.Error()}
			}
			if kind == "specs" {
				sp, lerr := decodeSpec(data, n)
				if lerr != nil {
					lerr.File = rel
					return lerr
				}
				s.projects[key].specs[n] = sp
			} else {
				tk, lerr := decodeTicket(data, n)
				if lerr != nil {
					lerr.File = rel
					return lerr
				}
				s.projects[key].tickets[n] = tk
			}
		}
	}
	return nil
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
	sp := Spec{Number: n, Title: f.Title, Labels: f.Labels, Tags: f.Tags, Link: f.Link, Body: string(body)}
	var err error
	if sp.Created, err = parseTime(f.Created); err != nil {
		return Spec{}, &LoadError{Field: "created", Msg: err.Error()}
	}
	if sp.Updated, err = parseTime(f.Updated); err != nil {
		return Spec{}, &LoadError{Field: "updated", Msg: err.Error()}
	}
	if sp.Edits, err = decodeEdits(f.Edits, "title", "body"); err != nil {
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
		BlockedBy: f.BlockedBy, Replacements: f.Replacements, Link: f.Link, Description: f.Description,
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
	var err error
	if t.Created, err = parseTime(f.Created); err != nil {
		return Ticket{}, &LoadError{Field: "created", Msg: err.Error()}
	}
	if t.Updated, err = parseTime(f.Updated); err != nil {
		return Ticket{}, &LoadError{Field: "updated", Msg: err.Error()}
	}
	for i, c := range f.Claims {
		cl := Claim{Number: c.Number, ID: c.ID, SessionID: c.SessionID, EndedBy: c.EndedBy}
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
	for i, r := range f.FailureReports {
		fr := FailureReport{
			Number: r.Number, Claim: r.Claim, SessionID: r.SessionID, Actor: r.Actor,
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
		mv := Move{From: State(m.From), To: State(m.To), Actor: m.Actor, Claim: m.Claim, Via: m.Via}
		field := fmt.Sprintf("moves[%d]", i)
		if !mv.From.Valid() {
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
	if t.Edits, err = decodeEdits(f.Edits, "title", "description"); err != nil {
		return Ticket{}, err.(*LoadError)
	}
	return t, nil
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
	var rel string
	var data []byte
	var err error
	switch r := record.(type) {
	case Project:
		rel = filepath.Join("projects", r.Key, "project.yaml")
		data, err = encodeYAML(projectFile{Key: r.Key, DisplayName: r.DisplayName, Created: formatTime(r.Created), Mode: r.Mode, Archived: r.Archived})
	case Spec:
		rel = filepath.Join("projects", project, "specs", strconv.Itoa(r.Number)+".md")
		var front []byte
		front, err = encodeYAML(specFront{ID: r.Number, Title: r.Title, Labels: r.Labels, Tags: r.Tags, Link: r.Link, Created: formatTime(r.Created), Updated: formatTime(r.Updated), Edits: encodeEdits(r.Edits)})
		data = append(append(append([]byte("---\n"), front...), "---\n"...), r.Body...)
	case Ticket:
		rel = filepath.Join("projects", project, "tickets", strconv.Itoa(r.Number)+".yaml")
		f := ticketFile{
			Title: r.Title, State: string(r.State), Spec: r.Spec, Labels: r.Labels, Tags: r.Tags,
			BlockedBy: r.BlockedBy, Replacements: r.Replacements, Link: r.Link,
			Created: formatTime(r.Created), Updated: formatTime(r.Updated), Description: r.Description,
			Claims: []claimFile{}, FailureReports: []failureReportFile{}, Comments: []commentFile{}, Moves: []moveFile{}, Edits: encodeEdits(r.Edits),
		}
		for _, c := range r.Claims {
			cf := claimFile{Number: c.Number, ID: c.ID, SessionID: c.SessionID, Started: formatTime(c.Started), EndedBy: c.EndedBy}
			if !c.Ended.IsZero() {
				cf.Ended = formatTime(c.Ended)
			}
			f.Claims = append(f.Claims, cf)
		}
		for _, fr := range r.FailureReports {
			f.FailureReports = append(f.FailureReports, failureReportFile{
				Number: fr.Number, Claim: fr.Claim, SessionID: fr.SessionID, Actor: fr.Actor, Time: formatTime(fr.Time),
				WhatBroke: fr.WhatBroke, Why: fr.Why, WhatToFix: fr.WhatToFix, WhereWorkStands: fr.WhereWorkStands,
			})
		}
		for _, c := range r.Comments {
			f.Comments = append(f.Comments, commentFile{Number: c.Number, Actor: c.Actor, Time: formatTime(c.Time), Claim: c.Claim, Body: c.Body})
		}
		for _, m := range r.Moves {
			f.Moves = append(f.Moves, moveFile{From: string(m.From), To: string(m.To), Actor: m.Actor, Time: formatTime(m.Time), Claim: m.Claim, Via: m.Via})
		}
		data, err = encodeYAML(f)
	default:
		return fmt.Errorf("store: cannot write %T", record)
	}
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.dir, rel), data)
}

// atomicWrite writes a temp file beside path, fsyncs it, renames it into
// place and fsyncs the directory.
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
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
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
