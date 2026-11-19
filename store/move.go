// Store Move's half in the store (Spec code-central-1032, ADR 0070):
// copying one Store into another, of any Kinds, through the Store
// interface, and tallying what a Store holds so the copy can be checked.
// The command around it, which opens both Stores and refuses an unsafe
// move, is the cli's.

package store

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/usage"
)

// Copy writes everything from holds into to, an empty Store: each
// Project, then its Specs and Tickets with every History part as one
// Import, then its CI Runs and its Usage, day by day; then the Options,
// unless they are the defaults. Each Project is its own Updates, so a
// failure leaves to holding part of the copy. A Usage line from could
// not parse isn't copied, since no read could give it, and neither is
// Usage under a key with no Project, which a File Store couldn't load.
func Copy(to, from Store) error {
	var keys []string
	if err := from.View(func(r Reader) error {
		for _, p := range r.Projects() {
			keys = append(keys, p.Key)
		}
		return nil
	}); err != nil {
		return err
	}
	days, err := from.Usage().Days()
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := copyProject(to, from, key); err != nil {
			return fmt.Errorf("copying %s: %w", key, err)
		}
		if err := copyUsage(to.Usage(), from.Usage(), key, days[key]); err != nil {
			return fmt.Errorf("copying %s's Usage: %w", key, err)
		}
	}
	var opts domain.Options
	if err := from.View(func(r Reader) error { opts = r.Options(); return nil }); err != nil {
		return err
	}
	if opts != domain.DefaultOptions() {
		if err := to.Update(func(w Writer) error { return w.PutOptions(opts) }); err != nil {
			return fmt.Errorf("copying the Options: %w", err)
		}
	}
	return nil
}

// copyProject copies the Project key, its records and its CI Runs.
func copyProject(to, from Store, key string) error {
	var p domain.Project
	var im Import
	var runs []domain.CIRun
	if err := from.View(func(r Reader) error {
		p, _ = r.Project(key)
		for rec := range r.Records(key) {
			if rec.Spec != nil {
				im.Specs = append(im.Specs, *rec.Spec)
			} else {
				im.Tickets = append(im.Tickets, *rec.Ticket)
			}
		}
		runs = r.CIRuns(key)
		return nil
	}); err != nil {
		return err
	}
	if err := to.Update(func(w Writer) error { return w.PutProject(p) }); err != nil {
		return err
	}
	if len(im.Specs)+len(im.Tickets) > 0 {
		if err := to.Update(func(w Writer) error { return w.PutImport(key, im) }); err != nil {
			return err
		}
	}
	if len(runs) > 0 {
		if err := to.Update(func(w Writer) error { return w.PutCIRuns(key, runs) }); err != nil {
			return fmt.Errorf("its CI Runs: %w", err)
		}
	}
	return nil
}

// copyUsage appends the Project key's Usage in from, for each of days,
// to to: each day's calls, then its responses.
func copyUsage(to, from usage.Store, key string, days []string) error {
	for _, day := range days {
		recs, err := from.Read(key, []string{day})
		if err != nil {
			return err
		}
		for _, c := range recs.Calls {
			if err := to.Append(key, day, usage.Record{Call: &c}); err != nil {
				return err
			}
		}
		for _, r := range recs.Responses {
			if err := to.Append(key, day, usage.Record{Response: &r}); err != nil {
				return err
			}
		}
	}
	return nil
}

// Tally is what a Store holds, counted as Copy copies it: each Project's
// Specs, Tickets, CI Runs and Usage days, by key, and the Options.
type Tally struct {
	Projects map[string]ProjectTally
	Options  domain.Options
}

// ProjectTally is one Project's counts in a Tally.
type ProjectTally struct {
	Specs, Tickets, CIRuns, UsageDays int
}

// Empty is whether the Store holds nothing Copy would copy: no Project,
// and the default Options.
func (t Tally) Empty() bool {
	return len(t.Projects) == 0 && t.Options == domain.DefaultOptions()
}

// IsEmpty is whether s holds nothing at all: an empty Tally, and no Usage
// under any key, even one with no Project, which a Tally leaves out. Only
// an empty Store is a Store Move's target, so two are never merged.
func IsEmpty(s Store) (bool, error) {
	t, err := TallyOf(s)
	if err != nil || !t.Empty() {
		return false, err
	}
	days, err := s.Usage().Days()
	return err == nil && len(days) == 0, err
}

// CheckStopped is an error naming each service registered live on s at
// now, by host and version, or nil when there is none: a Store Move's
// source must have none. It is what shows a service runs on a Postgres
// Store, which takes no lock.
func CheckStopped(s Store, now time.Time) error {
	regs, err := s.Services().Registrations(now)
	if err != nil {
		return fmt.Errorf("cannot read its Service Registrations: %w", err)
	}
	if len(regs) > 0 {
		return fmt.Errorf("a service is running on it: %s; stop it and move again, or wait %s for one that stopped without saying so",
			registered(regs), ServiceLife)
	}
	return nil
}

// TallyOf counts what s holds.
func TallyOf(s Store) (Tally, error) {
	t := Tally{Projects: map[string]ProjectTally{}}
	err := s.View(func(r Reader) error {
		for _, p := range r.Projects() {
			pt := ProjectTally{Specs: len(r.FindSpecs(p.Key, SpecFilter{})), CIRuns: len(r.CIRuns(p.Key))}
			for _, n := range r.StateCounts(p.Key) {
				pt.Tickets += n
			}
			t.Projects[p.Key] = pt
		}
		t.Options = r.Options()
		return nil
	})
	if err != nil {
		return t, err
	}
	days, err := s.Usage().Days()
	if err != nil {
		return t, err
	}
	for key, pt := range t.Projects {
		pt.UsageDays = len(days[key])
		t.Projects[key] = pt
	}
	return t, nil
}

// CompareTallies names each way target's Tally differs from source's,
// Projects in key order and the Options last; none when they match.
func CompareTallies(source, target Tally) []string {
	var out []string
	keys := slices.Sorted(maps.Keys(source.Projects))
	for key := range target.Projects {
		if _, ok := source.Projects[key]; !ok {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		s, inSource := source.Projects[key]
		t, inTarget := target.Projects[key]
		switch {
		case !inTarget:
			out = append(out, key+": in the source, not in the target")
			continue
		case !inSource:
			out = append(out, key+": in the target, not in the source")
			continue
		}
		for _, c := range []struct {
			what string
			s, t int
		}{{"Specs", s.Specs, t.Specs}, {"Tickets", s.Tickets, t.Tickets}, {"CI Runs", s.CIRuns, t.CIRuns}, {"Usage days", s.UsageDays, t.UsageDays}} {
			if c.s != c.t {
				out = append(out, fmt.Sprintf("%s: %d %s in the source, %d in the target", key, c.s, c.what, c.t))
			}
		}
	}
	if source.Options != target.Options {
		out = append(out, "the Options differ")
	}
	return out
}
