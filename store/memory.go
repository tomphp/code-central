package store

import (
	"cmp"
	"slices"
	"sync"
)

// index is the in-memory copy of the whole store. The file-backed store
// wraps it with a persist hook; the fake uses it bare.
type index struct {
	mu       sync.RWMutex
	projects map[string]*projectData
	// persist, when set, makes a record durable before memory changes.
	persist func(record any, project string) error
}

type projectData struct {
	project Project
	specs   map[int]Spec
	tickets map[int]Ticket
}

func newIndex() *index { return &index{projects: map[string]*projectData{}} }

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

type reader struct{ x *index }

func (r reader) Project(key string) (Project, bool) {
	p, ok := r.x.projects[key]
	if !ok {
		return Project{}, false
	}
	return p.project, true
}

func (r reader) Projects() []Project {
	out := make([]Project, 0, len(r.x.projects))
	for _, p := range r.x.projects {
		out = append(out, p.project)
	}
	slices.SortFunc(out, func(a, b Project) int { return cmp.Compare(a.Key, b.Key) })
	return out
}

func (r reader) Spec(project string, n int) (Spec, bool) {
	p, ok := r.x.projects[project]
	if !ok {
		return Spec{}, false
	}
	s, ok := p.specs[n]
	return s.clone(), ok
}

func (r reader) Specs(project string) []Spec {
	p, ok := r.x.projects[project]
	if !ok {
		return nil
	}
	out := make([]Spec, 0, len(p.specs))
	for _, s := range p.specs {
		out = append(out, s.clone())
	}
	slices.SortFunc(out, func(a, b Spec) int { return cmp.Compare(a.Number, b.Number) })
	return out
}

func (r reader) Ticket(project string, n int) (Ticket, bool) {
	p, ok := r.x.projects[project]
	if !ok {
		return Ticket{}, false
	}
	t, ok := p.tickets[n]
	return t.clone(), ok
}

func (r reader) Tickets(project string) []Ticket {
	p, ok := r.x.projects[project]
	if !ok {
		return nil
	}
	out := make([]Ticket, 0, len(p.tickets))
	for _, t := range p.tickets {
		out = append(out, t.clone())
	}
	slices.SortFunc(out, func(a, b Ticket) int { return cmp.Compare(a.Number, b.Number) })
	return out
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

type writer struct {
	reader
	wrote bool
}

func (w *writer) begin(record any, project string) error {
	if w.wrote {
		return ErrSecondWrite
	}
	if _, ok := record.(Project); !ok {
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

func (w *writer) PutProject(p Project) error {
	if err := w.begin(p, p.Key); err != nil {
		return err
	}
	w.x.setProject(p)
	return nil
}

func (w *writer) PutSpec(project string, s Spec) error {
	s = s.clone()
	if err := w.begin(s, project); err != nil {
		return err
	}
	w.x.projects[project].specs[s.Number] = s
	return nil
}

func (w *writer) PutTicket(project string, t Ticket) error {
	t = t.clone()
	if err := w.begin(t, project); err != nil {
		return err
	}
	w.x.projects[project].tickets[t.Number] = t
	return nil
}

func (x *index) setProject(p Project) {
	if d, ok := x.projects[p.Key]; ok {
		d.project = p
		return
	}
	x.projects[p.Key] = &projectData{project: p, specs: map[int]Spec{}, tickets: map[int]Ticket{}}
}
