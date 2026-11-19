package core

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// A Factory's Heartbeat (Spec code-central-305): each running Factory
// announces itself with report_heartbeat on every poll. Code Central keeps
// the latest Heartbeat of each Factory in memory, writes it through to the
// Store's Services side for the other services sharing the Store (Spec
// code-central-1107), and works out the Factory Status from memory on
// every read.

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

// FactoryStatusNames are the Factory Statuses.
func FactoryStatusNames() []FactoryStatusName {
	return []FactoryStatusName{FactoryOnline, FactoryDraining, FactoryOffline}
}

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
	// PlanLimits are the most recently observed Plan Limits among the
	// Factories that aren't Offline (Spec code-central-861); left out
	// while none has any.
	PlanLimits *PlanLimits `json:"plan_limits,omitempty"`
}

// FactoryView is one Factory as the Project view lists it, from its last
// Heartbeat.
type FactoryView struct {
	Name   string            `json:"name"`
	Host   string            `json:"host"`
	Status FactoryStatusName `json:"status"`
	// Note is empty when none; a stopped Factory keeps its last one.
	Note            string    `json:"note"`
	LastHeartbeat   time.Time `json:"last_heartbeat"`
	IntervalSeconds int       `json:"interval_seconds"`
	// Chat is Chat support: the Factory can run Conversations. Chat is
	// available for the Project while an Online Factory has it.
	Chat bool `json:"chat"`
	// ReportsTo is the host of the Factory's Reporting Service, the
	// service its Heartbeats reach, when that is another service sharing
	// the Store; empty when it is this one.
	ReportsTo string `json:"reports_to,omitempty"`
}

// heartbeat is a Factory's latest Heartbeat.
type heartbeat struct {
	name, host string
	state      HeartbeatState
	note       string
	interval   int
	chat       bool
	at         time.Time
	// limits are the Factory's Plan Limits as kept, observed_at capped
	// at receipt, and reported are them as the Factory sent them; both
	// nil when it reports none.
	limits, reported *PlanLimits
	// service and serviceHost are the ID and host of the Factory's
	// Reporting Service.
	service, serviceHost string
}

type heartbeatKey struct{ project, name, host string }

// heartbeats holds each Factory's latest Heartbeat, in memory, as this
// service received it or last read it from the Store.
type heartbeats struct {
	mu  sync.Mutex
	all map[heartbeatKey]heartbeat
	// seen is each Project's Factory Status as the watch last saw it.
	seen map[string]string
	// base is each Project's reported bases (code-central-394), by
	// Repository Name (Spec code-central-848).
	base map[string]map[string]BaseView
	// clones is each Project's reported clones, by Repository Name
	// (Spec code-central-848).
	clones map[string]map[string]CloneView
	// reportsAt is when each Project's base and clone reports last
	// changed, here or on the service whose reports were last read, so
	// the newest wins.
	reportsAt map[string]time.Time
	// self is this service's ID: a Factory whose Reporting Service is
	// another lists that service's host.
	self string
}

// deadline is how long after h a Factory with no newer Heartbeat goes
// Offline.
func (h heartbeat) deadline() time.Duration {
	return max(heartbeatMissed*time.Duration(h.interval)*time.Second, heartbeatDeadlineFloor)
}

// status is h's status at now.
func (h heartbeat) status(now time.Time) FactoryStatusName {
	switch {
	case h.state == HeartbeatStopping, now.Sub(h.at) >= h.deadline():
		return FactoryOffline
	case h.state == HeartbeatDraining:
		return FactoryDraining
	}
	return FactoryOnline
}

// noticed says whether h, replacing prev (seen says whether there was
// one), changes what the human sees at now: so the Project's notice goes
// out. A steady Factory's Heartbeat changes nothing.
func noticed(prev heartbeat, seen bool, h heartbeat, now time.Time) bool {
	return !seen || prev.status(now) != h.status(now) || prev.note != h.note || prev.chat != h.chat ||
		prev.service != h.service || !sameLimits(prev.limits, h.limits)
}

// view is project's Factory Status at now.
func (hs *heartbeats) view(project string, now time.Time) FactoryStatusView {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	var latest heartbeatKey // the Factory whose Plan Limits v holds
	v := FactoryStatusView{Status: FactoryOffline, Factories: []FactoryView{}}
	for k, h := range hs.all {
		if k.project != project {
			continue
		}
		s := h.status(now)
		var reportsTo string
		if h.service != hs.self {
			reportsTo = h.serviceHost
		}
		v.Factories = append(v.Factories, FactoryView{Name: h.name, Host: h.host, Status: s, Note: h.note, LastHeartbeat: h.at,
			IntervalSeconds: h.interval, Chat: h.chat, ReportsTo: reportsTo})
		switch {
		case s == FactoryOnline, s == FactoryDraining && v.Status == FactoryOffline:
			v.Status = s
		}
		if s != FactoryOffline && h.limits != nil && (v.PlanLimits == nil || newerLimits(*h.limits, k, *v.PlanLimits, latest)) {
			v.PlanLimits, latest = h.limits, k
		}
	}
	// The Home Repository's, named by the Project Key.
	v.Base = hs.baseView(project, project, now)
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
	// Chat is Chat support: the Factory can run Conversations. It
	// defaults to false, so a Factory that doesn't know Chat has none.
	Chat bool `json:"chat"`
	// PlanLimits are the Plan Limits the Factory last read from its
	// Turns; left out when it has none. Plan Limits with neither window
	// are none.
	PlanLimits *PlanLimits `json:"plan_limits,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportHeartbeat is the Factory's report_heartbeat. The Heartbeat replaces
// the earlier one of the same factory and host, in memory and then in the
// Store, with this service as its Reporting Service; a Store it can't
// write to is logged, and the Heartbeat still counts here. It sends the
// Project's notice only when a Factory is new or what the human sees of it
// changed, so a steady Factory doesn't make the UI refetch each poll.
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
	reported, err := checkPlanLimits(in.PlanLimits)
	if err != nil {
		return Written[ProjectView]{}, err
	}
	note := strings.TrimSpace(in.Note)
	if r := []rune(note); len(r) > maxHeartbeatNote {
		note = strings.TrimSpace(string(r[:maxHeartbeatNote]))
	}
	var before ProjectView
	var project domain.Project
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
	h := heartbeat{name: name, host: host, state: in.State, note: note, interval: in.IntervalSeconds, chat: in.Chat, at: now, reported: reported,
		service: c.service.ID, serviceHost: c.service.Host}
	hs := c.heartbeats
	hs.mu.Lock()
	prev, seen := hs.all[k]
	h.limits = keptLimits(prev, reported, now)
	hs.all[k] = h
	cleared := !seen || prev.status(now) == FactoryOffline || in.State == HeartbeatStopping
	if cleared {
		// A Factory that is new, back from a lapse or stopping holds
		// no base or clone report of its own to trust.
		delete(hs.base, key)
		delete(hs.clones, key)
		hs.reportsAt[key] = now
	}
	hs.mu.Unlock()
	c.recordHeartbeat(k, h)
	if cleared {
		c.recordReports(key)
	}
	if noticed(prev, seen, h, now) {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
	c.chatAfterHeartbeat(key)
	after := c.projectView(project)
	return acknowledgeProject(before, after, in.Full, now)
}

// WatchFactoryStatus is the background ticker behind Going Offline with no
// write. On each tick it reads every ping (see refreshPings), Heartbeat and
// Factory report from the Store, so it sees those other services received
// (see refreshFactories), then compares every Project's Factory Status,
// combined and per Factory, with what it saw last time and
// calls changed(project) for each Project that differs, or whose Factories
// another service changed, so live views refetch. It also settles each
// Conversation (see settleChat), which sends its own chat.changed. The
// time comes from the injected clock, not the tick; serve ticks every 10s.
// It returns when ctx ends.
func (c *Core) WatchFactoryStatus(ctx context.Context, ticks <-chan time.Time, changed func(project string)) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			c.refreshPings()
			refreshed := c.refreshFactories()
			for _, p := range c.factoryStatusChanges() {
				if !slices.Contains(refreshed, p) {
					refreshed = append(refreshed, p)
				}
			}
			for _, p := range refreshed {
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
