package store

import (
	"fmt"
	"slices"
)

// ExternalState is a linked item's status in its external system, as its
// Integration last saw it.
type ExternalState string

// The four external states.
const (
	ExternalOpen       ExternalState = "open"
	ExternalClosed     ExternalState = "closed"
	ExternalDeleted    ExternalState = "deleted"
	ExternalOutOfScope ExternalState = "out_of_scope"
)

// ExternalStates lists every external state.
var ExternalStates = []ExternalState{ExternalOpen, ExternalClosed, ExternalDeleted, ExternalOutOfScope}

// Valid reports whether s is one of the four external states.
func (s ExternalState) Valid() bool { return slices.Contains(ExternalStates, s) }

// Link records the external item a Spec or Ticket mirrors (ADR 0004). The
// zero Link means the item isn't linked. A detached Link is kept so
// lookups still find it, but no longer owns the item's definition.
type Link struct {
	Integration   string
	ID            string
	URL           string
	ExternalState ExternalState
	Detached      bool
	// Written are the keys of the comments the Integration has written
	// to the external item for it (Spec code-central-279), so none is
	// written twice.
	Written []string
}

// Linked reports whether the item has a Link at all, detached or not.
func (l Link) Linked() bool { return l.Integration != "" }

// Owned reports whether the Link still owns the item's definition: it is
// linked and not detached.
func (l Link) Owned() bool { return l.Linked() && !l.Detached }

type linkFile struct {
	Integration   string   `yaml:"integration"`
	ID            string   `yaml:"id"`
	URL           string   `yaml:"url,omitempty"`
	ExternalState string   `yaml:"external_state"`
	Detached      bool     `yaml:"detached,omitempty"`
	Written       []string `yaml:"written,omitempty"`
}

func encodeLink(l Link) *linkFile {
	if !l.Linked() {
		return nil
	}
	return &linkFile{Integration: l.Integration, ID: l.ID, URL: l.URL, ExternalState: string(l.ExternalState), Detached: l.Detached, Written: l.Written}
}

func decodeLink(f *linkFile) (Link, *LoadError) {
	if f == nil {
		return Link{}, nil
	}
	l := Link{Integration: f.Integration, ID: f.ID, URL: f.URL, ExternalState: ExternalState(f.ExternalState), Detached: f.Detached, Written: f.Written}
	switch {
	case l.Integration == "":
		return Link{}, &LoadError{Field: "link.integration", Msg: "required"}
	case l.ID == "":
		return Link{}, &LoadError{Field: "link.id", Msg: "required"}
	case !l.ExternalState.Valid():
		return Link{}, &LoadError{Field: "link.external_state", Msg: fmt.Sprintf(
			"unknown external state %q (open, closed, deleted or out_of_scope)", f.ExternalState)}
	}
	return l, nil
}
