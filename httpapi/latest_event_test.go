package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
)

type latestEvent struct {
	Kind string    `json:"kind"`
	Text string    `json:"text"`
	Time time.Time `json:"time"`
}

// latest is the latest_event of Ticket n's Claim c, and of its find_tickets
// row, nil where left out.
func (f transcriptFixture) latest(t *testing.T, n, c int) (claim, row *latestEvent) {
	t.Helper()
	var v struct {
		Claims []struct {
			LatestEvent *latestEvent `json:"latest_event"`
		} `json:"claims"`
	}
	json.Unmarshal(f.ok(t, "GET", fmt.Sprintf("/api/p/acme/tickets/%d", n), ""), &v)
	var rows []struct {
		ID          string       `json:"id"`
		LatestEvent *latestEvent `json:"latest_event"`
	}
	json.Unmarshal(f.ok(t, "GET", "/api/p/acme/tickets?state=In+Progress&state=In+Review&state=Ready", ""), &rows)
	for _, r := range rows {
		if r.ID == fmt.Sprintf("acme-%d", n) {
			row = r.LatestEvent
		}
	}
	return v.Claims[c-1].LatestEvent, row
}

func TestTheLatestEventIsTheNewestSessionEventAsOneShortLine(t *testing.T) {
	f := newTranscriptFixture(t)
	f.claim(t, 2, "s1")
	at := time.Date(2026, 11, 7, 9, 0, 7, 0, time.UTC)
	long := strings.Repeat("é", 130)
	for _, c := range []struct{ kind, event, want string }{
		{"tool_call", `"text":"Bash: go test ./...","tool":"Bash","tool_use_id":"tu1"`, "Bash: go test ./..."},
		{"text", `"text":"\n  \nI'll run the tests.\nThen fix them."`, "I'll run the tests."},
		{"tool_result", `"text":"ok","tool":"Bash","tool_use_id":"tu1"`, "Bash result"},
		{"tool_result", `"text":"FAIL","tool":"Bash","tool_use_id":"tu1","is_error":true`, "Bash failed"},
		// A result with no tool is named by the call it answers.
		{"tool_call", `"text":"Read: core/core.go","tool":"Read","tool_use_id":"tu2"`, "Read: core/core.go"},
		{"tool_result", `"text":"package core","tool_use_id":"tu2"`, "Read result"},
		{"factory", `"text":"code-central-2 claimed (implementation): \"A\""`, `code-central-2 claimed (implementation): "A"`},
		{"text", fmt.Sprintf(`"text":%q`, long), strings.Repeat("é", 119) + "…"},
	} {
		f.post(t, fmt.Sprintf(`{"session_id":"s1","events":[
			{"time":"2026-11-07T09:00:00Z","kind":"text","text":"older"},
			{"time":"2026-11-07T09:00:07Z","kind":%q,%s}
		]}`, c.kind, c.event))
		claim, row := f.latest(t, 2, 1)
		want := latestEvent{Kind: c.kind, Text: c.want, Time: at}
		if claim == nil || *claim != want {
			t.Errorf("after %s %s: Claim's latest_event %+v, want %+v", c.kind, c.event, claim, want)
		}
		if row == nil || *row != want {
			t.Errorf("after %s %s: row's latest_event %+v, want %+v", c.kind, c.event, row, want)
		}
	}
}

func TestATicketRowsLatestEventIsItsLiveClaimsOnly(t *testing.T) {
	f := newTranscriptFixture(t)
	id := f.claim(t, 2, "s1")
	if claim, row := f.latest(t, 2, 1); claim != nil || row != nil {
		t.Fatalf("a Claim with no Session Events: %+v, %+v; want none", claim, row)
	}
	f.events(t, "s1", "working")
	f.ok(t, "POST", "/api/p/acme/tickets/2/finish", fmt.Sprintf(`{"claim_id":%q,"summary":"done"}`, id))
	f.events(t, "s1", "turn ended")

	claim, row := f.latest(t, 2, 1)
	if claim == nil || claim.Text != "turn ended" {
		t.Fatalf("the ended Claim's latest_event %+v, want its last event", claim)
	}
	if row != nil {
		t.Fatalf("a row with no live Claim has latest_event %+v", row)
	}
}

func TestTicketChangedOnANewLatestEventIsThrottledPerTicketWithATrailingNotice(t *testing.T) {
	f := newTranscriptFixture(t)
	f.claim(t, 2, "s1")
	f.claim(t, 3, "s2")
	hs := httptest.NewServer(f.h)
	defer hs.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := sseStream(t, ctx, hs.URL)
	next := func(want string) {
		t.Helper()
		if name, data := nextEvent(t, r); name != "ticket.changed" || data != fmt.Sprintf(`{"project":"acme","id":"%s"}`, want) {
			t.Fatalf("got %s %s, want ticket.changed for %s", name, data, want)
		}
	}
	// A write to acme-3 is a marker: any notice for acme-2 sent before it
	// would be read first.
	marker := func() {
		t.Helper()
		f.ok(t, "POST", "/api/p/acme/tickets/3/comment", `{"body":"marker"}`)
		next("acme-3")
	}

	f.events(t, "s1", "first")
	next("acme-2") // the first change is sent at once
	f.events(t, "s1", "second")
	f.events(t, "s1", "third")
	f.post(t, `{"session_id":"s1","events":[]}`) // a ping changes no latest event
	f.clock.BlockUntil(1)
	marker() // the second and third wait out the throttle

	f.events(t, "s2", "elsewhere")
	next("acme-3") // the throttle is per Ticket

	f.clock.Advance(core.LatestEventThrottle - time.Second)
	marker()
	f.clock.Advance(time.Second)
	next("acme-2") // one trailing notice for the last change
	marker()       // and only one

	f.clock.Advance(core.LatestEventThrottle)
	f.events(t, "s1", "after a quiet spell")
	next("acme-2")
}
