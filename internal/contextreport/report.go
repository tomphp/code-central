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
	Stages   []Stage   `json:"stages"`
	// Largest is the largest tool results, largest first.
	Largest []Result `json:"largest"`
	// ReReads is the Specs and Tickets read more than once, most read
	// first. WithinSession counts the reads of an ID after its first in
	// the same session, and AcrossSessions the sessions that read an ID
	// after the first that did, over every ID, not only those listed.
	ReReads        []ReRead `json:"re_reads"`
	WithinSession  int      `json:"within_session"`
	AcrossSessions int      `json:"across_sessions"`
	// Widenings is the reads called again for more, in session order.
	Widenings []Widening `json:"widenings"`
}

// Stage is one Stage's figures over its sessions. Tools counts every
// Code Central operation as the one tool Code Central, and Operations
// breaks it down. Rounds and Questions count post_round calls and their
// Questions, and Widenings the reads called again for more: both signs
// that a cut left out something an agent needed.
type Stage struct {
	Name       string   `json:"name"`
	Sessions   int      `json:"sessions"`
	Bytes      int      `json:"bytes"`
	Tools      []Figure `json:"tools,omitempty"`
	Operations []Figure `json:"operations,omitempty"`
	Rounds     int      `json:"rounds,omitempty"`
	Questions  int      `json:"questions,omitempty"`
	Widenings  int      `json:"widenings,omitempty"`
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
	Stage  string `json:"stage"`
	Ticket string `json:"ticket"`
	Tool   string `json:"tool"`
	Input  string `json:"input"`
	Bytes  int    `json:"bytes"`
}

// ReRead is a Spec or Ticket read more than once with get_spec or
// get_ticket: its reads, the sessions that read it, and those sessions'
// Tickets and Stages.
type ReRead struct {
	ID       string   `json:"id"`
	Reads    int      `json:"reads"`
	Sessions int      `json:"sessions"`
	Readers  []string `json:"readers"`
}

// Widening is a get_spec or get_ticket called again in the same session
// for something the earlier calls for that ID didn't fetch: more parts,
// more sections, or full.
type Widening struct {
	Stage     string `json:"stage"`
	Ticket    string `json:"ticket"`
	Operation string `json:"operation"`
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
}

// codeCentral prefixes the tool name of every Code Central operation.
const codeCentral = "mcp__code-central__"

// stageOrder is the order Stages are listed in: the Factory's, then the
// subagents'. Any other Stage follows, by name.
var stageOrder = []string{"grilling", "spec", "tickets", "implementation", "review", "subagent"}

// Build works out the report's figures from the sessions, listing the
// top largest results, re-read IDs and widenings.
func Build(sessions []Session, top int) Report {
	r := Report{Sessions: len(sessions)}
	stages := map[string]*Stage{}
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
		st := stages[s.Stage]
		if st == nil {
			st = &Stage{Name: s.Stage}
			stages[s.Stage] = st
			tools[s.Stage] = map[string]*Figure{}
			ops[s.Stage] = map[string]*Figure{}
		}
		st.Sessions++
		scopes := map[string]map[string]bool{}
		firsts := map[string]string{}
		for _, c := range s.Calls {
			st.Bytes += c.Bytes
			tool := c.Tool
			if op, ok := strings.CutPrefix(c.Tool, codeCentral); ok {
				tool = "Code Central"
				add(ops[s.Stage], op, c.Bytes)
			}
			add(tools[s.Stage], tool, c.Bytes)
			r.Largest = append(r.Largest, Result{Stage: s.Stage, Ticket: s.Ticket, Tool: c.Tool, Input: shortInput(c, s.Cwd), Bytes: c.Bytes})

			switch c.Tool {
			case codeCentral + "post_round":
				st.Rounds++
				questions, _ := c.Input["questions"].([]any)
				st.Questions += len(questions)
			case codeCentral + "get_spec", codeCentral + "get_ticket":
				op := strings.TrimPrefix(c.Tool, codeCentral)
				id := readID(c, s.Ticket)
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
				rd.readers[s.Ticket+" "+s.Stage] = true

				key := op + " " + id
				scope := readScope(c)
				if seen, ok := scopes[key]; !ok {
					scopes[key] = scope
					firsts[key] = shortInput(c, s.Cwd)
				} else if !covered(scope, seen) {
					st.Widenings++
					r.Widenings = append(r.Widenings, Widening{Stage: s.Stage, Ticket: s.Ticket, Operation: op, ID: id, From: firsts[key], To: shortInput(c, s.Cwd)})
					for e := range scope {
						seen[e] = true
					}
				}
			}
		}
	}

	for name, st := range stages {
		st.Tools = figures(tools[name])
		st.Operations = figures(ops[name])
		r.Stages = append(r.Stages, *st)
	}
	sort.Slice(r.Stages, func(i, j int) bool {
		a, b := stageRank(r.Stages[i].Name), stageRank(r.Stages[j].Name)
		if a != b {
			return a < b
		}
		return r.Stages[i].Name < r.Stages[j].Name
	})

	sort.SliceStable(r.Largest, func(i, j int) bool { return r.Largest[i].Bytes > r.Largest[j].Bytes })
	r.Largest = first(r.Largest, top)

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
	r.ReReads = first(r.ReReads, top)
	r.Widenings = first(r.Widenings, top)
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

func stageRank(name string) int {
	for i, s := range stageOrder {
		if s == name {
			return i
		}
	}
	return len(stageOrder)
}

func first[T any](list []T, n int) []T {
	if len(list) > n {
		return list[:n]
	}
	return list
}

var digits = regexp.MustCompile(`^[0-9]+$`)

// readID is the Spec or Ticket a get_spec or get_ticket call reads, as
// <key>-<n>. A bare number takes the key of the session's Ticket.
func readID(c Call, ticket string) string {
	var id string
	switch v := c.Input[strings.TrimPrefix(strings.TrimPrefix(c.Tool, codeCentral), "get_")].(type) {
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

// readScope is what a read fetches: each of its parts (every part when
// it names none), each of its sections (the whole body when it names
// none), and full when it asks for it.
func readScope(c Call) map[string]bool {
	scope := map[string]bool{}
	list := func(name string) {
		values, _ := c.Input[name+"s"].([]any)
		if len(values) == 0 {
			scope[name+":*"] = true
		}
		for _, v := range values {
			scope[name+":"+strings.ToLower(fmt.Sprint(v))] = true
		}
	}
	list("part")
	list("section")
	if full, _ := c.Input["full"].(bool); full {
		scope["full"] = true
	}
	return scope
}

// covered reports whether every element of scope was already fetched.
func covered(scope, seen map[string]bool) bool {
	for e := range scope {
		kind, _, _ := strings.Cut(e, ":")
		if !seen[e] && !seen[kind+":*"] {
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
