package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// The Pull Request Watcher's own operations (code-central-388):
// get_pull_request_watch, the one read it polls, and report_pull_request,
// which records a pull request on every Ticket its pushed work covers.
// Under Pull Request the Watcher opens and follows the pull requests; the
// service never calls the Code Host.

// PullRequestView is a pull request as reads show it: on a Ticket, on the
// Spec page and in the Pull Request Watcher's list.
type PullRequestView struct {
	Number int                    `json:"number"`
	Link   string                 `json:"link"`
	State  store.PullRequestState `json:"state"`
	// MergeCommit is the commit the Code Host merged it at, once merged.
	MergeCommit string `json:"merge_commit,omitempty"`
}

func pullRequestView(p *store.PullRequest) *PullRequestView {
	if p == nil {
		return nil
	}
	return &PullRequestView{Number: p.Number, Link: p.Link, State: p.State, MergeCommit: p.MergeCommit}
}

// PullRequestWatch is get_pull_request_watch: everything the Pull Request
// Watcher needs on each poll. Work is empty unless the Landing Method is
// Pull Request.
type PullRequestWatch struct {
	Project string `json:"project"`
	// On says the Landing Method is Pull Request.
	On         bool   `json:"on"`
	CodeHost   string `json:"code_host"`
	Repository string `json:"repository"`
	Base       string `json:"base"`
	// PATEnv names the environment variable holding the Azure Repos PAT.
	PATEnv string `json:"pat_env"`
	GHPath string `json:"gh_path"`
	// Work is every piece of pushed work to watch, by when it was pushed.
	Work []PushedWorkView `json:"work"`
}

// PushedWorkView is one piece of pushed work: a Ticket's branch, with its
// latest pushed commit, what to open its pull request with, and its
// current pull request, if any.
type PushedWorkView struct {
	Branch string    `json:"branch"`
	Commit string    `json:"commit"`
	Pushed time.Time `json:"pushed"`
	// Tickets are the Tickets the work covers.
	Tickets []string `json:"tickets"`
	Title   string   `json:"title"`
	Body    string   `json:"body"`
	// PullRequest is nil until the Watcher has reported one.
	PullRequest *PullRequestView `json:"pull_request,omitempty"`
}

// GetPullRequestWatch is the Pull Request Watcher's get_pull_request_watch.
// Pushed work is listed while any of its Tickets is Accepted, and while
// one is In Review after its pull request was opened, so that a merge
// while it is In Review is still seen.
func (c *Core) GetPullRequestWatch(_ context.Context, key string) (PullRequestWatch, error) {
	var out PullRequestWatch
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		lv := landingView(p)
		out = PullRequestWatch{Project: key, On: p.Landing.Method == store.LandingPullRequest, CodeHost: lv.CodeHost, Repository: lv.Repository,
			Base: p.Base(), PATEnv: lv.PATEnv, GHPath: r.Options().GHPath, Work: []PushedWorkView{}}
		if out.On {
			out.Work = pushedWork(r, key)
		}
		return nil
	})
	return out, err
}

// branchOf is the branch t's latest pushed commit was pushed as: the
// report's, or the Ticket's own branch for a push reported without one.
func branchOf(t store.Ticket) string {
	if n := len(t.PushedCommits); n > 0 && t.PushedCommits[n-1].Branch != "" {
		return t.PushedCommits[n-1].Branch
	}
	return t.Branch
}

// watchesPullRequest reports whether t's pushed work belongs on the
// Pull Request Watcher's list.
func watchesPullRequest(t store.Ticket) bool {
	if len(t.PushedCommits) == 0 || branchOf(t) == "" {
		return false
	}
	return t.State == store.Accepted || (t.State == store.InReview && t.PullRequest != nil)
}

// pushedWork is every piece of pushed work to watch, in the order of its
// first Ticket's number. Under Land per Spec a Spec's Tickets share its
// Spec Branch, which is one piece of work titled with the Spec's title.
func pushedWork(r store.Reader, key string) []PushedWorkView {
	out := []PushedWorkView{}
	at := map[string]int{}
	p, _ := r.Project(key)
	var covered [][]store.Ticket
	for _, t := range r.Tickets(key) {
		if !watchesPullRequest(t) {
			continue
		}
		branch, latest := branchOf(t), t.PushedCommits[len(t.PushedCommits)-1]
		if i, ok := at[branch]; ok {
			out[i].Tickets = append(out[i].Tickets, DisplayID(key, t.Number))
			covered[i] = append(covered[i], t)
			if latest.Pushed.After(out[i].Pushed) {
				out[i].Commit, out[i].Pushed = latest.Commit, latest.Pushed
			}
			continue
		}
		at[branch] = len(out)
		covered = append(covered, []store.Ticket{t})
		out = append(out, PushedWorkView{Branch: branch, Commit: latest.Commit, Pushed: latest.Pushed, Tickets: []string{DisplayID(key, t.Number)},
			Title: t.Title, Body: ticketPullRequestBody(key, t, r), PullRequest: pullRequestView(t.PullRequest)})
	}
	for i, ts := range covered {
		if !specLanded(r, p, ts[0]) {
			continue
		}
		spec, _ := r.Spec(key, ts[0].Spec)
		out[i].Title, out[i].Body = spec.Title, specPullRequestBody(key, spec, ts, p.DisplayName)
		// The pull request on a Spec shows on its Tickets alike; the most
		// recently reported one stands for it.
		for _, t := range ts {
			if t.PullRequest != nil {
				out[i].PullRequest = pullRequestView(t.PullRequest)
			}
		}
	}
	return out
}

// specPullRequestBody is the body a Spec's pull request opens with: one
// section per waiting Ticket with its ID, title and finish summary, under
// the Spec's ID.
func specPullRequestBody(key string, spec store.Spec, tickets []store.Ticket, project string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Spec %s in %s.\n", DisplayID(key, spec.Number), project)
	for _, t := range tickets {
		fmt.Fprintf(&b, "\n## %s: %s\n\n", DisplayID(key, t.Number), t.Title)
		if s := lastFinishSummary(t); s != "" {
			b.WriteString(s + "\n")
		}
	}
	return b.String()
}

// ticketPullRequestBody is the body a Ticket's pull request opens with:
// its finish summary, then a line naming the Ticket and the Project.
func ticketPullRequestBody(key string, t store.Ticket, r store.Reader) string {
	var b strings.Builder
	if s := lastFinishSummary(t); s != "" {
		b.WriteString(s + "\n\n")
	}
	p, _ := r.Project(key)
	fmt.Fprintf(&b, "Ticket %s in %s.", DisplayID(key, t.Number), p.DisplayName)
	return b.String()
}

// lastFinishSummary is the comment stored with t's last move into In
// Review, which stays its finish summary after t is accepted; "" when
// that move had none.
func lastFinishSummary(t store.Ticket) string {
	for _, m := range slices.Backward(t.Moves) {
		if m.To != store.InReview {
			continue
		}
		for _, cm := range slices.Backward(t.Comments) {
			if cm.Time.Equal(m.Time) && cm.Actor != string(Human) {
				return strings.TrimSpace(cm.Body)
			}
		}
		return ""
	}
	return ""
}

// ReportPullRequestInput is the Pull Request Watcher's
// report_pull_request(branch, number, link, state, merge_commit?, full?).
type ReportPullRequestInput struct {
	// Branch is the pushed work's branch.
	Branch string                 `json:"branch"`
	Number int                    `json:"number"`
	Link   string                 `json:"link"`
	State  store.PullRequestState `json:"state"`
	// MergeCommit is the commit the Code Host merged the pull request at;
	// needed with the merged state.
	MergeCommit string `json:"merge_commit,omitempty"`
	// Full asks for the Tickets covered in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// PullRequestReport is report_pull_request's record: the Tickets it
// recorded the pull request on.
type PullRequestReport struct {
	Branch  string          `json:"branch"`
	Tickets []string        `json:"tickets"`
	Record  PullRequestView `json:"pull_request"`
}

// ReportPullRequest is the Pull Request Watcher's report_pull_request: the
// pull request of one piece of pushed work, recorded on every Ticket the
// work covers: those not yet Done or Cancelled whose latest pushed commit
// was pushed as the branch. A merged pull request stores the merge commit
// the Code Host reports. Any state may follow any other, so a closed pull
// request reopened is reported open again.
func (c *Core) ReportPullRequest(_ context.Context, key string, in ReportPullRequestInput) (Written[PullRequestReport], error) {
	branch, link, merge := strings.TrimSpace(in.Branch), strings.TrimSpace(in.Link), strings.TrimSpace(in.MergeCommit)
	switch {
	case branch == "":
		return Written[PullRequestReport]{}, invalid("branch", "report_pull_request needs the pushed work's branch")
	case in.Number <= 0:
		return Written[PullRequestReport]{}, invalid("number", "report_pull_request needs the pull request's number on the Code Host")
	case link == "":
		return Written[PullRequestReport]{}, invalid("link", "report_pull_request needs the pull request's link")
	case !slices.Contains(store.PullRequestStates, in.State):
		return Written[PullRequestReport]{}, invalid("state", "the state is one of %v, not %q", store.PullRequestStates, in.State)
	case in.State == store.PRMerged && merge == "":
		return Written[PullRequestReport]{}, invalid("merge_commit", "a merged pull request needs the merge commit the Code Host reports")
	case in.State != store.PRMerged && merge != "":
		return Written[PullRequestReport]{}, invalid("merge_commit", "only a merged pull request has a merge commit")
	}
	pr := store.PullRequest{Number: in.Number, Link: link, State: in.State, MergeCommit: merge}
	var covered []int
	if err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		for _, t := range r.Tickets(key) {
			if t.State != store.Done && t.State != store.Cancelled && len(t.PushedCommits) > 0 && branchOf(t) == branch {
				covered = append(covered, t.Number)
			}
		}
		return nil
	}); err != nil {
		return Written[PullRequestReport]{}, err
	}
	if len(covered) == 0 {
		return Written[PullRequestReport]{}, notFound("no Ticket of %s has work pushed as branch %q", key, branch)
	}
	report := PullRequestReport{Branch: branch, Tickets: []string{}, Record: *pullRequestView(&pr)}
	var changed []string
	// A store update writes one record, so each Ticket has its own.
	for _, n := range covered {
		moved := false
		err := c.store.Update(func(w store.Writer) error {
			t, ok := w.Ticket(key, n)
			if !ok || (t.PullRequest != nil && *t.PullRequest == pr) {
				return nil
			}
			t.PullRequest, t.Updated, moved = &pr, c.now(), true
			return w.PutTicket(key, t)
		})
		if err != nil {
			return Written[PullRequestReport]{}, err
		}
		id := DisplayID(key, n)
		report.Tickets = append(report.Tickets, id)
		if moved {
			changed = append(changed, id)
			c.events.Publish(events.Event{Type: events.TicketChanged, Project: key, ID: id})
		}
	}
	if in.Full {
		return Written[PullRequestReport]{Record: &report}, nil
	}
	ack := Acknowledgement{ID: key, Updated: c.now(), Changed: []string{}}
	if len(changed) > 0 {
		ack.Changed = []string{"pull_request"}
	}
	return Written[PullRequestReport]{Acknowledgement: ack}, nil
}

// pullRequestOpen says t waits on a pull request that is still open on the
// Code Host, conflicting or not.
func pullRequestOpen(t store.Ticket) bool {
	return t.PullRequest != nil && (t.PullRequest.State == store.PROpen || t.PullRequest.State == store.PRConflicting)
}

// openPullRequests is the notice after a switch away from Pull Request:
// the open pull request of each Accepted Ticket, by Ticket. A Spec's one
// pull request shows for each of its Tickets. It is empty when lp lands by
// Pull Request.
func openPullRequests(r store.Reader, key string, lp LandingPolicy) []OpenPullRequest {
	out := []OpenPullRequest{}
	if lp.LandsByPullRequest() {
		return out
	}
	for _, t := range r.Tickets(key) {
		if t.State == store.Accepted && pullRequestOpen(t) {
			out = append(out, OpenPullRequest{Ticket: DisplayID(key, t.Number), Link: t.PullRequest.Link})
		}
	}
	return out
}
