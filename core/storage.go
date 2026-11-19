package core

import (
	"cmp"
	"context"
	"fmt"
	"runtime/metrics"
	"slices"

	"github.com/tomoram/code-central/store"
)

// The Storage tab (Spec code-central-1032): where this service's Store is
// and how big, each Project's records, the process's memory, and the
// services registered on the Store. Every figure is as it is now: no
// history is kept.

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
