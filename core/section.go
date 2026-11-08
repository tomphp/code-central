package core

import (
	"regexp"
	"strings"
)

// Sections of a Markdown body (ticket 134): its outline, and the cut of
// the Sections a reader names. It knows nothing about Specs. A heading is
// an ATX heading outside fenced code; setext headings aren't recognised.

// Heading is one heading of a body's outline.
type Heading struct {
	Text  string `json:"text"`
	Level int    `json:"level"`
}

// heading is a Heading and the line it starts on.
type heading struct {
	Heading
	line int
}

var (
	atx     = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?[ \t]*$`)
	closing = regexp.MustCompile(`(?:^|[ \t]+)#+$`)
	fence   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
)

// Outline is every heading of body, in order.
func Outline(body string) []Heading {
	out := []Heading{}
	for _, h := range headings(strings.Split(body, "\n")) {
		out = append(out, h.Heading)
	}
	return out
}

// Sections cuts from body the Sections whose headings match names, in
// document order and joined by a blank line, or returns the names that
// matched no heading. A Section runs from its heading to the next heading
// at the same level or higher, so it includes its subsections; one inside
// another matched Section is not repeated.
func Sections(body string, names []string) (string, []string) {
	lines := strings.Split(body, "\n")
	hs := headings(lines)
	want := map[string]bool{}
	for _, n := range names {
		want[headingKey(n)] = true
	}
	found := map[string]bool{}
	var cuts []string
	covered := 0
	for i, h := range hs {
		key := strings.ToLower(h.Text)
		if !want[key] {
			continue
		}
		found[key] = true
		if h.line < covered {
			continue
		}
		covered = len(lines)
		for _, next := range hs[i+1:] {
			if next.Level <= h.Level {
				covered = next.line
				break
			}
		}
		cuts = append(cuts, strings.TrimRight(strings.Join(lines[h.line:covered], "\n"), " \t\n"))
	}
	var unmatched []string
	for _, n := range names {
		if !found[headingKey(n)] {
			unmatched = append(unmatched, n)
		}
	}
	if len(unmatched) > 0 {
		return "", unmatched
	}
	return strings.Join(cuts, "\n\n"), nil
}

// headings finds the ATX headings in lines, skipping fenced code. A fence
// closes on a line of the same character at least as long.
func headings(lines []string) []heading {
	var out []heading
	open := ""
	for i, l := range lines {
		if m := fence.FindStringSubmatch(l); m != nil {
			switch {
			case open == "":
				open = m[1]
			case m[1][0] == open[0] && len(m[1]) >= len(open) && strings.TrimSpace(l[strings.Index(l, m[1])+len(m[1]):]) == "":
				open = ""
			}
			continue
		}
		if open != "" {
			continue
		}
		if m := atx.FindStringSubmatch(l); m != nil {
			out = append(out, heading{Heading{Text: headingText(m[2]), Level: len(m[1])}, i})
		}
	}
	return out
}

// headingText is a heading's content without its closing # marks.
func headingText(s string) string {
	return strings.TrimSpace(closing.ReplaceAllString(s, ""))
}

// headingKey is how a name matches a heading: trimmed, without leading or
// closing # marks, case-insensitively.
func headingKey(name string) string {
	name = strings.TrimSpace(name)
	if m := atx.FindStringSubmatch(name); m != nil {
		name = m[2]
	}
	return strings.ToLower(headingText(name))
}
