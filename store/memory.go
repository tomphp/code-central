package store

import (
	"cmp"
	"fmt"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/usage"
	"slices"
	"sync"
)

// index is the in-memory copy of the whole store. The file-backed store
// wraps it with a persist hook; the fake uses it bare.
type index struct {
	mu       sync.RWMutex
	projects map[string]*projectData
	options  domain.Options
	// persist, when set, makes a record durable before memory changes.
	persist func(record any, project string) error
	// usage is where the Store keeps Usage: in memory here, the File
	// Store's files in the file-backed store.
	usage usage.Store
}

// Usage is the Store's Usage (ADR 0066): appended to, read and pruned
// outside any Update, under its own lock.
func (x *index) Usage() usage.Store { return x.usage }

type projectData struct {
	project domain.Project
	specs   map[int]domain.Spec
	tickets map[int]domain.Ticket
	ciRuns  domain.CIRuns
}

func newIndex() *index {
	return &index{projects: map[string]*projectData{}, options: domain.DefaultOptions(), usage: usage.NewMemoryStore()}
}

// NewMemory returns the in-memory fake store.
func NewMemory() Store { return newIndex() }

func (x *index) View(fn func(Reader) error) error {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return fn(reader{x})
}

func (x *index) Update(fn func(Writer) error) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	return fn(&writer{reader: reader{x}})
}

// Changes never delivers: no other service shares the store.
func (x *index) Changes(func(Change)) {}

type reader struct{ x *index }

func (r reader) Project(key string) (domain.Project, bool) {
	p, ok := r.x.projects[key]
	if !ok {
		return domain.Project{}, false
	}
	return p.project, true
}

func (r reader) Projects() []domain.Project {
	out := make([]domain.Project, 0, len(r.x.projects))
	for _, p := range r.x.projects {
		out = append(out, p.project)
	}
	slices.SortFunc(out, func(a, b domain.Project) int { return cmp.Compare(a.Key, b.Key) })
	return out
}

func (r reader) Spec(project string, n int) (domain.Spec, bool) {
	p, ok := r.x.projects[project]
	if !ok {
		return domain.Spec{}, false
	}
	s, ok := p.specs[n]
	return cloneSpec(s), ok
}

// readOf is a copy of the stored Ticket t with only the History parts
// named, as every Store Kind reads one.
func readOf(t domain.Ticket, parts []Part) domain.Ticket {
	return cloneTicket(withParts(t, domain.Ticket{}, parts))
}

func (r reader) Ticket(project string, n int, parts ...Part) (domain.Ticket, bool) {
	p, ok := r.x.projects[project]
	if !ok {
		return domain.Ticket{}, false
	}
	t, ok := p.tickets[n]
	return readOf(t, parts), ok
}

func (r reader) NextNumber(project string) int {
	p, ok := r.x.projects[project]
	if !ok {
		return 1
	}
	high := 0
	for n := range p.specs {
		high = max(high, n)
	}
	for n := range p.tickets {
		high = max(high, n)
	}
	return high + 1
}

func (r reader) Options() domain.Options { return r.x.options }

func (r reader) CIRuns(project string) []domain.CIRun {
	p, ok := r.x.projects[project]
	if !ok {
		return nil
	}
	return cloneCIRuns(p.ciRuns).Runs
}

type writer struct {
	reader
	wrote bool
}

func (w *writer) begin(record any, project string) error {
	if w.wrote {
		return ErrSecondWrite
	}
	switch record.(type) {
	case domain.Project, domain.Options:
	default:
		if _, ok := w.x.projects[project]; !ok {
			return ErrUnknownProject
		}
	}
	if w.x.persist != nil {
		if err := w.x.persist(record, project); err != nil {
			return err
		}
	}
	w.wrote = true
	return nil
}

func (w *writer) PutProject(p domain.Project) error {
	if err := w.begin(p, p.Key); err != nil {
		return err
	}
	w.x.setProject(p)
	return nil
}

func (w *writer) PutSpec(project string, s domain.Spec) error {
	s = cloneSpec(s)
	if err := w.begin(s, project); err != nil {
		return err
	}
	w.x.projects[project].specs[s.Number] = s
	return nil
}

func (w *writer) PutTicket(project string, t domain.Ticket, parts ...Part) error {
	var stored domain.Ticket
	if p, ok := w.x.projects[project]; ok {
		stored = p.tickets[t.Number]
	}
	t = withHead(cloneTicket(withParts(t, stored, parts)))
	if err := w.begin(t, project); err != nil {
		return err
	}
	w.x.projects[project].tickets[t.Number] = t
	return nil
}

func (w *writer) PutImport(project string, im Import) error {
	im = im.clone()
	for i, t := range im.Tickets {
		im.Tickets[i] = withHead(t)
	}
	for i, t := range im.Rewritten {
		im.Rewritten[i] = withHead(t)
	}
	if p, ok := w.x.projects[project]; ok {
		hasSpec := func(n int) bool { _, ok := p.specs[n]; return ok }
		hasTicket := func(n int) bool { _, ok := p.tickets[n]; return ok }
		if err := checkImport(project, im, hasSpec, hasTicket); err != nil {
			return err
		}
	}
	if err := w.begin(im, project); err != nil {
		return err
	}
	p := w.x.projects[project]
	for _, s := range im.Specs {
		p.specs[s.Number] = s
	}
	for _, t := range im.Tickets {
		p.tickets[t.Number] = t
	}
	for _, t := range im.Rewritten {
		p.tickets[t.Number] = t
	}
	for _, sp := range im.RewrittenSpecs {
		p.specs[sp.Number] = sp
	}
	return nil
}

// checkImport is the Import's numbering rule, which every Kind keeps:
// a new Spec or Ticket takes a number no record has (ErrExists), and a
// rewritten one the number of a record of its kind (ErrNotExists), each
// number once in the Import.
func checkImport(project string, im Import, hasSpec, hasTicket func(int) bool) error {
	seen := map[int]bool{}
	taken := func(n int) bool {
		dup := seen[n]
		seen[n] = true
		return hasSpec(n) || hasTicket(n) || dup
	}
	for _, s := range im.Specs {
		if taken(s.Number) {
			return fmt.Errorf("%w: %s-%d", ErrExists, project, s.Number)
		}
	}
	for _, t := range im.Tickets {
		if taken(t.Number) {
			return fmt.Errorf("%w: %s-%d", ErrExists, project, t.Number)
		}
	}
	for _, t := range im.Rewritten {
		if !hasTicket(t.Number) || seen[t.Number] {
			return fmt.Errorf("%w: %s-%d", ErrNotExists, project, t.Number)
		}
		seen[t.Number] = true
	}
	for _, sp := range im.RewrittenSpecs {
		if !hasSpec(sp.Number) || seen[sp.Number] {
			return fmt.Errorf("%w: %s-%d", ErrNotExists, project, sp.Number)
		}
		seen[sp.Number] = true
	}
	return nil
}

// checkOptions refuses an Option set to a value it can't take.
func checkOptions(o domain.Options) error {
	if !o.FocusClear.Valid() {
		return fmt.Errorf("%w: focus_clear %q", ErrInvalidOption, o.FocusClear)
	}
	if !o.LimitsPopup.Valid() {
		return fmt.Errorf("%w: limits_popup %q", ErrInvalidOption, o.LimitsPopup)
	}
	if !o.FocusPopup.Valid() {
		return fmt.Errorf("%w: focus_popup %q", ErrInvalidOption, o.FocusPopup)
	}
	return nil
}

func (w *writer) PutOptions(o domain.Options) error {
	if err := checkOptions(o); err != nil {
		return err
	}
	if err := w.begin(o, ""); err != nil {
		return err
	}
	w.x.options = o
	return nil
}

// Announce is ignored: no other service shares the store.
func (w *writer) Announce(Change) {}

func (w *writer) PutCIRuns(project string, runs []domain.CIRun) error {
	rs := cloneCIRuns(domain.CIRuns{Runs: runs})
	if err := w.begin(rs, project); err != nil {
		return err
	}
	w.x.projects[project].ciRuns = rs
	return nil
}

func (x *index) setProject(p domain.Project) {
	p.Landing = landingOrDefault(p.Landing)
	if d, ok := x.projects[p.Key]; ok {
		d.project = p
		return
	}
	x.projects[p.Key] = &projectData{project: p, specs: map[int]domain.Spec{}, tickets: map[int]domain.Ticket{}}
}
