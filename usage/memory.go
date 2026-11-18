// The in-memory fake's form of usage, beside the in-memory store.

package usage

import (
	"maps"
	"slices"
	"sync"
)

// Memory keeps usage in memory, as the in-memory store keeps its records.
type Memory struct {
	// mu is the usage's own lock, as Files's.
	mu       sync.RWMutex
	projects map[string]map[string]*memoryDay
	// failWith is what each Project's Appends return instead of adding.
	failWith map[string]error
}

// memoryDay is one Project's records for one day.
type memoryDay struct {
	calls     []Call
	responses []Response
}

// NewMemoryStore returns empty usage kept in memory.
func NewMemoryStore() *Memory { return &Memory{projects: map[string]map[string]*memoryDay{}} }

// FailAppends makes every later Append to project return err, as a full
// disk does the File Store's, until it is called with nil.
func (m *Memory) FailAppends(project string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		delete(m.failWith, project)
		return
	}
	if m.failWith == nil {
		m.failWith = map[string]error{}
	}
	m.failWith[project] = err
}

func (m *Memory) Append(project, day string, r Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.failWith[project]; err != nil {
		return errAppend(project, day, err)
	}
	if m.projects[project] == nil {
		m.projects[project] = map[string]*memoryDay{}
	}
	if m.projects[project][day] == nil {
		m.projects[project][day] = &memoryDay{}
	}
	d := m.projects[project][day]
	switch {
	case r.Call != nil:
		d.calls = append(d.calls, *r.Call)
	case r.Response != nil:
		d.responses = append(d.responses, *r.Response)
	}
	return nil
}

func (m *Memory) Read(project string, days []string) (Records, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	projects := []string{project}
	if project == "" {
		projects = slices.Sorted(maps.Keys(m.projects))
	}
	var out Records
	for _, day := range days {
		for _, p := range projects {
			d := m.projects[p][day]
			if d == nil {
				continue
			}
			out.Calls = append(out.Calls, d.calls...)
			for _, resp := range d.responses {
				resp.Project, resp.Day = p, day
				out.Responses = append(out.Responses, resp)
			}
		}
	}
	return out, nil
}

func (m *Memory) Days(project string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Sorted(maps.Keys(m.projects[project])), nil
}

func (m *Memory) Prune(before string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, days := range m.projects {
		maps.DeleteFunc(days, func(day string, _ *memoryDay) bool { return day < before })
	}
	return nil
}
