package core

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// A Factory's Heartbeat (Spec code-central-305): each running Factory
// announces itself with report_heartbeat on every poll. Code Central keeps
// the latest Heartbeat of each Factory in memory only, never in the store,
// and works out the Factory Status from them on every read.

const (
	maxHeartbeatNote       = 200
	maxHeartbeatInterval   = 86400
	heartbeatDeadlineFloor = 90 * time.Second
	heartbeatMissed        = 3
)

// heartbeat is a Factory's latest Heartbeat.
type heartbeat struct {
	name, host string
	state      api.HeartbeatState
	note       string
	interval   int
	chat       bool
	at         time.Time
	// limits are the Factory's Plan Limits as kept, observed_at capped
	// at receipt, and reported are them as the Factory sent them; both
	// nil when it reports none.
	limits, reported *api.PlanLimits
}

type heartbeatKey struct{ project, name, host string }

// heartbeats holds each Factory's latest Heartbeat, in memory only.
type heartbeats struct {
	mu  sync.Mutex
	all map[heartbeatKey]heartbeat
	// seen is each Project's Factory Status as the watch last saw it.
	seen map[string]string
	// base is each Project's reported bases (code-central-394), by
	// Repository Name (Spec code-central-848).
	base map[string]map[string]api.BaseView
	// clones is each Project's reported clones, by Repository Name
	// (Spec code-central-848).
	clones map[string]map[string]api.CloneView
}

// status is h's status at now.
func (h heartbeat) status(now time.Time) api.FactoryStatusName {
	deadline := max(heartbeatMissed*time.Duration(h.interval)*time.Second, heartbeatDeadlineFloor)
	switch {
	case h.state == api.HeartbeatStopping, now.Sub(h.at) >= deadline:
		return api.FactoryOffline
	case h.state == api.HeartbeatDraining:
		return api.FactoryDraining
	}
	return api.FactoryOnline
}

// view is project's Factory Status at now.
func (hs *heartbeats) view(project string, now time.Time) api.FactoryStatusView {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	var latest heartbeatKey // the Factory whose Plan Limits v holds
	v := api.FactoryStatusView{Status: api.FactoryOffline, Factories: []api.FactoryView{}}
	for k, h := range hs.all {
		if k.project != project {
			continue
		}
		s := h.status(now)
		v.Factories = append(v.Factories, api.FactoryView{Name: h.name, Host: h.host, Status: s, Note: h.note, LastHeartbeat: h.at, IntervalSeconds: h.interval, Chat: h.chat})
		switch {
		case s == api.FactoryOnline, s == api.FactoryDraining && v.Status == api.FactoryOffline:
			v.Status = s
		}
		if s != api.FactoryOffline && h.limits != nil && (v.PlanLimits == nil || newerLimits(*h.limits, k, *v.PlanLimits, latest)) {
			v.PlanLimits, latest = h.limits, k
		}
	}
	// The Home Repository's, named by the Project Key.
	v.Base = hs.baseView(project, project, now)
	slices.SortFunc(v.Factories, func(a, b api.FactoryView) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Host, b.Host)
	})
	return v
}

// ReportHeartbeat is the Factory's report_heartbeat. The Heartbeat replaces
// the earlier one of the same factory and host, and is kept in memory only.
// It sends the Project's notice only when a Factory is new or its status or
// note changed, so a steady Factory doesn't make the UI refetch each poll.
func (c *Core) ReportHeartbeat(_ context.Context, key string, in api.ReportHeartbeatInput) (api.Written[api.ProjectView], error) {
	name, host := strings.TrimSpace(in.Factory), strings.TrimSpace(in.Host)
	switch {
	case name == "":
		return api.Written[api.ProjectView]{}, invalid("factory", "factory is required: the Factory's name")
	case host == "":
		return api.Written[api.ProjectView]{}, invalid("host", "host is required: the machine the Factory runs on")
	case !slices.Contains([]api.HeartbeatState{api.HeartbeatOnline, api.HeartbeatDraining, api.HeartbeatStopping}, in.State):
		return api.Written[api.ProjectView]{}, invalid("state", "state is one of online, draining, stopping, not %q", in.State)
	case in.IntervalSeconds < 1 || in.IntervalSeconds > maxHeartbeatInterval:
		return api.Written[api.ProjectView]{}, invalid("interval_seconds", "interval_seconds is from 1 to %d, not %d", maxHeartbeatInterval, in.IntervalSeconds)
	}
	reported, err := checkPlanLimits(in.PlanLimits)
	if err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	note := strings.TrimSpace(in.Note)
	if r := []rune(note); len(r) > maxHeartbeatNote {
		note = strings.TrimSpace(string(r[:maxHeartbeatNote]))
	}
	var before api.ProjectView
	var project domain.Project
	if err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		project, _ = r.Project(key)
		before = c.projectView(project)
		return nil
	}); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	now := c.now()
	k := heartbeatKey{key, name, host}
	h := heartbeat{name: name, host: host, state: in.State, note: note, interval: in.IntervalSeconds, chat: in.Chat, at: now, reported: reported}
	hs := c.heartbeats
	hs.mu.Lock()
	prev, seen := hs.all[k]
	h.limits = keptLimits(prev, reported, now)
	hs.all[k] = h
	if !seen || prev.status(now) == api.FactoryOffline || in.State == api.HeartbeatStopping {
		// A Factory that is new, back from a lapse or stopping holds
		// no base or clone report of its own to trust.
		delete(hs.base, key)
		delete(hs.clones, key)
	}
	hs.mu.Unlock()
	if !seen || prev.status(now) != h.status(now) || prev.note != note || prev.chat != in.Chat || !sameLimits(prev.limits, h.limits) {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
	c.chatAfterHeartbeat(key)
	after := c.projectView(project)
	return acknowledgeProject(before, after, in.Full, now)
}

// WatchFactoryStatus is the background ticker behind Going Offline with no
// write. On each tick it compares every Project's Factory Status, combined
// and per Factory, with what it saw last time and calls changed(project)
// for each Project that differs, so live views refetch. It also settles
// each Conversation (see settleChat), which sends its own chat.changed.
// The time comes from the injected clock, not the tick; serve ticks every
// 10s. It returns when ctx ends.
func (c *Core) WatchFactoryStatus(ctx context.Context, ticks <-chan time.Time, changed func(project string)) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			for _, p := range c.factoryStatusChanges() {
				changed(p)
			}
			c.watchChats()
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
		if v.PlanLimits != nil {
			sig += "\x00plan_limits"
		}
		for _, f := range v.Factories {
			sig += "\x00" + f.Name + "\x00" + f.Host + "\x00" + string(f.Status)
		}
		hs.mu.Lock()
		prev, ok := hs.seen[key]
		if !ok {
			prev = string(api.FactoryOffline) // a Project with no Factories
		}
		hs.seen[key] = sig
		hs.mu.Unlock()
		if sig != prev {
			out = append(out, key)
		}
	}
	return out
}
