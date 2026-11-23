package core

import (
	"reflect"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

func TestStateWriteIsTheFurthestPhaseReachedAfterItsMapping(t *testing.T) {
	since := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	later, earlier := since.Add(time.Hour), since.Add(-time.Hour)
	all := domain.WorkItemStates{
		InProgress: domain.WorkItemState{Name: "Committed", Since: since},
		Accepted:   domain.WorkItemState{Name: "Resolved", Since: since},
		Done:       domain.WorkItemState{Name: "Done", Since: since},
	}
	noAccepted := all
	noAccepted.Accepted = domain.WorkItemState{}
	for name, c := range map[string]struct {
		states  domain.WorkItemStates
		written []string
		at      reached
		want    *api.WorkItemStateWrite
	}{
		"nothing reached":              {all, nil, reached{}, nil},
		"in progress":                  {all, nil, reached{later}, &api.WorkItemStateWrite{Name: "Committed", Key: domain.StateKeyInProgress, Nearer: []string{}}},
		"reached as it was mapped":     {all, nil, reached{since}, nil},
		"reached before it was mapped": {all, nil, reached{earlier}, nil},
		"furthest, nearer unwritten":   {all, nil, reached{later, {}, later}, &api.WorkItemStateWrite{Name: "Done", Key: domain.StateKeyDone, Nearer: []string{domain.StateKeyInProgress, domain.StateKeyAccepted}}},
		"furthest, nearer written":     {all, []string{domain.StateKeyInProgress}, reached{later, later}, &api.WorkItemStateWrite{Name: "Resolved", Key: domain.StateKeyAccepted, Nearer: []string{}}},
		"blank mapping passed over":    {noAccepted, nil, reached{later, later}, &api.WorkItemStateWrite{Name: "Committed", Key: domain.StateKeyInProgress, Nearer: []string{}}},
		"a further phase written":      {all, []string{domain.StateKeyAccepted}, reached{later}, nil},
		"set once":                     {all, []string{domain.StateKeyDone}, reached{later, {}, later}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			if got := stateWrite(c.states, c.written, c.at); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("stateWrite = %+v, want %+v", got, c.want)
			}
		})
	}
}
