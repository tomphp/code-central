package core

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Write-backs (Spec code-central-279): Code Central works out from the
// work's state, never from events, what each linked issue should have,
// and get_issue_watch lists it; the Issue Watcher makes it so. It is
// worked out per Integration (Spec code-central-894): each Watcher's
// watch lists only its own Integration's Links, and get_work_item_watch
// lists a work item's with its status label as a tag and never a close.
// A comment is written once: report_issue_written records its key on the
// item's Link. Labels and closing need no record, as the Watcher changes
// only what differs. Nothing is ever due from Rounds, answers, Failure
// Reports, Transcripts or Claims.

// The keys of the comments written to an issue.
const (
	// CommentContext is the key of the comment with the context the human
	// gave To Requirement.
	CommentContext = "context"
	// CommentWontFix is the key of the comment with the human's Won't fix
	// reason.
	CommentWontFix = "wont-fix"
	// CommentGrilling is the key of the comment with the grilling
	// Stage's resolution.
	CommentGrilling = "grilling"
	// CommentSpec is the key of the comment with the Spec's body as the
	// spec Stage left it.
	CommentSpec = "spec"
	// CommentLanded starts the key of the comment for each implementation
	// Ticket Done, such as landed:acme-12.
	CommentLanded = "landed:"
	// CommentClosed is the key of the comment listing what landed, once
	// the Spec is Finished.
	CommentClosed = "closed"
)

// that a comment links back to. Call it before serving; without it,
// the link is relative to the UI.
func (c *Core) SetUIURL(url string) { c.uiURL = strings.TrimRight(url, "/") }

// The UI's query parameters that open the item a comment links back to:
// a Ticket, or a Requirement in the Requirements list.
const (
	ticketLink      = "ticket="
	requirementLink = "list=requirements&requirement="
)

// issueComment is the comment key on target, with text, as it is posted.
// Its link back opens target in the UI by open, ticketLink or
// requirementLink. Hidden, it is the opaque marker and the text alone.
func (c *Core) issueComment(hide bool, target, open, key, text string) api.IssueComment {
	if hide {
		return api.IssueComment{Key: key, Body: domain.OpaqueMarker(target, key) + "\n" + strings.TrimSpace(text)}
	}
	link := fmt.Sprintf("[%s in Code Central](%s/?view=inbox&%s%s)", target, c.uiURL, open, url.QueryEscape(target))
	return api.IssueComment{Key: key, Body: domain.IssueMarker(target, key) + "\n" + strings.TrimSpace(text) + "\n\n" + link}
}

// triageComment is the comment the human's triage action via gave when
// it Cancelled t, if it ever did: empty when it gave none. latest
// reports whether that Cancel is still t's last Move.
func triageComment(t domain.Ticket, via api.TriageAction) (comment string, latest, ok bool) {
	for i, m := range slices.Backward(t.Moves) {
		if m.To != domain.Cancelled || m.Via != string(via) {
			continue
		}
		latest = i == len(t.Moves)-1 && t.State == domain.Cancelled
		for _, cm := range slices.Backward(t.Comments) {
			if cm.Time.Equal(m.Time) && cm.Actor == string(api.Human) {
				return cm.Body, latest, true
			}
		}
		return "", latest, true
	}
	return "", false, false
}

// integrationWrites is what is due on each of integration's Links in key,
// in number order: on the Tickets on its inbox Spec, and on the Specs
// with an outbound Link through it. There is none before the Integration
// is first turned on. hide writes as Hide Code Central does: each comment
// in the hidden form and no status label. The comments due are the same
// either way.
func (c *Core) integrationWrites(r store.Reader, key, integration string, hide bool) []api.IssueWrites {
	out := []api.IssueWrites{}
	in := inboxOf(r, key, integration)
	if in.spec == 0 {
		return out
	}
	byNumber := map[int]api.IssueWrites{}
	link := &store.LinkFilter{Integration: integration}
	for _, s := range r.FindSpecs(key, store.SpecFilter{Link: link}) {
		if w, ok := c.specIssueWrites(r, key, s, hide); ok {
			byNumber[s.Number] = w
		}
	}
	for _, t := range r.FindTickets(key, store.TicketFilter{Spec: []int{in.spec}, Link: link, Parts: []store.Part{store.Moves, store.Comments}}) {
		if w, ok := c.ticketIssueWrites(r, key, t, hide); ok {
			byNumber[t.Number] = w
		}
	}
	for n := 1; n < r.NextNumber(key); n++ {
		if w, ok := byNumber[n]; ok {
			if hide {
				w.Label = ""
			}
			out = append(out, w)
		}
	}
	return out
}

// ticketIssueWrites is what is due on the issue t, a Ticket on key's
// inbox Spec, is linked to, its comments hidden as hide asks; ok is false
// when nothing is.
func (c *Core) ticketIssueWrites(r store.Reader, key string, t domain.Ticket, hide bool) (w api.IssueWrites, ok bool) {
	if !t.Link.Attached() {
		return api.IssueWrites{}, false
	}
	id := DisplayID(key, t.Number)
	w = api.IssueWrites{Target: id, Link: *linkView(t.Link), Comments: []api.IssueComment{}}
	if reason, latest, _ := triageComment(t, api.OptionWontFix); latest {
		w.Label, w.Close = domain.LabelWontFix, api.CloseNotPlanned
		if strings.TrimSpace(reason) != "" && !slices.Contains(t.Link.Written, CommentWontFix) {
			w.Comments = append(w.Comments, c.issueComment(hide, id, ticketLink, CommentWontFix, writtenText(r, key, reason, hide)))
		}
	} else if t.State == domain.Draft || t.State == domain.Ready {
		w.Label = domain.LabelDraft
	}
	return dueWhile(t.Link.ExternalState, w)
}

// specIssueWrites is what is due on the issue s, a Spec with an outbound
// Link that isn't detached, is linked to, its comments hidden as hide
// asks; ok is false when nothing is.
// The comments come due as the Requirement goes on: the context To
// Requirement was given, the grilling resolution and the Spec's body as
// their Stages are Done, each implementation Ticket as it is Done, and
// what landed once the Spec is Finished. The label follows the work, and
// the issue is closed once the Spec is Finished: as completed if
// anything landed, as not planned if nothing did.
func (c *Core) specIssueWrites(r store.Reader, key string, s domain.Spec, hide bool) (w api.IssueWrites, ok bool) {
	if !s.Link.Attached() || !s.Link.Outbound {
		return api.IssueWrites{}, false
	}
	id := DisplayID(key, s.Number)
	w = api.IssueWrites{Target: id, Link: *linkView(s.Link), Comments: []api.IssueComment{}}
	due := func(k, text string) {
		if strings.TrimSpace(text) != "" && !slices.Contains(s.Link.Written, k) {
			w.Comments = append(w.Comments, c.issueComment(hide, id, requirementLink, k, text))
		}
	}
	due(CommentContext, writtenText(r, key, requirementContext(r, key, s.Link), hide))
	tickets := specTickets(r, key, s.Number, store.Moves, store.Comments)
	stages, stagesDone := map[domain.StageName]domain.Ticket{}, true
	var landed []domain.Ticket
	for _, t := range tickets {
		if stage := StageOf(t.Labels); stage != "" {
			stages[stage] = t
			stagesDone = stagesDone && (t.State == domain.Done || t.State == domain.Cancelled)
		} else if t.State == domain.Done {
			landed = append(landed, t)
		}
	}
	if g, done := stages["grilling"]; done && g.State == domain.Done {
		due(CommentGrilling, writtenText(r, key, resolution(g), hide))
	}
	if sp, done := stages["spec"]; done && sp.State == domain.Done {
		due(CommentSpec, writtenText(r, key, bodyAt(s, doneMove(sp).Time), hide))
	}
	slices.SortStableFunc(landed, func(a, b domain.Ticket) int { return doneMove(a).Time.Compare(doneMove(b).Time) })
	p, _ := r.Project(key)
	var list []string
	for _, t := range landed {
		line := landedLine(r, p, t, hide)
		due(CommentLanded+DisplayID(key, t.Number), "Landed "+line+".")
		list = append(list, "- "+line)
	}
	_, finishedSpec := summarise(tickets)
	if finishedSpec && len(landed) > 0 {
		text := "Finished. What landed:\n\n" + strings.Join(list, "\n")
		if severalRepositories(p) {
			var names []string
			for _, name := range specRepositories(p, landed) {
				names = append(names, repositoryLabel(p, name))
			}
			if len(names) > 0 {
				text += "\n\nChanged Repositories: " + strings.Join(names, ", ")
			}
		}
		due(CommentClosed, text)
	}
	w.Label, w.Close = requirementStatus(finishedSpec, len(landed) > 0, stagesDone)
	return dueWhile(s.Link.ExternalState, w)
}

// requirementStatus is the status label and closing due on the issue of
// a Requirement whose Spec is finished or not, with something landed or
// not, and whose Stages are all Done or Cancelled or not.
func requirementStatus(finished, landed, stagesDone bool) (string, api.IssueClose) {
	switch {
	case finished && landed:
		return domain.LabelDone, api.CloseCompleted
	case finished:
		return domain.LabelWontFix, api.CloseNotPlanned
	case stagesDone:
		return domain.LabelBuilding, ""
	default:
		return domain.LabelRequirement, ""
	}
}

// resolution is the grilling Stage Ticket t's resolution comment: the
// latest agent comment on it that isn't its finish summary, nor any
// other note stored under a Claim; empty when there is none.
func resolution(t domain.Ticket) string {
	for _, cm := range slices.Backward(t.Comments) {
		if cm.Actor == string(api.Agent) && cm.Claim == 0 {
			return cm.Body
		}
	}
	return ""
}

// doneMove is the Move that last made t, a Done Ticket, Done, with its
// land commit if it landed; a Ticket with none, such as one imported
// Done, is taken as Done when it was last updated.
func doneMove(t domain.Ticket) domain.Move {
	for _, m := range slices.Backward(t.Moves) {
		if m.To == domain.Done {
			return m
		}
	}
	return domain.Move{To: domain.Done, Time: t.Updated}
}

// bodyAt is s's body as it was at the time at, undoing each later edit
// to it.
func bodyAt(s domain.Spec, at time.Time) string {
	body := s.Body
	for _, e := range slices.Backward(s.Edits) {
		if !e.Time.After(at) {
			break
		}
		if prev, ok := e.Previous["body"]; ok {
			body = prev
		}
	}
	return body
}

// landedLine is t, a Done implementation Ticket, as a write-back names
// it: its title and ID, and its land commit if it has one. With several
// Repositories it gives one commit per Changed Repository that landed,
// in landing order, each as repositoryLabel@commit. Hidden, it has no ID
// and its title names other items by title as writtenText does.
func landedLine(r store.Reader, p domain.Project, t domain.Ticket, hide bool) string {
	line := fmt.Sprintf("**%s** (%s)", t.Title, DisplayID(p.Key, t.Number))
	if hide {
		line = fmt.Sprintf("**%s**", writtenText(r, p.Key, t.Title, true))
	}
	if !severalRepositories(p) {
		if commit := doneMove(t).Commit; commit != "" {
			line += " in " + commit
		}
		return line
	}
	changed := slices.Clone(changedRepositories(p.Key, t))
	inLandingOrder(p, changed, repositoryName)
	var commits []string
	for _, r := range changed {
		commit := ""
		switch {
		case r.Landed != nil:
			commit = r.Landed.Commit
		case r.Name == p.Key:
			commit = doneMove(t).Commit
		}
		if commit != "" {
			commits = append(commits, repositoryLabel(p, r.Name)+"@"+commit)
		}
	}
	if len(commits) > 0 {
		line += " in " + strings.Join(commits, ", ")
	}
	return line
}

// repositoryLabel names the Repository called name for an issue's
// reader: owner/repo when its Clone URL is on GitHub, else its Repository
// Name.
func repositoryLabel(p domain.Project, name string) string {
	for _, rv := range repositoriesView(p) {
		if rv.Name != name {
			continue
		}
		if host, repository := codeHostOf(rv.CloneURL); host == api.CodeHostGitHub {
			return repository
		}
	}
	return name
}

// requirementContext is the context the human gave To Requirement for
// the issue l links to: the comment to_requirement made on its Incoming
// Issue, empty when it gave none. It stays due if the Incoming Issue is
// moved on later.
func requirementContext(r store.Reader, key string, l domain.Link) string {
	for _, t := range r.FindTickets(key, store.TicketFilter{Link: &store.LinkFilter{Integration: l.Integration, ID: l.ID}, Parts: []store.Part{store.Moves, store.Comments}}) {
		if extra, _, ok := triageComment(t, api.OptionToRequirement); ok {
			return extra
		}
	}
	return ""
}

// dueWhile is w, the writes due on an issue last seen as state: its label
// and closing are due only while it is open, and nothing is due on a
// deleted issue. ok is false when nothing is due.
func dueWhile(state domain.ExternalState, w api.IssueWrites) (api.IssueWrites, bool) {
	if state != domain.ExternalOpen {
		w.Label, w.Close = "", ""
	}
	if state == domain.ExternalDeleted {
		w.Comments = []api.IssueComment{}
	}
	return w, len(w.Comments) > 0 || w.Label != "" || w.Close != ""
}

// ReportIssueWritten is the Issue Watcher's report_issue_written(target,
// key), accepted from the integration Actor only: the comment key is
// written to target's issue, so it is never due again. Reporting it
// again changes nothing.
func (c *Core) ReportIssueWritten(ctx context.Context, key string, in api.ReportIssueWrittenInput) (api.LinkedRow, error) {
	if err := requireActor(ctx, "report_issue_written", api.Integration); err != nil {
		return api.LinkedRow{}, err
	}
	if strings.TrimSpace(in.Key) == "" {
		return api.LinkedRow{}, invalid("key", "name the key of the comment written")
	}
	return c.updateLink(ctx, key, in.Target, func(l *domain.Link) (map[string]string, error) {
		if slices.Contains(l.Written, in.Key) {
			return nil, nil
		}
		l.Written = append(l.Written, in.Key)
		return map[string]string{}, nil
	})
}
