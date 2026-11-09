package factory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// claimInfo is what the Factory's lines say of a held Claim's Ticket. It
// is kept in memory only, filled at the claim and from each read of the
// Ticket, so a restart fills it again from the first read.
type claimInfo struct {
	title string
	stage string // the guide's name, one of core.GuideNames
}

// noteTicket keeps what the lines say of c's Ticket, as tk gives it,
// unless c has ended.
func (f *factory) noteTicket(c claim, tk core.TicketView) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ended[c.ClaimID] {
		return
	}
	f.info[c.ClaimID] = claimInfo{title: tk.Title, stage: core.GuideName(tk.Labels)}
}

// about is "(<stage>): "<title>"", naming c's Ticket in a line.
func (f *factory) about(c claim) string {
	i := f.claimInfo(c)
	return fmt.Sprintf("(%s): %q", i.stage, i.title)
}

// claimInfo is what the lines say of c's Ticket; with nothing noted, its
// stage is implementation.
func (f *factory) claimInfo(c claim) claimInfo {
	f.mu.Lock()
	i, ok := f.info[c.ClaimID]
	f.mu.Unlock()
	if !ok {
		i.stage = core.ImplementationStage
	}
	return i
}

// claimf prints a line about the held Claim c and queues it as a factory
// Session Event for c's session. Every such line goes through here, the
// one per-Claim logger; lines with no Claim are only printed, and so is
// a --verbose tool-call line, whose tool call is posted already.
func (f *factory) claimf(c claim, format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	f.printf("%s", line)
	f.events.add(c.SessionID, core.SessionEvent{Time: time.Now(), Kind: eventFactory, Text: line})
}

// claimed prints the line for a new Claim on tk, once its worktree is
// known and before its turn starts.
func (f *factory) claimed(c claim, tk core.TicketView, wt string) {
	f.claimf(c, "%s claimed %s in Spec %s; worktree %s; resume: claude --resume %s", c.Ticket, f.about(c), tk.Spec.ID, wt, c.SessionID)
}

// turnEnded prints the line for a turn that has ended and freed its
// slot: how it exited, the State the Factory left its Ticket in, if it
// read one, and how long it ran.
func (f *factory) turnEnded(t *turn, end turnEnd, state store.State, ran time.Duration) {
	c := t.claim
	now := ""
	if state != "" {
		now = "; now " + string(state)
	}
	f.claimf(c, "%s turn ended %s: %s%s; ran %s", c.Ticket, f.about(c), end, now, ran.Round(time.Second))
}

// summaryMax is the most characters a tool call's summary runs to.
const summaryMax = 120

// toolCall is a tool call as one line, "<Tool>: <summary>", or "<Tool>"
// when there is nothing to say of its input. Paths inside the worktree
// wt are given relative to it.
func toolCall(tool string, input json.RawMessage, wt string) string {
	var in map[string]any
	_ = json.Unmarshal(input, &in)
	str := func(key string) string {
		s, _ := in[key].(string)
		return s
	}
	path := func(key string) string {
		p := str(key)
		if r, err := filepath.Rel(wt, p); err == nil && filepath.IsAbs(p) && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return r
		}
		return p
	}
	var s string
	switch tool {
	case "Bash":
		s = firstLine(strings.TrimSpace(str("command")))
	case "Read", "Edit", "Write":
		s = path("file_path")
	case "NotebookEdit":
		s = path("notebook_path")
	case "Grep":
		s = str("pattern")
		if str("path") != "" {
			s += " in " + path("path")
		}
	case "Glob":
		s = str("pattern")
	case "WebFetch":
		s = str("url")
	case "WebSearch":
		s = str("query")
	case "Agent", "Task":
		s = str("description")
	case "Skill":
		s = str("skill")
	default:
		if rest, ok := strings.CutPrefix(tool, "mcp__"); ok {
			server, name, _ := strings.Cut(rest, "__")
			s = server + " " + name
			if tk, ok := in["ticket"]; ok && tk != nil {
				s += " " + fmt.Sprint(tk)
			}
		} else {
			s = firstString(input)
		}
	}
	s = shorten(strings.Join(strings.Fields(s), " "), summaryMax)
	if s == "" {
		return tool
	}
	return tool + ": " + s
}

// firstString is the first non-empty string value of a JSON object, in
// the order it is written.
func firstString(input json.RawMessage) string {
	dec := json.NewDecoder(bytes.NewReader(input))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return ""
	}
	for dec.More() {
		if _, err := dec.Token(); err != nil {
			return ""
		}
		var v any
		if err := dec.Decode(&v); err != nil {
			return ""
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
