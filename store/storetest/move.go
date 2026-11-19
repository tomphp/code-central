// The contract for Store Move's copy (Spec code-central-1032): every
// record, with its History, the Options, CI Runs and Usage go from one
// Store into an empty one, and the two then hold the same.

package storetest

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/usage"
)

// Fill writes one of every kind of record Store Move copies into s, an
// empty Store: two Projects, one with Specs, Tickets with every History
// part, CI Runs and Usage over two days, and Options other than the
// defaults.
func Fill(t testing.TB, s store.Store) {
	t.Helper()
	write := func(fn func(store.Writer) error) {
		t.Helper()
		if err := s.Update(fn); err != nil {
			t.Fatalf("filling the store: %v", err)
		}
	}
	for _, key := range []string{"acme", "beta"} {
		write(func(w store.Writer) error {
			return w.PutProject(domain.Project{Key: key, DisplayName: key, Created: t0, Mode: "committed"})
		})
	}
	im := ImportBatch()
	hist := historyTicket()
	hist.Number, hist.Spec = 7, 5
	im.Tickets = append(im.Tickets, hist)
	write(func(w store.Writer) error { return w.PutImport("acme", im) })
	write(func(w store.Writer) error {
		return w.PutCIRuns("acme", []domain.CIRun{
			{ID: 9001, Attempt: 2, Workflow: "ci.yml", Name: "CI", Branch: "main", Commit: "abc123", Status: domain.CICompleted,
				Conclusion: domain.CIFailure, Started: t0, Finished: t0.Add(5 * time.Minute), Ticket: 3,
				Jobs: []domain.CIJob{{Name: "test", Conclusion: domain.CIFailure}}},
			{ID: 9002, Attempt: 1, Workflow: "nightly.yml", Name: "Nightly", Branch: "main", Commit: "abc123", Status: domain.CIQueued},
		})
	})
	write(func(w store.Writer) error {
		return w.PutOptions(domain.Options{FocusClear: domain.FocusAllClear, FocusPopup: domain.FocusPopupOn, LimitsPopup: domain.LimitsPopupOff, GHPath: "/opt/tools/gh"})
	})
	for _, day := range []string{"2026-11-06", "2026-11-07"} {
		for _, rec := range []usage.Record{usageCall("find_tickets"), usageResponse("acme-3"), usageCall("get_ticket")} {
			if err := s.Usage().Append("acme", day, rec); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// Contents is everything a Store holds that Store Move copies, read so
// two Stores' can be compared: each Project with its records, CI Runs
// and Usage, and the Options.
type Contents struct {
	Projects map[string]ProjectContents
	Options  domain.Options
}

// ProjectContents is one Project's part of a Store's Contents. Project
// is the zero value for usage with no Project record.
type ProjectContents struct {
	Project domain.Project
	Records []store.Record
	CIRuns  []domain.CIRun
	Usage   map[string]usage.Records
}

// ContentsOf reads everything s holds that Store Move copies.
func ContentsOf(t testing.TB, s store.Store) Contents {
	t.Helper()
	c := Contents{Projects: map[string]ProjectContents{}}
	err := s.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			c.Projects[p.Key] = ProjectContents{Project: p, Records: slices.Collect(r.Records(p.Key)), CIRuns: r.CIRuns(p.Key)}
		}
		c.Options = r.Options()
		return nil
	})
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	days, err := s.Usage().Days()
	if err != nil {
		t.Fatalf("listing usage's days: %v", err)
	}
	for key, ds := range days {
		pc := c.Projects[key]
		pc.Usage = map[string]usage.Records{}
		for _, d := range ds {
			recs, err := s.Usage().Read(key, []string{d})
			if err != nil {
				t.Fatalf("reading %s's usage for %s: %v", key, d, err)
			}
			pc.Usage[d] = recs
		}
		c.Projects[key] = pc
	}
	for key, pc := range c.Projects {
		for _, r := range pc.Records {
			emptyToNil(reflect.ValueOf(r.Spec))
			emptyToNil(reflect.ValueOf(r.Ticket))
		}
		c.Projects[key] = pc
	}
	return c
}

// emptyToNil sets each empty slice or map v holds, through pointers and
// struct fields, to nil: a Store may give either back for none, and
// they hold the same.
func emptyToNil(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			emptyToNil(v.Elem())
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				emptyToNil(v.Field(i))
			}
		}
	case reflect.Slice, reflect.Map:
		if v.Len() == 0 && !v.IsNil() && v.CanSet() {
			v.Set(reflect.Zero(v.Type()))
		}
		if v.Kind() == reflect.Slice {
			for i := range v.Len() {
				emptyToNil(v.Index(i))
			}
		}
	}
}

// SameContents fails t unless a and b hold the same records, Options, CI
// Runs and Usage, naming the first Project or part that differs.
func SameContents(t testing.TB, a, b Contents) {
	t.Helper()
	if a.Options != b.Options {
		t.Errorf("Options %+v, then %+v", a.Options, b.Options)
	}
	keys := slices.Sorted(maps.Keys(a.Projects))
	if got := slices.Sorted(maps.Keys(b.Projects)); !slices.Equal(keys, got) {
		t.Fatalf("Projects %v, then %v", keys, got)
	}
	for _, key := range keys {
		pa, pb := a.Projects[key], b.Projects[key]
		for _, part := range []struct {
			name string
			a, b any
		}{
			{"Project", pa.Project, pb.Project},
			{"records", pa.Records, pb.Records},
			{"CI Runs", pa.CIRuns, pb.CIRuns},
			{"Usage", pa.Usage, pb.Usage},
		} {
			if !reflect.DeepEqual(part.a, part.b) {
				t.Errorf("%s's %s differ:\n%s\nthen\n%s", key, part.name, show(part.a), show(part.b))
			}
		}
	}
}

// show prints v whole, following the pointers in a Record.
func show(v any) string {
	if rs, ok := v.([]store.Record); ok {
		out := ""
		for _, r := range rs {
			if r.Spec != nil {
				out += fmt.Sprintf("spec %+v\n", *r.Spec)
			} else {
				out += fmt.Sprintf("ticket %+v\n", *r.Ticket)
			}
		}
		return out
	}
	return fmt.Sprintf("%+v", v)
}

func runMove(t *testing.T, f Factory) {
	t.Run("a copy into an empty store holds everything the source does", func(t *testing.T) {
		from, to := f.New(t), f.New(t)
		Fill(t, from)
		if err := store.Copy(to, from); err != nil {
			t.Fatalf("copy: %v", err)
		}
		SameContents(t, ContentsOf(t, from), ContentsOf(t, to))
		tallyFrom, err := store.TallyOf(from)
		if err != nil {
			t.Fatal(err)
		}
		tallyTo, err := store.TallyOf(to)
		if err != nil {
			t.Fatal(err)
		}
		if diff := store.CompareTallies(tallyFrom, tallyTo); len(diff) > 0 {
			t.Errorf("the tallies differ: %v", diff)
		}
		want := map[string]store.ProjectTally{
			"acme": {Specs: 2, Tickets: 4, CIRuns: 2, UsageDays: 2},
			"beta": {},
		}
		if !maps.Equal(tallyTo.Projects, want) {
			t.Errorf("the copy's tally %+v, want %+v", tallyTo.Projects, want)
		}
	})

	t.Run("usage under a key with no project is neither copied nor counted", func(t *testing.T) {
		from, to := f.New(t), f.New(t)
		seedProject(t, from)
		appendUsage(t, from, "acme", "2026-11-07", usageCall("get_ticket"))
		appendUsage(t, from, "gone", "2026-11-07", usageCall("get_spec"))
		if err := store.Copy(to, from); err != nil {
			t.Fatalf("copy: %v", err)
		}
		days, err := to.Usage().Days()
		if err != nil {
			t.Fatal(err)
		}
		if want := map[string][]string{"acme": {"2026-11-07"}}; !maps.EqualFunc(days, want, slices.Equal) {
			t.Errorf("the copy's usage days %v, want %v", days, want)
		}
		tl, err := store.TallyOf(from)
		if err != nil {
			t.Fatal(err)
		}
		if want := map[string]store.ProjectTally{"acme": {UsageDays: 1}}; !maps.Equal(tl.Projects, want) {
			t.Errorf("the source's tally %+v, want %+v", tl.Projects, want)
		}
	})

	t.Run("a new store is empty, and one holding anything isn't", func(t *testing.T) {
		s := f.New(t)
		if tl, err := store.TallyOf(s); err != nil || !tl.Empty() {
			t.Fatalf("a new store's tally %+v, %v; want empty", tl, err)
		}
		if empty, err := store.IsEmpty(s); err != nil || !empty {
			t.Fatalf("a new store isn't empty (%v)", err)
		}
		for name, fill := range map[string]func(store.Store){
			"a Project": func(s store.Store) { seedProject(t, s) },
			"Options": func(s store.Store) {
				put(t, s, func(w store.Writer) error {
					o := domain.DefaultOptions()
					o.GHPath = "/opt/gh"
					return w.PutOptions(o)
				})
			},
			"Usage under a key with no Project": func(s store.Store) {
				appendUsage(t, s, "gone", "2026-11-07", usageCall("get_spec"))
			},
		} {
			s := f.New(t)
			fill(s)
			if empty, err := store.IsEmpty(s); err != nil || empty {
				t.Errorf("a store holding only %s is empty (%v)", name, err)
			}
		}
	})

	t.Run("comparing tallies names each difference", func(t *testing.T) {
		a := store.Tally{Projects: map[string]store.ProjectTally{"acme": {Specs: 2, Tickets: 4, CIRuns: 1, UsageDays: 3}, "beta": {}}, Options: domain.DefaultOptions()}
		b := store.Tally{Projects: map[string]store.ProjectTally{"acme": {Specs: 2, Tickets: 3, CIRuns: 1, UsageDays: 3}}, Options: domain.Options{GHPath: "/x"}}
		got := store.CompareTallies(a, b)
		want := []string{
			"acme: 4 Tickets in the source, 3 in the target",
			"beta: in the source, not in the target",
			"the Options differ",
		}
		if !slices.Equal(got, want) {
			t.Errorf("differences %q, want %q", got, want)
		}
	})

	t.Run("a store with a live service isn't stopped, and names each one", func(t *testing.T) {
		s := f.New(t)
		at := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
		if err := store.CheckStopped(s, at); err != nil {
			t.Errorf("no service registered: %v", err)
		}
		register(t, s, registration("a", "laptop", at))
		register(t, s, registration("b", "desktop", at))
		err := store.CheckStopped(s, at.Add(store.ServiceLife-time.Second))
		if err == nil || !strings.Contains(err.Error(), "desktop (v1.2.3), laptop (v1.2.3)") {
			t.Errorf("two services live: %v", err)
		}
		if err := store.CheckStopped(s, at.Add(store.ServiceLife+time.Second)); err != nil {
			t.Errorf("both lapsed: %v", err)
		}
	})
}
