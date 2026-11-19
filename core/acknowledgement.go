package core

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/internal/fieldwalk"
)

// A write that used to echo the whole record returns an Acknowledgement
// by default, and the record only when its input asks for it with full
// (ADR 0009). acknowledge and acknowledgeCreate are the one place that
// chooses between the two, and the one place changed is derived.

// acknowledge is a write's result from the record's full view before and
// after it: after itself with full, else the Acknowledgement.
func acknowledge[V any](before, after V, full bool) (api.Written[V], error) {
	return written(before, after, full, "updated")
}

// acknowledgeCreate is a create's result, as acknowledge with the empty
// record as before; the id and times every create sets aren't changed.
func acknowledgeCreate[V any](empty, after V, full bool) (api.Written[V], error) {
	return written(empty, after, full, "id", "created", "updated")
}

// acknowledgeProject is a Project write's result, as acknowledge, leaving
// out factory_status, which time moves and no write sets. A
// Project has no id or updated, so its Acknowledgement gives its key as
// the id and the write's time as updated.
func acknowledgeProject(before, after api.ProjectView, full bool, at time.Time) (api.Written[api.ProjectView], error) {
	w, err := written(before, after, full, "factory_status")
	if err == nil && w.Record == nil {
		w.ID, w.Updated = after.Key, at
	}
	return w, err
}

// offers are a Ticket view's fields that say what the UI offers the
// human, which follow from what a write changed but aren't written by it,
// so no Acknowledgement lists them.
var offers = []string{"editable", "retroable"}

// written derives the Acknowledgement from the views' JSON, listing in
// changed every top-level field that differs except those unlisted and
// the offers. It reads the record's id and updated, the number of the
// last of its edits when they changed, and a Ticket's state, by those
// JSON names, which the Spec and Ticket views share (a Spec view has no
// state).
func written[V any](before, after V, full bool, unlisted ...string) (api.Written[V], error) {
	if full {
		return api.Written[V]{Record: &after}, nil
	}
	unlisted = append(unlisted, offers...)
	b, err := topLevel(before)
	if err != nil {
		return api.Written[V]{}, err
	}
	a, err := topLevel(after)
	if err != nil {
		return api.Written[V]{}, err
	}
	ack := api.Acknowledgement{Changed: []string{}}
	for _, name := range JSONNames(reflect.TypeFor[V]()) {
		if !bytes.Equal(b[name], a[name]) && !slices.Contains(unlisted, name) {
			ack.Changed = append(ack.Changed, name)
		}
	}
	// A Project's view has neither; acknowledgeProject fills them in.
	if id, ok := a["id"]; ok {
		if err := json.Unmarshal(id, &ack.ID); err != nil {
			return api.Written[V]{}, err
		}
	}
	if updated, ok := a["updated"]; ok {
		if err := json.Unmarshal(updated, &ack.Updated); err != nil {
			return api.Written[V]{}, err
		}
	}
	if state, ok := a["state"]; ok {
		if err := json.Unmarshal(state, &ack.State); err != nil {
			return api.Written[V]{}, err
		}
	}
	if slices.Contains(ack.Changed, "edits") {
		var edits []struct {
			Number int `json:"number"`
		}
		if err := json.Unmarshal(a["edits"], &edits); err != nil {
			return api.Written[V]{}, err
		}
		if len(edits) > 0 {
			ack.Edit = edits[len(edits)-1].Number
		}
	}
	return api.Written[V]{Acknowledgement: ack}, nil
}

// topLevel is v's JSON, field by top-level field.
func topLevel(v any) (map[string]json.RawMessage, error) {
	j, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]json.RawMessage
	return out, json.Unmarshal(j, &out)
}

// JSONNames are the JSON field names of struct type t, in field order,
// with embedded structs' fields in their place: for a record's view, the
// names its writes' changed can list, in that order. The UI's generated
// field lists (internal/wiretypes) and the MCP door's schemas read them
// the same way, through internal/fieldwalk.
func JSONNames(t reflect.Type) []string {
	return fieldwalk.Names(t)
}
