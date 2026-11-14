package usage_test

import (
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/usage"
)

// recorders are the Recorder's backings, each made with the clock and
// zone it dates and prunes by: every case runs on both.
var recorders = map[string]func(t *testing.T, clk clock.Clock, loc *time.Location) *usage.Recorder{
	"file": func(t *testing.T, clk clock.Clock, loc *time.Location) *usage.Recorder {
		return usage.New(t.TempDir(), clk, loc, nil)
	},
	"memory": func(t *testing.T, clk clock.Clock, loc *time.Location) *usage.Recorder {
		return usage.NewMemory(clk, loc, nil)
	},
}

func eachRecorder(t *testing.T, test func(t *testing.T, newRecorder func(clk clock.Clock, loc *time.Location) *usage.Recorder)) {
	for name, build := range recorders {
		t.Run(name, func(t *testing.T) {
			test(t, func(clk clock.Clock, loc *time.Location) *usage.Recorder { return build(t, clk, loc) })
		})
	}
}

var start = time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)

func ops(recs usage.Records) []string {
	var out []string
	for _, c := range recs.Calls {
		out = append(out, c.Op)
	}
	return out
}

func TestRecorderReadsBackWhatItRecordedForOneProjectOrAll(t *testing.T) {
	eachRecorder(t, func(t *testing.T, newRecorder func(clock.Clock, *time.Location) *usage.Recorder) {
		clk := clock.NewFake(start)
		r := newRecorder(clk, time.UTC)
		r.RecordCall("acme", usage.Call{Time: start, Op: "find_tickets", RequestBytes: 10, ResponseBytes: 20})
		r.RecordCall("beta", usage.Call{Time: start, Op: "get_ticket", Error: "not_found"})
		r.RecordResponses("acme", []usage.Response{{
			Time: start, Ticket: "acme-1", Claim: 2, SessionID: "s1", Kind: "turn", Model: "claude-opus-5-5",
			Tokens: usage.Tokens{InputTokens: 3, OutputTokens: 4},
		}})

		acme := r.Read("acme", 1)
		if want := []string{"find_tickets"}; !slices.Equal(ops(acme), want) {
			t.Errorf("acme's calls %v, want %v", ops(acme), want)
		}
		if got := acme.Calls[0]; got.RequestBytes != 10 || got.ResponseBytes != 20 || !got.Time.Equal(start) {
			t.Errorf("call %+v, want the recorded sizes and time", got)
		}
		if len(acme.Responses) != 1 {
			t.Fatalf("acme's responses %+v, want 1", acme.Responses)
		}
		resp := acme.Responses[0]
		if resp.Project != "acme" || resp.Day != "2026-01-05" || resp.Model != "claude-opus-5-5" || resp.Tokens.OutputTokens != 4 || resp.Claim != 2 {
			t.Errorf("response %+v, want acme's on 2026-01-05 with its model and tokens", resp)
		}
		if want := []string{"2026-01-05"}; !slices.Equal(acme.Days, want) {
			t.Errorf("days %v, want %v", acme.Days, want)
		}

		all := r.Read("", 1)
		if want := []string{"find_tickets", "get_ticket"}; !slices.Equal(ops(all), want) {
			t.Errorf("every Project's calls %v, want %v", ops(all), want)
		}
		if all.Calls[1].Error != "not_found" {
			t.Errorf("beta's call %+v, want its error", all.Calls[1])
		}
		if got := r.Read("gamma", 1); len(got.Calls)+len(got.Responses) != 0 {
			t.Errorf("a Project with no records read %+v", got)
		}
	})
}

// A record is dated by its local day in the recorder's zone, and a read
// counts back from the clock's local today: here UTC+10.
func TestRecorderDatesAndReadsByLocalDay(t *testing.T) {
	eachRecorder(t, func(t *testing.T, newRecorder func(clock.Clock, *time.Location) *usage.Recorder) {
		loc := time.FixedZone("AEST", 10*60*60)
		at := time.Date(2026, 1, 5, 23, 30, 0, 0, loc)
		clk := clock.NewFake(at)
		r := newRecorder(clk, loc)
		r.RecordCall("acme", usage.Call{Time: at.UTC(), Op: "find_tickets"})

		clk.Set(time.Date(2026, 1, 11, 23, 59, 0, 0, loc))
		got := r.Read("acme", 7)
		if len(got.Calls) != 1 || got.Days[0] != "2026-01-05" || got.Days[6] != "2026-01-11" {
			t.Errorf("on the seventh local day: %d calls over %v, want 1 from 2026-01-05", len(got.Calls), got.Days)
		}
		// Half an hour into the eighth local day: still the 11th in UTC.
		clk.Set(time.Date(2026, 1, 12, 0, 30, 0, 0, loc))
		if got := r.Read("acme", 7); len(got.Calls) != 0 {
			t.Errorf("on the eighth local day: %d calls, want 0", len(got.Calls))
		}
		if got := r.Read("acme", 8); len(got.Calls) != 1 {
			t.Errorf("8 days on the eighth local day: %d calls, want 1", len(got.Calls))
		}
	})
}

func TestRecorderPrunesOldDaysOnTheFirstRecordOfANewDay(t *testing.T) {
	eachRecorder(t, func(t *testing.T, newRecorder func(clock.Clock, *time.Location) *usage.Recorder) {
		clk := clock.NewFake(start)
		r := newRecorder(clk, time.UTC)
		old, kept := start, start.AddDate(0, 0, 1)
		for _, at := range []time.Time{old, kept} {
			clk.Set(at)
			r.RecordCall("acme", usage.Call{Time: at, Op: at.Format(time.DateOnly)})
			r.RecordCall("beta", usage.Call{Time: at, Op: at.Format(time.DateOnly)})
		}

		// There is no timer: moving the clock alone deletes nothing.
		clk.Set(old.AddDate(0, 0, usage.KeepDays+1))
		const all = usage.KeepDays + 2
		if got, want := ops(r.Read("", all)), []string{"2026-01-05", "2026-01-05", "2026-01-06", "2026-01-06"}; !slices.Equal(got, want) {
			t.Fatalf("before any record of the new day: %v, want %v", got, want)
		}
		r.RecordCall("acme", usage.Call{Time: clk.Now(), Op: "today"})
		if got, want := ops(r.Read("", all)), []string{"2026-01-06", "2026-01-06", "today"}; !slices.Equal(got, want) {
			t.Errorf("after the first record of the new day: %v, want %v", got, want)
		}
	})
}

func TestANilRecorderRecordsAndReadsNothing(t *testing.T) {
	var r *usage.Recorder
	r.RecordCall("acme", usage.Call{Time: start, Op: "find_tickets"})
	if got := r.Read("acme", 1); len(got.Calls) != 0 || len(got.Days) != 0 {
		t.Errorf("a nil recorder read %+v", got)
	}
}
