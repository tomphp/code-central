package core

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Sections of a Markdown body (ticket 134): its outline, and the cut of
// the Sections a reader names. It knows nothing about Specs. A heading is
// an ATX heading outside fenced code; setext headings aren't recognised.

// Heading is one heading of a body's outline.
type Heading struct {
	Text  string `json:"text"`
	Level int    `json:"level"`
	// Size is the bytes of the Section the heading opens, subsections
	// included, so a reader can weigh a Section before fetching it.
	Size int `json:"size"`
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

// Outline is every heading of body, in order, each with its Section's size.
func Outline(body string) []Heading {
	lines := strings.Split(body, "\n")
	hs := headings(lines)
	out := []Heading{}
	for i, h := range hs {
		end := len(lines)
		for _, next := range hs[i+1:] {
			if next.Level <= h.Level {
				end = next.line
				break
			}
		}
		h.Size = len(strings.TrimRight(strings.Join(lines[h.line:end], "\n"), " \t\r\n"))
		out = append(out, h.Heading)
	}
	return out
}

// Sections cuts from body the Sections whose headings match names (see
// matchHeadings), in document order and joined by a blank line, or returns
// the names that matched no heading. A Section runs from its heading to the next heading
// at the same level or higher, so it includes its subsections; one inside
// another matched Section is not repeated.
func Sections(body string, names []string) (string, []string) {
	lines := strings.Split(body, "\n")
	hs := headings(lines)
	want := map[int]bool{}
	var unmatched []string
	for _, n := range names {
		idx := matchHeadings(hs, headingKey(n))
		for _, i := range idx {
			want[i] = true
		}
		if len(idx) == 0 {
			unmatched = append(unmatched, n)
		}
	}
	if len(unmatched) > 0 {
		return "", unmatched
	}
	var cuts []string
	covered := 0
	for i, h := range hs {
		if !want[i] {
			continue
		}
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
		cuts = append(cuts, strings.TrimRight(strings.Join(lines[h.line:covered], "\n"), " \t\r\n"))
	}
	return strings.Join(cuts, "\n\n"), nil
}

// headings finds the ATX headings in lines, skipping fenced code.
func headings(lines []string) []heading {
	var out []heading
	open := ""
	for i, l := range lines {
		l = strings.TrimSuffix(l, "\r")
		if m := fence.FindStringSubmatch(l); m != nil {
			if open == "" {
				open = m[1]
			} else if closesFence(m[1], strings.TrimPrefix(strings.TrimLeft(l, " "), m[1]), open) {
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

// closesFence reports whether a fence line (its marks, then the rest of
// the line) closes the open fence: the same character, at least as many,
// and nothing after them.
func closesFence(marks, rest, open string) bool {
	return marks[0] == open[0] && len(marks) >= len(open) && strings.TrimSpace(rest) == ""
}

// headingText is a heading's content without its closing # marks.
func headingText(s string) string {
	return strings.TrimSpace(closing.ReplaceAllString(s, ""))
}

// headingKey is how a name matches a heading: trimmed, without leading or
// closing # marks, case-insensitively.
func headingKey(name string) string {
	return strings.ToLower(headingText(strings.TrimLeft(strings.TrimSpace(name), "#")))
}

// matchHeadings is the indexes of the headings key names: those equal to it
// or, when none is, those that start with it at a word boundary, so "Who
// works out what" finds "Who works out what (new ADR 0024)" but "Store"
// doesn't find "Storefront".
func matchHeadings(hs []heading, key string) []int {
	var exact, prefix []int
	for i, h := range hs {
		text := strings.ToLower(h.Text)
		switch {
		case text == key:
			exact = append(exact, i)
		case key != "" && strings.HasPrefix(text, key) && !wordChar(nextRune(text[len(key):])):
			prefix = append(prefix, i)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return prefix
}

// nextRune is the first character of s.
func nextRune(s string) rune {
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

// wordChar reports whether r continues a word.
func wordChar(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
