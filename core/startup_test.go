package core_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// startupBudget is generous on purpose: the fast suite runs many tests
// at once. A real slowdown in loading or in the reads the UI makes on
// every notice blows well past it (5,000 Tickets load in a few hundred
// milliseconds here).
const startupBudget = 3 * time.Second

const (
	genProjects        = 5
	genSpecsPerProject = 20
	genTicketsPerSpec  = 50 // 5 × 20 × 50 = 5,000 Tickets
)

// A store of about 5,000 Tickets, most of them finished as a long-lived
// store's are, loads and serves the UI's startup reads within the
// budget, so slowness shows up in the fast suite rather than in use.
func TestAStoreOf5000TicketsStartsUpQuickly(t *testing.T) {
	t.Parallel() // writing the files is most of its time
	dir := filepath.Join(t.TempDir(), "code-central")
	writeGeneratedStore(t, dir)

	start := time.Now()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	loaded := time.Since(start)
	c := core.New(s, clock.NewFake(t0), &ids.Sequence{})

	rows, err := c.ListProjects(ctx)
	if err != nil || len(rows) != genProjects {
		t.Fatalf("list_projects: %d rows, %v", len(rows), err)
	}
	if _, err := c.NeedsMe(ctx); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, p := range rows {
		all, err := c.FindTickets(ctx, p.Key, core.FindTicketsInput{State: store.States})
		if err != nil {
			t.Fatal(err)
		}
		total += len(all)
	}
	if total != genProjects*genSpecsPerProject*genTicketsPerSpec {
		t.Fatalf("found %d Tickets", total)
	}
	took := time.Since(start)
	t.Logf("loaded in %v, startup reads in %v", loaded, took-loaded)
	if took > startupBudget {
		t.Fatalf("loading took %v and the startup reads %v: over the %v budget", loaded, took-loaded, startupBudget)
	}
}

func writeGeneratedStore(t *testing.T, dir string) {
	t.Helper()
	write := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Error(err)
		}
	}
	write("store.yaml", fmt.Sprintf("format: %d\n", store.Format))
	var wg sync.WaitGroup
	for p := 1; p <= genProjects; p++ {
		key := fmt.Sprintf("proj-%d", p)
		write("projects/"+key+"/project.yaml", fmt.Sprintf("key: %s\ndisplay_name: Project %d\ncreated: 2026-01-01T00:00:00Z\nmode: committed\n", key, p))
		for sp := range genSpecsPerProject {
			wg.Go(func() {
				spec := 1 + sp*(genTicketsPerSpec+1)
				write(fmt.Sprintf("projects/%s/specs/%d.md", key, spec), fmt.Sprintf(
					"---\nid: %d\ntitle: Spec %d\nlabels: []\ntags: {}\ncreated: 2026-01-01T00:00:00Z\nupdated: 2026-01-01T00:00:00Z\nedits: []\n---\n# Spec %d\n", spec, spec, spec))
				for i := 1; i <= genTicketsPerSpec; i++ {
					write(fmt.Sprintf("projects/%s/tickets/%d.yaml", key, spec+i), generatedTicket(spec+i, spec))
				}
			})
		}
	}
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}
}

// generatedTicket is Done, Cancelled or (one in ten) Ready, with a short
// history: a claim, a few comments and the moves to get there.
func generatedTicket(n, spec int) string {
	at := func(m int) string {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n*10+m) * time.Minute).Format(time.RFC3339)
	}
	state := "Done"
	switch n % 10 {
	case 0:
		state = "Ready"
	case 1:
		state = "Cancelled"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "title: Ticket %d\nstate: %s\nspec: %d\nlabels:\n  - backend\ntags: {}\nblocked_by: []\nreplacements: []\n", n, state, spec)
	fmt.Fprintf(&b, "created: %s\nupdated: %s\ndescription: |\n  What Ticket %d delivers, in a line or two.\n", at(0), at(9), n)
	b.WriteString("claims: []\nrounds: []\nfailure_reports: []\ncomments:\n")
	for c := 1; c <= 3; c++ {
		fmt.Fprintf(&b, "  - number: %d\n    actor: agent\n    time: %s\n    body: |\n      Progress note %d.\n", c, at(c), c)
	}
	fmt.Fprintf(&b, "moves:\n  - from: Draft\n    to: Ready\n    actor: human\n    time: %s\n", at(1))
	if state != "Ready" {
		fmt.Fprintf(&b, "  - from: Ready\n    to: %s\n    actor: human\n    time: %s\n", state, at(9))
	}
	b.WriteString("edits: []\n")
	return b.String()
}
