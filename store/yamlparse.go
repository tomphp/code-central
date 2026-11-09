// A fast parser for the YAML the store writes. Startup decodes every
// file, and yaml.v3's general parser was most of that time
// (code-central-205), so decodeStrict reads the block style encodeYAML
// writes with this parser instead, into the same yaml.Node tree, and
// leaves anything else to yaml.v3.

package store

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// parseBlockYAML parses data into the tree yaml.Unmarshal would build,
// for the subset of YAML encodeYAML writes: a block mapping at the top,
// nested block mappings and sequences indented under their keys, plain
// and single-line quoted scalars, literal block scalars, and [] and {}.
// ok is false for anything else, including whatever yaml.v3 would
// refuse; the caller then parses with yaml.v3, which reads hand edits
// and names their errors. Plain scalars carry no tag: yaml.v3 resolves
// one from the value when it decodes them.
func parseBlockYAML(data []byte) (doc *yaml.Node, ok bool) {
	if len(data) == 0 || data[len(data)-1] != '\n' || !printable(data) {
		return nil, false
	}
	p := &blockParser{lines: strings.Split(string(data[:len(data)-1]), "\n")}
	defer func() {
		if r := recover(); r != nil {
			if r != errNotBlockYAML {
				panic(r)
			}
			doc, ok = nil, false
		}
	}()
	p.skipBlank()
	if p.eof() || indentOf(p.lines[p.i]) != 0 {
		return nil, false
	}
	root := p.mapping(0)
	if !p.eof() {
		return nil, false
	}
	return &yaml.Node{Kind: yaml.DocumentNode, Line: root.Line, Content: []*yaml.Node{root}}, true
}

// errNotBlockYAML unwinds the parser when the file leaves the subset.
var errNotBlockYAML = new(int)

// notBlockYAML stops the parse: parseBlockYAML recovers and is not ok.
func notBlockYAML() { panic(errNotBlockYAML) }

// blockParser reads a file line by line, each node from its first line.
type blockParser struct {
	lines []string
	i     int // the current line
	// itemIndent, when set, stands in for the current line's
	// indentation: a sequence item's "- " read as spaces, so the mapping
	// it opens reads like any other.
	itemIndent int
	// nodes are allocated from slab, a few dozen at a time.
	slab []yaml.Node
}

// node is a new node on the current line.
func (p *blockParser) node(kind yaml.Kind, style yaml.Style, tag, value string) *yaml.Node {
	if len(p.slab) == 0 {
		p.slab = make([]yaml.Node, 64)
	}
	n := &p.slab[0]
	p.slab = p.slab[1:]
	// The line yaml.v3 gives it, which a wrong type's error names.
	n.Kind, n.Style, n.Tag, n.Value, n.Line = kind, style, tag, value, p.i+1
	return n
}

func (p *blockParser) eof() bool { return p.i >= len(p.lines) }

// skipBlank skips empty lines; a line of only spaces leaves the subset.
func (p *blockParser) skipBlank() {
	for ; !p.eof() && p.lines[p.i] == ""; p.i++ {
	}
}

// current is the current line's indentation and text after it.
func (p *blockParser) current() (int, string) {
	ln := p.lines[p.i]
	if strings.HasPrefix(ln, "---") || strings.HasPrefix(ln, "...") { // document markers
		notBlockYAML()
	}
	ind := indentOf(ln)
	if p.itemIndent > ind {
		ind = p.itemIndent
	}
	text := ln[ind:]
	if text == "" || text[0] == '\t' || text[0] == '#' {
		notBlockYAML()
	}
	return ind, text
}

// next moves to the line below.
func (p *blockParser) next() { p.i, p.itemIndent = p.i+1, 0 }

// mapping reads a block mapping whose keys sit at column indent.
func (p *blockParser) mapping(indent int) *yaml.Node {
	p.skipBlank()
	m := p.node(yaml.MappingNode, 0, "!!map", "")
	for ; !p.eof(); p.skipBlank() {
		ind, text := p.current()
		if ind < indent {
			break
		}
		if ind > indent {
			notBlockYAML()
		}
		key, rest := splitKey(text)
		k := p.node(yaml.ScalarNode, 0, "", key)
		var v *yaml.Node
		if rest == "" {
			p.next()
			if v = p.nested(indent); v.Kind == yaml.ScalarNode {
				v.Line = k.Line // null, on its key's line
			}
		} else {
			v = p.inline(rest, indent)
		}
		m.Content = append(m.Content, k, v)
	}
	return m
}

// nested reads the value of a key at column indent with nothing after
// its colon: a mapping or sequence on the lines below, or null.
func (p *blockParser) nested(indent int) *yaml.Node {
	p.skipBlank()
	if p.eof() {
		return p.node(yaml.ScalarNode, 0, "", "")
	}
	ind, text := p.current()
	switch {
	case ind < indent:
		return p.node(yaml.ScalarNode, 0, "", "")
	case ind == indent:
		if isItem(text) { // a sequence not indented under its key
			notBlockYAML()
		}
		return p.node(yaml.ScalarNode, 0, "", "")
	case isItem(text):
		return p.sequence(ind)
	default:
		return p.mapping(ind)
	}
}

// sequence reads a block sequence whose "- " sits at column indent.
func (p *blockParser) sequence(indent int) *yaml.Node {
	p.skipBlank()
	s := p.node(yaml.SequenceNode, 0, "!!seq", "")
	for ; !p.eof(); p.skipBlank() {
		ind, text := p.current()
		if ind < indent {
			break
		}
		if ind > indent || !strings.HasPrefix(text, "- ") {
			notBlockYAML()
		}
		rest := text[2:]
		if rest == "" || rest[0] == ' ' || rest[0] == '|' || isItem(rest) {
			notBlockYAML()
		}
		if !strings.ContainsRune(`'"[{`, rune(rest[0])) && (strings.Contains(rest, ": ") || strings.HasSuffix(rest, ":")) {
			p.itemIndent = indent + 2
			s.Content = append(s.Content, p.mapping(indent+2))
			continue
		}
		s.Content = append(s.Content, p.inline(rest, indent))
	}
	return s
}

// inline reads the value written on its key's or item's line, which
// sits at column indent, and moves past it.
func (p *blockParser) inline(text string, indent int) *yaml.Node {
	var n *yaml.Node
	switch text[0] {
	case '|':
		return p.literal(text[1:], indent)
	case '"':
		n = p.node(yaml.ScalarNode, yaml.DoubleQuotedStyle, "!!str", doubleQuoted(text))
	case '\'':
		n = p.node(yaml.ScalarNode, yaml.SingleQuotedStyle, "!!str", singleQuoted(text))
	case '[':
		if text != "[]" {
			notBlockYAML()
		}
		n = p.node(yaml.SequenceNode, yaml.FlowStyle, "!!seq", "")
	case '{':
		if text != "{}" {
			notBlockYAML()
		}
		n = p.node(yaml.MappingNode, yaml.FlowStyle, "!!map", "")
	default:
		n = p.node(yaml.ScalarNode, 0, "", plain(text))
	}
	p.next()
	// Nothing may continue the value on the lines below.
	if p.skipBlank(); !p.eof() {
		if ind, _ := p.current(); ind > indent {
			notBlockYAML()
		}
	}
	return n
}

// literal reads a literal block scalar from its header (after the |) to
// the end of its content, for a key at column indent.
func (p *blockParser) literal(header string, indent int) *yaml.Node {
	line := p.i + 1
	chomp, indentIndicator := byte(0), 0
	for _, c := range []byte(header) {
		switch {
		case (c == '-' || c == '+') && chomp == 0:
			chomp = c
		case c >= '1' && c <= '9' && indentIndicator == 0:
			indentIndicator = int(c - '0')
		default:
			notBlockYAML()
		}
	}
	p.next()
	block := -1
	if indentIndicator > 0 {
		block = indent + indentIndicator
	}
	var content []string
	for ; !p.eof(); p.i++ {
		ln := p.lines[p.i]
		if ln == "" {
			content = append(content, "")
			continue
		}
		ind := indentOf(ln)
		if ind == len(ln) {
			notBlockYAML()
		}
		if block < 0 {
			if ind <= indent {
				break
			}
			if ln[ind] == '\t' { // yaml.v3 refuses a tab while finding the indentation
				notBlockYAML()
			}
			block = ind
		}
		if ind < block {
			break
		}
		content = append(content, ln[block:])
	}
	trailing := 0
	for trailing < len(content) && content[len(content)-1-trailing] == "" {
		trailing++
	}
	body := content[:len(content)-trailing]
	if len(body) == 1 { // Join would return the slice of the file
		body[0] = strings.Clone(body[0])
	}
	value := ""
	switch {
	case len(body) == 0:
		if chomp == '+' {
			notBlockYAML()
		}
	case chomp == '-':
		value = strings.Join(body, "\n")
	case chomp == '+':
		value = strings.Join(body, "\n") + strings.Repeat("\n", trailing+1)
	default:
		value = strings.Join(body, "\n") + "\n"
	}
	n := p.node(yaml.ScalarNode, yaml.LiteralStyle, "!!str", value)
	n.Line = line
	return n
}

// splitKey splits a mapping line into its plain key and what follows
// the colon, "" when the value is on the lines below.
func splitKey(text string) (key, rest string) {
	key, rest, found := strings.Cut(text, ": ")
	if !found {
		var ok bool
		if key, ok = strings.CutSuffix(text, ":"); !ok {
			notBlockYAML()
		}
	}
	// yaml.v3 refuses an implicit key over 1024 characters, counting
	// from the line's start; 1000 leaves room for the indentation.
	if len(key) > 1000 || strings.HasPrefix(rest, " ") {
		notBlockYAML()
	}
	return plain(key), rest
}

// plain checks text is a whole plain scalar on one line that yaml.v3
// reads as written, and returns it.
func plain(text string) string {
	if text == "" {
		notBlockYAML()
	}
	switch text[0] {
	case '[', ']', '{', '}', ',', '#', '&', '*', '!', '|', '>', '\'', '"', '%', '@', '`':
		notBlockYAML()
	case '-', '?', ':':
		if len(text) == 1 || text[1] == ' ' {
			notBlockYAML()
		}
	}
	last := text[len(text)-1]
	if last == ' ' || last == ':' || text == "<<" || strings.Contains(text, ": ") ||
		strings.Contains(text, " #") || strings.IndexByte(text, '\t') >= 0 {
		notBlockYAML()
	}
	return strings.Clone(text) // not a slice of the file, which a record would keep alive
}

// singleQuoted reads a single-quoted scalar that is the whole of text.
func singleQuoted(text string) string {
	var b strings.Builder
	for i := 1; i < len(text); i++ {
		if text[i] != '\'' {
			b.WriteByte(text[i])
			continue
		}
		if i+1 < len(text) && text[i+1] == '\'' {
			b.WriteByte('\'')
			i++
			continue
		}
		if i != len(text)-1 {
			notBlockYAML()
		}
		return b.String()
	}
	notBlockYAML()
	return ""
}

// doubleQuoted reads a double-quoted scalar that is the whole of text,
// with YAML's escapes.
func doubleQuoted(text string) string {
	var b strings.Builder
	for i := 1; i < len(text); i++ {
		c := text[i]
		switch c {
		case '"':
			if i != len(text)-1 {
				notBlockYAML()
			}
			return b.String()
		case '\\':
		default:
			b.WriteByte(c)
			continue
		}
		if i++; i >= len(text) {
			notBlockYAML()
		}
		if r, ok := simpleEscapes[text[i]]; ok {
			b.WriteRune(r)
			continue
		}
		width := strings.IndexByte("x.u...U", text[i]) + 2
		if width < 2 || i+width >= len(text) {
			notBlockYAML()
		}
		code, err := strconv.ParseUint(text[i+1:i+1+width], 16, 32)
		if err != nil || !utf8.ValidRune(rune(code)) {
			notBlockYAML()
		}
		b.WriteRune(rune(code))
		i += width
	}
	notBlockYAML()
	return ""
}

// simpleEscapes are the one-character escapes of a double-quoted
// scalar.
var simpleEscapes = map[byte]rune{
	'0': 0, 'a': '\a', 'b': '\b', 't': '\t', '\t': '\t', 'n': '\n', 'v': '\v', 'f': '\f', 'r': '\r', 'e': 0x1b,
	' ': ' ', '"': '"', '/': '/', '\\': '\\', 'N': 0x85, '_': 0xa0, 'L': 0x2028, 'P': 0x2029,
}

// printable is whether data is UTF-8 that YAML allows in a stream,
// without a byte order mark or any line break but \n: yaml.v3 also
// breaks lines at \r, U+0085, U+2028 and U+2029, which encodeYAML
// escapes.
func printable(data []byte) bool {
	for i := 0; i < len(data); {
		c := data[i]
		if c < utf8.RuneSelf {
			if (c < ' ' && c != '\n' && c != '\t') || c == 0x7f {
				return false
			}
			i++
			continue
		}
		r, size := utf8.DecodeRune(data[i:])
		switch {
		case r == utf8.RuneError && size == 1, r < 0xa0, r == 0x2028, r == 0x2029, r == 0xfeff, r == 0xfffe, r == 0xffff:
			return false
		}
		i += size
	}
	return true
}

func indentOf(line string) int {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	return i
}

// isItem is whether text starts a block sequence item.
func isItem(text string) bool { return text == "-" || strings.HasPrefix(text, "- ") }
