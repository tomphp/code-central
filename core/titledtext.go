package core

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tomoram/code-central/store"
)

// Written text as Hide Code Central writes it (Spec code-central-1050):
// each reference to a Spec or Ticket of the Project is named by its
// title. A reference is a full ID standing as a whole word, or an ID
// Link; it is rewritten only when its number names a Spec or Ticket.
// Code spans and code blocks, fenced and indented, other Projects' keys
// and all other prose are left as written.

var (
	// codeFence opens or closes a fenced code block. It allows any
	// indentation, as a fence inside a list item is indented, so a fence
	// is never mistaken for prose.
	codeFence = regexp.MustCompile("^[ \t]*(`{3,}|~{3,})")
	// listItem is a line that starts a list item.
	listItem = regexp.MustCompile(`^ {0,3}(?:[-+*]|[0-9]{1,9}[.)])(?:[ \t]|$)`)
	// urlStart is a URL with a scheme, up to the next space.
	urlStart = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://\S*`)
)

// writtenText is text, a human's or an agent's, as a comment on key's
// issue writes it: as written, or, when hide asks for Hide Code Central,
// with each reference to a Spec or Ticket of key named by its title.
func writtenText(r store.Reader, key, text string, hide bool) string {
	if !hide {
		return text
	}
	return titledText(text, key, func(n int) (string, bool) {
		if s, ok := r.Spec(key, n); ok {
			return s.Title, true
		}
		if t, ok := r.Ticket(key, n); ok {
			return t.Title, true
		}
		return "", false
	})
}

// titledText is text, Markdown, with each reference to an item of key
// that title names rewritten: a bare ID to the item's title in straight
// double quotes, an ID Link whose text is the ID likewise, and an ID Link
// with other text to that text, its own references rewritten. Code
// blocks are left as written; the rest is rewritten a paragraph at a
// time, so a code span may run over lines.
func titledText(text, key string, title func(n int) (string, bool)) string {
	t := idTitler{key: key, title: title,
		link: regexp.MustCompile(`^\[([^\[\]]*)\]\(` + regexp.QuoteMeta(key) + `-([1-9][0-9]*)\)`),
		bare: regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `-([1-9][0-9]*)`)}
	var out, para []string
	flush := func() {
		if len(para) > 0 {
			out = append(out, t.inline(strings.Join(para, "\n")))
			para = nil
		}
	}
	fence, indented, inList, afterBlank := "", false, false, true
	for _, raw := range strings.Split(text, "\n") {
		l := strings.TrimSuffix(raw, "\r")
		switch {
		case fence != "":
			if m := codeFence.FindStringSubmatch(l); m != nil && closesFence(m[1], strings.TrimLeft(l, " \t")[len(m[1]):], fence) {
				fence = ""
			}
			out = append(out, raw)
			continue
		case strings.TrimSpace(l) == "":
			flush()
			out = append(out, raw)
			afterBlank = true
			continue
		}
		indent := indentation(l)
		indented = indent >= 4 && (indented || afterBlank && !inList)
		afterBlank = false
		if indented {
			flush()
			out = append(out, raw)
			continue
		}
		if m := codeFence.FindStringSubmatch(l); m != nil {
			flush()
			fence = m[1]
			out = append(out, raw)
			continue
		}
		if indent < 4 {
			inList = listItem.MatchString(l) || inList && len(para) > 0
		}
		para = append(para, raw)
	}
	flush()
	return strings.Join(out, "\n")
}

// indentation is the columns of l's leading spaces and tabs, a tab
// running to the next multiple of four.
func indentation(l string) int {
	n := 0
	for _, c := range l {
		switch c {
		case ' ':
			n++
		case '\t':
			n += 4 - n%4
		default:
			return n
		}
	}
	return n
}

// idTitler rewrites the references to key's items in prose.
type idTitler struct {
	key   string
	title func(n int) (string, bool)
	// link is an ID Link to one of key's items, bare a full ID of one.
	link, bare *regexp.Regexp
}

// inline is s, prose, with its references rewritten outside code spans
// and URLs.
func (t idTitler) inline(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		rest := s[i:]
		switch {
		case rest[0] == '\\' && len(rest) > 1 && rest[1] < utf8.RuneSelf && unicode.IsPunct(rune(rest[1])):
			b.WriteString(rest[:2])
			i += 2
			continue
		case rest[0] == '`':
			n := codeSpan(rest)
			b.WriteString(rest[:n])
			i += n
			continue
		case rest[0] == '[':
			if m := t.link.FindStringSubmatch(rest); m != nil {
				b.WriteString(t.linkText(m[0], m[1], m[2]))
				i += len(m[0])
				continue
			}
		case !wordRune(lastRune(s[:i])):
			if m := urlStart.FindString(rest); m != "" {
				b.WriteString(m)
				i += len(m)
				continue
			}
			if m := t.bare.FindStringSubmatch(rest); m != nil && wholeID(s[:i], rest[len(m[0]):]) {
				if title, ok := t.titleOf(m[1]); ok {
					b.WriteString(`"` + title + `"`)
					i += len(m[0])
					continue
				}
			}
		}
		_, size := utf8.DecodeRuneInString(rest)
		b.WriteString(rest[:size])
		i += size
	}
	return b.String()
}

// linkText is what the ID Link whole, with text, to number n, is
// rewritten to: the quoted title when its text is the ID, else the text
// alone with its references rewritten; whole as written when n names
// nothing.
func (t idTitler) linkText(whole, text, n string) string {
	title, ok := t.titleOf(n)
	switch {
	case !ok:
		return whole
	case strings.TrimSpace(text) == t.key+"-"+n:
		return `"` + title + `"`
	default:
		return t.inline(text)
	}
}

// titleOf is the title of key's item numbered n, a number in decimal.
func (t idTitler) titleOf(n string) (string, bool) {
	num := 0
	for _, d := range n {
		num = num*10 + int(d-'0')
		if num > 1<<30 {
			return "", false
		}
	}
	return t.title(num)
}

// wholeID reports whether an ID between before and after stands as a
// whole word: not part of a longer word, ID, path or file name.
func wholeID(before, after string) bool {
	switch prev := lastRune(before); {
	case prev == '-' || prev == '/' || prev == '.':
		return false
	case prev == '(' && strings.HasSuffix(before, "]("):
		return false
	}
	next, size := utf8.DecodeRuneInString(after)
	switch {
	case wordRune(next) || next == '-' || next == '/':
		return false
	case next == '.':
		following, _ := utf8.DecodeRuneInString(after[size:])
		return !wordRune(following)
	}
	return true
}

// codeSpan is the length of the code span s starts with: from its run of
// backticks to the next run of the same length; just the run when no
// such run closes it, as the backticks are then literal.
func codeSpan(s string) int {
	open := len(s) - len(strings.TrimLeft(s, "`"))
	for i := open; i < len(s); {
		j := strings.IndexByte(s[i:], '`')
		if j < 0 {
			break
		}
		start := i + j
		end := start + len(s[start:]) - len(strings.TrimLeft(s[start:], "`"))
		if end-start == open {
			return end
		}
		i = end
	}
	return open
}

// lastRune is the last rune of s; utf8.RuneError when s is empty.
func lastRune(s string) rune {
	r, _ := utf8.DecodeLastRuneInString(s)
	return r
}

// wordRune reports whether r is part of a word: a letter, a digit or an
// underscore.
func wordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
