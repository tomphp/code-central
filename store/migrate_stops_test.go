package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

func TestTheRealChainMigratesAFormat58StoreToTheStopsFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 58\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: Form\nstate: In Progress\nspec: 1\nclaims:\n  - number: 1\n    id: ABC\n    session_id: s1\n    started: 2026-01-02T03:04:05Z\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n")

	s := openFile(t, dir)
	s.View(func(r store.Reader) error {
		tk, _ := r.Ticket("acme", 2, store.History...)
		if cl := tk.Claims[0]; cl.Stopped() || cl.FactoryClaim() || len(cl.Stops) != 0 {
			t.Errorf("claim after migrating = %+v, want not Stopped and not a Factory Claim", cl)
		}
		return nil
	})

	stopped := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	if err := s.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", 2, store.History...)
		tk.Claims[0].Factory, tk.Claims[0].Host = "factory", "laptop"
		tk.Claims[0].Stops = []domain.Stop{{Stopped: stopped.Add(-time.Hour), Resumed: stopped}, {Stopped: stopped.Add(time.Hour)}}
		return w.PutTicket("acme", tk, store.History...)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		tk, _ := r.Ticket("acme", 2, store.History...)
		cl := tk.Claims[0]
		if !cl.FactoryClaim() || cl.Host != "laptop" || len(cl.Stops) != 2 || !cl.Stops[0].Resumed.Equal(stopped) || !cl.Stopped() {
			t.Errorf("claim = %+v, want a Stopped Factory Claim with two Stops", cl)
		}
		return nil
	})
}
