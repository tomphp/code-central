package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// The Pull Request Watcher's own operations (code-central-388):
// get_pull_request_watch, the one read it polls, and report_pull_request,
// which records a pull request on every Ticket its pushed work covers.
// Under Pull Request the Watcher opens and follows the pull requests; the
// service never calls the Code Host. With several Repositories (Spec
// code-central-848, ADR 0050) a Ticket has a pull request per Changed
// Repository, each into that Repository's own Base, and each body lists
// its siblings.

func pullRequestView(p *domain.PullRequest) *api.PullRequestView {
	if p == nil {
		return nil
	}
	return &api.PullRequestView{Number: p.Number, Link: p.Link, State: p.State, MergeCommit: p.MergeCommit}
}

// pullRequestViewIn is p's view in Project pr, naming its Repository
// while pr has several.
func pullRequestViewIn(pr domain.Project, p domain.PullRequest) api.PullRequestView {
	v := *pullRequestView(&p)
	if severalRepositories(pr) {
		v.Repository = recordRepository(pr.Key, p.Repository)
	}
	return v
}

// pullRequestIn is t's pull request in the Repository name, of a Project
// whose Home Repository is home; nil when it has none there.
func pullRequestIn(home string, t domain.Ticket, name string) *domain.PullRequest {
	for i, p := range t.PullRequests {
		if recordRepository(home, p.Repository) == name {
			return &t.PullRequests[i]
		}
	}
	return nil
}

// homePullRequest is t's pull request in the Home Repository, home: the
// one the Ticket's top-level fields show.
func homePullRequest(home string, t domain.Ticket) *domain.PullRequest {
	return pullRequestIn(home, t, home)
}

// anyPullRequest reports whether one of t's pull requests is in state.
func anyPullRequest(t domain.Ticket, state domain.PullRequestState) bool {
	return slices.ContainsFunc(t.PullRequests, func(p domain.PullRequest) bool { return p.State == state })
}

// closedPullRequests is t's pull requests closed without merging, in
// p's landing order: each is a Pull request closed.
func closedPullRequests(p domain.Project, t domain.Ticket) []api.PullRequestView {
	out := []api.PullRequestView{}
	for _, pr := range t.PullRequests {
		if pr.State == domain.PRClosed {
			out = append(out, pullRequestViewIn(p, pr))
		}
	}
	inLandingOrder(p, out, func(v api.PullRequestView) string { return recordRepository(p.Key, v.Repository) })
	return out
}

// setPullRequest records pr as t's pull request in its Repository,
// replacing the one there, and reports whether that changed anything. A
// mark to close holds while the pull request is still open.
func setPullRequest(t *domain.Ticket, pr domain.PullRequest) bool {
	i := slices.IndexFunc(t.PullRequests, func(o domain.PullRequest) bool { return o.Repository == pr.Repository })
	if i < 0 {
		t.PullRequests = append(t.PullRequests, pr)
		return true
	}
	pr.Close = t.PullRequests[i].Close && (pr.State == domain.PROpen || pr.State == domain.PRConflicting)
	if t.PullRequests[i] == pr {
		return false
	}
	t.PullRequests[i] = pr
	return true
}

// GetPullRequestWatch is the Pull Request Watcher's get_pull_request_watch.
// Pushed work is listed while any of its Tickets is Accepted, and while
// one is In Review after its pull request was opened, so that a merge
// while it is In Review is still seen.
func (c *Core) GetPullRequestWatch(_ context.Context, key string) (api.PullRequestWatch, error) {
	var out api.PullRequestWatch
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		lv := landingView(p)
		ps := landingPolicies(p)
		out = api.PullRequestWatch{Project: key, On: ps.RunsPullRequestWatcher(), CodeHost: lv.CodeHost, Repository: lv.Repository,
			Base: p.Base(), Repositories: []api.PullRequestRepositoryView{}, PATEnv: lv.PATEnv, GHPath: r.Options().GHPath, Work: []api.PushedWorkView{}, Close: []api.PullRequestToClose{}}
		if out.On {
			for _, rv := range repositoriesView(p) {
				if !ps.In(rv.Name).LandsByPullRequest() {
					continue
				}
				_, repository := codeHostOf(rv.CloneURL)
				out.Repositories = append(out.Repositories, api.PullRequestRepositoryView{Name: rv.Name, CodeHost: rv.CodeHost, Repository: repository, Base: rv.Base})
			}
			out.Work = pushedWork(r, key)
			out.Close = pullRequestsToClose(r, p)
		}
		return nil
	})
	return out, err
}

// branchOf is the branch t's latest pushed commit was pushed as: the
// report's, or the Ticket's own branch for a push reported without one.
func branchOf(t domain.Ticket) string {
	if n := len(t.PushedCommits); n > 0 && t.PushedCommits[n-1].Branch != "" {
		return t.PushedCommits[n-1].Branch
	}
	return t.Branch
}

// latestPushedTo is t's latest pushed commit in the Repository name, of
// a Project whose Home Repository is home; ok is false when none was
// pushed there.
func latestPushedTo(home string, t domain.Ticket, name string) (domain.PushedCommit, bool) {
	for i := len(t.PushedCommits) - 1; i >= 0; i-- {
		if pc := t.PushedCommits[i]; recordRepository(home, pc.Repository) == name {
			return pc, true
		}
	}
	return domain.PushedCommit{}, false
}

// pushedRepositories is the Repositories t has pushed commits in, in
// the order of their first push.
func pushedRepositories(home string, t domain.Ticket) []string {
	var names []string
	for _, pc := range t.PushedCommits {
		if name := recordRepository(home, pc.Repository); !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// branchIn is the branch t's latest pushed commit in the Repository name
// was pushed as: the report's, or for a push reported without one, the
// Ticket's branch there.
func branchIn(home string, t domain.Ticket, name string) string {
	if pc, ok := latestPushedTo(home, t, name); ok && pc.Branch != "" {
		return pc.Branch
	}
	for _, r := range changedRepositories(home, t) {
		if r.Name == name {
			return r.Branch
		}
	}
	return ""
}

// watchesPullRequest reports whether t's pushed work belongs on the
// Pull Request Watcher's list.
func watchesPullRequest(t domain.Ticket) bool {
	if len(t.PushedCommits) == 0 {
		return false
	}
	return t.State == domain.Accepted || (t.State == domain.InReview && len(t.PullRequests) > 0)
}

// pushedWork is every piece of pushed work to watch, in the order of its
// first Ticket's number, then of its Repositories' first push. Under
// Land per Spec a Spec's Tickets share its Spec Branch in each
// Repository, which is one piece of work titled with the Spec's title.
func pushedWork(r store.Reader, key string) []api.PushedWorkView {
	out := []api.PushedWorkView{}
	type at struct{ repository, branch string }
	index := map[at]int{}
	p, _ := r.Project(key)
	var covered [][]domain.Ticket
	// The pull request's body is its finish summary.
	watched := store.TicketFilter{States: []domain.State{domain.Accepted, domain.InReview}, HasPushes: true, Parts: []store.Part{store.Moves, store.Comments}}
	for _, t := range r.FindTickets(key, watched) {
		if !watchesPullRequest(t) {
			continue
		}
		for _, name := range pushedRepositories(key, t) {
			branch := branchIn(key, t, name)
			if branch == "" {
				continue
			}
			latest, _ := latestPushedTo(key, t, name)
			if i, ok := index[at{name, branch}]; ok {
				out[i].Tickets = append(out[i].Tickets, DisplayID(key, t.Number))
				covered[i] = append(covered[i], t)
				if latest.Pushed.After(out[i].Pushed) {
					out[i].Commit, out[i].Pushed = latest.Commit, latest.Pushed
				}
				continue
			}
			index[at{name, branch}] = len(out)
			covered = append(covered, []domain.Ticket{t})
			out = append(out, api.PushedWorkView{Repository: name, Branch: branch, Commit: latest.Commit, Pushed: latest.Pushed, Tickets: []string{DisplayID(key, t.Number)},
				Title: t.Title, PullRequest: pullRequestView(pullRequestIn(key, t, name))})
		}
	}
	spec := func(ts []domain.Ticket) (domain.Spec, bool) {
		if !specLanded(r, p, ts[0]) {
			return domain.Spec{}, false
		}
		s, _ := r.Spec(key, ts[0].Spec)
		return s, true
	}
	for i, ts := range covered {
		s, landsWithSpec := spec(ts)
		if !landsWithSpec {
			out[i].Body = ticketPullRequestBody(key, ts[0], p.DisplayName)
		} else {
			out[i].Title, out[i].Body = s.Title, specPullRequestBody(key, s, ts, p.DisplayName)
			// The pull request on a Spec shows on its Tickets alike; the
			// most recently reported one stands for it.
			for _, t := range ts {
				if pr := pullRequestIn(key, t, out[i].Repository); pr != nil {
					out[i].PullRequest = pullRequestView(pr)
				}
			}
		}
		if s, ok := r.Spec(key, ts[0].Spec); ok {
			out[i].Body += issueMention(p, s, out[i].Repository)
		}
		out[i].Body += siblingsSection(p, ts, out[i].Repository)
	}
	return out
}

// pullRequestsToClose is every open pull request of p's unfinished
// Tickets marked to close, by Ticket.
func pullRequestsToClose(r store.Reader, p domain.Project) []api.PullRequestToClose {
	out := []api.PullRequestToClose{}
	for _, t := range r.FindTickets(p.Key, store.TicketFilter{States: unfinished}) {
		for _, pr := range t.PullRequests {
			if !pr.Close || (pr.State != domain.PROpen && pr.State != domain.PRConflicting) {
				continue
			}
			name := recordRepository(p.Key, pr.Repository)
			out = append(out, api.PullRequestToClose{Repository: name, Branch: branchIn(p.Key, t, name), Number: pr.Number, Link: pr.Link})
		}
	}
	return out
}

// specPullRequestBody is the body a Spec's pull request opens with: one
// section per waiting Ticket with its ID, title and finish summary, under
// the Spec's ID.
func specPullRequestBody(key string, spec domain.Spec, tickets []domain.Ticket, project string) string {
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
func ticketPullRequestBody(key string, t domain.Ticket, project string) string {
	var b strings.Builder
	if s := lastFinishSummary(t); s != "" {
		b.WriteString(s + "\n\n")
	}
	fmt.Fprintf(&b, "Ticket %s in %s.", DisplayID(key, t.Number), project)
	return b.String()
}

// issueMention is the line a pull request in the Repository name has
// for spec's linked GitHub issue: owner/repo#n when the Repository is on
// GitHub, else the issue's URL. It never has a closing keyword, as
// Write-back closes the issue (ADR 0050). "" when spec has no attached
// GitHub issue.
func issueMention(p domain.Project, spec domain.Spec, name string) string {
	if !spec.Link.Attached() || spec.Link.Integration != string(api.CodeHostGitHub) {
		return ""
	}
	ref := spec.Link.ID
	if host, _ := codeHostOf(cloneURLOf(p, name)); host != api.CodeHostGitHub && spec.Link.URL != "" {
		ref = spec.Link.URL
	}
	return "\n\nIssue: " + ref
}

// cloneURLOf is the Clone URL of p's Repository name: the reported Push
// remote URL for the Home Repository.
func cloneURLOf(p domain.Project, name string) string {
	for _, rv := range repositoriesView(p) {
		if rv.Name == name {
			return rv.CloneURL
		}
	}
	return ""
}

// siblingsSection is the "Pull requests" list a pull request in the
// Repository name has when the work of tickets spans several
// Repositories: each of them by Repository Name in landing order, with
// its pull request's link, or not opened yet. "" for work in one
// Repository, so a single-repository body is as it always was.
func siblingsSection(p domain.Project, tickets []domain.Ticket, name string) string {
	names := specRepositories(p, tickets)
	if len(names) < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Pull requests\n")
	for _, n := range names {
		fmt.Fprintf(&b, "\n- %s: ", n)
		var link string
		for _, t := range tickets {
			if pr := pullRequestIn(p.Key, t, n); pr != nil {
				link = pr.Link
			}
		}
		switch {
		case n == name:
			b.WriteString("this pull request")
		case link != "":
			b.WriteString(link)
		default:
			b.WriteString("not opened yet")
		}
	}
	return b.String()
}

// lastFinishSummary is the comment stored with t's last move into In
// Review, which stays its finish summary after t is accepted; "" when
// that move had none. It is stored under the move's Claim, which tells
// it from a comment made in the same second, such as the Factory's on a
// finish that skipped code-review.
func lastFinishSummary(t domain.Ticket) string {
	for _, m := range slices.Backward(t.Moves) {
		if m.To != domain.InReview {
			continue
		}
		for _, cm := range slices.Backward(t.Comments) {
			if cm.Time.Equal(m.Time) && cm.Claim == m.Claim && cm.Actor != string(api.Human) {
				return strings.TrimSpace(cm.Body)
			}
		}
		return ""
	}
	return ""
}

// ReportPullRequest is the Pull Request Watcher's report_pull_request: the
// pull request of one piece of pushed work, recorded on every Ticket the
// work covers: those not yet Done or Cancelled whose latest pushed commit
// in the Repository was pushed as the branch. A merged pull request
// stores the merge commit the Code Host reports. Any state may follow any
// other, so a closed pull request reopened is reported open again.
func (c *Core) ReportPullRequest(_ context.Context, key string, in api.ReportPullRequestInput) (api.Written[api.PullRequestReport], error) {
	branch, link, merge := strings.TrimSpace(in.Branch), strings.TrimSpace(in.Link), strings.TrimSpace(in.MergeCommit)
	switch {
	case branch == "":
		return api.Written[api.PullRequestReport]{}, invalid("branch", "report_pull_request needs the pushed work's branch")
	case in.Number <= 0:
		return api.Written[api.PullRequestReport]{}, invalid("number", "report_pull_request needs the pull request's number on the Code Host")
	case link == "":
		return api.Written[api.PullRequestReport]{}, invalid("link", "report_pull_request needs the pull request's link")
	case !slices.Contains(domain.PullRequestStates, in.State):
		return api.Written[api.PullRequestReport]{}, invalid("state", "the state is one of %v, not %q", domain.PullRequestStates, in.State)
	case in.State == domain.PRMerged && merge == "":
		return api.Written[api.PullRequestReport]{}, invalid("merge_commit", "a merged pull request needs the merge commit the Code Host reports")
	case in.State != domain.PRMerged && merge != "":
		return api.Written[api.PullRequestReport]{}, invalid("merge_commit", "only a merged pull request has a merge commit")
	}
	pr := domain.PullRequest{Number: in.Number, Link: link, State: in.State, MergeCommit: merge}
	var covered []int
	if err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		p, _ := r.Project(key)
		name, err := repositoryOf(p, "repository", in.Repository)
		if err != nil {
			return err
		}
		if name != p.Key {
			pr.Repository = name
		}
		// A push reported without a branch was pushed as the Ticket's, so
		// the Tickets are found by their pushes, not by the branch.
		for _, t := range r.FindTickets(key, store.TicketFilter{States: unfinished, HasPushes: true}) {
			if pc, ok := latestPushedTo(key, t, name); ok && pushedAs(pc, t, branch) {
				covered = append(covered, t.Number)
			}
		}
		if len(covered) == 0 {
			return notFound("no Ticket of %s has work pushed as branch %q in %s", key, branch, name)
		}
		return nil
	}); err != nil {
		return api.Written[api.PullRequestReport]{}, err
	}
	report := api.PullRequestReport{Branch: branch, Tickets: []string{}, Record: *pullRequestView(&pr)}
	var changed []string
	// A store update writes one record, so each Ticket has its own.
	for _, n := range covered {
		moved := false
		err := c.store.Update(func(w store.Writer) error {
			moved = false
			// The pull request is on the head: no History is written.
			t, ok := w.Ticket(key, n)
			if !ok || !setPullRequest(&t, pr) {
				return nil
			}
			t.Updated, moved = c.now(), true
			return w.PutTicket(key, t)
		})
		if err != nil {
			return api.Written[api.PullRequestReport]{}, err
		}
		id := DisplayID(key, n)
		report.Tickets = append(report.Tickets, id)
		if moved {
			changed = append(changed, id)
			c.events.Publish(events.Event{Type: events.TicketChanged, Project: key, ID: id})
		}
	}
	if in.Full {
		return api.Written[api.PullRequestReport]{Record: &report}, nil
	}
	ack := api.Acknowledgement{ID: key, Updated: c.now(), Changed: []string{}}
	if len(changed) > 0 {
		ack.Changed = []string{"pull_request"}
	}
	return api.Written[api.PullRequestReport]{Acknowledgement: ack}, nil
}

// pushedAs reports whether pc, t's latest pushed commit in a Repository,
// was pushed as branch: the report's, or t's own branch for a push
// reported without one.
func pushedAs(pc domain.PushedCommit, t domain.Ticket, branch string) bool {
	if pc.Branch != "" {
		return pc.Branch == branch
	}
	return t.Branch == branch
}

// pullRequestOpen says t waits on a pull request that is still open on the
// Code Host, conflicting or not.
func pullRequestOpen(p domain.PullRequest) bool {
	return p.State == domain.PROpen || p.State == domain.PRConflicting
}

// openPullRequests is the notice after a switch away from Pull Request:
// the open pull requests of each Accepted Ticket of p, by Ticket, in the
// Repositories no longer under Pull Request. A Spec's one pull request
// shows for each of its Tickets.
func openPullRequests(r store.Reader, p domain.Project) []api.OpenPullRequest {
	out := []api.OpenPullRequest{}
	ps := landingPolicies(p)
	for _, t := range r.FindTickets(p.Key, store.TicketFilter{States: []domain.State{domain.Accepted}}) {
		for _, pr := range t.PullRequests {
			if pullRequestOpen(pr) && !ps.In(recordRepository(p.Key, pr.Repository)).LandsByPullRequest() {
				out = append(out, api.OpenPullRequest{Ticket: DisplayID(p.Key, t.Number), Link: pr.Link})
			}
		}
	}
	return out
}

// markPullRequestsToClose marks t's open pull requests for the Pull
// Request Watcher to close, as sending it back does while it has
// siblings in other Repositories and none of them has merged; once one
// has, the rest stay open for the next push. A Ticket with one pull
// request keeps it open, as it always did, for the next push to update.
func markPullRequestsToClose(t *domain.Ticket) {
	if len(t.PullRequests) < 2 || anyPullRequest(*t, domain.PRMerged) {
		return
	}
	for i, pr := range t.PullRequests {
		if pullRequestOpen(pr) {
			t.PullRequests[i].Close = true
		}
	}
}
