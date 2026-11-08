package core_test

import (
	"reflect"
	"testing"

	"github.com/tomoram/code-central/core"
)

// Sections and the outline (ticket 134): table tests on plain Markdown.

const sectioned = `Preamble text.

# Title

Intro.

## Solution ##

Do it.

### Detail

Small print.

## Testing Decisions

` + "```" + `
# not a heading
` + "```" + `

Seams.

## Notes

First notes.

# Appendix

## notes

Second notes.`

func TestTheOutlineListsEveryHeadingWithItsLevel(t *testing.T) {
	want := []core.Heading{
		{Text: "Title", Level: 1}, {Text: "Solution", Level: 2}, {Text: "Detail", Level: 3},
		{Text: "Testing Decisions", Level: 2}, {Text: "Notes", Level: 2}, {Text: "Appendix", Level: 1}, {Text: "notes", Level: 2},
	}
	if got := core.Outline(sectioned); !reflect.DeepEqual(got, want) {
		t.Fatalf("outline %v, want %v", got, want)
	}
	cases := map[string]struct {
		body string
		want []core.Heading
	}{
		"no headings":           {"Just text.\nMore.", []core.Heading{}},
		"six levels, not seven": {"###### Six\n####### Seven", []core.Heading{{Text: "Six", Level: 6}}},
		"a # needs a space":     {"#hashtag\n# Real", []core.Heading{{Text: "Real", Level: 1}}},
		"up to three spaces":    {"   # Indented\n    # Code", []core.Heading{{Text: "Indented", Level: 1}}},
		"setext isn't":          {"Title\n=====\n\nSub\n---", []core.Heading{}},
		"closing marks":         {"## Both ends ##\n# C# #", []core.Heading{{Text: "Both ends", Level: 2}, {Text: "C#", Level: 1}}},
		"tilde fences":          {"~~~\n# no\n~~~\n# yes", []core.Heading{{Text: "yes", Level: 1}}},
		"a longer fence closes": {"````\n```\n# no\n````\n# yes", []core.Heading{{Text: "yes", Level: 1}}},
		"an empty heading":      {"#\n## Next", []core.Heading{{Text: "", Level: 1}, {Text: "Next", Level: 2}}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := core.Outline(c.body); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("outline %v, want %v", got, c.want)
			}
		})
	}
}

func TestSectionsCutTheNamedPartsOfABody(t *testing.T) {
	cases := map[string]struct {
		names []string
		want  string
	}{
		"a Section includes its subsections": {[]string{"Solution"}, "## Solution ##\n\nDo it.\n\n### Detail\n\nSmall print."},
		"names match case-insensitively, trimmed, ignoring # marks": {[]string{"  detail ", "## TESTING DECISIONS ##"},
			"### Detail\n\nSmall print.\n\n## Testing Decisions\n\n```\n# not a heading\n```\n\nSeams."},
		"matched Sections come back in document order": {[]string{"Notes", "Detail"}, "### Detail\n\nSmall print.\n\n## Notes\n\nFirst notes.\n\n## notes\n\nSecond notes."},
		"a nested match is not repeated":               {[]string{"Detail", "Solution"}, "## Solution ##\n\nDo it.\n\n### Detail\n\nSmall print."},
		"a top-level Section runs to the end":          {[]string{"Appendix"}, "# Appendix\n\n## notes\n\nSecond notes."},
		"the whole body after the preamble":            {[]string{"Title", "Appendix"}, sectioned[len("Preamble text.\n\n"):]},
		"a name given twice is cut once":               {[]string{"Detail", "detail"}, "### Detail\n\nSmall print."},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, unmatched := core.Sections(sectioned, c.names)
			if len(unmatched) != 0 {
				t.Fatalf("unmatched %v", unmatched)
			}
			if got != c.want {
				t.Fatalf("cut\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

func TestSectionsNameTheNamesThatMatchedNothing(t *testing.T) {
	cut, unmatched := core.Sections(sectioned, []string{"Solution", "Preamble text.", "not a heading", "Missing"})
	if !reflect.DeepEqual(unmatched, []string{"Preamble text.", "not a heading", "Missing"}) {
		t.Fatalf("unmatched %v: the preamble and fenced lines are in no Section", unmatched)
	}
	if cut != "" {
		t.Fatalf("cut %q alongside unmatched names", cut)
	}
}
