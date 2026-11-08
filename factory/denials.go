package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// denial is one call a turn had denied, from its result's
// permission_denials.
type denial struct {
	Tool  string          `json:"tool_name"`
	ID    string          `json:"tool_use_id"`
	Input json.RawMessage `json:"tool_input"`
}

// input is the call's input re-encoded with sorted keys, so the same
// call reads the same however it was printed.
func (d denial) input() string {
	var v any
	if json.Unmarshal(d.Input, &v) != nil {
		return string(d.Input)
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSpace(b.String())
}

// pattern is the --allowedTools rule that allows this call: a Bash
// command exactly, a file tool on its path, WebFetch on its domain, and
// any other tool by name.
func (d denial) pattern() string {
	var in struct {
		Command  string `json:"command"`
		FilePath string `json:"file_path"`
		URL      string `json:"url"`
	}
	_ = json.Unmarshal(d.Input, &in)
	switch {
	case d.Tool == "Bash" && in.Command != "":
		return "Bash(" + in.Command + ")"
	case in.FilePath != "":
		return d.Tool + "(" + in.FilePath + ")"
	case d.Tool == "WebFetch" && in.URL != "":
		if u, err := url.Parse(in.URL); err == nil && u.Host != "" {
			return "WebFetch(domain:" + u.Hostname() + ")"
		}
	}
	return d.Tool
}

// distinct is the denied calls with repeats of the same call dropped.
func distinct(ds []denial) []denial {
	seen := map[string]bool{}
	var out []denial
	for _, d := range ds {
		k := d.Tool + "\x00" + d.input()
		if !seen[k] {
			seen[k] = true
			out = append(out, d)
		}
	}
	return out
}

// postDenials posts a Round asking the human about each distinct call
// t's turn had denied, and saves it so the answers can be read. It
// reports whether it posted the Round.
func (f *factory) postDenials(ctx context.Context, t *turn, ds []denial) bool {
	c := t.claim
	ds = distinct(ds)
	var qs []core.QuestionInput
	var patterns []string
	for _, d := range ds {
		p := d.pattern()
		patterns = append(patterns, p)
		qs = append(qs, core.QuestionInput{
			Title: "Allow " + shorten(p, 100) + "?",
			Body: fmt.Sprintf("Auto mode denied this call, so the agent's turn stopped:\n\n```\n%s %s\n```\n\n"+
				"Answer one of:\n\n- `allow`: allow `%s` for the rest of this Claim\n- `allow <pattern>`: allow this --allowedTools rule instead, such as `Bash(npm test:*)`\n- `deny <why>`: don't allow it, and tell the agent why\n",
				d.Tool, d.input(), p),
		})
	}
	res, err := f.client.PostRound(ctx, f.project, core.PostRoundInput{Ticket: core.Ref(c.Ticket), ClaimID: c.ClaimID, SessionID: c.SessionID, Questions: qs})
	if err != nil {
		f.claimf(c, "%s: post_round: %v", c.Ticket, err)
		return false
	}
	if err := f.updateClaim(c.ClaimID, func(c *claim) { c.Denials = &denialRound{Round: res.Round, Patterns: patterns} }); err != nil {
		f.claimf(c, "%s: can't write the state file: %v", c.Ticket, err)
	}
	f.claimf(c, "%s: the turn stopped on %d denied call(s): posted Round %d", c.Ticket, len(ds), res.Round)
	return true
}

// commentDenials lists on the Ticket the distinct calls t's turn had
// denied, for a turn that ended its own way.
func (f *factory) commentDenials(ctx context.Context, t *turn, ds []denial) {
	c := t.claim
	var b strings.Builder
	b.WriteString("code-central-factory: auto mode denied these calls during the last turn (session " + c.SessionID + "):\n")
	for _, d := range distinct(ds) {
		fmt.Fprintf(&b, "\n- `%s`", d.pattern())
	}
	if _, err := f.client.Comment(ctx, f.project, core.CommentInput{Ticket: core.Ref(c.Ticket), Body: b.String()}); err != nil {
		f.claimf(c, "%s: comment: %v", c.Ticket, err)
	}
}

// takeDenialAnswers reads the answers to c's Round about denied calls,
// if res is that Round: what is allowed joins the Claim's allowed tools
// and is saved. It returns the Claim as it now is and a note for the
// agent on what was decided; res isn't that Round, it changes nothing.
func (f *factory) takeDenialAnswers(c claim, res core.PickUpAnswersResult) (claim, string) {
	if c.Denials == nil || c.Denials.Round != res.Round.Number {
		return c, ""
	}
	var allowed []string
	var b strings.Builder
	b.WriteString("This Round was code-central-factory asking the human about calls auto mode denied you. What they decided:\n")
	for i, q := range res.Round.Questions {
		if i >= len(c.Denials.Patterns) {
			break
		}
		def := c.Denials.Patterns[i]
		verb, rest, _ := strings.Cut(strings.TrimSpace(q.Answer), " ")
		rest = strings.TrimSpace(rest)
		switch {
		case q.Status != store.QuestionAnswered:
			fmt.Fprintf(&b, "\n- `%s`: not allowed (no answer).", def)
		case strings.EqualFold(verb, "allow") && rest == "":
			allowed = append(allowed, def)
			fmt.Fprintf(&b, "\n- `%s`: allowed. Try it again.", def)
		case strings.EqualFold(verb, "allow"):
			allowed = append(allowed, rest)
			fmt.Fprintf(&b, "\n- `%s`: allowed as `%s`.", def, rest)
		case strings.EqualFold(verb, "deny"):
			why := rest
			if why == "" {
				why = "no reason given"
			}
			fmt.Fprintf(&b, "\n- `%s`: denied: %s. Don't try it again; find another way or explain in a Round.", def, why)
		default:
			fmt.Fprintf(&b, "\n- `%s`: not allowed; the human said: %s", def, q.Answer)
		}
	}
	err := f.updateClaim(c.ClaimID, func(held *claim) {
		for _, p := range allowed {
			if !slices.Contains(held.AllowedTools, p) {
				held.AllowedTools = append(held.AllowedTools, p)
			}
		}
		held.Denials = nil
		c = *held
		c.AllowedTools = slices.Clone(held.AllowedTools)
	})
	if err != nil {
		f.claimf(c, "%s: can't write the state file: %v", c.Ticket, err)
	}
	return c, b.String()
}

func shorten(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
