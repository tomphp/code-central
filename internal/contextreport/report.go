// The report's figures, worked out from the sessions' tool calls.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Report is the figures the report prints, and what a baseline saves:
// sizes, counts, IDs and short inputs, never a tool result's content.
type Report struct {
	Sessions int       `json:"sessions"`
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	Kinds    []Kind    `json:"kinds"`
	// Largest is the largest tool results, largest first.
	Largest []Result `json:"largest"`
	// ReReads is the Specs and Tickets read more than once, most read
	// first. WithinSession counts the reads of an ID after its first in
	// the same session, and AcrossSessions the sessions that read an ID
	// after the first that did, over every ID, not only those listed.
	ReReads        []ReRead `json:"re_reads"`
	WithinSession  int      `json:"within_session"`
	AcrossSessions int      `json:"across_sessions"`
	// WiderReads is the reads called again for more, in session order.
	WiderReads []WiderRead `json:"wider_reads"`
}

// Kind is the figures of one kind of session: a Stage (grilling, spec
// or tickets), implementation, review (the code-review subagents) or
// subagent (any other). Tools counts every Code Central operation as the
// one tool Code Central, and Operations breaks it down. Rounds and
// Questions count post_round calls and their Questions, and WiderReads
// the reads called again for more: both signs that a cut left out
// something an agent needed.
type Kind struct {
	Name       string   `json:"name"`
	Sessions   int      `json:"sessions"`
	Bytes      int      `json:"bytes"`
	Tools      []Figure `json:"tools,omitempty"`
	Operations []Figure `json:"operations,omitempty"`
	Rounds     int      `json:"rounds,omitempty"`
	Questions  int      `json:"questions,omitempty"`
	WiderReads int      `json:"wider_reads,omitempty"`
}

// Figure is a tool's or an operation's calls and the bytes of their
// results.
type Figure struct {
	Name  string `json:"name"`
	Calls int    `json:"calls"`
	Bytes int    `json:"bytes"`
}

// Result is one tool result, with a short form of its call's input.
type Result struct {
	Kind   string `json:"kind"`
	Ticket string `json:"ticket"`
	Tool   string `json:"tool"`
	Input  string `json:"input"`
	Bytes  int    `json:"bytes"`
}

// ReRead is a Spec or Ticket read more than once with get_spec or
// get_ticket: its reads, the sessions that read it, and those sessions'
// Tickets and kinds.
type ReRead struct {
	ID       string   `json:"id"`
	Reads    int      `json:"reads"`
	Sessions int      `json:"sessions"`
	Readers  []string `json:"readers"`
}

// WiderRead is a get_spec or get_ticket called again in the same
// session for something the earlier calls for that ID didn't fetch:
// more parts, more sections, or full.
type WiderRead struct {
	Kind      string `json:"kind"`
	Ticket    string `json:"ticket"`
	Operation string `json:"operation"`
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
}

// codeCentral prefixes the tool name of every Code Central operation.
const codeCentral = "mcp__code-central__"

// kindOrder is the order kinds are listed in: the Factory's, then the
// subagents'. Any other kind follows, by name.
var kindOrder = []string{"grilling", "spec", "tickets", "implementation", "review", "subagent"}

// Build works out the report's figures from the sessions, listing the
// top largest results, re-read IDs and wider reads.
func Build(sessions []Session, top int) Report {
	r := Report{Sessions: len(sessions)}
	kinds := map[string]*Kind{}
	tools := map[string]map[string]*Figure{}
	ops := map[string]map[string]*Figure{}
	type read struct {
		reads    int
		sessions map[int]bool
		readers  map[string]bool
	}
	reads := map[string]*read{}

	for i, s := range sessions {
		if r.From.IsZero() || s.Started.Before(r.From) {
			r.From = s.Started
		}
		if s.Started.After(r.To) {
			r.To = s.Started
		}
		k := kinds[s.Kind]
		if k == nil {
			k = &Kind{Name: s.Kind}
			kinds[s.Kind] = k
			tools[s.Kind] = map[string]*Figure{}
			ops[s.Kind] = map[string]*Figure{}
		}
		k.Sessions++
		scopes := map[string]map[string]bool{}
		firsts := map[string]string{}
		for _, c := range s.Calls {
			k.Bytes += c.Bytes
			tool := c.Tool
			op, isOperation := operation(c.Tool)
			if isOperation {
				tool = "Code Central"
				add(ops[s.Kind], op, c.Bytes)
			}
			add(tools[s.Kind], tool, c.Bytes)
			r.Largest = append(r.Largest, Result{Kind: s.Kind, Ticket: s.Ticket, Tool: c.Tool, Input: shortInput(c, s.Cwd), Bytes: c.Bytes})

			if !isOperation {
				continue
			}
			switch op {
			case "post_round":
				k.Rounds++
				questions, _ := c.Input["questions"].([]any)
				k.Questions += len(questions)
			case "get_spec", "get_ticket":
				id := readID(op, c.Input, s.Ticket)
				if id == "" {
					continue
				}
				rd := reads[id]
				if rd == nil {
					rd = &read{sessions: map[int]bool{}, readers: map[string]bool{}}
					reads[id] = rd
				}
				rd.reads++
				rd.sessions[i] = true
				rd.readers[s.Ticket+" "+s.Kind] = true

				key := op + " " + id
				scope := readScope(op, c.Input)
				if seen, ok := scopes[key]; !ok {
					scopes[key] = scope
					firsts[key] = shortInput(c, s.Cwd)
				} else if !covered(scope, seen) {
					k.WiderReads++
					r.WiderReads = append(r.WiderReads, WiderRead{Kind: s.Kind, Ticket: s.Ticket, Operation: op, ID: id, From: firsts[key], To: shortInput(c, s.Cwd)})
					for e := range scope {
						seen[e] = true
					}
				}
			}
		}
	}

	for name, k := range kinds {
		k.Tools = figures(tools[name])
		k.Operations = figures(ops[name])
		r.Kinds = append(r.Kinds, *k)
	}
	sort.Slice(r.Kinds, func(i, j int) bool {
		a, b := kindRank(r.Kinds[i].Name), kindRank(r.Kinds[j].Name)
		if a != b {
			return a < b
		}
		return r.Kinds[i].Name < r.Kinds[j].Name
	})

	sort.SliceStable(r.Largest, func(i, j int) bool { return r.Largest[i].Bytes > r.Largest[j].Bytes })
	r.Largest = atMost(r.Largest, top)

	for id, rd := range reads {
		r.WithinSession += rd.reads - len(rd.sessions)
		r.AcrossSessions += len(rd.sessions) - 1
		if rd.reads < 2 {
			continue
		}
		var readers []string
		for who := range rd.readers {
			readers = append(readers, who)
		}
		sort.Strings(readers)
		r.ReReads = append(r.ReReads, ReRead{ID: id, Reads: rd.reads, Sessions: len(rd.sessions), Readers: readers})
	}
	sort.Slice(r.ReReads, func(i, j int) bool {
		if r.ReReads[i].Reads != r.ReReads[j].Reads {
			return r.ReReads[i].Reads > r.ReReads[j].Reads
		}
		return r.ReReads[i].ID < r.ReReads[j].ID
	})
	r.ReReads = atMost(r.ReReads, top)
	r.WiderReads = atMost(r.WiderReads, top)
	return r
}

func add(figures map[string]*Figure, name string, bytes int) {
	f := figures[name]
	if f == nil {
		f = &Figure{Name: name}
		figures[name] = f
	}
	f.Calls++
	f.Bytes += bytes
}

// figures lists the figures largest first, nil when there are none.
func figures(m map[string]*Figure) []Figure {
	var list []Figure
	for _, f := range m {
		list = append(list, *f)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Bytes != list[j].Bytes {
			return list[i].Bytes > list[j].Bytes
		}
		return list[i].Name < list[j].Name
	})
	return list
}

func kindRank(name string) int {
	for i, s := range kindOrder {
		if s == name {
			return i
		}
	}
	return len(kindOrder)
}

// operation is the Code Central operation a tool calls, if it is one.
func operation(tool string) (string, bool) {
	return strings.CutPrefix(tool, codeCentral)
}

// atMost is the first n of list, or all of it when it is shorter.
func atMost[T any](list []T, n int) []T {
	if len(list) > n {
		return list[:n]
	}
	return list
}

var digits = regexp.MustCompile(`^[0-9]+$`)

// readID is the Spec or Ticket a get_spec or get_ticket call reads, as
// <key>-<n>. A bare number takes the key of the session's Ticket.
func readID(op string, input map[string]any, ticket string) string {
	var id string
	switch v := input[strings.TrimPrefix(op, "get_")].(type) {
	case string:
		id = v
	case float64:
		id = fmt.Sprint(int(v))
	}
	if digits.MatchString(id) {
		if i := strings.LastIndex(ticket, "-"); i >= 0 {
			id = ticket[:i+1] + id
		}
	}
	return id
}

// defaultParts is the parts each read fetches when it names none.
var defaultParts = map[string][]string{
	"get_spec":   {"body", "tickets", "edits"},
	"get_ticket": {"claims", "failure_reports", "comments"},
}

// readScope is what a read fetches: each of its parts (its default parts
// when it names none, or only the body when a get_spec names sections),
// each of the body's sections it names (or the whole body), and full when
// it asks for it. The sections rule mirrors core's GetSpecInput.parts.
func readScope(op string, input map[string]any) map[string]bool {
	scope := map[string]bool{}
	var parts []string
	for _, p := range listInput(input, "parts") {
		parts = append(parts, strings.ToLower(p))
	}
	switch {
	case len(parts) > 0:
	case op == "get_spec" && len(listInput(input, "sections")) > 0:
		parts = []string{"body"}
	default:
		parts = defaultParts[op]
	}
	if len(parts) == 0 {
		scope["part:default"] = true
	}
	for _, p := range parts {
		if p != "body" {
			scope["part:"+p] = true
			continue
		}
		sections := listInput(input, "sections")
		if len(sections) == 0 {
			scope["section:*"] = true
		}
		for _, section := range sections {
			scope["section:"+strings.ToLower(section)] = true
		}
	}
	if full, _ := input["full"].(bool); full {
		scope["full"] = true
	}
	return scope
}

func listInput(input map[string]any, name string) []string {
	values, _ := input[name].([]any)
	var list []string
	for _, v := range values {
		list = append(list, fmt.Sprint(v))
	}
	return list
}

// covered reports whether every element of scope was already fetched,
// where the whole body covers any of its sections.
func covered(scope, seen map[string]bool) bool {
	for e := range scope {
		if !seen[e] && !(strings.HasPrefix(e, "section:") && seen["section:*"]) {
			return false
		}
	}
	return true
}

// shortInput is a call's input in at most inputLength characters: a
// Bash command, a Read's file, or else the input as JSON without the
// Claim and session IDs. The session's folder is cut from paths, and
// the home folder shortened to ~. A file keeps its end, which names it.
func shortInput(c Call, cwd string) string {
	var s string
	switch c.Tool {
	case "Bash":
		s, _ = c.Input["command"].(string)
	case "Read":
		s, _ = c.Input["file_path"].(string)
	default:
		input := map[string]any{}
		for k, v := range c.Input {
			if k != "claim_id" && k != "session_id" {
				input[k] = v
			}
		}
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		enc.Encode(input)
		s = b.String()
	}
	if cwd != "" {
		s = strings.ReplaceAll(s, cwd+"/", "")
	}
	if home != "" {
		s = strings.ReplaceAll(s, home, "~")
	}
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	switch {
	case len(runes) <= inputLength:
		return s
	case c.Tool == "Read":
		return "…" + string(runes[len(runes)-inputLength+1:])
	}
	return string(runes[:inputLength-1]) + "…"
}

// home is the user's home folder, shortened to ~ in short inputs.
var home, _ = os.UserHomeDir()

// inputLength is the most characters of a short input.
const inputLength = 80
