package store

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/domain"
)

// A record written as a JSON document reads back the same as its YAML
// file does, keys and all, so both Store Kinds hold one record shape.
func TestAJSONDocumentReadsBackAsItsFileDoes(t *testing.T) {
	at := time.Date(2026, 11, 7, 9, 30, 0, 123456789, time.UTC)
	yes := false
	tk := domain.Ticket{
		Number: 3, Title: "Add \"quotes\"\nand lines", State: domain.InProgress, Spec: 1, Labels: []string{"a", "b"}, Tags: map[string]string{"k": "v"},
		BlockedBy: []int{2}, Created: at, Updated: at, Description: "## Body\n\n- one\n", Branch: "acme-3",
		Link:          domain.Link{Integration: "github", ID: "9", URL: "https://example.test/9", ExternalState: "open"},
		PushedCommits: []domain.PushedCommit{{Commit: "abc", Branch: "acme-3", Pushed: at}},
		Claims:        []domain.Claim{{Number: 1, ID: "C1", SessionID: "s", Started: at, Model: "opus"}},
		Rounds:        []domain.Round{{Number: 1, Claim: 1, Posted: at, Questions: []domain.Question{{Number: 1, Title: "Q", Body: "b", Status: domain.QuestionOpen}}}},
		Comments:      []domain.Comment{{Number: 1, Actor: "human", Time: at, Body: "hi"}},
		Moves:         []domain.Move{{From: domain.Ready, To: domain.InProgress, Actor: "agent", Time: at, Claim: 1}},
		Edits:         []domain.Edit{{Actor: "human", Time: at, Previous: map[string]string{"title": "old"}}},
	}
	doc, err := jsonDoc(ticketFileOf(tk))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"title":"Add \"quotes\"\nand lines"`, `"spec":1`, `"land_per_spec"`, `"claims":[{"number":1`} {
		if key == `"land_per_spec"` {
			continue
		}
		if !strings.Contains(string(doc), key) {
			t.Errorf("document lacks %s:\n%s", key, doc)
		}
	}
	var f ticketFile
	if field, err := fromJSONDoc(doc, &f); err != nil {
		t.Fatalf("decode: field %s: %v", field, err)
	}
	got, lerr := ticketOf(f, 3)
	if lerr != nil {
		t.Fatal(lerr)
	}
	yamlBytes, _ := encodeYAML(ticketFileOf(tk))
	want, lerr := decodeTicket(yamlBytes, 3)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON read back\n%+v\nYAML read back\n%+v", got, want)
	}

	p := domain.Project{Key: "acme", DisplayName: "Acme", Created: at, Mode: "committed", Landing: domain.Landing{Method: domain.LandingMerge, Remote: "origin", LandPerSpec: yes}}
	doc, err = jsonDoc(projectFileOf(p))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), `"land_per_spec":false`) {
		t.Errorf("project document lacks land_per_spec:\n%s", doc)
	}
	var pf projectFile
	if field, err := fromJSONDoc(doc, &pf); err != nil {
		t.Fatalf("decode project: field %s: %v", field, err)
	}
	gotP, lerr := projectOf(pf)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if !reflect.DeepEqual(gotP, p) {
		t.Fatalf("project read back %+v, want %+v", gotP, p)
	}
}

func TestAJSONDocumentWithAnUnknownKeyIsRefusedByName(t *testing.T) {
	var f ticketFile
	field, err := fromJSONDoc([]byte(`{"title":"t","claims":[{"id":"C1","colour":"red"}]}`), &f)
	if err == nil || field != "claims[0].colour" {
		t.Fatalf("got field %q, err %v", field, err)
	}
}
