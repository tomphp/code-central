package domain

import (
	"slices"
	"strings"
)

// StagePrefix is the reserved Label prefix that marks a Stage Ticket.
const StagePrefix = "stage:"

// StageName is a Stage's name, its Label's suffix.
type StageName string

// The Stages' names.
const (
	StageGrilling StageName = "grilling"
	StageSpec     StageName = "spec"
	StageTickets  StageName = "tickets"
)

// Stages are the three Stages in order.
func Stages() []StageName { return []StageName{StageGrilling, StageSpec, StageTickets} }

// StageLabel is the Label that marks a Ticket as the named Stage.
func StageLabel(stage StageName) string { return StagePrefix + string(stage) }

// StageOf is the Stage a Ticket's Labels mark it as, or "" for an
// implementation Ticket.
func StageOf(labels []string) StageName {
	for _, l := range labels {
		if name, ok := strings.CutPrefix(l, StagePrefix); ok && slices.Contains(Stages(), StageName(name)) {
			return StageName(name)
		}
	}
	return ""
}
