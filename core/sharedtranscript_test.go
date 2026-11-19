package core_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Transcripts on a Shared Store (Spec code-central-1107): a Claim's
// Transcript stays on the service its Session Events reach, and another
// service, holding none, names that service's host while it is running.

// claimedOnTwo is the laptop and the desktop on Store s, both
// registered, with acme-2 claimed by session s1, and acme-3 by s2.
func claimedOnTwo(t *testing.T, s store.Store) (laptop, desktop fixture) {
	t.Helper()
	laptop, desktop = twoServices(t, s)
	for _, f := range []fixture{laptop, desktop} {
		if err := f.core.Register(ctx); err != nil {
			t.Fatal(err)
		}
	}
	laptop.spec(t, "acme", "Login")
	laptop.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Ta", State: domain.Ready})
	laptop.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Tb", State: domain.Ready})
	laptop.claim(t, core.ClaimInput{Ticket: "2", SessionID: "s1"})
	laptop.claim(t, core.ClaimInput{Ticket: "3", SessionID: "s2"})
	return laptop, desktop
}

// post posts Session Events with texts for session to f.
func post(f fixture, session string, texts ...string) {
	var events []core.SessionEvent
	for _, text := range texts {
		events = append(events, core.SessionEvent{Time: f.clock.Now(), Kind: core.EventText, Text: text})
	}
	f.core.PostSessionEvents(ctx, session, events)
}

// transcriptOf is f's Transcript of acme-<ticket>'s first Claim.
func transcriptOf(t *testing.T, f fixture, ticket string) core.TranscriptView {
	t.Helper()
	v, err := f.core.Transcript(ctx, "acme", core.Ref(ticket), 1)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// texts is the texts of v's events.
func texts(v core.TranscriptView) []string {
	out := []string{}
	for _, e := range v.Events {
		out = append(out, e.Text)
	}
	return out
}

func TestATranscriptOnAnotherServiceNamesItsHost(t *testing.T) {
	laptop, desktop := claimedOnTwo(t, store.NewMemory())
	post(laptop, "s1", "one", "two")

	if v := transcriptOf(t, laptop, "2"); v.Host != "" || len(v.Events) != 2 || v.Tail {
		t.Fatalf("on the laptop, %+v, want its two events and no host", v)
	}
	if v := transcriptOf(t, desktop, "2"); v.Host != "laptop" || len(v.Events) != 0 || v.Tail {
		t.Fatalf("on the desktop, %+v, want none, on the laptop", v)
	}
}

// A hand-run session posts no Session Events, so no service holds its
// Transcript and none names a host.
func TestAHandRunSessionsTranscriptNamesNoHost(t *testing.T) {
	laptop, desktop := claimedOnTwo(t, store.NewMemory())
	laptop.core.Ping(ctx, "s2")
	for name, f := range map[string]fixture{"laptop": laptop, "desktop": desktop} {
		if v := transcriptOf(t, f, "3"); v.Host != "" || len(v.Events) != 0 {
			t.Errorf("on the %s, %+v, want an empty Transcript", name, v)
		}
	}
}

// When a Factory switches from the laptop to the desktop, the desktop
// holds the rest of the session as a tail, and the laptop, holding the
// start, names the desktop: it holds the newest events.
func TestATranscriptFollowsItsFactoryToAnotherService(t *testing.T) {
	laptop, desktop := claimedOnTwo(t, store.NewMemory())
	post(laptop, "s1", "one")
	post(desktop, "s1", "two", "three")

	if v := transcriptOf(t, desktop, "2"); v.Host != "" || !v.Tail || !slices.Equal(texts(v), []string{"two", "three"}) {
		t.Fatalf("on the desktop, %+v, want its two events as a tail", v)
	}
	if v := transcriptOf(t, laptop, "2"); v.Host != "desktop" || len(v.Events) != 0 {
		t.Fatalf("on the laptop, %+v, want none, on the desktop", v)
	}
	feed, err := desktop.core.FollowTranscript(ctx, "acme", "2", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer feed.Stop()
	if tail, events := feed.Next(); !tail || len(events) != 2 || feed.Host() != "" {
		t.Fatalf("following on the desktop, tail %v, %d events, host %q, want a tail of 2", tail, len(events), feed.Host())
	}
}

// Following a Transcript another service holds ends at once, naming it.
func TestFollowingATranscriptOnAnotherServiceNamesItsHost(t *testing.T) {
	laptop, desktop := claimedOnTwo(t, store.NewMemory())
	post(laptop, "s1", "one")
	feed, err := desktop.core.FollowTranscript(ctx, "acme", "2", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer feed.Stop()
	if feed.Host() != "laptop" {
		t.Fatalf("host %q, want laptop", feed.Host())
	}
	post(laptop, "s1", "two")
	select {
	case <-feed.Wake():
		t.Fatal("the feed woke, want it ended")
	default:
	}
}

// Once the service holding a Transcript stops, nobody holds it: another
// service shows it empty rather than send the human to a dead host.
func TestATranscriptOnAStoppedServiceNamesNoHost(t *testing.T) {
	laptop, desktop := claimedOnTwo(t, store.NewMemory())
	post(laptop, "s1", "one")
	if err := laptop.core.Deregister(ctx); err != nil {
		t.Fatal(err)
	}
	if v := transcriptOf(t, desktop, "2"); v.Host != "" || len(v.Events) != 0 {
		t.Fatalf("on the desktop, %+v, want an empty Transcript", v)
	}
}

// A Store that can't be reached costs a Transcript its host, never its
// events: the post and the read still work, as this service's own.
func TestATranscriptHolderTheStoreCannotWriteIsLoggedOnly(t *testing.T) {
	s := &failingServices{Store: store.NewMemory()}
	laptop, desktop := claimedOnTwo(t, s)

	s.err = errors.New("connection refused")
	post(laptop, "s1", "one")
	if v := transcriptOf(t, laptop, "2"); v.Host != "" || v.Tail || !slices.Equal(texts(v), []string{"one"}) {
		t.Fatalf("on the laptop, %+v, want its event", v)
	}
	s.err = nil
	if v := transcriptOf(t, desktop, "2"); v.Host != "" {
		t.Fatalf("on the desktop, %+v, want no host, as none was written", v)
	}
}

// A Transcript holder is pruned once it is over a week old, on the tick,
// as the pings are.
func TestATranscriptHolderIsPrunedAfterAWeek(t *testing.T) {
	laptop, desktop := claimedOnTwo(t, store.NewMemory())
	tick := watchFactories(t, desktop)
	post(laptop, "s1", "one")

	laptop.clock.Advance(7 * 24 * time.Hour)
	if err := laptop.core.Register(ctx); err != nil {
		t.Fatal(err)
	}
	tick()
	if v := transcriptOf(t, desktop, "2"); v.Host != "laptop" {
		t.Fatalf("a week on, %+v, want the laptop named", v)
	}
	laptop.clock.Advance(time.Second)
	if err := laptop.core.Register(ctx); err != nil {
		t.Fatal(err)
	}
	tick()
	if v := transcriptOf(t, desktop, "2"); v.Host != "" {
		t.Fatalf("after a week, %+v, want no host", v)
	}
}
