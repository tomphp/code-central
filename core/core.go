// Package core holds every domain rule behind one list of named
// operations (see Operations). Front doors (httpapi, mcp, the CLI via
// client) only translate onto these methods.
//
// Every write runs inside one store.Update, so the check and the write
// are atomic under the store-wide lock, and writes exactly one file.
package core

import (
	"context"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/usage"
)

// Core is the domain. Build one with New.
type Core struct {
	store  store.Store
	clock  clock.Clock
	ids    ids.Source
	events *events.Bus
	// activity is the in-memory last-activity record behind Silent Claims.
	activity *activity
	// transcripts holds each Claim's Session Events, in memory only.
	transcripts *transcripts
	// latestNotices throttles the ticket.changed a new latest event sends.
	latestNotices *latestNotices
	// usage records agents' MCP calls (Usage); nil records nothing.
	usage *usage.Recorder
	// ciCheck is the CI Watcher's check mode, run when CI Tracking is
	// turned on; nil checks nothing.
	ciCheck api.CICheck
	// issuesCheck is the Issue Watcher's check mode, run when GitHub
	// Issues is turned on; nil checks nothing.
	issuesCheck api.IssuesCheck
	// azureBoardsCheck is the Azure Boards check, run when Azure Boards is
	// turned on or what it runs changes while on; nil checks nothing.
	azureBoardsCheck api.AzureBoardsCheck
	// pullRequestCheck is the Pull Request Watcher's check mode, run when
	// Pull Request is turned on; nil checks nothing.
	pullRequestCheck api.PullRequestCheck
	// watchers holds each Project's Watchers' statuses, in memory only.
	watchers *watchers
	// heartbeats holds each Factory's latest Heartbeat, in memory only.
	heartbeats *heartbeats
	// chats holds each Project's current Conversation, in memory only.
	chats *chats
	// factoryWakes holds the Factories' open next_factory_wake calls, in
	// memory only.
	factoryWakes *factoryWakes
	// uiURL is the UI's address, which a comment written to an issue
	// links back to; empty links relative to the UI.
	uiURL string
	// autoFixLocks holds each Project's Auto-fix lock, a *sync.Mutex by
	// Project key (lockAutoFix).
	autoFixLocks sync.Map
	// service is this service as the Store's record of the services
	// running on it names it.
	service Service
}

// New returns a Core persisting through s, telling time with clk and
// issuing Claim IDs from idSource. Every write sends a change notice on
// Events.
func New(s store.Store, clk clock.Clock, idSource ids.Source) *Core {
	bus, wakes := events.New(), newFactoryWakes()
	c := &Core{store: newNotifying(archiveGuard{s}, bus, wakes), clock: clk, ids: idSource, events: bus, factoryWakes: wakes}
	c.activity = newActivity(c.now())
	c.transcripts = &transcripts{all: map[markKey]*transcript{}, feeds: map[markKey]map[*TranscriptFeed]struct{}{}}
	c.latestNotices = &latestNotices{sent: map[markKey]time.Time{}, pending: map[markKey]bool{}}
	c.watchers = &watchers{all: map[watcherKey]watcherStatus{}, leases: map[string]store.Lease{}}
	c.heartbeats = &heartbeats{all: map[heartbeatKey]heartbeat{}, seen: map[string]string{}, base: map[string]map[string]api.BaseView{}, clones: map[string]map[string]api.CloneView{}}
	c.chats = newChats()
	return c
}

// now is the current time as stored: UTC, whole seconds.
func (c *Core) now() time.Time { return c.clock.Now().UTC().Truncate(time.Second) }

// moveView is one Move as reads show it.
func moveView(m domain.Move) api.MoveView {
	return api.MoveView{From: m.From, To: m.To, Actor: m.Actor, Time: m.Time, Claim: m.Claim, Via: m.Via, Commit: m.Commit, Cause: m.Cause}
}

var keyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// CreateProject is create_project(key, display_name). The display name
// defaults to the key.
func (c *Core) CreateProject(_ context.Context, in api.CreateProjectInput) (api.ProjectView, error) {
	if !keyPattern.MatchString(in.Key) {
		return api.ProjectView{}, invalid("key", "a Project Key is lowercase letters, digits, '.', '_' and '-', starting with a letter or digit")
	}
	name := in.DisplayName
	if name == "" {
		name = in.Key
	}
	mode := in.Mode
	if mode == "" {
		mode = domain.ModeCommitted
	}
	if !mode.Valid() {
		return api.ProjectView{}, invalidMode(mode)
	}
	p := domain.Project{Key: in.Key, DisplayName: name, Created: c.now(), Mode: mode}
	err := c.store.Update(func(w store.Writer) error {
		if _, ok := w.Project(in.Key); ok {
			return invalid("key", "a Project with key %q already exists", in.Key)
		}
		return w.PutProject(p)
	})
	if err != nil {
		return api.ProjectView{}, err
	}
	return c.projectView(p), nil
}

// GetProject is get_project(key).
func (c *Core) GetProject(_ context.Context, key string) (api.ProjectView, error) {
	var v api.ProjectView
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		v = c.projectView(p)
		v.Landing.OpenPullRequests = openPullRequests(r, p)
		fillChangedBy(r, &v)
		return nil
	})
	return v, err
}

// CreateSpec is create_spec(title, body, full?).
func (c *Core) CreateSpec(_ context.Context, key string, in api.CreateSpecInput) (api.Written[api.SpecView], error) {
	if strings.TrimSpace(in.Title) == "" {
		return api.Written[api.SpecView]{}, invalid("title", "a Spec needs a title")
	}
	var v api.Written[api.SpecView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		now := c.now()
		s := domain.Spec{Number: w.NextNumber(key), Title: in.Title, Created: now, Updated: now, Body: in.Body}
		if err := w.PutSpec(key, s); err != nil {
			return err
		}
		var err error
		v, err = acknowledgeCreate(specView(key, domain.Spec{}), specView(key, s), in.Full)
		return err
	})
	return v, err
}

// CreateTicket is create_ticket(spec, title, description, state, labels,
// blocked_by, tags?, full?).
func (c *Core) CreateTicket(ctx context.Context, key string, in api.CreateTicketInput) (api.Written[api.TicketView], error) {
	if strings.TrimSpace(in.Title) == "" {
		return api.Written[api.TicketView]{}, invalid("title", "a Ticket needs a title")
	}
	state := in.State
	if state == "" {
		state = domain.Draft
	}
	if state != domain.Draft && state != domain.Ready {
		return api.Written[api.TicketView]{}, invalid("state", "a new Ticket is Draft or Ready, not %q", state)
	}
	if in.Spec == "" {
		return api.Written[api.TicketView]{}, invalid("spec", "every Ticket belongs to a Spec")
	}
	if err := checkTicketLabels("labels", in.Labels); err != nil {
		return api.Written[api.TicketView]{}, err
	}
	if err := checkHumanLabels(ctx, "labels", in.Labels); err != nil {
		return api.Written[api.TicketView]{}, err
	}
	if err := checkTagSyntax(in.Tags); err != nil {
		return api.Written[api.TicketView]{}, err
	}
	var v api.Written[api.TicketView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		if err := checkTicketTags(w, key, in.Tags); err != nil {
			return err
		}
		sp, err := resolveSpec(w, key, "create_ticket", "spec", in.Spec)
		if err != nil {
			return err
		}
		specN := sp.Number
		// A new Ticket has no dependants yet, so its Blockers can't form a cycle.
		blockedBy, err := ticketRefs(w, key, "create_ticket", "blocked_by", in.BlockedBy)
		if err != nil {
			return err
		}
		now := c.now()
		t := domain.Ticket{
			Number: w.NextNumber(key), Title: in.Title, State: state, Spec: specN,
			Labels: in.Labels, Tags: maps.Clone(in.Tags), BlockedBy: blockedBy, Created: now, Updated: now, Description: in.Description,
		}
		if err := whole.put(w, key, &t); err != nil {
			return err
		}
		v, err = acknowledgeCreate(c.ticketView(w, key, domain.Ticket{}), c.ticketView(w, key, t), in.Full)
		return err
	})
	return v, err
}

// GetTicket is get_ticket(ticket, parts?): the Ticket with the parts asked
// for, or its default parts.
func (c *Core) GetTicket(_ context.Context, key string, in api.GetTicketInput) (api.TicketView, error) {
	parts, err := ticketReadParts(in)
	if err != nil {
		return api.TicketView{}, err
	}
	var v api.TicketView
	err = c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		// It is read whole: left_out counts the parts it leaves out.
		t, err := resolveTicket(r, key, "get_ticket", "ticket", in.Ticket, whole...)
		if err != nil {
			return err
		}
		v = shapeView(c.ticketView(r, key, t), r, p, t, in.Parts, parts, in.WithModelChoice)
		v.Retros = retrosOf(r, key, DisplayID(key, t.Number))
		return nil
	})
	return v, err
}

// shapeView is v as a read asking for the parts asked, which parts is as
// a set, returns it: the default view when it names none.
func shapeView(v api.TicketView, r store.Reader, p domain.Project, t domain.Ticket, asked []string, parts map[string]bool, withUIFields bool) api.TicketView {
	if len(asked) == 0 {
		v = defaultView(v)
	} else {
		v = v.Only(parts)
	}
	if withUIFields {
		mc := modelChoice(p, t)
		v.ModelChoice = &mc
		sp, _ := r.Spec(p.Key, t.Spec)
		v.AutoAcceptKind, _ = autoAcceptKind(sp, t)
	}
	return v
}

// FindTickets is find_tickets(state?, title?, labels?, tags?, spec?, claimable?). Done and
// Cancelled Tickets are left out unless the State filter names them.
// Rows are in claim order: created, then number.
func (c *Core) FindTickets(_ context.Context, key string, in api.FindTicketsInput) ([]api.TicketRow, error) {
	rows := []api.TicketRow{}
	err := c.store.View(func(r store.Reader) error {
		e, tickets, err := c.findIn(r, key, in)
		if err != nil {
			return err
		}
		maps := map[int]bool{}
		for _, s := range r.FindSpecs(key, store.SpecFilter{Label: domain.MapLabel}) {
			maps[s.Number] = true
		}
		for _, t := range tickets {
			rows = append(rows, c.ticketRow(r, e, t, maps[t.Spec]))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// FindTicketViews is find_tickets with full: the Tickets find_tickets
// would give as rows, each as get_ticket returns it, all read from one
// evaluation of the Project.
func (c *Core) FindTicketViews(_ context.Context, key string, in api.FindTicketsInput) ([]api.TicketView, error) {
	parts, err := ticketReadParts(api.GetTicketInput{Parts: in.Parts})
	if err != nil {
		return nil, err
	}
	views := []api.TicketView{}
	err = c.store.View(func(r store.Reader) error {
		e, tickets, err := c.findIn(r, key, in)
		if err != nil {
			return err
		}
		for _, t := range tickets {
			// Each is read whole, as get_ticket reads it.
			t, _ = r.Ticket(key, t.Number, whole...)
			views = append(views, shapeView(c.ticketViewIn(r, e, t), r, e.p, t, in.Parts, parts, in.WithModelChoice))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// findIn is the Tickets of Project key matching in's filters, in claim
// order, with the Project's evaluation they are read from.
func (c *Core) findIn(r store.Reader, key string, in api.FindTicketsInput) (*evaluation, []domain.Ticket, error) {
	for _, s := range in.State {
		if !s.Valid() {
			return nil, nil, invalid("state", "unknown State %q", s)
		}
	}
	if err := checkTagFilter(in.Tags); err != nil {
		return nil, nil, err
	}
	p, ok := r.Project(key)
	if !ok {
		return nil, nil, unknownProject(key)
	}
	f := store.TicketFilter{States: in.State, Order: store.ByClaim}
	if len(f.States) == 0 {
		f.States = unfinished
	}
	if in.Spec != "" {
		n, err := refNumber(in.Spec, key, "spec")
		if err != nil {
			return nil, nil, err
		}
		// A Ticket is rejected naming its Spec; a Spec that doesn't
		// exist matches no Tickets.
		if err := ticketNotSpec(r, key, "find_tickets", "spec", n); err != nil {
			return nil, nil, err
		}
		f.Spec = []int{n}
	}
	tickets := r.FindTickets(key, f)
	e := c.evaluate(r, p)
	found := tickets[:0]
	for _, t := range tickets {
		if !matches(t, in) {
			continue
		}
		if in.Claimable != nil && *in.Claimable != (t.State == domain.Ready && !e.rule.blocked(t)) {
			continue
		}
		found = append(found, t)
	}
	return e, found, nil
}

// matches reports whether t matches in's filters that the Store's query
// leaves to core: its title, Labels and Tags.
func matches(t domain.Ticket, in api.FindTicketsInput) bool {
	if in.Title != "" && !strings.Contains(strings.ToLower(t.Title), strings.ToLower(in.Title)) {
		return false
	}
	return hasAll(t.Labels, t.Tags, in.Labels, in.Tags)
}

// hasAll reports whether every wanted Label is present and every wanted
// Tag pair matches exactly, a wanted Tag with an empty value matching
// any value: no Tag has an empty value.
func hasAll(labels []string, tags map[string]string, wantLabels []string, wantTags map[string]string) bool {
	for _, l := range wantLabels {
		if !slices.Contains(labels, l) {
			return false
		}
	}
	for k, v := range wantTags {
		if got, ok := tags[k]; !ok || (v != "" && got != v) {
			return false
		}
	}
	return true
}

// ticketRow is t as a find_tickets row, its CI and reason read from its
// Project's evaluation e; onMap says t is on a Map, so the row carries
// its node type.
func (c *Core) ticketRow(r store.Reader, e *evaluation, t domain.Ticket, onMap bool) api.TicketRow {
	key := e.p.Key
	row := api.TicketRow{
		ID: DisplayID(key, t.Number), Title: t.Title, State: t.State, Spec: DisplayID(key, t.Spec),
		Labels: orEmpty(t.Labels), Blocked: c.blockRule(r, key).blocked(t), Created: t.Created, Updated: t.Updated, Link: linkView(t.Link),
		CIResult: e.ci.ticket(t).result, PullRequest: pullRequestView(homePullRequest(key, t)),
		WaitingForSpec: waitingForSpec(r, e.p, t),
	}
	row.NeedsMeReason, _ = e.reason(t)
	if onMap {
		row.NodeType = NodeTypeOf(t.Labels)
		row.BlockedBy = displayIDs(key, t.BlockedBy)
	}
	if cl, ok := liveClaim(t); ok {
		row.LatestEvent = c.latestEvent(key, t.Number, cl.Number)
	}
	if severalRepositories(e.p) {
		row.Repositories = namesInLandingOrder(e.p, changedRepositoryNames(key, t))
	}
	row.PartlyLanded = partlyLanded(key, t)
	return row
}

func finished(s domain.State) bool { return s == domain.Done || s == domain.Cancelled }

func (c *Core) projectView(p domain.Project) api.ProjectView {
	now := c.now()
	v := api.ProjectView{Key: p.Key, DisplayName: p.DisplayName, Created: p.Created, Mode: p.Mode,
		Landing: landingViewWith(p, c.watchers.view(watcherKey{api.WatcherPullRequests, p.Key}, now)), CI: ciView(p, c.watchers.view(watcherKey{api.WatcherCI, p.Key}, now)),
		GitHubIssues: githubIssuesView(p.Key, p.GitHubIssues, c.watchers.view(watcherKey{api.WatcherIssues, p.Key}, now)), AzureBoards: azureBoardsView(p, c.watchers.view(watcherKey{api.WatcherWorkItems, p.Key}, now)), Models: modelsView(p.Models),
		ReportedModels: reportedModelsView(p.ReportedModels), StageGuides: stageGuidesView(p.StageGuides), AutoAccept: autoAcceptView(p),
		FocusKinds: focusKindsView(p.FocusKindsOff), FactoryStatus: c.heartbeats.view(p.Key, c.clock.Now()), Repositories: repositoriesView(p)}
	c.fillClones(p.Key, v.Repositories)
	if !p.Archived.IsZero() {
		at := p.Archived
		v.Archived, v.ArchivedAt = true, &at
	}
	return v
}

func specView(key string, s domain.Spec) api.SpecView {
	return api.SpecView{
		ID: DisplayID(key, s.Number), Title: s.Title, Labels: orEmpty(s.Labels), Tags: orEmptyMap(s.Tags),
		Created: s.Created, Updated: s.Updated, Body: s.Body, Edits: editViews(s.Edits), Link: linkView(s.Link),
	}
}

func (c *Core) ticketView(r store.Reader, key string, t domain.Ticket) api.TicketView {
	pr, _ := r.Project(key)
	// A write's view of t may come before t is stored.
	return c.ticketViewIn(r, c.evaluateWith(r, pr, t), t)
}

// ticketViewIn is t's view, read from its Project's evaluation e.
func (c *Core) ticketViewIn(r store.Reader, e *evaluation, t domain.Ticket) api.TicketView {
	key, pr := e.p.Key, e.p
	sp, _ := r.Spec(key, t.Spec)
	rule := e.rule
	v := api.TicketView{
		ID: DisplayID(key, t.Number), Title: t.Title, State: t.State,
		Spec:   api.SpecRef{ID: DisplayID(key, t.Spec), Title: sp.Title, Branch: domain.SpecBranch(DisplayID(key, t.Spec), sp.Tags)},
		Labels: orEmpty(t.Labels), Tags: orEmptyMap(t.Tags),
		BlockedBy: displayIDs(key, t.BlockedBy), Replacements: displayIDs(key, t.Replacements),
		Blockers: rule.blockerViews(t),
		Created:  t.Created, Updated: t.Updated, Description: t.Description,
		Branch: t.Branch, Commit: t.Commit, Recommendation: t.Recommendation,
		Claims: []api.ClaimView{}, Rounds: roundViews(t.Rounds), FailureReports: failureReportViews(t.FailureReports),
		Comments: commentViews(t.Comments), Moves: []api.MoveView{}, Edits: editViews(t.Edits),
		Blocked: rule.blocked(t), Link: linkView(t.Link),
		Editable: ticketEditable(pr, t), Retroable: ticketRetroable(pr, t),
	}
	for _, cl := range t.Claims {
		cv := api.ClaimView{Number: cl.Number, SessionID: cl.SessionID, WorkingDir: cl.WorkingDir, ResumeCommand: cl.ResumeCommand, Model: cl.Model, Effort: cl.Effort, Started: cl.Started, LastActivity: c.claimActivity(t, cl), EndedBy: cl.EndedBy,
			LatestEvent: c.latestEvent(key, t.Number, cl.Number)}
		if !cl.Live() {
			ended := cl.Ended
			cv.Ended = &ended
		}
		v.Claims = append(v.Claims, cv)
	}
	for _, m := range t.Moves {
		v.Moves = append(v.Moves, moveView(m))
	}
	v.PushedCommit, v.CIResult, v.CICommit, v.CIRuns = e.ci.view(t)
	v.PullRequest = pullRequestView(homePullRequest(key, t))
	if t.SpecMerge != nil {
		v.SpecMerge = &api.SpecMergeView{Repository: t.SpecMerge.Repository, Commit: t.SpecMerge.Commit, Merged: t.SpecMerge.Merged, Counts: specMergeCounts(t)}
	}
	v.SpecLanded = specLanded(r, pr, t)
	v.WaitingForSpec = waitingForSpec(r, pr, t)
	if severalRepositories(pr) {
		v.Repositories = changedRepositoryViews(pr, t, e.ci)
		for i, rv := range v.Repositories {
			if t.State == domain.Accepted && rv.Landed == nil {
				v.Repositories[i].NotLanded = c.notLandedReason(pr, t, rv.Name, e.ci)
			}
			v.Repositories[i].CIAutoFix = e.ciAutoFix(t, rv.Name)
		}
		for _, it := range e.itemsOf(t) {
			v.CIItems = append(v.CIItems, *e.ciItemView(it))
		}
	}
	v.PartlyLanded = partlyLanded(key, t)
	if it, ok := e.itemOf(t); ok {
		v.CIItem = e.ciItemView(it)
		if it.reason != api.ReasonCIUnavailable {
			v.CIFailureOptions = v.CIItem.Options
		}
	}
	v.ToMergeOptions = e.toMergeOptions(t)
	v.LandAnyway = e.ci.landAnyway(e.ci.ticket(t).in(key))
	v.CIAutoFix = e.ciAutoFix(t, key)
	v.Claimable = t.State == domain.Ready && !v.Blocked
	v.SilentSince = c.silentSince(t)
	v.NeedsMeReason, _ = e.reason(t)
	v.AwaitingTriage, v.TriageActions = t.AwaitingTriage, ticketTriageActions(r, key, t)
	if v.TriageActions != nil {
		v.TriageNotes = triageNotes(t)
	}
	v.SpecMergeTicket = t.SpecMergeTicket
	v.AutoFix = autoFixMarkView(pr, t.AutoFix)
	return v
}

func displayIDs(key string, ns []int) []string {
	out := []string{}
	for _, n := range ns {
		out = append(out, DisplayID(key, n))
	}
	return out
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orEmptyMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// landingView is p's Landing with the Code Host worked out from the Push
// remote URL. Push reads as off under Pull Request, which implies it.
func landingView(p domain.Project) api.LandingView {
	return landingViewWith(p, api.WatcherView{State: api.WatcherNotReported})
}

// landingViewWith is landingView with the Pull Request Watcher's status.
func landingViewWith(p domain.Project, watcher api.WatcherView) api.LandingView {
	host, repo := codeHostOf(p.CI.PushRemoteURL)
	push := p.Landing.Normalized().Push
	var azure []string
	for _, rv := range repositoriesView(p) {
		if rv.CodeHost == api.CodeHostAzureRepos {
			azure = append(azure, rv.Name)
		}
	}
	ps := landingPolicies(p)
	return api.LandingView{Method: p.Landing.Method, Push: push, Remote: p.Landing.Remote, LandPerSpec: p.Landing.LandPerSpec,
		Rebase: p.Landing.Rebase, CodeHost: host, Repository: repo, PATEnv: patEnvOf(domain.CI{PATEnv: p.Landing.PATEnv}), UsesAzureReposPAT: ps.UsesAzureReposPAT(azure),
		Methods: landingCapabilities(p, azure), RunsPullRequestWatcher: ps.RunsPullRequestWatcher(),
		RepositoryLanding: repositoryLandingOf(p.Landing.Method, p.Landing.Push), PushesNothing: pushesNothing(p),
		Base: p.Base(), Watcher: watcher, OpenPullRequests: []api.OpenPullRequest{}}
}
