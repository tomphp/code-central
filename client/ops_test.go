package client

import (
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

func TestPathOfFillsAndEscapesTheHoles(t *testing.T) {
	got := pathOf("GET /api/p/{key}/tickets/{n}", "ac me", core.GetTicketInput{Ticket: "ac me-7"})
	if want := "GET /api/p/ac%20me/tickets/ac%20me-7"; got != want {
		t.Fatalf("path %q, want %q", got, want)
	}
	if got := pathOf("POST /api/p/{key}/add_labels", "acme", core.LabelsInput{Target: "7"}); got != "POST /api/p/acme/add_labels" {
		t.Fatalf("path %q: with no {n}, the body's ref stays in the body", got)
	}
}

func TestQueryOfEncodesByTheSameTagsTheHandlerDecodesWith(t *testing.T) {
	yes := true
	in := core.FindTicketsInput{
		State: []store.State{"Ready", "Draft"}, Title: "A", Labels: []string{"x"},
		Tags: map[string]string{"model": "fable", "effort": "high"}, Spec: "3", Claimable: &yes,
		Full: true, Parts: []string{"claims"},
	}
	got := queryOf(in).Encode()
	want := "claimable=true&full=true&label=x&part=claims&spec=3&state=Ready&state=Draft&tag=effort%3Dhigh&tag=model%3Dfable&title=A"
	if got != want {
		t.Fatalf("query\n%s\nwant\n%s", got, want)
	}
	no := false
	if got := queryOf(core.FindTicketsInput{Claimable: &no}).Encode(); got != "claimable=false" {
		t.Fatalf("query %q: a *bool encodes false, and zero values are left out", got)
	}
	if got := queryOf(core.GetSpecInput{Spec: "3", Parts: []string{"outline"}}).Encode(); got != "part=outline" {
		t.Fatalf("query %q: the path-bound field is not a parameter", got)
	}
}
