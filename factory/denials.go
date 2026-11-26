package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory/harness"
)

// denialInput is the call's input re-encoded with sorted keys, so the
// same call reads the same however it was printed.
func denialInput(d harness.Denial) string {
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

// denialsOf is the calls out's turn had denied that the Harness can
// allow, with the rules that allow them: none for a Harness with no
// denial list.
func (f *factory) denialsOf(out harness.Outcome) (harness.DenialRules, []harness.Denial) {
	rules := f.runner.harness.Denials()
	if rules == nil {
		return nil, nil
	}
	return rules, out.Denials
}

// distinct is the denied calls with repeats of the same call dropped.
func distinct(ds []harness.Denial) []harness.Denial {
	seen := map[string]bool{}
	var out []harness.Denial
	for _, d := range ds {
		k := d.Tool + "\x00" + denialInput(d)
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
func (f *factory) postDenials(ctx context.Context, t *turn, rules harness.DenialRules, ds []harness.Denial) bool {
	c := t.claim
	ds = distinct(ds)
	var qs []api.QuestionInput
	var patterns []string
	for _, d := range ds {
		p := rules.Rule(d)
		patterns = append(patterns, p)
		qs = append(qs, api.QuestionInput{
			Title: "Allow " + shorten(p, 100) + "?",
			Body: fmt.Sprintf("Auto mode denied this call, so the agent's turn stopped:\n\n```\n%s %s\n```\n\n"+
				"Answer one of:\n\n- `allow`: allow `%s` for the rest of this Claim\n- `allow <pattern>`: allow this %s instead, such as `%s`\n- `deny <why>`: don't allow it, and tell the agent why\n",
				d.Tool, denialInput(d), p, rules.RuleName(), rules.RuleExample()),
		})
	}
	res, err := f.client.PostRound(ctx, f.project, api.PostRoundInput{Ticket: api.Ref(c.Ticket), ClaimID: c.ClaimID, SessionID: c.SessionID, Questions: qs})
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
func (f *factory) commentDenials(ctx context.Context, t *turn, rules harness.DenialRules, ds []harness.Denial) {
	c := t.claim
	body := "code-central-factory: auto mode denied these calls during the last turn (session " + c.SessionID + "):\n" + denialList(rules, ds)
	if _, err := f.client.Comment(ctx, f.project, api.CommentInput{Ticket: api.Ref(c.Ticket), Body: body}); err != nil {
		f.claimf(c, "%s: comment: %v", c.Ticket, err)
	}
}

// denialList is the distinct denied calls' patterns, a Markdown list item
// each, every one on a line of its own.
func denialList(rules harness.DenialRules, ds []harness.Denial) string {
	var b strings.Builder
	for _, d := range distinct(ds) {
		fmt.Fprintf(&b, "\n- `%s`", rules.Rule(d))
	}
	return b.String()
}

// takeDenialAnswers reads the answers to c's Round about denied calls,
// if res is that Round: what is allowed joins the Claim's allowed tools
// and is saved. It returns the Claim as it now is and a note for the
// agent on what was decided; res isn't that Round, it changes nothing.
func (f *factory) takeDenialAnswers(c claim, res api.PickUpAnswersResult) (claim, string) {
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
		case q.Status != domain.QuestionAnswered:
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
