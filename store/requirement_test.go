package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// A submitted Requirement is the staged write's second user (ADR 0003):
// its Spec and three Stage Tickets land together or not at all, so a
// half-built chain is never visible, even after a crash.
func TestASubmittedRequirementStoppedAtAnyStepRecoversToAllOrNothing(t *testing.T) {
	const files = 4 // the Spec and its three Stage Tickets
	// stage 4 files, write the manifest, 4 renames, delete the manifest
	const steps = 2*files + 2
	for k := 1; k <= steps+1; k++ {
		t.Run(fmt.Sprintf("stopped after step %d", k), func(t *testing.T) {
			t.Parallel() // fsync-bound
			s := seedAcme(t, t.TempDir())
			store.StopImportAfter(s, k)
			c := core.New(s, clock.NewFake(t0), &ids.Sequence{})
			_, err := c.SubmitRequirement(context.Background(), "acme", core.SubmitRequirementInput{Text: "Dark mode\n"})
			if k > steps {
				if err != nil {
					t.Fatalf("the Requirement was not submitted: %v", err)
				}
				return
			}
			if !errors.Is(err, store.ErrImportStopped) {
				t.Fatalf("step %d: %v", k, err)
			}
			if sp, tk := counts(t, s); sp != 0 || tk != 0 {
				t.Fatalf("step %d: memory changed before the write finished (%d specs, %d tickets)", k, sp, tk)
			}
			s = reopen(t, s)
			sp, tk := counts(t, s)
			committed := k > files // the manifest is written at step files+1
			switch {
			case committed && (sp != 1 || tk != 3):
				t.Fatalf("step %d: after recovery %d specs, %d tickets; want the Spec and all three Stages", k, sp, tk)
			case !committed && (sp != 0 || tk != 0):
				t.Fatalf("step %d: after recovery %d specs, %d tickets; want none", k, sp, tk)
			}
			if committed {
				s.View(func(r store.Reader) error {
					spec, _ := r.Ticket("acme", 3)
					if len(spec.BlockedBy) != 1 || spec.BlockedBy[0] != 2 {
						t.Fatalf("step %d: the spec Stage lost its Blocker: %+v", k, spec)
					}
					return nil
				})
			}
			noStaging(t, s.Dir())
		})
	}
}
