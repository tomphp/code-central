// The FileStore on disk: finding, opening and locking its directory,
// loading every Project's files at startup, and writing a record's file
// atomically. records.go holds the files' shapes and how a record is
// decoded from and encoded to its file.

package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

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
	if f.FocusPopup != "" {
		o.FocusPopup = FocusPopup(f.FocusPopup)
	}
	o.GHPath = f.GHPath
	if !o.FocusClear.Valid() {
		return &LoadError{File: rel, Field: "focus_clear", Msg: fmt.Sprintf("unknown value %q (%s)", f.FocusClear, strings.Join(FocusClearNames(), " or "))}
	}
	if !o.FocusPopup.Valid() {
		return &LoadError{File: rel, Field: "focus_popup", Msg: fmt.Sprintf("unknown value %q (%s)", f.FocusPopup, strings.Join(FocusPopupNames(), " or "))}
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
		p.Landing = Landing{Method: LandingMethod(l.Method), Push: l.Push, Remote: l.Remote, LandPerSpec: l.LandPerSpec == nil || *l.LandPerSpec, PATEnv: l.PATEnv, Base: l.Base}
		if !p.Landing.Method.Valid() {
			return &LoadError{File: rel, Field: "landing.method", Msg: fmt.Sprintf("unknown landing method %q (%s)", l.Method, strings.Join(landingMethodNames(), " or "))}
		}
	}
	if c := pf.CI; c != nil {
		p.CI = CI{Tracking: c.Tracking, Provider: CIProvider(c.Provider), Repository: c.Repository, ProjectURL: c.ProjectURL, PATEnv: c.PATEnv,
			IgnoredWorkflows: c.IgnoredWorkflows, PushRemoteURL: c.PushRemoteURL, PushRemoteBase: c.PushRemoteBase}
		if !p.CI.Provider.Valid() {
			return &LoadError{File: rel, Field: "ci.provider", Msg: fmt.Sprintf("unknown CI provider %q (%s)", c.Provider, strings.Join(ciProviderNames(), " or "))}
		}
		for i, r := range c.Rules {
			field := fmt.Sprintf("ci.rules[%d]", i)
			rule := CIRule{Spec: r.Spec, Workflow: r.Workflow, Branch: r.Branch, State: State(r.State)}
			if rule.State != Draft && rule.State != Ready {
				return &LoadError{File: rel, Field: field + ".state", Msg: fmt.Sprintf("a CI Rule's Tickets open as Draft or Ready, not %q", r.State)}
			}
			if rule.Created, err = parseTime(r.Created); err != nil {
				return &LoadError{File: rel, Field: field + ".created", Msg: err.Error()}
			}
			p.CI.Rules = append(p.CI.Rules, rule)
		}
	}
	if g := pf.GitHubIssues; g != nil {
		p.GitHubIssues = GitHubIssues{On: g.On, Repository: g.Repository, Label: g.Label, NewIssuesNeedMe: g.NewIssuesNeedMe, Spec: g.Spec}
	}
	if m := pf.Models; m != nil {
		var field string
		if p.Models, field, err = decodeModels(*m); err != nil {
			return &LoadError{File: rel, Field: "models." + field, Msg: err.Error()}
		}
	}
	if m := pf.ReportedModels; m != nil {
		var field string
		if p.ReportedModels, field, err = decodeReportedModels(*m); err != nil {
			return &LoadError{File: rel, Field: "reported_models." + field, Msg: err.Error()}
		}
	}
	if g := pf.StageGuides; g != nil {
		p.StageGuides = StageGuides{SubagentExplorationOff: g.SubagentExplorationOff, ModelAssignmentOn: g.ModelAssignmentOn}
	}
	rs, at, err := decodeCIResponses(pf.CIResponses)
	if err != nil {
		return &LoadError{File: rel, Field: "ci_responses" + at, Msg: err.Error()}
	}
	p.CIResponses = rs
	p.CIFixesSpec = pf.CIFixesSpec
	p.SmallChangesSpec = pf.SmallChangesSpec
	s.setProject(p)
	if err := s.loadCIRuns(key); err != nil {
		return err
	}

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

// ciRunsName is the file in a Project's folder holding its CI Runs.
const ciRunsName = "ci-runs.yaml"

// loadCIRuns reads the Project's ci-runs.yaml, if it has one.
func (s *FileStore) loadCIRuns(key string) error {
	rel := "projects/" + key + "/" + ciRunsName
	data, err := os.ReadFile(filepath.Join(s.dir, rel))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &LoadError{File: rel, Field: "file", Msg: err.Error()}
	}
	runs, lerr := decodeCIRuns(data)
	if lerr != nil {
		lerr.File = rel
		return lerr
	}
	s.projects[key].ciRuns = CIRuns{Runs: runs}
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
