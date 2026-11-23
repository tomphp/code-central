// Refused States: a State Azure DevOps won't move a work item to, unknown
// to its type or barred by a workflow rule, is collected while write-back
// goes on, and reported as the state_refused error after the poll.

package workitemwatcher

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/domain"
)

// refusedState is Azure DevOps refusing to move work item ID, linked to
// Target, to the State w maps for the phase Key, with its reason.
type refusedState struct {
	ID     int
	Target string
	State  string
	Key    string
	Reason string
}

func (r *refusedState) Error() string {
	return fmt.Sprintf("Azure DevOps refused moving work item %d, for %s, to %s: %s", r.ID, r.Target, r.State, r.Reason)
}

// sentence is r ending in one full stop, whether or not Azure DevOps'
// reason ends in one.
func (r *refusedState) sentence() string { return strings.TrimSuffix(r.Error(), ".") + "." }

// asRefusedState is err as a refusedState when it is Azure DevOps' 400
// refusal of w's State on work item id, else err.
func asRefusedState(err error, id int, w api.WorkItemWrites) error {
	var refusal *azdo.RefusedError
	if !errors.As(err, &refusal) || refusal.Status != http.StatusBadRequest {
		return err
	}
	return &refusedState{ID: id, Target: w.Target, State: w.State.Name, Key: w.State.Key, Reason: refusal.Message}
}

// phaseNames are the Work Item States mappings, by their phase's key.
var phaseNames = map[string]string{
	domain.StateKeyInProgress: "In Progress",
	domain.StateKeyAccepted:   "Accepted",
	domain.StateKeyDone:       "Done",
}

// stateRefusedProblem is the state_refused error naming each of refused.
func stateRefusedProblem(refused []*refusedState) api.WatcherCheckResult {
	var reasons, phases []string
	for _, r := range refused {
		reasons = append(reasons, r.sentence())
		if name := phaseNames[r.Key]; !slices.Contains(phases, name) {
			phases = append(phases, name)
		}
	}
	return api.WatcherCheckResult{Problem: api.ProblemStateRefused,
		Reason: strings.Join(reasons, " "),
		Fix: fmt.Sprintf("Correct or blank %s in Work Item States, in the Azure Boards settings of Project Options, or allow the move in the work item type's workflow in Azure DevOps.",
			strings.Join(phases, " and "))}
}
