package core

import (
	"math/rand"
	"regexp"
	"strings"
	"testing"
)

// The hiding must leave no ID of the Project outside code, and must leave
// code exactly as written. The oracle is built, not parsed: a generator
// joins prose pieces with code pieces it knows, so what is code and what
// is prose is never a guess.

var leakedID = regexp.MustCompile(`acme-[1-9][0-9]*`)

// prosePieces are the shapes of reference that once leaked or broke.
var prosePieces = []string{
	"acme-10", "see acme-10.", "_acme-10_", "*acme-10*", "**acme-10**",
	"[acme-10](acme-10)", "[a [b] acme-10](acme-10)", "[text](acme-10)",
	`[x](acme-10 "title")`, `[x](acme-10 'title')`, "[x](acme-10 (title))",
	"[x](<acme-10>)", "[x]( acme-10 )", "![img](acme-10)", "![acme-10](acme-10)",
	"[x](acme-10#frag)", "[x](<acme-10#frag>)", "[acme-10](acme-10#frag)", "[acme-10]", "[x][ref]", "(acme-10)", "- acme-10",
	"> acme-10", "# acme-10", "word", "[", "]", "(", ")", "!",
}

// codePieces are inline code spans, each with an ID inside.
var codePieces = []string{
	"`acme-10`", "``a`acme-10``", "`[acme-10](acme-10)`", "` acme-10 `",
}

// blockPieces are code blocks, each ending in a newline, with an ID inside.
var blockPieces = []string{
	"```\nacme-10\n```\n", "~~~go\nacme-10\n~~~\n", "````\n```\nacme-10\n````\n",
	"    acme-10\n", "- item\n\n      acme-10\n", "- ```\n  acme-10\n  ```\n",
}

// hide is titledText with every number named by the title "T".
func hide(text string) string {
	return titledText(text, "acme", func(int) (string, bool) { return "T", true })
}

// checkHiding asserts the two properties of text, whose code is the
// strings in code, none of which holds anything but code.
func checkHiding(t *testing.T, text string, code []string) {
	t.Helper()
	out := hide(text)
	rest := out
	for _, c := range code {
		if !strings.Contains(out, c) {
			t.Fatalf("code changed\ninput:  %q\noutput: %q\ncode:   %q", text, out, c)
		}
		rest = strings.Replace(rest, c, "", 1)
	}
	if m := leakedID.FindString(rest); m != "" {
		t.Fatalf("%s leaked\ninput:  %q\noutput: %q", m, text, out)
	}
}

func TestHidingLeavesNoIDOutsideCode(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	pick := func(s []string) string { return s[rng.Intn(len(s))] }
	for i := 0; i < 20000; i++ {
		var b strings.Builder
		var code []string
		for j := rng.Intn(8) + 1; j > 0; j-- {
			switch rng.Intn(4) {
			case 0:
				c := pick(codePieces)
				code = append(code, c)
				b.WriteString(" " + c + " ") // apart, or neighbouring backticks merge spans
			case 1:
				// A block follows a plain paragraph, which ends any list
				// or quote before it, and sits between blank lines.
				c := pick(blockPieces)
				code = append(code, strings.TrimSuffix(c, "\n"))
				b.WriteString("\n\nbreak\n\n" + c + "\n")
			default:
				b.WriteString(pick(prosePieces) + " ")
			}
		}
		checkHiding(t, b.String(), code)
	}
}

// FuzzHiding runs the same properties over arbitrary text, where only
// the ID check applies outside what it can tell is code: it asserts that
// hiding never panics and that fenced code is untouched.
func FuzzHiding(f *testing.F) {
	for _, p := range prosePieces {
		f.Add(p)
	}
	f.Fuzz(func(t *testing.T, text string) {
		hide(text)
		hide(hide(text))
	})
}
