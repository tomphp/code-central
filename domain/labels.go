package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// StatusLabelPrefix starts every status label, so a Watcher knows which
// of an issue's labels, or a work item's tags, are Code Central's.
const StatusLabelPrefix = "code-central:"

// The status labels an issue carries, one at a time, and the tags of the
// same names a work item carries.
const (
	// LabelDraft is on an Incoming Issue, a Draft or Ready Ticket on an
	// inbox Spec that no triage action has taken off it.
	LabelDraft = StatusLabelPrefix + "draft"
	// LabelRequirement is on an issue To Requirement made a Requirement
	// of, while the Requirement's Stages aren't all Done or Cancelled.
	LabelRequirement = StatusLabelPrefix + "requirement"
	// LabelBuilding is on it once the Stages are all Done or Cancelled, until the
	// Spec is Finished.
	LabelBuilding = StatusLabelPrefix + "building"
	// LabelDone is on it once the Spec is Finished with at least one
	// implementation Ticket Done.
	LabelDone = StatusLabelPrefix + "done"
	// LabelWontFix is on an issue the human chose Won't fix for, or whose
	// Requirement's Spec is Finished with no implementation Ticket Done.
	LabelWontFix = StatusLabelPrefix + "wont-fix"
)

// IssueMarker is the hidden HTML comment in the comment key posted for
// target, so the Issue Watcher finds it on the issue before posting it
// again after a crash.
func IssueMarker(target, key string) string {
	return fmt.Sprintf("<!-- code-central %s %s -->", target, key)
}

// OpaqueMarker is IssueMarker as Hide Code Central writes it: an HTML
// comment holding only the first 12 hex digits of the SHA-256 of target
// and key joined by a space, so it names neither Code Central nor the
// item.
func OpaqueMarker(target, key string) string {
	sum := sha256.Sum256([]byte(target + " " + key))
	return "<!-- " + hex.EncodeToString(sum[:])[:12] + " -->"
}

// WayfinderPrefix is the reserved Label prefix of the Wayfinder Labels.
const WayfinderPrefix = "wayfinder:"

// MapLabel marks a Spec as a Map.
const MapLabel = WayfinderPrefix + "map"

// PrototypeLabel marks a Ticket as a Prototype.
const PrototypeLabel = "prototype"

// RetroLabel marks a Ticket as a Retro (ADR 0061).
const RetroLabel = "retro"

// LongLivedLabel marks a Spec as a Long-lived Spec, one that never
// finishes, such as a CI Rule's Spec or an Issues Spec.
const LongLivedLabel = "long-lived"

// AutoAcceptLabel is the reserved Label that, on a Ticket or its Spec,
// turns every Auto-accept kind on for the Ticket. Only the human adds or
// removes it.
const AutoAcceptLabel = "auto-accept"

// GuidedTag is the Tag that marks a Map as guided when its value is
// GuidedOn; a missing Tag or any other value means off.
const (
	GuidedTag = "guided"
	GuidedOn  = "on"
)
