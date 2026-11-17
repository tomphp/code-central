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
// service never calls the Code Host. With several Repositories (Spec
// code-central-848, ADR 0050) a Ticket has a pull request per Changed
// Repository, each into that Repository's own Base, and each body lists
// its siblings.

// PullRequestView is a pull request as reads show it: on a Ticket, on the
// Spec page and in the Pull Request Watcher's list.
type PullRequestView struct {
	// Repository is the Repository Name the pull request is in; left out
	// while the Project has one Repository.
	Repository string                 `json:"repository,omitempty"`
	Number     int                    `json:"number"`
	Link       string                 `json:"link"`
	State      store.PullRequestState `json:"state"`
	// MergeCommit is the commit the Code Host merged it at, once merged.
	MergeCommit string `json:"merge_commit,omitempty"`
}

func pullRequestView(p *store.PullRequest) *PullRequestView {
	if p == nil {
		return nil
	}
	return &PullRequestView{Number: p.Number, Link: p.Link, State: p.State, MergeCommit: p.MergeCommit}
}

// pullRequestViewIn is p's view in Project pr, naming its Repository
// while pr has several.
func pullRequestViewIn(pr store.Project, p store.PullRequest) PullRequestView {
	v := *pullRequestView(&p)
	if severalRepositories(pr) {
		v.Repository = recordRepository(pr.Key, p.Repository)
	}
	return v
}

// pullRequestIn is t's pull request in the Repository name, of a Project
// whose Home Repository is home; nil when it has none there.
func pullRequestIn(home string, t store.Ticket, name string) *store.PullRequest {
	for i, p := range t.PullRequests {
		if recordRepository(home, p.Repository) == name {
			return &t.PullRequests[i]
		}
	}
	return nil
}

// homePullRequest is t's pull request in the Home Repository, home: the
// one the Ticket's top-level fields show.
func homePullRequest(home string, t store.Ticket) *store.PullRequest {
	return pullRequestIn(home, t, home)
}

// anyPullRequest reports whether one of t's pull requests is in state.
func anyPullRequest(t store.Ticket, state store.PullRequestState) bool {
	return slices.ContainsFunc(t.PullRequests, func(p store.PullRequest) bool { return p.State == state })
}

// closedPullRequests is t's pull requests closed without merging, in
// p's landing order: each is a Pull request closed.
func closedPullRequests(p store.Project, t store.Ticket) []PullRequestView {
	out := []PullRequestView{}
	for _, pr := range t.PullRequests {
		if pr.State == store.PRClosed {
			out = append(out, pullRequestViewIn(p, pr))
		}
	}
	inLandingOrder(p, out, func(v PullRequestView) string { return recordRepository(p.Key, v.Repository) })
	return out
}

// setPullRequest records pr as t's pull request in its Repository,
// replacing the one there, and reports whether that changed anything. A
// mark to close holds while the pull request is still open.
func setPullRequest(t *store.Ticket, pr store.PullRequest) bool {
	i := slices.IndexFunc(t.PullRequests, func(o store.PullRequest) bool { return o.Repository == pr.Repository })
	if i < 0 {
		t.PullRequests = append(t.PullRequests, pr)
		return true
	}
	pr.Close = t.PullRequests[i].Close && (pr.State == store.PROpen || pr.State == store.PRConflicting)
	if t.PullRequests[i] == pr {
		return false
	}
	t.PullRequests[i] = pr
	return true
}

// PullRequestWatch is get_pull_request_watch: everything the Pull Request
// Watcher needs on each poll. Work and Close are empty unless the Landing
// Method is Pull Request.
type PullRequestWatch struct {
	Project string `json:"project"`
	// On says the Landing Method is Pull Request.
	On bool `json:"on"`
	// CodeHost, Repository and Base are the Home Repository's.
	CodeHost   CodeHost `json:"code_host"`
	Repository string   `json:"repository"`
	Base       string   `json:"base"`
	// Repositories are every Repository the watcher watches, in landing
	// order, the Home Repository's included.
	Repositories []PullRequestRepositoryView `json:"repositories"`
	// PATEnv names the environment variable holding the Azure Repos PAT,
	// for every Repository on Azure Repos.
	PATEnv string `json:"pat_env"`
	GHPath string `json:"gh_path"`
	// Work is every piece of pushed work to watch, by when it was pushed.
	Work []PushedWorkView `json:"work"`
	// Close is every open pull request marked to close.
	Close []PullRequestToClose `json:"close"`
}

// PullRequestRepositoryView is one Repository as the Pull Request Watcher
// works it: its Code Host and the repository there, worked out from its
// Clone URL, and its Base.
type PullRequestRepositoryView struct {
	Name     string   `json:"name"`
	CodeHost CodeHost `json:"code_host"`
	// Repository is owner/name on GitHub and
	// organization/project/repository on Azure Repos.
	Repository string `json:"repository"`
	Base       string `json:"base"`
}

// PushedWorkView is one piece of pushed work: a Ticket's branch in one
// Repository, with its latest pushed commit there, what to open its pull
// request with, and its current pull request, if any.
type PushedWorkView struct {
	// Repository is the Repository Name the work is in.
	Repository string    `json:"repository"`
	Branch     string    `json:"branch"`
	Commit     string    `json:"commit"`
	Pushed     time.Time `json:"pushed"`
	// Tickets are the Tickets the work covers.
	Tickets []string `json:"tickets"`
	Title   string   `json:"title"`
	// Body is what the pull request's body should be: the watcher opens
	// it with that, and with several Repositories edits it to match.
	Body string `json:"body"`
	// PullRequest is nil until the Watcher has reported one.
	PullRequest *PullRequestView `json:"pull_request,omitempty"`
}

// PullRequestToClose is an open pull request marked to close, as sending
// its Ticket back marks its open pull requests while none has merged.
type PullRequestToClose struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Number     int    `json:"number"`
	Link       string `json:"link"`
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
		out = PullRequestWatch{Project: key, On: landingPolicyOf(p).RunsPullRequestWatcher(), CodeHost: lv.CodeHost, Repository: lv.Repository,
			Base: p.Base(), Repositories: []PullRequestRepositoryView{}, PATEnv: lv.PATEnv, GHPath: r.Options().GHPath, Work: []PushedWorkView{}, Close: []PullRequestToClose{}}
		if out.On {
			for _, rv := range repositoriesView(p) {
				_, repository := codeHostOf(rv.CloneURL)
				out.Repositories = append(out.Repositories, PullRequestRepositoryView{Name: rv.Name, CodeHost: rv.CodeHost, Repository: repository, Base: rv.Base})
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
func branchOf(t store.Ticket) string {
	if n := len(t.PushedCommits); n > 0 && t.PushedCommits[n-1].Branch != "" {
		return t.PushedCommits[n-1].Branch
	}
	return t.Branch
}

// latestPushedTo is t's latest pushed commit in the Repository name, of
// a Project whose Home Repository is home; ok is false when none was
// pushed there.
func latestPushedTo(home string, t store.Ticket, name string) (store.PushedCommit, bool) {
	for i := len(t.PushedCommits) - 1; i >= 0; i-- {
		if pc := t.PushedCommits[i]; recordRepository(home, pc.Repository) == name {
			return pc, true
		}
	}
	return store.PushedCommit{}, false
}

// pushedRepositories is the Repositories t has pushed commits in, in
// the order of their first push.
func pushedRepositories(home string, t store.Ticket) []string {
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
func branchIn(home string, t store.Ticket, name string) string {
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
func watchesPullRequest(t store.Ticket) bool {
	if len(t.PushedCommits) == 0 {
		return false
	}
	return t.State == store.Accepted || (t.State == store.InReview && len(t.PullRequests) > 0)
}

// pushedWork is every piece of pushed work to watch, in the order of its
// first Ticket's number, then of its Repositories' first push. Under
// Land per Spec a Spec's Tickets share its Spec Branch in each
// Repository, which is one piece of work titled with the Spec's title.
func pushedWork(r store.Reader, key string) []PushedWorkView {
	out := []PushedWorkView{}
	type at struct{ repository, branch string }
	index := map[at]int{}
	p, _ := r.Project(key)
	var covered [][]store.Ticket
	for _, t := range r.Tickets(key) {
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
			covered = append(covered, []store.Ticket{t})
			out = append(out, PushedWorkView{Repository: name, Branch: branch, Commit: latest.Commit, Pushed: latest.Pushed, Tickets: []string{DisplayID(key, t.Number)},
				Title: t.Title, PullRequest: pullRequestView(pullRequestIn(key, t, name))})
		}
	}
	spec := func(ts []store.Ticket) (store.Spec, bool) {
		if !specLanded(r, p, ts[0]) {
			return store.Spec{}, false
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
func pullRequestsToClose(r store.Reader, p store.Project) []PullRequestToClose {
	out := []PullRequestToClose{}
	for _, t := range r.Tickets(p.Key) {
		if finished(t.State) {
			continue
		}
		for _, pr := range t.PullRequests {
			if !pr.Close || (pr.State != store.PROpen && pr.State != store.PRConflicting) {
				continue
			}
			name := recordRepository(p.Key, pr.Repository)
			out = append(out, PullRequestToClose{Repository: name, Branch: branchIn(p.Key, t, name), Number: pr.Number, Link: pr.Link})
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
func ticketPullRequestBody(key string, t store.Ticket, project string) string {
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
func issueMention(p store.Project, spec store.Spec, name string) string {
	if !spec.Link.Attached() || spec.Link.Integration != string(CodeHostGitHub) {
		return ""
	}
	ref := spec.Link.ID
	if host, _ := codeHostOf(cloneURLOf(p, name)); host != CodeHostGitHub && spec.Link.URL != "" {
		ref = spec.Link.URL
	}
	return "\n\nIssue: " + ref
}

// cloneURLOf is the Clone URL of p's Repository name: the reported Push
// remote URL for the Home Repository.
func cloneURLOf(p store.Project, name string) string {
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
func siblingsSection(p store.Project, tickets []store.Ticket, name string) string {
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
// report_pull_request(repository?, branch, number, link, state,
// merge_commit?, full?).
type ReportPullRequestInput struct {
	// Repository is the Repository Name the pull request is in; empty for
	// the Home Repository.
	Repository string `json:"repository,omitempty"`
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
// in the Repository was pushed as the branch. A merged pull request
// stores the merge commit the Code Host reports. Any state may follow any
// other, so a closed pull request reopened is reported open again.
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
		p, _ := r.Project(key)
		name, err := repositoryOf(p, "repository", in.Repository)
		if err != nil {
			return err
		}
		if name != p.Key {
			pr.Repository = name
		}
		for _, t := range r.Tickets(key) {
			if t.State == store.Done || t.State == store.Cancelled {
				continue
			}
			if pc, ok := latestPushedTo(key, t, name); ok && pushedAs(pc, t, branch) {
				covered = append(covered, t.Number)
			}
		}
		if len(covered) == 0 {
			return notFound("no Ticket of %s has work pushed as branch %q in %s", key, branch, name)
		}
		return nil
	}); err != nil {
		return Written[PullRequestReport]{}, err
	}
	report := PullRequestReport{Branch: branch, Tickets: []string{}, Record: *pullRequestView(&pr)}
	var changed []string
	// A store update writes one record, so each Ticket has its own.
	for _, n := range covered {
		moved := false
		err := c.store.Update(func(w store.Writer) error {
			t, ok := w.Ticket(key, n)
			if !ok || !setPullRequest(&t, pr) {
				return nil
			}
			t.Updated, moved = c.now(), true
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

// pushedAs reports whether pc, t's latest pushed commit in a Repository,
// was pushed as branch: the report's, or t's own branch for a push
// reported without one.
func pushedAs(pc store.PushedCommit, t store.Ticket, branch string) bool {
	if pc.Branch != "" {
		return pc.Branch == branch
	}
	return t.Branch == branch
}

// pullRequestOpen says t waits on a pull request that is still open on the
// Code Host, conflicting or not.
func pullRequestOpen(p store.PullRequest) bool {
	return p.State == store.PROpen || p.State == store.PRConflicting
}

// openPullRequests is the notice after a switch away from Pull Request:
// the open pull requests of each Accepted Ticket, by Ticket. A Spec's one
// pull request shows for each of its Tickets. It is empty when lp lands by
// Pull Request.
func openPullRequests(r store.Reader, key string, lp LandingPolicy) []OpenPullRequest {
	out := []OpenPullRequest{}
	if lp.LandsByPullRequest() {
		return out
	}
	for _, t := range r.Tickets(key) {
		if t.State != store.Accepted {
			continue
		}
		for _, pr := range t.PullRequests {
			if pullRequestOpen(pr) {
				out = append(out, OpenPullRequest{Ticket: DisplayID(key, t.Number), Link: pr.Link})
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
func markPullRequestsToClose(t *store.Ticket) {
	if len(t.PullRequests) < 2 || anyPullRequest(*t, store.PRMerged) {
		return
	}
	for i, pr := range t.PullRequests {
		if pullRequestOpen(pr) {
			t.PullRequests[i].Close = true
		}
	}
}
