package domain

import "slices"

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
// lookups still find it, but no longer owns the item's definition. An
// outbound Link (Spec code-central-279) doesn't own it either: Code
// Central owns the item and only writes to the external item.
type Link struct {
	Integration   string
	ID            string
	URL           string
	ExternalState ExternalState
	Detached      bool
	Outbound      bool
	// Written are the keys of the comments the Integration has written
	// to the external item for it (Spec code-central-279), so none is
	// written twice.
	Written []string
}

// Linked reports whether the item has a Link at all, detached or not.
func (l Link) Linked() bool { return l.Integration != "" }

// Owned reports whether the Link still owns the item's definition: it is
// linked, not detached and not outbound.
func (l Link) Owned() bool { return l.Attached() && !l.Outbound }

// Attached reports whether the Link is linked and not detached, so the
// Integration still writes to the external item.
func (l Link) Attached() bool { return l.Linked() && !l.Detached }

// The Integrations' names, as a Link names them.
const (
	IntegrationGitHub      = "github"
	IntegrationAzureBoards = "azure-boards"
)

// The keys of the comments written to an issue.
const (
	// CommentContext is the key of the comment with the context the human
	// gave To Requirement.
	CommentContext = "context"
	// CommentWontFix is the key of the comment with the human's Won't fix
	// reason.
	CommentWontFix = "wont-fix"
	// CommentGrilling is the key of the comment with the grilling
	// Stage's resolution.
	CommentGrilling = "grilling"
	// CommentSpec is the key of the comment with the Spec's body as the
	// spec Stage left it.
	CommentSpec = "spec"
	// CommentLanded starts the key of the comment for each implementation
	// Ticket Done, such as landed:acme-12.
	CommentLanded = "landed:"
	// CommentClosed is the key of the comment listing what landed, once
	// the Spec is Finished.
	CommentClosed = "closed"
)

// The Written keys of the work item States set for each phase the linked
// work reached, nearest first (Spec code-central-1215).
const (
	StateKeyInProgress = "state:in-progress"
	StateKeyAccepted   = "state:accepted"
	StateKeyDone       = "state:done"
)
