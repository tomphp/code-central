//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// startupBudget is what one startup may take, migration included: it
// runs before the port is bound, inside Claude Code's ~31s reconnect window.
const startupBudget = 10 * time.Second

const (
	synthProjects        = 10
	synthSpecsPerProject = 20
	synthTicketsPerSpec  = 50 // 10 × 20 × 50 = 10,000 Tickets
)

// 3. Upgrades: a synthetic store of 10,000 Tickets with long histories
// loads within the budget, then a build one format ahead (a migrationtest
// build, whose test-only step is additive) migrates and loads it within
// the same budget.
func TestA10000TicketStoreLoadsAndMigratesWithinTheBudget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeSyntheticStore(t, dir)

	next := filepath.Join(t.TempDir(), "code-central-next")
	build := exec.Command("go", "build", "-tags", "migrationtest", "-o", next, "../cmd/code-central")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the migrationtest binary: %v\n%s", err, out)
	}

	// Loading without a migration.
	p := timedStart(t, bin, dir, "loading")
	checkSyntheticTicket(t, p.addr)
	p.stop(t)

	// Migrating to the next format, then loading.
	p = timedStart(t, next, dir, "migrating and loading")
	checkSyntheticTicket(t, p.addr)
	p.stop(t)

	if got, _ := os.ReadFile(filepath.Join(dir, "store.yaml")); string(got) != fmt.Sprintf("format: %d\n", store.Format+1) {
		t.Fatalf("store.yaml after migrating = %q", got)
	}
	bak := filepath.Join(filepath.Dir(dir), fmt.Sprintf("code-central.format-%d.bak", store.Format))
	if got, _ := os.ReadFile(filepath.Join(bak, "store.yaml")); string(got) != fmt.Sprintf("format: %d\n", store.Format) {
		t.Fatalf("backup store.yaml = %q", got)
	}

	// The shipping binary now refuses the newer store by name.
	if p, msg := startServe(t, dir, "127.0.0.1:0"); p != nil || !strings.Contains(msg, fmt.Sprintf("unknown format %d", store.Format+1)) {
		t.Fatalf("an older binary opened a newer store, or said %q", msg)
	}
}

func timedStart(t *testing.T, binary, dir, what string) *process {
	t.Helper()
	start := time.Now()
	p, msg := startServeBin(t, binary, dir, "127.0.0.1:0")
	took := time.Since(start)
	if p == nil {
		t.Fatalf("%s: serve did not start: %s", what, msg)
	}
	t.Logf("%s 10,000 Tickets took %v", what, took)
	if took > startupBudget {
		t.Fatalf("%s 10,000 Tickets took %v, over the %v budget", what, took, startupBudget)
	}
	return p
}

// checkSyntheticTicket reads one Ticket back with its whole history.
func checkSyntheticTicket(t *testing.T, addr string) {
	t.Helper()
	resp, err := http.Get("http://" + addr + "/api/p/proj-7/tickets/500")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var tk struct {
		Title    string            `json:"title"`
		Moves    []json.RawMessage `json:"moves"`
		Comments []json.RawMessage `json:"comments"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&tk) != nil ||
		tk.Title != "Ticket 500" || len(tk.Moves) != synthMoves || len(tk.Comments) != synthComments {
		t.Fatalf("get ticket proj-7-500: %d %+v", resp.StatusCode, tk)
	}
}

const (
	synthClaims   = 8
	synthMoves    = 3 * synthClaims
	synthComments = 30
)

// writeSyntheticStore writes a store at this build's format, file by
// file, as the shipping binary writes it. If the schema changes, update
// these templates with it.
func writeSyntheticStore(t *testing.T, dir string) {
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
	for p := 1; p <= synthProjects; p++ {
		key := fmt.Sprintf("proj-%d", p)
		write("projects/"+key+"/project.yaml", fmt.Sprintf("key: %s\ndisplay_name: Project %d\ncreated: 2026-01-01T00:00:00Z\nmode: committed\n", key, p))
		for s := range synthSpecsPerProject {
			wg.Go(func() {
				spec := 1 + s*(synthTicketsPerSpec+1)
				write(fmt.Sprintf("projects/%s/specs/%d.md", key, spec), synthSpec(spec))
				for i := 1; i <= synthTicketsPerSpec; i++ {
					write(fmt.Sprintf("projects/%s/tickets/%d.yaml", key, spec+i), synthTicket(spec+i, spec))
				}
			})
		}
	}
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}
}

func synthSpec(n int) string {
	return fmt.Sprintf(`---
id: %d
title: Spec %d
labels:
  - synthetic
tags: {}
created: 2026-01-01T00:00:00Z
updated: 2026-01-02T00:00:00Z
edits:
  - number: 1
    actor: agent
    time: 2026-01-02T00:00:00Z
    previous:
      title: Old spec %d
---
# Spec %d

%s
`, n, n, n, n, strings.Repeat("A paragraph of the Spec's body, long enough to be realistic. ", 20))
}

func synthTicket(n, spec int) string {
	var b strings.Builder
	at := func(i int) string {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute).Format(time.RFC3339)
	}
	fmt.Fprintf(&b, "title: Ticket %d\nstate: In Review\nspec: %d\nlabels:\n  - backend\n  - synthetic\ntags:\n  source: synth-%d\n", n, spec, n)
	fmt.Fprintf(&b, "blocked_by: []\nreplacements: []\ncreated: %s\nupdated: %s\ndescription: |\n", at(0), at(1000))
	for range 15 {
		b.WriteString("  A line of the description, which runs on for a while like real ones do.\n")
	}
	b.WriteString("claims:\n")
	for c := 1; c <= synthClaims; c++ {
		fmt.Fprintf(&b, "  - number: %d\n    id: CLAIM%dID%dXXXXXXXXXXXXXXXXXXXX\n    session_id: sess-%d-%d\n    started: %s\n", c, n, c, n, c, at(c*100))
		if c < synthClaims {
			fmt.Fprintf(&b, "    ended: %s\n    ended_by: give_up\n", at(c*100+50))
		}
	}
	b.WriteString("rounds:\n")
	for r := 1; r <= 3; r++ {
		fmt.Fprintf(&b, "  - number: %d\n    claim: %d\n    session_id: sess-%d-%d\n    posted: %s\n    questions:\n", r, r, n, r, at(r*100+10))
		for q := 1; q <= 3; q++ {
			fmt.Fprintf(&b, "      - number: %d\n        title: Question %d?\n        body: |\n          Which way should this go, and why?\n        recommended: |\n          The simple way.\n        status: answered\n        answer: |\n          The simple way, for now.\n        closed: %s\n", q, q, at(r*100+20))
		}
	}
	b.WriteString("failure_reports:\n")
	for f := 1; f <= 3; f++ {
		fmt.Fprintf(&b, "  - number: %d\n    claim: %d\n    session_id: sess-%d-%d\n    actor: agent\n    time: %s\n    what_broke: |\n      The tests failed.\n    why: |\n      A flaky dependency.\n    what_to_fix: |\n      Pin it.\n    where_work_stands: |\n      Pushed to the branch.\n", f, f, n, f, at(f*100+40))
	}
	b.WriteString("comments:\n")
	for c := 1; c <= synthComments; c++ {
		fmt.Fprintf(&b, "  - number: %d\n    actor: agent\n    time: %s\n    claim: %d\n    body: |\n      Progress note %d: did a thing, then another thing.\n", c, at(c*30), 1+c%synthClaims, c)
	}
	b.WriteString("moves:\n")
	for c := 1; c <= synthClaims; c++ {
		fmt.Fprintf(&b, "  - from: Ready\n    to: In Progress\n    actor: agent\n    time: %s\n    claim: %d\n", at(c*100), c)
		if c < synthClaims {
			fmt.Fprintf(&b, "  - from: In Progress\n    to: Failed\n    actor: agent\n    time: %s\n    claim: %d\n", at(c*100+50), c)
			fmt.Fprintf(&b, "  - from: Failed\n    to: Ready\n    actor: human\n    time: %s\n    via: retry\n", at(c*100+60))
		} else {
			fmt.Fprintf(&b, "  - from: Draft\n    to: Ready\n    actor: human\n    time: %s\n", at(1))
			fmt.Fprintf(&b, "  - from: In Progress\n    to: In Review\n    actor: agent\n    time: %s\n    claim: %d\n", at(c*100+90), c)
		}
	}
	b.WriteString("edits:\n")
	for e := 1; e <= 5; e++ {
		fmt.Fprintf(&b, "  - number: %d\n    actor: human\n    time: %s\n    previous:\n      title: Old title %d\n", e, at(e), e)
	}
	return b.String()
}
