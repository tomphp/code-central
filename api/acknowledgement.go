package api

import (
	"encoding/json"
	"github.com/tomoram/code-central/domain"
	"time"
)

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
	State domain.State `json:"state,omitempty"`
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
