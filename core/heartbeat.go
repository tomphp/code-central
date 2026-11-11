package core

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// A Factory's Heartbeat (Spec code-central-305): each running Factory
// announces itself with report_heartbeat on every poll. Code Central keeps
// the latest Heartbeat of each Factory in memory only, never in the store,
// and works out the Factory Status from them on every read.

// HeartbeatState is the state a Factory reports in a Heartbeat.
type HeartbeatState string

const (
	HeartbeatOnline   HeartbeatState = "online"
	HeartbeatDraining HeartbeatState = "draining"
	HeartbeatStopping HeartbeatState = "stopping"
)

// FactoryStatusName is a Factory's, or a Project's combined, Factory Status.
type FactoryStatusName string

const (
	FactoryOnline   FactoryStatusName = "online"
	FactoryDraining FactoryStatusName = "draining"
	FactoryOffline  FactoryStatusName = "offline"
)

const (
	maxHeartbeatNote       = 200
	maxHeartbeatInterval   = 86400
	heartbeatDeadlineFloor = 90 * time.Second
	heartbeatMissed        = 3
)

// FactoryStatusView is a Project's Factory Status as every Project read
// returns it, always filled in.
type FactoryStatusView struct {
	// Status is the combined status: Online if any Factory is, else
	// Draining if any is, else Offline (also with no Factories).
	Status    FactoryStatusName `json:"status"`
	Factories []FactoryView     `json:"factories"`
	// Base is the base the Factory last reported under Pull Request
	// (code-central-394); left out while none is reporting.
	Base *BaseView `json:"base,omitempty"`
}

// FactoryView is one Factory as the Project view lists it.
type FactoryView struct {
	Name   string            `json:"name"`
	Host   string            `json:"host"`
	Status FactoryStatusName `json:"status"`
	// Note is empty when none; a stopped Factory keeps its last one.
	Note            string    `json:"note"`
	LastHeartbeat   time.Time `json:"last_heartbeat"`
	IntervalSeconds int       `json:"interval_seconds"`
}

// heartbeat is a Factory's latest Heartbeat.
type heartbeat struct {
	name, host string
	state      HeartbeatState
	note       string
	interval   int
	at         time.Time
}

type heartbeatKey struct{ project, name, host string }

// heartbeats holds each Factory's latest Heartbeat, in memory only.
type heartbeats struct {
	mu  sync.Mutex
	all map[heartbeatKey]heartbeat
	// seen is each Project's Factory Status as the watch last saw it.
	seen map[string]string
	// base is each Project's reported base (code-central-394).
	base map[string]BaseView
}

// status is h's status at now.
func (h heartbeat) status(now time.Time) FactoryStatusName {
	deadline := max(heartbeatMissed*time.Duration(h.interval)*time.Second, heartbeatDeadlineFloor)
	switch {
	case h.state == HeartbeatStopping, now.Sub(h.at) >= deadline:
		return FactoryOffline
	case h.state == HeartbeatDraining:
		return FactoryDraining
	}
	return FactoryOnline
}

// view is project's Factory Status at now.
func (hs *heartbeats) view(project string, now time.Time) FactoryStatusView {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	v := FactoryStatusView{Status: FactoryOffline, Factories: []FactoryView{}}
	for k, h := range hs.all {
		if k.project != project {
			continue
		}
		s := h.status(now)
		v.Factories = append(v.Factories, FactoryView{Name: h.name, Host: h.host, Status: s, Note: h.note, LastHeartbeat: h.at, IntervalSeconds: h.interval})
		switch {
		case s == FactoryOnline, s == FactoryDraining && v.Status == FactoryOffline:
			v.Status = s
		}
	}
	v.Base = hs.baseView(project, now)
	slices.SortFunc(v.Factories, func(a, b FactoryView) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Host, b.Host)
	})
	return v
}

// ReportHeartbeatInput is the Factory's report_heartbeat(factory, host,
// state, note?, interval_seconds, full?).
type ReportHeartbeatInput struct {
	Factory string         `json:"factory"`
	Host    string         `json:"host"`
	State   HeartbeatState `json:"state"`
	// Note is free text, cut to 200 characters; Code Central never
	// interprets it.
	Note string `json:"note"`
	// IntervalSeconds is the Factory's poll interval, 1 to 86400.
	IntervalSeconds int `json:"interval_seconds"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportHeartbeat is the Factory's report_heartbeat. The Heartbeat replaces
// the earlier one of the same factory and host, and is kept in memory only.
// It sends the Project's notice only when a Factory is new or its status or
// note changed, so a steady Factory doesn't make the UI refetch each poll.
func (c *Core) ReportHeartbeat(_ context.Context, key string, in ReportHeartbeatInput) (Written[ProjectView], error) {
	name, host := strings.TrimSpace(in.Factory), strings.TrimSpace(in.Host)
	switch {
	case name == "":
		return Written[ProjectView]{}, invalid("factory", "factory is required: the Factory's name")
	case host == "":
		return Written[ProjectView]{}, invalid("host", "host is required: the machine the Factory runs on")
	case !slices.Contains([]HeartbeatState{HeartbeatOnline, HeartbeatDraining, HeartbeatStopping}, in.State):
		return Written[ProjectView]{}, invalid("state", "state is one of online, draining, stopping, not %q", in.State)
	case in.IntervalSeconds < 1 || in.IntervalSeconds > maxHeartbeatInterval:
		return Written[ProjectView]{}, invalid("interval_seconds", "interval_seconds is from 1 to %d, not %d", maxHeartbeatInterval, in.IntervalSeconds)
	}
	note := strings.TrimSpace(in.Note)
	if r := []rune(note); len(r) > maxHeartbeatNote {
		note = strings.TrimSpace(string(r[:maxHeartbeatNote]))
	}
	var before ProjectView
	var project store.Project
	if err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		project, _ = r.Project(key)
		before = c.projectView(project)
		return nil
	}); err != nil {
		return Written[ProjectView]{}, err
	}
	now := c.now()
	k := heartbeatKey{key, name, host}
	h := heartbeat{name: name, host: host, state: in.State, note: note, interval: in.IntervalSeconds, at: now}
	hs := c.heartbeats
	hs.mu.Lock()
	prev, seen := hs.all[k]
	hs.all[k] = h
	if !seen || prev.status(now) == FactoryOffline || in.State == HeartbeatStopping {
		// A Factory that is new, back from a lapse or stopping holds
		// no base report of its own to trust.
		delete(hs.base, key)
	}
	hs.mu.Unlock()
	if !seen || prev.status(now) != h.status(now) || prev.note != note {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
	after := c.projectView(project)
	return acknowledgeProject(before, after, in.Full, now)
}

// WatchFactoryStatus is the background ticker behind Going Offline with no
// write. On each tick it compares every Project's Factory Status, combined
// and per Factory, with what it saw last time and calls changed(project)
// for each Project that differs, so live views refetch. The time comes from
// the injected clock, not the tick; serve ticks every 10s. It returns when
// ctx ends.
func (c *Core) WatchFactoryStatus(ctx context.Context, ticks <-chan time.Time, changed func(project string)) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			for _, p := range c.factoryStatusChanges() {
				changed(p)
			}
		}
	}
}

// factoryStatusChanges returns the Projects whose Factory Status changed
// since the last call, and remembers the new ones.
func (c *Core) factoryStatusChanges() []string {
	now := c.clock.Now()
	var keys []string
	c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			keys = append(keys, p.Key)
		}
		return nil
	})
	hs := c.heartbeats
	var out []string
	for _, key := range keys {
		v := hs.view(key, now)
		sig := string(v.Status)
		if v.Base != nil {
			sig += "\x00base\x00" + string(v.Base.State)
		}
		for _, f := range v.Factories {
			sig += "\x00" + f.Name + "\x00" + f.Host + "\x00" + string(f.Status)
		}
		hs.mu.Lock()
		prev, ok := hs.seen[key]
		if !ok {
			prev = string(FactoryOffline) // a Project with no Factories
		}
		hs.seen[key] = sig
		hs.mu.Unlock()
		if sig != prev {
			out = append(out, key)
		}
	}
	return out
}
