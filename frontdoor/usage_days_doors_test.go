package frontdoor_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// Token usage per day (Spec code-central-201): GET /api/usage has a row
// for every local day in the range, oldest first, each with its tokens by
// kind of session and by token type, so the UI filters by kind itself.

// aest is UTC+10, where the local day and the UTC day part at 10:00 local.
var aest = time.FixedZone("AEST", 10*60*60)

// tokens is one kind's tokens on a day: input, cache write, cache read
// and output.
type tokens struct {
	kind                       string
	input, write, read, output int
}

// dayTokens is u's days as each day's kinds, by day.
func dayTokens(u core.UsageView) map[string][]tokens {
	out := map[string][]tokens{}
	for _, d := range u.Days {
		var ks []tokens
		for _, k := range d.Kinds {
			ks = append(ks, tokens{k.Kind, k.InputTokens, k.CacheWriteTokens, k.CacheReadTokens, k.OutputTokens})
		}
		out[d.Day] = ks
	}
	return out
}

func TestTokenUsageHasARowForEveryLocalDayInTheRangeZerosToo(t *testing.T) {
	// 09:00 on the 5th in UTC+10 is still the 4th in UTC.
	now := time.Date(2026, 1, 5, 9, 0, 0, 0, aest)
	srv := acmeServer(t, codecentraltest.WithLocation(aest), codecentraltest.WithStart(now))
	for _, days := range []int{7, 30, 90} {
		u := readUsage(t, srv, fmt.Sprintf("?days=%d", days))
		if len(u.Days) != days {
			t.Fatalf("%d days: %d rows, want %d", days, len(u.Days), days)
		}
		for i, d := range u.Days {
			want := now.AddDate(0, 0, i-days+1).Format(time.DateOnly)
			if d.Day != want || len(d.Kinds) != 0 {
				t.Fatalf("%d days: row %d is %+v, want %s with no kinds", days, i, d, want)
			}
		}
	}
}

func TestTokenUsageBucketsByTheServicesLocalDay(t *testing.T) {
	srv := newServerWith(t, []codecentraltest.Option{codecentraltest.WithLocation(aest)})
	claim(t, srv, "acme", "s1")
	// Both on the 5th in UTC, but either side of midnight in UTC+10.
	before := time.Date(2026, 1, 5, 23, 59, 0, 0, aest)
	after := time.Date(2026, 1, 6, 0, 1, 0, 0, aest)
	postResponses(t, srv, "s1", response{time: before, kind: "implementation", input: 1, write: 2, read: 3, output: 4})
	postResponses(t, srv, "s1", response{time: after, kind: "implementation", input: 10, write: 20, read: 30, output: 40})
	srv.Clock.Set(after)

	got := dayTokens(readUsage(t, srv, ""))
	want := map[string][]tokens{
		"2026-01-05": {{"implementation", 1, 2, 3, 4}},
		"2026-01-06": {{"implementation", 10, 20, 30, 40}},
	}
	for day, ks := range want {
		if !reflect.DeepEqual(got[day], ks) {
			t.Errorf("%s: %+v, want %+v", day, got[day], ks)
		}
	}
	if u := readUsage(t, srv, ""); u.Days[len(u.Days)-1].Day != "2026-01-06" {
		t.Errorf("the last row is %s, want today in UTC+10, 2026-01-06", u.Days[len(u.Days)-1].Day)
	}
}

func TestTokenUsageSplitsEachDayByKindAndTokenType(t *testing.T) {
	srv := newServer(t)
	ctx := context.Background()
	must(t, func() error {
		_, err := srv.Core.CreateSpec(ctx, "other", core.CreateSpecInput{Title: "S"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.CreateTicket(ctx, "other", core.CreateTicketInput{Spec: "1", Title: "T", State: "Ready"})
		return err
	})
	claim(t, srv, "acme", "a")
	claim(t, srv, "other", "o")
	today := at(60)
	yesterday := at(60 - 24*60)
	postResponses(t, srv, "a", response{time: today, kind: "implementation", input: 1, write: 100, read: 1_000, output: 10})
	postResponses(t, srv, "a", response{time: today, kind: "implementation", input: 2, write: 200, read: 2_000, output: 20})
	postResponses(t, srv, "a", response{time: today, agent: "toolu_r", kind: "review", input: 3, write: 300, read: 0, output: 30})
	postResponses(t, srv, "a", response{time: today, agent: "toolu_x", kind: "custom", input: 4, write: 400, read: 0, output: 40})
	postResponses(t, srv, "o", response{time: today, kind: "implementation", input: 5, write: 500, read: 5_000, output: 50})
	postResponses(t, srv, "o", response{time: yesterday, kind: "spec", input: 6, write: 600, read: 6_000, output: 60})
	srv.Clock.Set(today)

	all := dayTokens(readUsage(t, srv, ""))
	want := map[string][]tokens{
		"2026-01-04": {{"spec", 6, 600, 6_000, 60}},
		"2026-01-05": {
			{"implementation", 8, 800, 8_000, 80},
			{"review", 3, 300, 0, 30},
			{"custom", 4, 400, 0, 40},
		},
		"2026-01-03": nil,
	}
	for day, ks := range want {
		if !reflect.DeepEqual(all[day], ks) {
			t.Errorf("all Projects on %s: %+v, want %+v", day, all[day], ks)
		}
	}

	acme := dayTokens(readUsage(t, srv, "?project=acme&days=30"))
	if len(acme) != 30 {
		t.Errorf("acme over 30 days: %d rows, want 30", len(acme))
	}
	wantAcme := []tokens{{"implementation", 3, 300, 3_000, 30}, {"review", 3, 300, 0, 30}, {"custom", 4, 400, 0, 40}}
	if got := acme["2026-01-05"]; !reflect.DeepEqual(got, wantAcme) {
		t.Errorf("acme today: %+v, want %+v", got, wantAcme)
	}
	if got := acme["2026-01-04"]; got != nil {
		t.Errorf("acme yesterday: %+v, want nothing", got)
	}
}
