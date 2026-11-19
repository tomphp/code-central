package core

import (
	"cmp"
	"context"
	"fmt"
	"runtime/metrics"
	"slices"
	"time"

	"github.com/tomoram/code-central/store"
)

// The Storage tab (Spec code-central-1032): where this service's Store is
// and how big, each Project's records, the process's memory, and the
// services registered on the Store. Every figure is as it is now: no
// history is kept.

// GetStorageInput is a Storage read: one Project's counts, or every
// Project's (Archived ones too) when Project is "". The Store, the memory
// and the services are whole either way.
type GetStorageInput struct {
	Project string `json:"project,omitempty" query:"project" desc:"Count only this Project's records; every Project's when left out."`
}

// StorageView is get_storage.
type StorageView struct {
	// Kind is the Store Kind: files, sqlite or postgres; memory for the
	// in-memory fake.
	Kind string `json:"kind"`
	// Path is the File Store's folder or the SQLite database file; empty
	// for Postgres and the fake.
	Path string `json:"path,omitempty"`
	// Host, Port and Database are the Postgres Store's address, as its
	// URL names them, without its user or password; empty for the other
	// Kinds.
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Database string `json:"database,omitempty"`
	// Size is the Store's size in bytes, Usage included, as its Kind can
	// tell it.
	Size int64 `json:"size"`
	// Projects are the Projects' counts, in key order: the scoped one's
	// alone, or every one's.
	Projects []StorageProject `json:"projects"`
	Memory   ProcessMemory    `json:"memory"`
	// Services are the live Service Registrations, in host order.
	Services []StorageRegistration `json:"services"`
}

// StorageProject is one Project's count of records.
type StorageProject struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Specs and Tickets are its Specs and Tickets, in every State.
	Specs   int `json:"specs"`
	Tickets int `json:"tickets"`
	// UsageDays is how many days of Usage it keeps.
	UsageDays int `json:"usage_days"`
}

// ProcessMemory is this service's memory.
type ProcessMemory struct {
	// Resident is the process's resident size in bytes, as the operating
	// system counts it; 0 where it doesn't say.
	Resident int64 `json:"resident,omitempty"`
	// Heap is the bytes of Go heap the process's objects occupy.
	Heap int64 `json:"heap"`
}

// StorageRegistration is a live Service Registration.
type StorageRegistration struct {
	Host    string    `json:"host"`
	Version string    `json:"version"`
	Renewed time.Time `json:"renewed"`
}

// GetStorage is get_storage.
func (c *Core) GetStorage(_ context.Context, in GetStorageInput) (StorageView, error) {
	var projects []StorageProject
	found := false
	err := c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			if in.Project != "" && p.Key != in.Project {
				continue
			}
			found = true
			tickets := 0
			for _, n := range r.StateCounts(p.Key) {
				tickets += n
			}
			projects = append(projects, StorageProject{Key: p.Key, Name: p.DisplayName, Specs: r.SpecCount(p.Key), Tickets: tickets})
		}
		return nil
	})
	if err != nil {
		return StorageView{}, err
	}
	if in.Project != "" && !found {
		return StorageView{}, notFound("no Project %q", in.Project)
	}
	slices.SortFunc(projects, func(a, b StorageProject) int { return cmp.Compare(a.Key, b.Key) })
	days := c.usage.Days()
	for i := range projects {
		projects[i].UsageDays = days[projects[i].Key]
	}
	size, err := c.store.Size()
	if err != nil {
		return StorageView{}, fmt.Errorf("cannot measure the store: %w", err)
	}
	regs, err := c.store.Services().Registrations(c.now())
	if err != nil {
		return StorageView{}, err
	}
	services := make([]StorageRegistration, len(regs))
	for i, r := range regs {
		services[i] = StorageRegistration{Host: r.Host, Version: r.Version, Renewed: r.Renewed}
	}
	l := c.store.Location()
	v := StorageView{Kind: string(l.Kind), Host: l.Host, Port: l.Port, Database: l.Database, Size: size,
		Projects: projects, Memory: processMemory(), Services: services}
	if l.Kind != store.Postgres {
		v.Path = l.Path
	}
	if v.Projects == nil {
		v.Projects = []StorageProject{}
	}
	return v, nil
}

// heapObjects is the runtime metric of the bytes the heap's objects
// occupy, live and not yet swept.
const heapObjects = "/memory/classes/heap/objects:bytes"

// processMemory is the process's memory now.
func processMemory() ProcessMemory {
	sample := []metrics.Sample{{Name: heapObjects}}
	metrics.Read(sample)
	m := ProcessMemory{Resident: residentSize()}
	if sample[0].Value.Kind() == metrics.KindUint64 {
		m.Heap = int64(sample[0].Value.Uint64())
	}
	return m
}
