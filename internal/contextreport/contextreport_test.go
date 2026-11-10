package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The fixture logs in testdata/projects are for a repository at /r/acme:
// an implementation session of acme-2 with a review subagent and another
// subagent, a grilling session of acme-3 that asks a Round, and three
// sessions the report leaves out (interactive, after the window, and one
// of the repository /r/acme-other, whose folder starts the same).
const fixtures = "testdata/projects"

func window(t *testing.T) Window {
	t.Helper()
	until, err := time.Parse(time.RFC3339, "2026-01-03T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	return Window{Until: until}
}

// load reads the fixture sessions in the window.
func load(t *testing.T) []Session {
	t.Helper()
	sessions, err := Load(fixtures, "/r/acme", window(t))
	if err != nil {
		t.Fatal(err)
	}
	return sessions
}

func build(t *testing.T) Report {
	t.Helper()
	return Build(load(t), 3)
}

func TestLoadReadsTheRepositorysFactorySessionsInTheWindow(t *testing.T) {
	var got []string
	for _, s := range load(t) {
		got = append(got, s.Ticket+" "+s.Kind+" "+s.ID)
	}
	want := []string{
		"acme-2 implementation s1",
		"acme-2 review agent-a1",
		"acme-2 subagent agent-a2",
		"acme-3 grilling s2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sessions = %q, want %q", got, want)
	}
}

func TestLoadReadsTheTicketAndStageFromTheSystemPromptTheFactoryAdds(t *testing.T) {
	since, _ := time.Parse(time.RFC3339, "2026-01-04T00:00:00Z")
	sessions, err := Load(fixtures, "/r/acme", Window{Since: since, Until: since.Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("loaded %d sessions, want the one of acme-5", len(sessions))
	}
	s := sessions[0]
	if got := s.Ticket + " " + s.Kind + " " + s.ID; got != "acme-5 spec s5" {
		t.Errorf("session = %q, want %q", got, "acme-5 spec s5")
	}
	// The skill's command and its expansion, then the Factory's part of
	// the first system prompt; not the rest of it, nor a later one.
	command := "<command-name>/to-spec</command-name>\n<command-args>acme-5</command-args>"
	skill := "Turn the conversation into a Spec.\n\nARGUMENTS: acme-5"
	factory := "Ticket: acme-5\nClaim: C5\n\nYou are running under code-central-factory.\n\nThe Stage Guide for spec follows: what this Ticket must produce and how to finish it.\n\n# Stage Guide: spec"
	if want := len(command) + len(skill) + len(factory); s.FirstTurn != want {
		t.Errorf("first turn = %dB, want %dB", s.FirstTurn, want)
	}
}

func TestLoadMeasuresTheFirstTurnOfASessionWhosePromptHoldsTheStageGuide(t *testing.T) {
	var got []int
	for _, s := range load(t) {
		got = append(got, s.FirstTurn)
	}
	if want := []int{201, 38, 15, 183}; !reflect.DeepEqual(got, want) {
		t.Errorf("first turns = %v, want %v", got, want)
	}
}

func TestLoadMeasuresEachToolResult(t *testing.T) {
	var got []string
	for _, c := range load(t)[0].Calls {
		got = append(got, c.Tool+" "+strconv.Itoa(c.Bytes))
	}
	want := []string{
		"mcp__code-central__get_ticket 40",
		"mcp__code-central__get_spec 20",
		"mcp__code-central__get_spec 300",
		"Bash 500",
		"ToolSearch 0",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

func TestBuildTotalsToolResultsPerKindToolAndOperation(t *testing.T) {
	r := build(t)
	want := []Kind{
		{
			Name: "grilling", Sessions: 1, Bytes: 310,
			Tools:      []Figure{{Name: "Code Central", Calls: 2, Bytes: 310}},
			Operations: []Figure{{Name: "get_spec", Calls: 1, Bytes: 300}, {Name: "post_round", Calls: 1, Bytes: 10}},
			Rounds:     1, Questions: 2, FirstTurn: 183,
		},
		{
			Name: "implementation", Sessions: 1, Bytes: 860,
			Tools:      []Figure{{Name: "Bash", Calls: 1, Bytes: 500}, {Name: "Code Central", Calls: 3, Bytes: 360}, {Name: "ToolSearch", Calls: 1, Bytes: 0}},
			Operations: []Figure{{Name: "get_spec", Calls: 2, Bytes: 320}, {Name: "get_ticket", Calls: 1, Bytes: 40}},
			WiderReads: 1, FirstTurn: 201,
		},
		{
			Name: "review", Sessions: 1, Bytes: 140,
			Tools:      []Figure{{Name: "Bash", Calls: 1, Bytes: 100}, {Name: "Code Central", Calls: 1, Bytes: 40}},
			Operations: []Figure{{Name: "get_ticket", Calls: 1, Bytes: 40}},
			FirstTurn:  38,
		},
		{
			Name: "subagent", Sessions: 1, Bytes: 60,
			Tools:     []Figure{{Name: "Read", Calls: 1, Bytes: 60}},
			FirstTurn: 15,
		},
	}
	if !reflect.DeepEqual(r.Kinds, want) {
		t.Errorf("kinds =\n%+v\nwant\n%+v", r.Kinds, want)
	}
	if r.Sessions != 4 {
		t.Errorf("sessions = %d, want 4", r.Sessions)
	}
}

func TestBuildListsTheLargestToolResultsWithAShortInput(t *testing.T) {
	r := build(t)
	want := []Result{
		{Kind: "implementation", Ticket: "acme-2", Tool: "Bash", Input: "cat store/file.go", Bytes: 500},
		{Kind: "implementation", Ticket: "acme-2", Tool: "mcp__code-central__get_spec", Input: `{"spec":"1"}`, Bytes: 300},
		{Kind: "grilling", Ticket: "acme-3", Tool: "mcp__code-central__get_spec", Input: `{"spec":"acme-1"}`, Bytes: 300},
	}
	if !reflect.DeepEqual(r.Largest, want) {
		t.Errorf("largest =\n%+v\nwant\n%+v", r.Largest, want)
	}
}

func TestBuildListsSpecsAndTicketsReadMoreThanOnce(t *testing.T) {
	r := build(t)
	want := []ReRead{
		{ID: "acme-1", Reads: 3, Sessions: 2, Readers: []string{"acme-2 implementation", "acme-3 grilling"}},
		{ID: "acme-2", Reads: 2, Sessions: 2, Readers: []string{"acme-2 implementation", "acme-2 review"}},
	}
	if !reflect.DeepEqual(r.ReReads, want) {
		t.Errorf("re-reads =\n%+v\nwant\n%+v", r.ReReads, want)
	}
	if r.WithinSession != 1 || r.AcrossSessions != 2 {
		t.Errorf("within, across = %d, %d; want 1, 2", r.WithinSession, r.AcrossSessions)
	}
}

func TestBuildFindsAReadCalledAgainForMore(t *testing.T) {
	r := build(t)
	want := []WiderRead{{
		Kind: "implementation", Ticket: "acme-2", Operation: "get_spec", ID: "acme-1",
		From: `{"sections":["Testing Decisions"],"spec":"acme-1"}`, To: `{"spec":"1"}`,
	}}
	if !reflect.DeepEqual(r.WiderReads, want) {
		t.Errorf("wider reads =\n%+v\nwant\n%+v", r.WiderReads, want)
	}
}

func TestOnlyReadsForMoreThanBeforeAreWiderReads(t *testing.T) {
	read := func(op string, input map[string]any) Call {
		return Call{Tool: codeCentral + op, Input: input}
	}
	spec := func(input map[string]any) Call { return read("get_spec", input) }
	sessions := []Session{{Ticket: "acme-2", Kind: "implementation", Calls: []Call{
		// Not wider: the whole Spec covers any part or section of it, and
		// one section then the tickets asks for no more of the body.
		spec(map[string]any{"spec": "acme-1"}),
		spec(map[string]any{"spec": "acme-1", "sections": []any{"Goal"}}),
		spec(map[string]any{"spec": "acme-1", "parts": []any{"tickets"}}),
		spec(map[string]any{"spec": "acme-3", "sections": []any{"Goal"}}),
		spec(map[string]any{"spec": "acme-3", "parts": []any{"tickets"}}),
		// Wider: the body after only the tickets.
		spec(map[string]any{"spec": "acme-9", "parts": []any{"tickets"}}),
		spec(map[string]any{"spec": "acme-9", "parts": []any{"body"}}),
		// Wider: a part of a Ticket its default leaves out, and full; the
		// comments it already had are not.
		read("get_ticket", map[string]any{"ticket": "acme-2"}),
		read("get_ticket", map[string]any{"ticket": "acme-2", "parts": []any{"comments"}}),
		read("get_ticket", map[string]any{"ticket": "acme-2", "parts": []any{"rounds"}}),
		read("get_ticket", map[string]any{"ticket": "acme-2", "full": true}),
	}}}
	var got []string
	for _, w := range Build(sessions, 10).WiderReads {
		got = append(got, w.Operation+" "+w.ID+" "+w.To)
	}
	want := []string{
		`get_spec acme-9 {"parts":["body"],"spec":"acme-9"}`,
		`get_ticket acme-2 {"parts":["rounds"],"ticket":"acme-2"}`,
		`get_ticket acme-2 {"full":true,"ticket":"acme-2"}`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wider reads = %q, want %q", got, want)
	}
}

func TestShortInputKeepsAFilesEnd(t *testing.T) {
	long := "/r/acme/" + strings.Repeat("deep/", 30) + "file.go"
	got := shortInput(Call{Tool: "Read", Input: map[string]any{"file_path": long}}, "")
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "deep/file.go") || len([]rune(got)) != inputLength {
		t.Errorf("short input = %q, want its last %d characters after …", got, inputLength-1)
	}
	got = shortInput(Call{Tool: "Bash", Input: map[string]any{"command": "cat " + long}}, "")
	if !strings.HasPrefix(got, "cat /r/acme/") || !strings.HasSuffix(got, "…") {
		t.Errorf("short input = %q, want its first %d characters then …", got, inputLength-1)
	}
}

func TestRunSavesABaselineAndPrintsTheChangeAgainstIt(t *testing.T) {
	file := filepath.Join(t.TempDir(), "baseline.json")
	args := []string{"-projects", fixtures, "-repo", "/r/acme", "-until", "2026-01-03T00:00:00Z", "-top", "3"}
	if err := run(append(args, "-save", file), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), strings.Repeat("x", 10)) {
		t.Error("the baseline holds tool result content, want figures only")
	}

	// Against itself every figure is unchanged.
	var out bytes.Buffer
	if err := run(append(args, "-baseline", file), &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"implementation", "860B", "Code Central", "get_spec", "cat store/file.go", "acme-1", "Testing Decisions", "First turn per session", "201B", "unchanged"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report doesn't contain %q:\n%s", want, out.String())
		}
	}

	// Without the grilling session, its kind has no sessions to compare,
	// and the re-reads across sessions fall by a third.
	out.Reset()
	if err := run([]string{"-projects", fixtures, "-repo", "/r/acme", "-until", "2026-01-02T10:59:00Z", "-top", "3", "-baseline", file}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"no sessions (was 310B per session)", "-33% (was 0.5)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report doesn't contain %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "post_round") {
		t.Errorf("report lists the operations of a kind with no sessions:\n%s", out.String())
	}
}

func TestCountKeepsSmallFiguresVisible(t *testing.T) {
	for n, want := range map[float64]string{0: "0", 0.0049: "0.0049", 1.27: "1.27", 28.71: "28.7", 1234.6: "1235"} {
		if got := count(n); got != want {
			t.Errorf("count(%v) = %q, want %q", n, got, want)
		}
	}
}

func TestChangeIsAPercentageOfTheBaselinesFigure(t *testing.T) {
	for _, c := range []struct {
		now, was float64
		want     string
	}{
		{100, 100, "unchanged"},
		{50, 100, "-50% (was 100B)"},
		{150, 100, "+50% (was 100B)"},
		{10, 0, "new"},
		{0, 0, "unchanged"},
	} {
		if got := change(c.now, c.was, size); got != c.want {
			t.Errorf("change(%v, %v) = %q, want %q", c.now, c.was, got, c.want)
		}
	}
}
