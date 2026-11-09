package store

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// awkwardText is text that makes the encoder pick each scalar style:
// plain, single- and double-quoted, and literal blocks with every
// chomping and an indentation indicator.
var awkwardText = []string{
	"", "plain", "a: b", "x #y", "null", "~", "123", "-5", "yes", " lead", "trail ", "tab\there",
	"quote's", `dq"x`, `back\slash`, "#hash", "key:", "- dash", "[flow]", "{flow}", "&anchor", "*alias", "!tag",
	"é unicode ✓", "\x01ctl", "a\r\nb\n", "\u0085next\u2028line", "line1\nline2", "line1\n", "line1\n\n", "ends\n\n\n",
	"\nleading break", "  indented\nnext\n", "multi\nline with trailing \n", "x\n  \ny\n", strings.Repeat("word ", 40) + "end",
}

// awkwardFiles are files as the store writes them, holding awkwardText in
// every kind of field.
func awkwardFiles(t testing.TB) [][]byte {
	t.Helper()
	at := time.Date(2026, 11, 9, 13, 0, 0, 0, time.UTC)
	var files [][]byte
	for i, s := range awkwardText {
		tk := ticketFile{
			Title: s, State: "Ready", Spec: i + 1, Labels: []string{strings.ReplaceAll(s, "\n", " "), "ui"}, Tags: map[string]string{"k": s},
			BlockedBy: []int{1, 20}, Replacements: []int{}, Link: &linkFile{Integration: "github", ID: s, ExternalState: "open", Detached: i%2 == 0},
			Created: formatTime(at), Updated: formatTime(at.Add(time.Duration(i) * time.Millisecond)), Description: s, Branch: s,
			Claims:         []claimFile{{Number: 1, ID: "C1", SessionID: s, WorkingDir: s, Started: formatTime(at), Ended: formatTime(at), EndedBy: "finish"}},
			Rounds:         []roundFile{{Number: 1, Claim: 1, Posted: formatTime(at), Questions: []questionFile{{Number: 1, Title: s, Body: s, Recommended: s, Status: "answered", Answer: s, Closed: formatTime(at)}}}},
			FailureReports: []failureReportFile{{Number: 1, Claim: 1, Actor: "agent", Time: formatTime(at), WhatBroke: s, Why: s, WhatToFix: s, WhereWorkStands: s}},
			Comments:       []commentFile{{Number: 1, Actor: "agent", Time: formatTime(at), Body: s}, {Number: 2, Actor: "human", Time: formatTime(at), Claim: 1, Body: "ok"}},
			Moves:          []moveFile{{From: "Draft", To: "Ready", Actor: "human", Time: formatTime(at), Via: s}},
			Edits:          []editFile{{Number: 1, Actor: "human", Time: formatTime(at), Previous: map[string]string{"title": s, "description": s}}},
		}
		pf := projectFile{Key: "acme", DisplayName: s, Created: formatTime(at), Mode: "committed", Landing: &landingFile{Method: "merge", Push: i%2 == 1, Remote: s}}
		sp := specFront{ID: i + 1, Title: s, Labels: []string{}, Tags: map[string]string{}, Created: formatTime(at), Updated: formatTime(at), Edits: []editFile{}}
		for _, v := range []any{tk, pf, sp} {
			// yaml.v3's emitter can't write some of these (a leading
			// space in a nested literal block): the write fails, so
			// there's no such file to load.
			if data, err := encodeYAML(v); err == nil {
				files = append(files, data)
			}
		}
	}
	return files
}

// addAwkwardFiles seeds a fuzz target with awkwardFiles.
func addAwkwardFiles(f *testing.F) {
	for _, data := range awkwardFiles(f) {
		f.Add(data)
	}
}

// The store's own files are the ones startup reads, so every one of
// them must take the fast path: falling back to yaml.v3 is several
// times slower (code-central-205).
func TestTheStoresOwnFilesTakeTheFastPath(t *testing.T) {
	for _, data := range awkwardFiles(t) {
		got, ok := parseBlockYAML(data)
		if !ok {
			t.Fatalf("the fast path refused a file the store wrote:\n%s", data)
		}
		var want yaml.Node
		if err := yaml.Unmarshal(data, &want); err != nil {
			t.Fatal(err)
		}
		if diff := sameTree(got, &want, ""); diff != "" {
			t.Fatalf("%s\nin:\n%s", diff, data)
		}
	}
}

// Hand edits outside what the store writes fall back to yaml.v3, which
// still reads them.
func TestHandEditsOutsideTheStoresOwnYAMLFallBack(t *testing.T) {
	for _, data := range handEdits {
		if _, ok := parseBlockYAML([]byte(data)); ok {
			t.Errorf("the fast path took %q", data)
		}
	}
}

var handEdits = []string{
	"title: T # a comment\n",
	"# a comment\ntitle: T\n",
	"labels: [a, b]\n",
	"tags: {a: b}\n",
	"labels:\n- a\n- b\n",
	"title: &a T\nstate: *a\n",
	"title: !!str 5\n",
	"title: <<\n",
	"description: >\n  folded\n  text\n",
	"title: one\n  two\n",
	"title: \"one\n  two\"\n",
	"title: T\r\nstate: Ready\r\n",
	"title:\tT\n",
	"title: T",
	"---\ntitle: T\n",
	"\ufefftitle: T\n",
	"\"title\": T\n",
	"? title\n: T\n",
	"",
}

// FuzzBlockYAMLMatchesYAMLv3 checks the fast path against yaml.v3: what
// it accepts, yaml.v3 parses to the same tree.
func FuzzBlockYAMLMatchesYAMLv3(f *testing.F) {
	addAwkwardFiles(f)
	for _, data := range handEdits {
		f.Add([]byte(data))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		got, ok := parseBlockYAML(data)
		if !ok {
			return
		}
		var want yaml.Node
		if err := yaml.Unmarshal(data, &want); err != nil {
			t.Fatalf("the fast path took %q, which yaml.v3 refuses: %v", data, err)
		}
		if diff := sameTree(got, &want, ""); diff != "" {
			t.Fatalf("%s\nin %q", diff, data)
		}
	})
}

// FuzzScalarsDecodeAsYAMLv3Does checks decodeLeaf against yaml.v3's own
// Node.Decode, for every node of a document and every leaf type the
// store's files use.
func FuzzScalarsDecodeAsYAMLv3Does(f *testing.F) {
	addAwkwardFiles(f)
	for _, s := range []string{"n: 0\n", "n: -0\n", "n: 007\n", "n: 0x1F\n", "n: 1_000\n", "n: 9223372036854775808\n", "n: 1e3\n", "n: 2.0\n", "n: !!str 5\n", "n:\n", "n: ~\n", "n: Null\n", "m: {}\n", "m:\n  a: 1\n  a: 2\n", "m:\n  ~: x\n", "s: []\n", "s:\n  - 1\n  - ~\n", "b: true\n", "b: True\n"} {
		f.Add([]byte(s))
	}
	types := []reflect.Type{reflect.TypeFor[string](), reflect.TypeFor[int](), reflect.TypeFor[bool](), reflect.TypeFor[[]string](), reflect.TypeFor[[]int](), reflect.TypeFor[map[string]string]()}
	f.Fuzz(func(t *testing.T, data []byte) {
		var doc yaml.Node
		if yaml.Unmarshal(data, &doc) != nil {
			return
		}
		var walk func(n *yaml.Node)
		walk = func(n *yaml.Node) {
			for _, typ := range types {
				got := reflect.New(typ).Elem()
				if !decodeLeaf(n, got) {
					continue
				}
				want := reflect.New(typ)
				if err := n.Decode(want.Interface()); err != nil {
					t.Fatalf("decodeLeaf took %s %q into %v, which yaml.v3 refuses: %v", n.Tag, n.Value, typ, err)
				}
				if !reflect.DeepEqual(got.Interface(), want.Elem().Interface()) {
					t.Fatalf("%s %q into %v: decodeLeaf gave %#v, yaml.v3 %#v", n.Tag, n.Value, typ, got.Interface(), want.Elem().Interface())
				}
			}
			for _, c := range n.Content {
				walk(c)
			}
		}
		walk(&doc)
	})
}

// sameTree says how got differs from yaml.v3's want, or "" if it
// doesn't. A plain scalar's tag isn't compared: yaml.v3 resolves it
// from the value either way. Lines are, since a wrong type's error
// names one; columns aren't, since no message does.
func sameTree(got, want *yaml.Node, path string) string {
	switch {
	case got.Kind != want.Kind:
		return fmt.Sprintf("%s: kind %v, want %v", path, got.Kind, want.Kind)
	case got.Style != want.Style:
		return fmt.Sprintf("%s: style %v, want %v", path, got.Style, want.Style)
	case got.Line != want.Line:
		return fmt.Sprintf("%s: line %d, want %d", path, got.Line, want.Line)
	case got.Value != want.Value:
		return fmt.Sprintf("%s: value %q, want %q", path, got.Value, want.Value)
	case got.Tag != want.Tag && !(got.Kind == yaml.ScalarNode && got.Style == 0):
		return fmt.Sprintf("%s: tag %q, want %q", path, got.Tag, want.Tag)
	case len(got.Content) != len(want.Content):
		return fmt.Sprintf("%s: %d children, want %d", path, len(got.Content), len(want.Content))
	}
	for i := range got.Content {
		if d := sameTree(got.Content[i], want.Content[i], fmt.Sprintf("%s/%d", path, i)); d != "" {
			return d
		}
	}
	return ""
}

// A wrong type's error names its line on the fast path too.
func TestAWrongTypeNamesItsLine(t *testing.T) {
	var f ticketFile
	field, err := decodeStrict([]byte("title: T\nstate: Ready\nspec: abc\n"), &f)
	if field != "spec" || err == nil || !strings.Contains(err.Error(), "line 3:") {
		t.Fatalf("decodeStrict = %q, %v; want spec, an error naming line 3", field, err)
	}
}
