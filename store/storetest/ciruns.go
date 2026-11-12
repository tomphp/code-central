// The contract for a Ticket's pushed commits and a Project's CI Runs
// (code-central-256).

package storetest

import (
	"reflect"
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// runCIRuns checks a Ticket's pushed commits and a Project's CI Runs,
// which are written apart from its Tickets.
func runCIRuns(t *testing.T, f Factory) {
	t.Run("a ticket's pushed commits read back after they are written, with the human's Land anyway and Re-run request", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		want := []store.PushedCommit{{Commit: "abc123", Pushed: t0, LandAnyway: t0.Add(time.Minute)}, {Commit: "def456", Pushed: t0.Add(time.Hour),
			Rerun: t0.Add(2 * time.Hour), Reruns: []store.CIRunAttempt{{ID: 9001, Attempt: 1}, {ID: 9003, Attempt: 2}}}}
		put(t, s, func(w store.Writer) error {
			return w.PutTicket("acme", store.Ticket{Number: 2, Title: "Pushed", State: store.Accepted, Spec: 1, Created: t0, Updated: t0, PushedCommits: want})
		})
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if tk, _ := r.Ticket("acme", 2); !reflect.DeepEqual(tk.PushedCommits, want) {
					t.Fatalf("pushed commits = %+v, want %+v", tk.PushedCommits, want)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("a project's ci responses read back after they are written, one per pushed commit", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		want := []store.CIResponse{{Commit: "abc123", LandAnyway: t0.Add(time.Minute)}, {Commit: "def456",
			Rerun: t0.Add(2 * time.Hour), Reruns: []store.CIRunAttempt{{ID: 9001, Attempt: 1}, {ID: 9003, Attempt: 2}}}}
		put(t, s, func(w store.Writer) error {
			p, _ := w.Project("acme")
			p.CIResponses = want
			return w.PutProject(p)
		})
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if p, _ := r.Project("acme"); !reflect.DeepEqual(p.CIResponses, want) {
					t.Fatalf("ci responses = %+v, want %+v", p.CIResponses, want)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("a project's ci runs read back after they are written", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		read(t, s, func(r store.Reader) {
			if got := r.CIRuns("acme"); len(got) != 0 {
				t.Fatalf("ci runs = %+v, want none before any are written", got)
			}
		})
		want := []store.CIRun{
			{ID: 9001, Attempt: 2, Workflow: "ci.yml", Name: "CI", Branch: "main", Commit: "abc123", Status: store.CICompleted,
				Conclusion: store.CIFailure, Started: t0, Finished: t0.Add(5 * time.Minute), URL: "https://github.com/acme/web/actions/runs/9001",
				Jobs:   []store.CIJob{{Name: "test", Conclusion: store.CIFailure, URL: "https://github.com/acme/web/actions/runs/9001/job/1"}, {Name: "lint", URL: "https://github.com/acme/web/actions/runs/9001/job/2"}},
				Ticket: 2},
			{ID: 9002, Attempt: 1, Workflow: "nightly.yml", Name: "Nightly", Branch: "main", Commit: "abc123", Status: store.CIQueued,
				URL: "https://github.com/acme/web/actions/runs/9002", Rule: 4, Ignored: true},
		}
		put(t, s, func(w store.Writer) error { return w.PutCIRuns("acme", want) })
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if got := r.CIRuns("acme"); !reflect.DeepEqual(got, want) {
					t.Fatalf("ci runs = %+v, want %+v", got, want)
				}
				if tk, ok := r.Ticket("acme", 2); ok {
					t.Fatalf("writing ci runs wrote a ticket: %+v", tk)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("ci runs need their project", func(t *testing.T) {
		s := f.New(t)
		err := s.Update(func(w store.Writer) error { return w.PutCIRuns("nope", nil) })
		if err != store.ErrUnknownProject {
			t.Fatalf("err = %v, want ErrUnknownProject", err)
		}
	})
}
