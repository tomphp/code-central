package core

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// A write that used to echo the whole record returns an Acknowledgement
// by default, and the record only when its input asks for it with full
// (ADR 0009). acknowledge and acknowledgeCreate are the one place that
// chooses between the two, and the one place changed is derived.

// Acknowledgement is a write's short result: what it wrote, never the
// values it wrote.
type Acknowledgement struct {
	ID      string    `json:"id"`
	Updated time.Time `json:"updated"`
	// Changed names the record's fields the write changed, in the record
	// view's field order, never updated. Empty when nothing changed.
	Changed []string `json:"changed"`
	// Edit is the number of the edit the write recorded, if it recorded one.
	Edit int `json:"edit,omitempty"`
	// State is a Ticket's State after the write; a Spec has none.
	State store.State `json:"state,omitempty"`
}

// Written is an acknowledging write's result: its Acknowledgement, or
// with full the record V. It marshals as one or the other.
type Written[V any] struct {
	Acknowledgement
	// Record is set, in place of the Acknowledgement, when the write was
	// asked for full.
	Record *V
}

// MarshalJSON is the record when there is one, else the Acknowledgement.
func (w Written[V]) MarshalJSON() ([]byte, error) {
	if w.Record != nil {
		return json.Marshal(w.Record)
	}
	return json.Marshal(w.Acknowledgement)
}

// acknowledge is a write's result from the record's full view before and
// after it: after itself with full, else the Acknowledgement.
func acknowledge[V any](before, after V, full bool) (Written[V], error) {
	return written(before, after, full, "updated")
}

// acknowledgeCreate is a create's result, as acknowledge with the empty
// record as before; the id and times every create sets aren't changed.
func acknowledgeCreate[V any](empty, after V, full bool) (Written[V], error) {
	return written(empty, after, full, "id", "created", "updated")
}

// acknowledgeProject is a Project write's result, as acknowledge. A
// Project has no id or updated, so its Acknowledgement gives its key as
// the id and the write's time as updated.
func acknowledgeProject(before, after ProjectView, full bool, at time.Time) (Written[ProjectView], error) {
	w, err := written(before, after, full)
	if err == nil && w.Record == nil {
		w.ID, w.Updated = after.Key, at
	}
	return w, err
}

// written derives the Acknowledgement from the views' JSON, listing in
// changed every top-level field that differs except those unlisted. It
// reads the record's id and updated, the number of the last of its edits
// when they changed, and a Ticket's state, by those JSON names, which the
// Spec and Ticket views share (a Spec view has no state).
func written[V any](before, after V, full bool, unlisted ...string) (Written[V], error) {
	if full {
		return Written[V]{Record: &after}, nil
	}
	b, err := topLevel(before)
	if err != nil {
		return Written[V]{}, err
	}
	a, err := topLevel(after)
	if err != nil {
		return Written[V]{}, err
	}
	ack := Acknowledgement{Changed: []string{}}
	for _, name := range jsonNames(reflect.TypeFor[V]()) {
		if !bytes.Equal(b[name], a[name]) && !slices.Contains(unlisted, name) {
			ack.Changed = append(ack.Changed, name)
		}
	}
	// A Project's view has neither; acknowledgeProject fills them in.
	if id, ok := a["id"]; ok {
		if err := json.Unmarshal(id, &ack.ID); err != nil {
			return Written[V]{}, err
		}
	}
	if updated, ok := a["updated"]; ok {
		if err := json.Unmarshal(updated, &ack.Updated); err != nil {
			return Written[V]{}, err
		}
	}
	if state, ok := a["state"]; ok {
		if err := json.Unmarshal(state, &ack.State); err != nil {
			return Written[V]{}, err
		}
	}
	if slices.Contains(ack.Changed, "edits") {
		var edits []struct {
			Number int `json:"number"`
		}
		if err := json.Unmarshal(a["edits"], &edits); err != nil {
			return Written[V]{}, err
		}
		if len(edits) > 0 {
			ack.Edit = edits[len(edits)-1].Number
		}
	}
	return Written[V]{Acknowledgement: ack}, nil
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

// jsonNames are the JSON field names of struct type t, in field order,
// with embedded structs' fields in their place.
func jsonNames(t reflect.Type) []string {
	var out []string
	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		switch {
		case name == "-" || !f.IsExported() && !f.Anonymous:
		case f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct:
			out = append(out, jsonNames(f.Type)...)
		case name == "":
			out = append(out, f.Name)
		default:
			out = append(out, name)
		}
	}
	return out
}
