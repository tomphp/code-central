package api

import (
	"slices"
	"strings"

	"github.com/tomoram/code-central/domain"
)

// ImplementationStage names the Stage Guide for a Ticket with no
// reserved type Label.
const ImplementationStage = "implementation"

// MapGuidePrefix starts the Guide name of each Map Ticket type, which
// keeps the Map's grilling apart from the grilling Stage.
const MapGuidePrefix = "map-"

// TypeGuide is the guide a reserved type Label gives its Ticket, which
// carries at most one: a Stage, a Map Ticket type or prototype. ok is
// false for any other Label.
func TypeGuide(l string) (guide string, ok bool) {
	if l == domain.PrototypeLabel || l == domain.RetroLabel {
		return l, true
	}
	if name, ok := strings.CutPrefix(l, domain.WayfinderPrefix); ok && slices.Contains(WayfinderTypes(), NodeType(name)) {
		return MapGuidePrefix + name, true
	}
	if name, ok := strings.CutPrefix(l, domain.StagePrefix); ok && slices.Contains(domain.Stages(), domain.StageName(name)) {
		return name, true
	}
	return "", false
}

// GuideName names the guide for a Ticket with these Labels: its Stage,
// map-<type> for a Map Ticket, prototype for a Prototype, or
// ImplementationStage only when it has no reserved type Label.
func GuideName(labels []string) string {
	for _, l := range labels {
		if g, ok := TypeGuide(l); ok {
			return g
		}
	}
	return ImplementationStage
}
