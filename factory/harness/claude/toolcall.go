// A tool call as one line, for the verbose echo and the Session Events.

package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

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

// shorten is s cut to n characters, ending in an ellipsis when cut.
func shorten(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// firstLine is s up to its first newline.
func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}
