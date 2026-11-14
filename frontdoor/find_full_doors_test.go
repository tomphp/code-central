package frontdoor_test

import (
	"reflect"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
)

// find_tickets with full (Spec code-central-568) gives each door's
// get_ticket view of every Ticket the rows would list.
func TestFindTicketsFullReturnsWhatGetTicketReturnsOnEveryDoor(t *testing.T) {
	srv := newServer(t, readyTicket, func(t *testing.T, srv *codecentraltest.Server) { ticketInReview(t, srv) })
	s := connect(t, srv, "acme")
	ids := []string{"acme-2", "acme-3", "acme-4"}

	doors := map[string]struct {
		find map[string]any
		get  func(id string) map[string]any
	}{
		"http": {
			find: httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets?full=true&state=Draft&state=Ready&state=In+Review", ""}),
			get:  func(id string) map[string]any { return httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/" + id, ""}) },
		},
		"mcp": {
			find: mcpOK(t, s, "find_tickets", map[string]any{"full": true, "state": []string{"Draft", "Ready", "In Review"}}),
			get:  func(id string) map[string]any { return mcpOK(t, s, "get_ticket", map[string]any{"ticket": id}) },
		},
		"cli": {
			find: cliOK(t, srv, []string{"find-tickets", "--full", "--state", "Draft", "--state", "Ready", "--state", "In Review"}),
			get:  func(id string) map[string]any { return cliOK(t, srv, []string{"get-ticket", id}) },
		},
	}
	for door, d := range doors {
		rows, _ := d.find["rows"].([]any)
		if len(rows) != len(ids) {
			t.Errorf("%s: %d Tickets, want %d: %v", door, len(rows), len(ids), d.find)
			continue
		}
		for i, id := range ids {
			if got, want := rows[i], any(d.get(id)); !reflect.DeepEqual(got, want) {
				t.Errorf("%s: find_tickets full %s = %v, want get_ticket's %v", door, id, got, want)
			}
		}
	}
}

func TestFindTicketsFullIsFilteredAsTheRowsAre(t *testing.T) {
	srv := newServer(t, readyTicket)
	s := connect(t, srv, "acme")
	out := mcpOK(t, s, "find_tickets", map[string]any{"full": true, "claimable": true})
	rows, _ := out["rows"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["id"] != "acme-3" || rows[1].(map[string]any)["id"] != "acme-4" {
		t.Fatalf("claimable full Tickets %v, want acme-3 and acme-4", out["rows"])
	}
	// A row's spec is an ID; a Ticket's is the Spec's ID and title.
	if _, ok := rows[0].(map[string]any)["spec"].(map[string]any); !ok {
		t.Fatalf("%v is a row, want the whole Ticket", rows[0])
	}
}
