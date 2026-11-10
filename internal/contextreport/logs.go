// Reading Claude Code's session logs into Sessions and their tool calls.

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Session is one agent's log: a Factory session, or a subagent one
// started. Its Ticket comes from the Factory's first prompt, or the part
// of the first system prompt the Factory adds, and its kind is the Stage,
// or implementation, its Stage Guide there names. A subagent takes its
// parent's Ticket and Started time, and its kind is review for the
// code-review subagents and subagent for any other.
type Session struct {
	ID      string
	Ticket  string
	Kind    string
	Started time.Time
	// FirstTurn is the bytes of text the session starts with: its
	// prompts before the first response, and the Factory's part of the
	// system prompt.
	FirstTurn int
	// Cwd is the folder the session ran in, cut from short inputs so
	// the same file reads the same in every worktree.
	Cwd   string
	Calls []Call
}

// Call is one tool call and the size of its result, in bytes of text.
type Call struct {
	Tool  string
	Input map[string]any
	Bytes int
}

// Window picks sessions by when they started: at or after Since and
// before Until, where a zero time leaves that end open.
type Window struct {
	Since, Until time.Time
}

func (w Window) contains(t time.Time) bool {
	return (w.Since.IsZero() || !t.Before(w.Since)) && (w.Until.IsZero() || t.Before(w.Until))
}

var (
	ticketLine = regexp.MustCompile(`(?m)^Ticket: (\S+)$`)
	stageGuide = regexp.MustCompile(`The Stage Guide for (\S+) follows`)
	// commandArgs is a skill's arguments in the prompt that runs it,
	// which the Factory keeps to the Ticket's ID.
	commandArgs = regexp.MustCompile(`<command-args>(\S+)</command-args>`)
)

// Load reads the Factory sessions of the repository whose main checkout
// is at repo, and their subagents, from Claude Code's projects folder.
// Claude Code names a session's folder after the path it ran in, so the
// repository's folders are the main checkout's and its worktrees'
// (repo.worktrees/<ticket>). Sessions without a Stage Guide in their
// first prompt or system prompt (run by hand, outside the Factory) are
// left out.
func Load(projects, repo string, w Window) ([]Session, error) {
	entries, err := os.ReadDir(projects)
	if err != nil {
		return nil, err
	}
	main := projectDir(repo)
	worktrees := projectDir(repo + ".worktrees/")
	var sessions []Session
	for _, e := range entries {
		if !e.IsDir() || (e.Name() != main && !strings.HasPrefix(e.Name(), worktrees)) {
			continue
		}
		logs, err := filepath.Glob(filepath.Join(projects, e.Name(), "*.jsonl"))
		if err != nil {
			return nil, err
		}
		for _, file := range logs {
			s, err := readLog(file)
			if err != nil {
				return nil, err
			}
			if s.Ticket == "" || s.Kind == "" || !w.contains(s.Started) {
				continue
			}
			sessions = append(sessions, s)
			subagents, err := readSubagents(strings.TrimSuffix(file, ".jsonl"), s)
			if err != nil {
				return nil, err
			}
			sessions = append(sessions, subagents...)
		}
	}
	sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].Started.Before(sessions[j].Started) })
	return sessions, nil
}

// projectDir is the name Claude Code gives the projects folder of a
// session run in path: every character but a letter, digit or dash
// becomes a dash.
func projectDir(path string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, path)
}

// readSubagents reads the subagents of parent, whose logs are in
// <session>/subagents/, each beside a .meta.json with its description.
func readSubagents(dir string, parent Session) ([]Session, error) {
	logs, err := filepath.Glob(filepath.Join(dir, "subagents", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var sessions []Session
	for _, file := range logs {
		s, err := readLog(file)
		if err != nil {
			return nil, err
		}
		var meta struct {
			Description string `json:"description"`
		}
		raw, err := os.ReadFile(strings.TrimSuffix(file, ".jsonl") + ".meta.json")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err == nil {
			if err := json.Unmarshal(raw, &meta); err != nil {
				return nil, err
			}
		}
		s.Ticket, s.Started, s.Kind = parent.Ticket, parent.Started, "subagent"
		if strings.HasPrefix(meta.Description, "Spec review") || strings.HasPrefix(meta.Description, "Standards review") {
			s.Kind = "review"
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// record is the part of a log line the report reads. Claude Code records
// the system prompt as a prompt_snapshot attachment, where the Factory's
// part of it, the Ticket and Stage Guide, is one of SystemPrompt's texts.
type record struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	Message   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	Attachment struct {
		Type         string   `json:"type"`
		SystemPrompt []string `json:"systemPrompt"`
	} `json:"attachment"`
}

// block is one block of a message's content: text, a tool call or a
// tool's result.
type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     map[string]any  `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

// readLog reads one session log: when it started, its Ticket and kind
// from its first prompt or its first system prompt, the size of its
// first turn, and its tool calls in order with their results' sizes. A
// call logged twice counts once.
func readLog(file string) (Session, error) {
	f, err := os.Open(file)
	if err != nil {
		return Session{}, err
	}
	defer f.Close()
	s := Session{ID: strings.TrimSuffix(filepath.Base(file), ".jsonl")}
	calls := map[string]int{}
	sizes := map[string]int{}
	prompted, snapshot, responded := false, false, false
	// identify takes the Ticket and kind from text, unless an earlier
	// text gave them. The Ticket is its Ticket: line, or else a skill's
	// arguments.
	identify := func(text string) {
		if s.Ticket == "" {
			if m := ticketLine.FindStringSubmatch(text); m != nil {
				s.Ticket = m[1]
			} else if m := commandArgs.FindStringSubmatch(text); m != nil {
				s.Ticket = m[1]
			}
		}
		if m := stageGuide.FindStringSubmatch(text); m != nil && s.Kind == "" {
			s.Kind = m[1]
		}
	}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			var rec record
			if jerr := json.Unmarshal(line, &rec); jerr != nil {
				return Session{}, jerr
			}
			if s.Started.IsZero() && rec.Timestamp != "" {
				started, terr := time.Parse(time.RFC3339, rec.Timestamp)
				if terr != nil {
					return Session{}, terr
				}
				s.Started = started
			}
			if s.Cwd == "" {
				s.Cwd = rec.Cwd
			}
			blocks, text := content(rec.Message.Content)
			switch rec.Type {
			case "attachment":
				if rec.Attachment.Type != "prompt_snapshot" || snapshot {
					break
				}
				snapshot = true
				for _, text := range rec.Attachment.SystemPrompt {
					if stageGuide.MatchString(text) {
						identify(text)
						s.FirstTurn += len(text)
					}
				}
			case "user":
				if !prompted && text != "" {
					prompted = true
					identify(text)
				}
				if !responded {
					s.FirstTurn += len(text)
				}
				for _, b := range blocks {
					if b.Type == "tool_result" {
						_, result := content(b.Content)
						sizes[b.ToolUseID] = len(result)
					}
				}
			case "assistant":
				responded = true
				for _, b := range blocks {
					if _, seen := calls[b.ID]; b.Type == "tool_use" && !seen {
						calls[b.ID] = len(s.Calls)
						s.Calls = append(s.Calls, Call{Tool: b.Name, Input: b.Input})
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return Session{}, err
		}
	}
	for id, i := range calls {
		s.Calls[i].Bytes = sizes[id]
	}
	return s, nil
}

// content splits a message's or a tool result's content, which is either
// a string or a list of blocks, into its blocks and its text.
func content(raw json.RawMessage) ([]block, string) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return nil, text
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return nil, ""
	}
	var b strings.Builder
	for _, bl := range blocks {
		if bl.Type == "text" {
			b.WriteString(bl.Text)
		}
	}
	return blocks, b.String()
}
