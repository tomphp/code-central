package core

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"time"

	"github.com/tomoram/code-central/store"
)

// Heartbeats shared across services (Spec code-central-1107): each
// Heartbeat, and each Project's base and clone reports, are written
// through to the Store's Services side after memory, and every service
// reads them all back on each Factory Status tick, so a Factory shows on
// every service sharing the Store whichever one it reports to. Reads stay
// in memory. A Store that can't be reached is logged and never fails the
// report: the service carries on with what it has.

// heartbeatExpiry is how long a Factory stays listed once Offline before
// every service, and the Store, forget it.
const heartbeatExpiry = 24 * time.Hour

// heartbeatDoc is a Heartbeat as the Store keeps it.
type heartbeatDoc struct {
	State           HeartbeatState `json:"state"`
	Note            string         `json:"note,omitempty"`
	IntervalSeconds int            `json:"interval_seconds"`
	Chat            bool           `json:"chat,omitempty"`
	At              time.Time      `json:"at"`
	PlanLimits      *PlanLimits    `json:"plan_limits,omitempty"`
	Reported        *PlanLimits    `json:"reported,omitempty"`
}

// factoryReportsDoc is a Project's base and clone reports as the Store
// keeps them, with when they last changed.
type factoryReportsDoc struct {
	At     time.Time            `json:"at"`
	Bases  map[string]BaseView  `json:"bases,omitempty"`
	Clones map[string]CloneView `json:"clones,omitempty"`
}

// SetLog makes c log what it can't write to or read from the Store, and
// carries on without, to w. Without one, nothing is logged.
func (c *Core) SetLog(w io.Writer) { c.log = w }

// logf logs a line to c's log, if it has one.
func (c *Core) logf(format string, a ...any) {
	if c.log != nil {
		fmt.Fprintf(c.log, "code-central serve: "+format+"\n", a...)
	}
}

// offlineAt is when h's Factory went, or goes, Offline with no newer
// Heartbeat.
func (h heartbeat) offlineAt() time.Time {
	if h.state == HeartbeatStopping {
		return h.at
	}
	return h.at.Add(h.deadline())
}

// expires is when every service forgets h.
func (h heartbeat) expires() time.Time { return h.offlineAt().Add(heartbeatExpiry) }

// recordHeartbeat writes h, the Heartbeat of k, through to the Store.
func (c *Core) recordHeartbeat(k heartbeatKey, h heartbeat) {
	doc, err := json.Marshal(heartbeatDoc{State: h.state, Note: h.note, IntervalSeconds: h.interval, Chat: h.chat, At: h.at,
		PlanLimits: h.limits, Reported: h.reported})
	if err == nil {
		err = c.store.Services().PutHeartbeat(store.Heartbeat{Project: k.project, Factory: k.name, FactoryHost: k.host, Doc: doc,
			Service: h.service, ServiceHost: h.serviceHost, Expires: h.expires()})
	}
	if err != nil {
		c.logf("cannot record %s's Heartbeat from %s on %s: %v", k.project, k.name, k.host, err)
	}
}

// recordReports writes the Project key's base and clone reports, as they
// are in memory, through to the Store.
func (c *Core) recordReports(key string) {
	hs := c.heartbeats
	hs.mu.Lock()
	doc, err := json.Marshal(factoryReportsDoc{At: hs.reportsAt[key], Bases: hs.base[key], Clones: hs.clones[key]})
	hs.mu.Unlock()
	if err == nil {
		err = c.store.Services().PutFactoryReports(store.FactoryReports{Project: key, Doc: doc})
	}
	if err != nil {
		c.logf("cannot record %s's base and clone reports: %v", key, err)
	}
}

// LoadFactories reads every Heartbeat, Factory report and ping from the
// Store, as serve does once before it serves, so a new service shows the
// Factories reporting to the others at once, and a Heartbeat reaching it
// first doesn't take a Factory it already has for a new one.
func (c *Core) LoadFactories() {
	c.refreshPings()
	c.refreshFactories()
}

// refreshFactories prunes the Heartbeats Offline for a day, here and in
// the Store, and reads every Heartbeat and Factory report from the Store
// into memory where it is newer than what this service has. It returns the
// Projects whose Factories changed in a way the human sees, as
// ReportHeartbeat, ReportBase and ReportRepositories would have noticed.
// What it can't read is logged, and memory stands.
func (c *Core) refreshFactories() []string {
	now := c.now()
	services := c.store.Services()
	if err := services.PruneHeartbeats(now); err != nil {
		c.logf("cannot prune the Heartbeats: %v", err)
	}
	rows, err := services.Heartbeats()
	if err != nil {
		c.logf("cannot read the Heartbeats: %v", err)
	}
	reports, rerr := services.FactoryReports()
	if rerr != nil {
		c.logf("cannot read the base and clone reports: %v", rerr)
	}

	var changed []string
	note := func(project string) {
		if !slices.Contains(changed, project) {
			changed = append(changed, project)
		}
	}
	hs := c.heartbeats
	hs.mu.Lock()
	defer hs.mu.Unlock()
	for k, h := range hs.all {
		if !now.Before(h.expires()) {
			delete(hs.all, k)
		}
	}
	for _, row := range rows {
		h, err := heartbeatOf(row)
		if err != nil {
			c.logf("cannot read %s's Heartbeat from %s on %s: %v", row.Project, row.Factory, row.FactoryHost, err)
			continue
		}
		if !now.Before(h.expires()) {
			continue
		}
		k := heartbeatKey{row.Project, row.Factory, row.FactoryHost}
		prev, seen := hs.all[k]
		if seen && h.at.Before(prev.at) {
			continue
		}
		hs.all[k] = h
		if noticed(prev, seen, h, now) {
			note(k.project)
		}
	}
	for _, r := range reports {
		var doc factoryReportsDoc
		if err := json.Unmarshal(r.Doc, &doc); err != nil {
			c.logf("cannot read %s's base and clone reports: %v", r.Project, err)
			continue
		}
		if at, ok := hs.reportsAt[r.Project]; ok && doc.At.Before(at) {
			continue
		}
		if !sameBases(hs.base[r.Project], doc.Bases) || !sameClones(hs.clones[r.Project], doc.Clones) {
			note(r.Project)
		}
		hs.reportsAt[r.Project] = doc.At
		setOrDelete(hs.base, r.Project, doc.Bases)
		setOrDelete(hs.clones, r.Project, doc.Clones)
	}
	return changed
}

// heartbeatOf is the Heartbeat the Store's row keeps.
func heartbeatOf(row store.Heartbeat) (heartbeat, error) {
	var doc heartbeatDoc
	if err := json.Unmarshal(row.Doc, &doc); err != nil {
		return heartbeat{}, err
	}
	return heartbeat{name: row.Factory, host: row.FactoryHost, state: doc.State, note: doc.Note, interval: doc.IntervalSeconds,
		chat: doc.Chat, at: doc.At, limits: doc.PlanLimits, reported: doc.Reported, service: row.Service, serviceHost: row.ServiceHost}, nil
}

// sameBases says whether a and b show the human the same bases: what
// ReportBase notices.
func sameBases(a, b map[string]BaseView) bool {
	return maps.EqualFunc(a, b, func(x, y BaseView) bool {
		return x.State == y.State && x.Local == y.Local && x.Remote == y.Remote
	})
}

// sameClones says whether a and b are the same clone reports: what
// ReportRepositories notices.
func sameClones(a, b map[string]CloneView) bool {
	return maps.EqualFunc(a, b, func(x, y CloneView) bool {
		return x.State == y.State && x.Path == y.Path && x.Problem == y.Problem && x.Since.Equal(y.Since)
	})
}

// setOrDelete sets m[key] to v, or deletes it when v is empty.
func setOrDelete[V any](m map[string]map[string]V, key string, v map[string]V) {
	if len(v) == 0 {
		delete(m, key)
		return
	}
	m[key] = v
}
