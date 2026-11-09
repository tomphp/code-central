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

// tokens is one kind's posted tokens: input, cache write, cache read and
// output.
type tokens struct {
	kind                      string
	input, write, read, output int
}

func postTokens(t *testing.T, srv *codecentraltest.Server, session string, at time.Time, agent string, tk tokens) {
	t.Helper()
	agentField := ""
	if agent != "" {
		agentField = fmt.Sprintf(`"agent":%q,`, agent)
	}
	postUsage(t, srv, fmt.Sprintf(
		`{"session_id":%q,"responses":[{"time":%q,%s"kind":%q,"input_tokens":%d,"cache_write_tokens":%d,"cache_read_tokens":%d,"output_tokens":%d}]}`,
		session, at.Format(time.RFC3339Nano), agentField, tk.kind, tk.input, tk.write, tk.read, tk.output))
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
	postTokens(t, srv, "s1", before, "", tokens{"implementation", 1, 2, 3, 4})
	postTokens(t, srv, "s1", after, "", tokens{"implementation", 10, 20, 30, 40})
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
	postTokens(t, srv, "a", today, "", tokens{"implementation", 1, 100, 1_000, 10})
	postTokens(t, srv, "a", today, "", tokens{"implementation", 2, 200, 2_000, 20})
	postTokens(t, srv, "a", today, "toolu_r", tokens{"review", 3, 300, 0, 30})
	postTokens(t, srv, "a", today, "toolu_x", tokens{"custom", 4, 400, 0, 40})
	postTokens(t, srv, "o", today, "", tokens{"implementation", 5, 500, 5_000, 50})
	postTokens(t, srv, "o", yesterday, "", tokens{"spec", 6, 600, 6_000, 60})
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
