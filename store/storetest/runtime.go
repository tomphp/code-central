// The runtime state on a Store's Services side (Spec code-central-1107):
// each Factory's latest Heartbeat, each Project's Factory reports, each
// session's last counted ping and the service holding each Claim's
// Transcript, so every service sharing the Store sees them. Every case
// runs on its own times, as a clock the test moves would give them.

package storetest

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// heartbeat is the Factory name on host's Heartbeat in project, as doc
// reports it to service a on laptop, expiring at expires.
func heartbeat(project, name, host, doc string, expires time.Time) store.Heartbeat {
	return store.Heartbeat{Project: project, Factory: name, FactoryHost: host, Doc: json.RawMessage(doc),
		Service: "a", ServiceHost: "laptop", Expires: expires}
}

func putHeartbeat(t *testing.T, s store.Store, h store.Heartbeat) {
	t.Helper()
	if err := s.Services().PutHeartbeat(h); err != nil {
		t.Fatalf("put %s's heartbeat: %v", h.Factory, err)
	}
}

// heartbeats is every Heartbeat s holds, one per line, in order.
func heartbeats(t *testing.T, s store.Store) string {
	t.Helper()
	hs, err := s.Services().Heartbeats()
	if err != nil {
		t.Fatalf("heartbeats: %v", err)
	}
	var out strings.Builder
	for _, h := range hs {
		fmt.Fprintf(&out, "%s/%s@%s %s via %s@%s until %s\n", h.Project, h.Factory, h.FactoryHost, h.Doc,
			h.Service, h.ServiceHost, stamp(h.Expires))
	}
	return out.String()
}

func putPing(t *testing.T, s store.Store, p store.Ping) {
	t.Helper()
	if err := s.Services().PutPing(p); err != nil {
		t.Fatalf("put %s's ping: %v", p.Session, err)
	}
}

// pings is every ping s holds, one per line, in order.
func pings(t *testing.T, s store.Store) string {
	t.Helper()
	ps, err := s.Services().Pings()
	if err != nil {
		t.Fatalf("pings: %v", err)
	}
	var out strings.Builder
	for _, p := range ps {
		fmt.Fprintf(&out, "%s at %s via %s@%s\n", p.Session, stamp(p.At), p.Service, p.ServiceHost)
	}
	return out.String()
}

func putHolder(t *testing.T, s store.Store, h store.TranscriptHolder) {
	t.Helper()
	if err := s.Services().PutTranscriptHolder(h); err != nil {
		t.Fatalf("put %s-%d claim %d's transcript holder: %v", h.Project, h.Ticket, h.Claim, err)
	}
}

// holder is the service holding project-ticket's Claim claim's
// Transcript, as service@host since when, or "none".
func holder(t *testing.T, s store.Store, project string, ticket, claim int) string {
	t.Helper()
	h, ok, err := s.Services().TranscriptHolder(project, ticket, claim)
	if err != nil {
		t.Fatalf("%s-%d claim %d's transcript holder: %v", project, ticket, claim, err)
	}
	if !ok {
		return "none"
	}
	if h.Project != project || h.Ticket != ticket || h.Claim != claim {
		t.Fatalf("asked for %s-%d claim %d's transcript holder, got %+v", project, ticket, claim, h)
	}
	return fmt.Sprintf("%s@%s since %s", h.Service, h.ServiceHost, stamp(h.At))
}

// factoryReports is every Project's Factory reports s holds, one per
// line, in order.
func factoryReports(t *testing.T, s store.Store) string {
	t.Helper()
	rs, err := s.Services().FactoryReports()
	if err != nil {
		t.Fatalf("factory reports: %v", err)
	}
	var out strings.Builder
	for _, r := range rs {
		fmt.Fprintf(&out, "%s %s\n", r.Project, r.Doc)
	}
	return out.String()
}

func putFactoryReports(t *testing.T, s store.Store, project, doc string) {
	t.Helper()
	if err := s.Services().PutFactoryReports(store.FactoryReports{Project: project, Doc: json.RawMessage(doc)}); err != nil {
		t.Fatalf("put %s's factory reports: %v", project, err)
	}
}

// stamp is at as the listings write it.
func stamp(at time.Time) string { return at.UTC().Format(time.RFC3339Nano) }

func runRuntime(t *testing.T, f Factory) {
	day := 24 * time.Hour

	t.Run("a heartbeat reads back whole, and one for the same factory replaces it", func(t *testing.T) {
		s := f.New(t)
		if got := heartbeats(t, s); got != "" {
			t.Fatalf("heartbeats in a fresh store: %q", got)
		}
		putHeartbeat(t, s, heartbeat("acme", "factory", "laptop", `{"state":"running"}`, t0.Add(day)))
		want := "acme/factory@laptop {\"state\":\"running\"} via a@laptop until " + stamp(t0.Add(day)) + "\n"
		if got := heartbeats(t, s); got != want {
			t.Fatalf("heartbeats:\n%s\nwant:\n%s", got, want)
		}
		// The Factory switches to service b: the newest Heartbeat wins.
		h := heartbeat("acme", "factory", "laptop", `{"state":"draining"}`, t0.Add(2*day))
		h.Service, h.ServiceHost = "b", "desktop"
		putHeartbeat(t, s, h)
		want = "acme/factory@laptop {\"state\":\"draining\"} via b@desktop until " + stamp(t0.Add(2*day)) + "\n"
		if got := heartbeats(t, s); got != want {
			t.Fatalf("heartbeats after the factory switched service:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("heartbeats are one per project, factory and host, listed in that order", func(t *testing.T) {
		s := f.New(t)
		for _, h := range []store.Heartbeat{
			heartbeat("other", "factory", "laptop", `{}`, t0),
			heartbeat("acme", "factory", "laptop", `{}`, t0),
			heartbeat("acme", "factory", "desktop", `{}`, t0),
			heartbeat("acme", "batch", "laptop", `{}`, t0),
		} {
			putHeartbeat(t, s, h)
		}
		until := stamp(t0)
		want := "acme/batch@laptop {} via a@laptop until " + until + "\n" +
			"acme/factory@desktop {} via a@laptop until " + until + "\n" +
			"acme/factory@laptop {} via a@laptop until " + until + "\n" +
			"other/factory@laptop {} via a@laptop until " + until + "\n"
		if got := heartbeats(t, s); got != want {
			t.Fatalf("heartbeats:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("pruning heartbeats deletes those whose expiry has passed", func(t *testing.T) {
		s := f.New(t)
		putHeartbeat(t, s, heartbeat("acme", "early", "laptop", `{}`, t0))
		putHeartbeat(t, s, heartbeat("acme", "late", "laptop", `{}`, t0.Add(time.Nanosecond)))
		if err := s.Services().PruneHeartbeats(t0); err != nil {
			t.Fatalf("prune: %v", err)
		}
		want := "acme/late@laptop {} via a@laptop until " + stamp(t0.Add(time.Nanosecond)) + "\n"
		if got := heartbeats(t, s); got != want {
			t.Fatalf("heartbeats after pruning at the early one's expiry:\n%s\nwant:\n%s", got, want)
		}
		// Pruning is idempotent: again at the same moment deletes nothing.
		if err := s.Services().PruneHeartbeats(t0); err != nil {
			t.Fatalf("prune again: %v", err)
		}
		if got := heartbeats(t, s); got != want {
			t.Fatalf("heartbeats after pruning again:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("a project's factory reports read back, and the next replaces them", func(t *testing.T) {
		s := f.New(t)
		if got := factoryReports(t, s); got != "" {
			t.Fatalf("factory reports in a fresh store: %q", got)
		}
		putFactoryReports(t, s, "other", `{"base":{"other":{"state":"ok"}}}`)
		putFactoryReports(t, s, "acme", `{"base":{"acme":{"state":"ok"}}}`)
		putFactoryReports(t, s, "acme", `{}`)
		want := "acme {}\n" + "other {\"base\":{\"other\":{\"state\":\"ok\"}}}\n"
		if got := factoryReports(t, s); got != want {
			t.Fatalf("factory reports:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("a session's ping reads back, the next replaces it, and pruning deletes the older ones", func(t *testing.T) {
		s := f.New(t)
		if got := pings(t, s); got != "" {
			t.Fatalf("pings in a fresh store: %q", got)
		}
		putPing(t, s, store.Ping{Session: "s2", At: t0, Service: "a", ServiceHost: "laptop"})
		putPing(t, s, store.Ping{Session: "s1", At: t0, Service: "a", ServiceHost: "laptop"})
		putPing(t, s, store.Ping{Session: "s1", At: t0.Add(time.Minute), Service: "b", ServiceHost: "desktop"})
		want := "s1 at " + stamp(t0.Add(time.Minute)) + " via b@desktop\n" + "s2 at " + stamp(t0) + " via a@laptop\n"
		if got := pings(t, s); got != want {
			t.Fatalf("pings:\n%s\nwant:\n%s", got, want)
		}
		if err := s.Services().PrunePings(t0.Add(time.Minute)); err != nil {
			t.Fatalf("prune: %v", err)
		}
		want = "s1 at " + stamp(t0.Add(time.Minute)) + " via b@desktop\n"
		if got := pings(t, s); got != want {
			t.Fatalf("pings after pruning those older than s1's:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("a claim's transcript holder reads back, the newest wins, and pruning deletes the older ones", func(t *testing.T) {
		s := f.New(t)
		if got := holder(t, s, "acme", 2, 1); got != "none" {
			t.Fatalf("holder in a fresh store: %s", got)
		}
		putHolder(t, s, store.TranscriptHolder{Project: "acme", Ticket: 2, Claim: 1, Service: "a", ServiceHost: "laptop", At: t0})
		putHolder(t, s, store.TranscriptHolder{Project: "acme", Ticket: 2, Claim: 2, Service: "a", ServiceHost: "laptop", At: t0.Add(day)})
		putHolder(t, s, store.TranscriptHolder{Project: "other", Ticket: 2, Claim: 1, Service: "c", ServiceHost: "server", At: t0})
		if got, want := holder(t, s, "acme", 2, 1), "a@laptop since "+stamp(t0); got != want {
			t.Fatalf("holder = %s, want %s", got, want)
		}
		// The session moved to service b.
		putHolder(t, s, store.TranscriptHolder{Project: "acme", Ticket: 2, Claim: 1, Service: "b", ServiceHost: "desktop", At: t0.Add(time.Hour)})
		if got, want := holder(t, s, "acme", 2, 1), "b@desktop since "+stamp(t0.Add(time.Hour)); got != want {
			t.Fatalf("holder after the session moved = %s, want %s", got, want)
		}
		if got, want := holder(t, s, "other", 2, 1), "c@server since "+stamp(t0); got != want {
			t.Fatalf("other's holder = %s, want %s", got, want)
		}
		if got := holder(t, s, "acme", 3, 1); got != "none" {
			t.Fatalf("holder of a claim with none: %s", got)
		}
		if err := s.Services().PruneTranscriptHolders(t0.Add(time.Hour)); err != nil {
			t.Fatalf("prune: %v", err)
		}
		if got := holder(t, s, "other", 2, 1); got != "none" {
			t.Fatalf("other's holder after pruning those older than an hour in: %s", got)
		}
		if got := holder(t, s, "acme", 2, 1); got == "none" {
			t.Fatal("acme's holder written at the horizon was pruned")
		}
		if got := holder(t, s, "acme", 2, 2); got == "none" {
			t.Fatal("acme's later holder was pruned")
		}
	})

	if f.Second == nil && f.Reopen != nil {
		// As with Registrations: one service holds the Store alone, so
		// what it kept is its own, and goes with it.
		t.Run("reopened, a store one service holds alone forgets the runtime state before", func(t *testing.T) {
			s := f.New(t)
			putHeartbeat(t, s, heartbeat("acme", "factory", "laptop", `{}`, t0.Add(day)))
			putFactoryReports(t, s, "acme", `{}`)
			putPing(t, s, store.Ping{Session: "s1", At: t0, Service: "a", ServiceHost: "laptop"})
			putHolder(t, s, store.TranscriptHolder{Project: "acme", Ticket: 2, Claim: 1, Service: "a", ServiceHost: "laptop", At: t0})
			s = f.Reopen(t, s)
			if got := heartbeats(t, s) + factoryReports(t, s) + pings(t, s); got != "" {
				t.Fatalf("after reopening:\n%s", got)
			}
			if got := holder(t, s, "acme", 2, 1); got != "none" {
				t.Fatalf("holder after reopening: %s", got)
			}
		})
	}

	if f.Second != nil {
		t.Run("a second service sees the first's heartbeat, reports, ping and transcript holder", func(t *testing.T) {
			a := f.New(t)
			b := f.Second(t, a)
			putHeartbeat(t, a, heartbeat("acme", "factory", "laptop", `{"state":"running"}`, t0.Add(day)))
			putFactoryReports(t, a, "acme", `{"clones":{}}`)
			putPing(t, a, store.Ping{Session: "s1", At: t0, Service: "a", ServiceHost: "laptop"})
			putHolder(t, a, store.TranscriptHolder{Project: "acme", Ticket: 2, Claim: 1, Service: "a", ServiceHost: "laptop", At: t0})
			if got, want := heartbeats(t, b), heartbeats(t, a); got != want || got == "" {
				t.Fatalf("heartbeats, as the second service reads them:\n%s\nwant:\n%s", got, want)
			}
			if got, want := factoryReports(t, b), "acme {\"clones\":{}}\n"; got != want {
				t.Fatalf("factory reports, as the second service reads them:\n%s\nwant:\n%s", got, want)
			}
			if got, want := pings(t, b), "s1 at "+stamp(t0)+" via a@laptop\n"; got != want {
				t.Fatalf("pings, as the second service reads them:\n%s\nwant:\n%s", got, want)
			}
			if got, want := holder(t, b, "acme", 2, 1), "a@laptop since "+stamp(t0); got != want {
				t.Fatalf("holder, as the second service reads it = %s, want %s", got, want)
			}
		})
	}
}
