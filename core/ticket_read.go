package core

import (
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
)

// parts is the set of parts the read returns.
func ticketReadParts(in api.GetTicketInput) (map[string]bool, error) {
	asked := in.Parts
	if len(asked) == 0 {
		asked = api.DefaultTicketParts()
	}
	return partSet(asked, api.TicketParts())
}

// partSet is the parts asked for as a set, each checked against all the
// parts there are.
func partSet(asked, all []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, p := range asked {
		if !slices.Contains(all, p) {
			return nil, invalid("parts", "unknown part %q (the parts are %s)", p, strings.Join(all, ", "))
		}
		out[p] = true
	}
	return out, nil
}

// defaultView is the Ticket as a read naming no parts returns it.
func defaultView(v api.TicketView) api.TicketView {
	parts := map[string]bool{}
	for _, p := range api.DefaultTicketParts() {
		parts[p] = true
	}
	return v.Only(parts).Brief()
}
