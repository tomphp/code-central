package core

import (
	"slices"
	"strings"
)

// Wayfinder Labels (Spec 213, ADR 0016). A Map is a Spec labelled
// wayfinder:map, and its Tickets carry a Wayfinder Label for their type;
// a Prototype is a Ticket labelled prototype. Like stage:, Code Central
// reads them on the read side only: they choose a Ticket's Guide and
// change no workflow rule.

// WayfinderPrefix is the reserved Label prefix of the Wayfinder Labels.
const WayfinderPrefix = "wayfinder:"

// MapLabel marks a Spec as a Map.
const MapLabel = WayfinderPrefix + "map"

// PrototypeLabel marks a Ticket as a Prototype.
const PrototypeLabel = "prototype"

// WayfinderTypes are the Map's Ticket types, each named as its Label's
// suffix.
var WayfinderTypes = []string{"chart", "grilling", "research", "task"}

// wayfinderNames are every name the wayfinder: prefix allows.
var wayfinderNames = append([]string{"map"}, WayfinderTypes...)

// wayfinderLabelList is the valid Wayfinder Labels, as the error that
// rejects another one lists them.
var wayfinderLabelList = func() string {
	ls := make([]string, len(wayfinderNames))
	for i, n := range wayfinderNames {
		ls[i] = WayfinderPrefix + n
	}
	return strings.Join(ls[:len(ls)-1], ", ") + " and " + ls[len(ls)-1]
}()

// typeGuide is the guide a reserved type Label gives its Ticket, which
// carries at most one: a Stage, a Map Ticket type or prototype. ok is
// false for any other Label.
func typeGuide(l string) (guide string, ok bool) {
	if l == PrototypeLabel {
		return PrototypeLabel, true
	}
	if name, ok := strings.CutPrefix(l, WayfinderPrefix); ok && slices.Contains(WayfinderTypes, name) {
		return mapGuidePrefix + name, true
	}
	if name, ok := strings.CutPrefix(l, StagePrefix); ok && slices.Contains(Stages, name) {
		return name, true
	}
	return "", false
}

// isWayfinderType reports whether l gives a Ticket a Map Ticket type or
// makes it a Prototype.
func isWayfinderType(l string) bool {
	g, ok := typeGuide(l)
	return ok && (g == PrototypeLabel || strings.HasPrefix(g, mapGuidePrefix))
}

// isTypeLabel reports whether l is a reserved type Label.
func isTypeLabel(l string) bool {
	_, ok := typeGuide(l)
	return ok
}

// checkReservedNames rejects a reserved-prefix Label that names nothing:
// a stage:* Label that names no Stage, or a wayfinder:* one that names
// no Wayfinder type.
func checkReservedNames(field string, labels []string) error {
	if err := checkStageLabels(field, labels); err != nil {
		return err
	}
	for _, l := range labels {
		name, ok := strings.CutPrefix(l, WayfinderPrefix)
		if ok && !slices.Contains(wayfinderNames, name) {
			return invalid(field, "%q uses the reserved %q prefix; the only Wayfinder Labels are %s", l, WayfinderPrefix, wayfinderLabelList)
		}
	}
	return nil
}

// checkPlacement rejects a reserved Label on the wrong kind of record:
// wayfinder:map belongs on a Spec, and the Map Ticket types and prototype
// on a Ticket.
func checkPlacement(field string, labels []string, onSpec bool) error {
	for _, l := range labels {
		switch {
		case l == MapLabel && !onSpec:
			return invalid(field, "%q marks a Spec as a Map; a Ticket can't carry it", l)
		case onSpec && l == PrototypeLabel:
			return invalid(field, "%q marks a Ticket as a Prototype; a Spec can't carry it", l)
		case onSpec && l != MapLabel && strings.HasPrefix(l, WayfinderPrefix):
			return invalid(field, "%q marks a Map's Ticket; a Spec can only carry %q", l, MapLabel)
		}
	}
	return nil
}

// checkOneType rejects a Ticket's Labels that hold more than one reserved
// type Label.
func checkOneType(field string, labels []string) error {
	var types []string
	for _, l := range labels {
		if isTypeLabel(l) && !slices.Contains(types, l) {
			types = append(types, l)
		}
	}
	if len(types) > 1 {
		return invalid(field, "a Ticket carries at most one of the reserved type Labels (stage:*, wayfinder:* or prototype), not %s", strings.Join(types, " and "))
	}
	return nil
}

// checkTicketLabels checks the reserved Labels in labels, the whole list
// a new Ticket will carry: each names a reserved name, belongs on a
// Ticket, and there is at most one type.
func checkTicketLabels(field string, labels []string) error {
	if err := checkReservedNames(field, labels); err != nil {
		return err
	}
	if err := checkPlacement(field, labels, false); err != nil {
		return err
	}
	return checkOneType(field, labels)
}
