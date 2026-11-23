package core

import (
	"context"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// Wayfinder Labels (Spec 213, ADR 0057). A Map is a Spec labelled
// wayfinder:map, and its Tickets carry a Wayfinder Label for their type;
// a Prototype is a Ticket labelled prototype. Like stage:, Code Central
// reads them on the read side only: they choose a Ticket's Guide and
// change no workflow rule.

// wayfinderNames are every name the wayfinder: prefix allows.
var wayfinderNames = func() []string {
	names := []string{"map"}
	for _, t := range api.WayfinderTypes() {
		names = append(names, string(t))
	}
	return names
}()

// wayfinderLabelList is the valid Wayfinder Labels, as the error that
// rejects another one lists them.
var wayfinderLabelList = func() string {
	ls := make([]string, len(wayfinderNames))
	for i, n := range wayfinderNames {
		ls[i] = domain.WayfinderPrefix + n
	}
	return strings.Join(ls[:len(ls)-1], ", ") + " and " + ls[len(ls)-1]
}()

// isWayfinderType reports whether l gives a Ticket a Map Ticket type or
// makes it a Prototype.
func isWayfinderType(l string) bool {
	g, ok := api.TypeGuide(l)
	return ok && (g == domain.PrototypeLabel || strings.HasPrefix(g, api.MapGuidePrefix))
}

// isTypeLabel reports whether l is a reserved type Label.
func isTypeLabel(l string) bool {
	_, ok := api.TypeGuide(l)
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
		name, ok := strings.CutPrefix(l, domain.WayfinderPrefix)
		if ok && !slices.Contains(wayfinderNames, name) {
			return api.InvalidField(field, "%q uses the reserved %q prefix; the only Wayfinder Labels are %s", l, domain.WayfinderPrefix, wayfinderLabelList)
		}
	}
	return nil
}

// checkHumanLabels rejects the auto-accept Label named by anyone but the
// human, who alone adds or removes it.
func checkHumanLabels(ctx context.Context, field string, labels []string) error {
	if slices.Contains(labels, domain.AutoAcceptLabel) && api.ActorOf(ctx) != api.Human {
		return api.InvalidField(field, "%q is reserved for the human, who alone adds or removes it", domain.AutoAcceptLabel)
	}
	return nil
}

// checkPlacement rejects a reserved Label on the wrong kind of record:
// wayfinder:map belongs on a Spec, and the Map Ticket types and prototype
// on a Ticket.
func checkPlacement(field string, labels []string, onSpec bool) error {
	for _, l := range labels {
		switch {
		case l == domain.MapLabel && !onSpec:
			return api.InvalidField(field, "%q marks a Spec as a Map; a Ticket can't carry it", l)
		case l == domain.LongLivedLabel && !onSpec:
			return api.InvalidField(field, "%q marks a Spec as long-lived; it goes on a Spec, not a Ticket", l)
		case onSpec && l == domain.PrototypeLabel:
			return api.InvalidField(field, "%q marks a Ticket as a Prototype; a Spec can't carry it", l)
		case onSpec && l == domain.RetroLabel:
			return api.InvalidField(field, "%q marks a Ticket as a Retro; a Spec can't carry it", l)
		case onSpec && l != domain.MapLabel && strings.HasPrefix(l, domain.WayfinderPrefix):
			return api.InvalidField(field, "%q marks a Map's Ticket; a Spec can only carry %q", l, domain.MapLabel)
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
		return api.InvalidField(field, "a Ticket carries at most one of the reserved type Labels (stage:*, wayfinder:*, prototype or retro), not %s", strings.Join(types, " and "))
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
