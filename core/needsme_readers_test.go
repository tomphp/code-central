package core_test

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Every reader of Needs Me reads one evaluation of a Project (Spec
// code-central-568), so get_ticket, needs_me, Attention and the
// timed-reason watch agree on each Ticket's reason, through core's
// public operations.

// cancelledBlocker gives acme's Ticket id a Blocker on spec, then
// cancels it with no Replacements.
func (f fixture) cancelledBlocker(t *testing.T, id, spec core.Ref) {
	t.Helper()
	b := f.ticket(t, "acme", core.CreateTicketInput{Spec: spec, Title: "Gone", State: store.Ready})
	f.addBlocker(t, id, core.Ref(b.ID))
	f.cancel(t, core.CancelInput{Ticket: core.Ref(b.ID)}, core.Human)
}

// inNeedsMe is the reasons Ticket id of Project key shows under in
// needs_me: its own row's, and each CI item's that lists it.
func inNeedsMe(t *testing.T, f fixture, key, id string) []core.NeedsMeReason {
	t.Helper()
	var out []core.NeedsMeReason
	for _, row := range f.needsMe(t) {
		if row.Project != key {
			continue
		}
		if row.ID == id || (row.CIItem != nil && slices.Contains(listed(row.CIItem), id)) {
			out = append(out, row.Reason)
		}
	}
	return out
}

// wantEverywhere checks that Ticket id of Project key waits for want, or
// for nothing when want is "", alike on get_ticket, in needs_me (once,
// as its own row or in a CI item) and in Attention.
func wantEverywhere(t *testing.T, f fixture, key, id string, want core.NeedsMeReason) {
	t.Helper()
	v, err := f.core.GetTicket(ctx, key, core.GetTicketInput{Ticket: core.Ref(id)})
	if err != nil {
		t.Fatal(err)
	}
	if v.NeedsMeReason != want {
		t.Errorf("get_ticket %s: needs_me_reason %q, want %q", id, v.NeedsMeReason, want)
	}
	rows, err := f.core.FindTickets(ctx, key, core.FindTicketsInput{State: store.States})
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(rows, func(r core.TicketRow) bool { return r.ID == id }); i < 0 || rows[i].NeedsMeReason != want {
		t.Errorf("find_tickets %s: row %+v, want needs_me_reason %q", id, rows, want)
	}
	wantIn := []core.NeedsMeReason{}
	if want != "" {
		wantIn = append(wantIn, want)
	}
	if got := inNeedsMe(t, f, key, id); !slices.Equal(got, wantIn) && len(got)+len(wantIn) > 0 {
		t.Errorf("needs_me shows %s under %v, want %v", id, got, wantIn)
	}
	a, ok := f.core.AttentionOf(ctx, key, id)
	if !ok {
		t.Fatalf("no Attention for %s", id)
	}
	if a.NeedsMe != (want != "") || (a.SilentClaim != 0) != (want == core.ReasonSilentClaim) ||
		(a.LongClaim != 0) != (want == core.ReasonLongClaim) || (a.CIFailedCommit != "") != (want == core.ReasonCIFailed) {
		t.Errorf("Attention %+v, want it to read %q", a, want)
	}
}

// Each pair of reasons that can apply to a Ticket together gives the
// earlier one in the chain, alike on get_ticket, in needs_me and in
// Attention.
func TestWhenTwoReasonsApplyTheEarlierOneWinsForEveryReader(t *testing.T) {
	cases := []struct {
		name string
		// world leaves acme's Ticket id with both reasons.
		world func(t *testing.T) fixture
		id    string
		want  core.NeedsMeReason
	}{
		{"Needs Answer over Blocked by a Cancelled Ticket", func(t *testing.T) fixture {
			f := newFixture(t)
			f.asking(t, qDB)
			f.cancelledBlocker(t, "acme-2", "acme-1")
			return f
		}, "acme-2", core.ReasonNeedsAnswer},
		{"Failed over Blocked by a Cancelled Ticket", func(t *testing.T) fixture {
			f := newFixture(t)
			f.failed(t)
			f.cancelledBlocker(t, "acme-2", "acme-1")
			return f
		}, "acme-2", core.ReasonFailed},
		{"In Review over Blocked by a Cancelled Ticket", func(t *testing.T) fixture {
			f := newFixture(t)
			f.inReview(t)
			f.cancelledBlocker(t, "acme-2", "acme-1")
			return f
		}, "acme-2", core.ReasonInReview},
		{"Silent Claim over Long Claim", func(t *testing.T) fixture {
			f := newFixture(t)
			f.work(t, store.Ready)
			f.claim(t, core.ClaimInput{SessionID: "s1"})
			f.clock.Advance(core.LongAfter + time.Minute)
			return f
		}, "acme-2", core.ReasonSilentClaim},
		{"Long Claim over Blocked by a Cancelled Ticket", func(t *testing.T) fixture {
			f := newFixture(t)
			f.work(t, store.Ready)
			f.claim(t, core.ClaimInput{SessionID: "s1"})
			f.cancelledBlocker(t, "acme-2", "acme-1")
			f.keepActive(core.LongAfter + time.Second)
			return f
		}, "acme-2", core.ReasonLongClaim},
		{"CI failed over Accepted but not landed", func(t *testing.T) fixture {
			f := itemWorld(t)
			f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
			f.clock.Advance(core.NotLandedAfter + time.Minute)
			return f
		}, "acme-5", core.ReasonCIFailed},
		{"CI failed over Blocked by a Cancelled Ticket", func(t *testing.T) fixture {
			f := itemWorld(t)
			f.cancelledBlocker(t, "acme-8", "acme-7")
			f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
			return f
		}, "acme-8", core.ReasonCIFailed},
		{"CI cancelled over Accepted but not landed", func(t *testing.T) fixture {
			f := itemWorld(t)
			f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CICancelled)
			f.clock.Advance(core.NotLandedAfter + time.Minute)
			return f
		}, "acme-5", core.ReasonCICancelled},
		{"CI failed over waiting on its Spec's pull request led by another Ticket", func(t *testing.T) fixture {
			f := specPullRequestWorld(t)
			f.withCITracking(t)
			f.acceptedOnSpecBranch(t, 5, "Built the first part.")
			f.acceptedOnSpecBranch(t, 6, "Built the second part.")
			f.pushSpecBranch(t, "abc123", "acme-5", "acme-6")
			f.ciRun(t, 1, "ci.yml", "abc123", "acme-6", store.CICompleted, store.CIFailure)
			return f
		}, "acme-6", core.ReasonCIFailed},
		{"CI failed over Pull request closed", func(t *testing.T) fixture {
			f := newFixture(t)
			f.pullRequestAccepted(t)
			f.withCITracking(t)
			f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PRClosed})
			f.reportRun(t, "acme-2", store.CICompleted, store.CIFailure)
			return f
		}, "acme-2", core.ReasonCIFailed},
		{"CI cancelled over To merge", func(t *testing.T) fixture {
			f := newFixture(t)
			f.pullRequestAccepted(t)
			f.withCITracking(t)
			f.reportRun(t, "acme-2", store.CICompleted, store.CICancelled)
			return f
		}, "acme-2", core.ReasonCICancelled},
		// A Spec's pull request is one item, on its lead Ticket: the
		// other Tickets waiting on it have no reason of their own.
		{"waiting on its Spec's pull request led by another Ticket over To merge", func(t *testing.T) fixture {
			f := specPullRequestWorld(t)
			f.acceptedOnSpecBranch(t, 5, "Built the first part.")
			f.acceptedOnSpecBranch(t, 6, "Built the second part.")
			f.pushSpecBranch(t, "abc123", "acme-5", "acme-6")
			return f
		}, "acme-6", ""},
		{"waiting on its Spec's pull request led by another Ticket over Blocked by a Cancelled Ticket", func(t *testing.T) fixture {
			f := specPullRequestWorld(t)
			f.acceptedOnSpecBranch(t, 5, "Built the first part.")
			f.acceptedOnSpecBranch(t, 6, "Built the second part.")
			f.pushSpecBranch(t, "abc123", "acme-5", "acme-6")
			f.cancelledBlocker(t, "acme-6", "acme-1")
			return f
		}, "acme-6", ""},
		{"Pull request closed over To merge", func(t *testing.T) fixture {
			f := newFixture(t)
			f.pullRequestAccepted(t)
			f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PRClosed})
			return f
		}, "acme-2", core.ReasonPullRequestClosed},
		{"Pull request closed over Blocked by a Cancelled Ticket", func(t *testing.T) fixture {
			f := newFixture(t)
			f.pullRequestAccepted(t)
			f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PRClosed})
			f.cancelledBlocker(t, "acme-2", "acme-1")
			return f
		}, "acme-2", core.ReasonPullRequestClosed},
		{"To merge over Blocked by a Cancelled Ticket", func(t *testing.T) fixture {
			f := newFixture(t)
			f.pullRequestAccepted(t)
			f.cancelledBlocker(t, "acme-2", "acme-1")
			return f
		}, "acme-2", core.ReasonToMerge},
		{"CI unavailable over Accepted but not landed", func(t *testing.T) fixture {
			f := itemWorld(t)
			f.clock.Advance(core.CIUnavailableAfter + core.NotLandedAfter)
			return f
		}, "acme-5", core.ReasonCIUnavailable},
		{"CI unavailable over Blocked by a Cancelled Ticket", func(t *testing.T) fixture {
			f := itemWorld(t)
			f.cancelledBlocker(t, "acme-8", "acme-7")
			f.clock.Advance(core.CIUnavailableAfter + time.Minute)
			return f
		}, "acme-8", core.ReasonCIUnavailable},
		{"Accepted but not landed over Blocked by a Cancelled Ticket", func(t *testing.T) fixture {
			f := newFixture(t)
			f.accepted(t)
			f.cancelledBlocker(t, "acme-2", "acme-1")
			f.clock.Advance(core.NotLandedAfter + time.Minute)
			return f
		}, "acme-2", core.ReasonAcceptedNotLanded},
		{"Blocked by a Cancelled Ticket over Closed externally", func(t *testing.T) fixture {
			f := newFixture(t)
			f.linked(t)
			f.cancelledBlocker(t, "acme-2", "acme-1")
			f.setExternal(t, "2", store.ExternalClosed)
			return f
		}, "acme-2", core.ReasonBlockedByCancelled},
		{"Closed externally over New issue", func(t *testing.T) fixture {
			f := newFixture(t)
			f.linked(t)
			f.incomingIssue(t, 1, 2)
			f.setExternal(t, "2", store.ExternalClosed)
			return f
		}, "acme-2", core.ReasonClosedExternally},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := c.world(t)
			wantEverywhere(t, f, "acme", c.id, c.want)
		})
	}
}

// incomingIssue makes acme's Spec spec its Issues Spec, with New issues
// need me on, and its linked Ticket n an Incoming Issue awaiting triage.
func (f fixture) incomingIssue(t *testing.T, spec, n int) {
	t.Helper()
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.GitHubIssues = store.GitHubIssues{On: true, Repository: "acme/web", NewIssuesNeedMe: true, Spec: spec}
		return w.PutProject(p)
	})
	if err == nil {
		err = f.store.Update(func(w store.Writer) error {
			tk, _ := w.Ticket("acme", n)
			tk.AwaitingTriage = true
			return w.PutTicket("acme", tk)
		})
	}
	if err != nil {
		t.Fatal(err)
	}
}

// agreementWorld has a Ticket waiting for each reason, and Tickets
// waiting for none, across two Projects. acme, under Merge + Push with
// CI Tracking, has a CI failed item listing acme-5 and acme-8 on their
// shared commit, acme-9 CI cancelled, acme-10 CI unavailable, acme-11
// Accepted but not landed, acme-12 Needs Answer, acme-13 Answered,
// acme-14 Failed, acme-15 In Review, acme-16 blocked by Cancelled acme-17,
// acme-18 a Silent Claim, acme-19 a Long Claim, and on linked Spec
// acme-20 acme-21 Closed externally and acme-22 a New issue; its base has
// diverged. web, under Pull Request, has web-2 To merge and web-3 Pull
// request closed; its base is missing.
func agreementWorld(t *testing.T) fixture {
	t.Helper()
	f := itemWorld(t)
	must := func(_ any, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	// next is a new Ready Ticket on acme-7, and its number.
	next := func(title string) (core.Ref, int) {
		tk := f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-7", Title: title, State: store.Ready})
		n, err := strconv.Atoi(strings.TrimPrefix(tk.ID, "acme-"))
		must(nil, err)
		return core.Ref(tk.ID), n
	}
	accepted := func(title string) string {
		ref, n := next(title)
		f.moveTo(t, n, store.Accepted)
		return string(ref)
	}
	f.pushed(t, accepted("Cancelled run"), "c4c4c4") // acme-9
	f.ciRun(t, 2, "ci.yml", "c4c4c4", "acme-9", store.CICompleted, store.CICancelled)
	f.pushed(t, accepted("No word of CI"), "d5d5d5") // acme-10
	accepted("Not landed")                           // acme-11
	claim := func(session string) (core.Ref, string) {
		ref, _ := next("Claimed")
		res, err := f.core.Claim(ctx, "acme", core.ClaimInput{Ticket: ref, SessionID: session})
		must(res, err)
		return ref, res.ClaimID
	}
	ref, id := claim("q1") // acme-12
	must(f.core.PostRound(ctx, "acme", core.PostRoundInput{Ticket: ref, ClaimID: id, Questions: []core.QuestionInput{qDB}}))
	ref, id = claim("q2") // acme-13
	must(f.core.PostRound(ctx, "acme", core.PostRoundInput{Ticket: ref, ClaimID: id, Questions: []core.QuestionInput{qDB}}))
	must(f.core.Answer(human, "acme", core.AnswerInput{Ticket: ref, Question: 1, Text: "SQLite"}))
	ref, id = claim("f1") // acme-14
	must(f.core.ReportFailure(ctx, "acme", core.ReportFailureInput{Ticket: ref, ClaimID: id, WhatBroke: "B", Why: "W", WhatToFix: "F", WhereWorkStands: "S"}))
	ref, id = claim("r1") // acme-15
	must(f.core.Finish(ctx, "acme", core.FinishInput{Ticket: ref, ClaimID: id, Summary: "Done."}))
	blocked, _ := next("Blocked") // acme-16
	f.cancelledBlocker(t, blocked, "acme-7")
	claim("s2") // acme-18, Silent once quiet
	claim("s1") // acme-19, Long once kept active
	must(f.core.CreateLinkedSpec(integration, "acme", core.CreateLinkedSpecInput{Title: "Issues", Body: "Issues.\n", Link: ghLink("10")}))
	must(f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "acme-20", Title: "Closed", Link: ghLink("11")}))
	must(f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "acme-20", Title: "New", Link: ghLink("12")}))
	f.setExternal(t, "acme-21", store.ExternalClosed)
	f.incomingIssue(t, 20, 22)

	f.project(t, "web")
	sp := f.spec(t, "web", "Login")
	must(f.core.AddLabels(ctx, "web", core.LabelsInput{Target: core.Ref(sp.ID), Labels: []string{core.LongLivedLabel}}))
	for _, branch := range []string{"web-2", "web-3"} {
		f.ticket(t, "web", core.CreateTicketInput{Spec: core.Ref(sp.ID), Title: "W", State: store.Ready})
		res, err := f.core.Claim(ctx, "web", core.ClaimInput{Ticket: core.Ref(branch)})
		must(res, err)
		must(f.core.Finish(ctx, "web", core.FinishInput{Ticket: core.Ref(branch), ClaimID: res.ClaimID, Summary: "Done.", Branch: branch}))
		must(f.core.Accept(human, "web", core.TicketInput{Ticket: core.Ref(branch)}))
	}
	must(f.core.ReportPushRemote(ctx, "web", core.ReportPushRemoteInput{URL: "git@github.com:acme/web.git", Base: "main"}))
	must(f.core.SetLandingMethod(human, "web", core.SetLandingMethodInput{Method: store.LandingPullRequest}))
	for _, branch := range []string{"web-2", "web-3"} {
		must(f.core.ReportPushedCommit(ctx, "web", core.ReportPushedCommitInput{Ticket: core.Ref(branch), Commit: branch + "-tip", Branch: branch}))
	}
	must(f.core.ReportPullRequest(ctx, "web", core.ReportPullRequestInput{Branch: "web-3", State: store.PRClosed, Number: 8, Link: "https://github.com/acme/web/pull/8"}))

	f.keepActive(core.LongAfter + time.Second)
	for _, key := range []string{"acme", "web"} {
		must(f.core.ReportHeartbeat(ctx, key, core.ReportHeartbeatInput{Factory: "cc", Host: "box", State: core.HeartbeatOnline, IntervalSeconds: 10}))
	}
	must(f.core.ReportBase(ctx, "acme", core.ReportBaseInput{State: core.BaseDiverged, Local: "a", Remote: "b"}))
	must(f.core.ReportBase(ctx, "web", core.ReportBaseInput{State: core.BaseMissing}))
	return f
}

// allReasons are the Needs Me reasons in group order.
var allReasons = []core.NeedsMeReason{core.ReasonNeedsAnswer, core.ReasonAnswered, core.ReasonSilentClaim, core.ReasonLongClaim, core.ReasonFailed,
	core.ReasonInReview, core.ReasonCIFailed, core.ReasonCICancelled, core.ReasonBaseDiverged, core.ReasonBaseMissing, core.ReasonPullRequestClosed,
	core.ReasonToMerge, core.ReasonCIUnavailable, core.ReasonAcceptedNotLanded, core.ReasonBlockedByCancelled, core.ReasonNewIssue, core.ReasonClosedExternally}

// Over a world with a Ticket waiting for each reason, a CI item listing
// two Tickets and both Base rows, every Ticket Attention flags is in
// needs_me once, as its own row or in a CI item, under the reason
// get_ticket gives, and every Ticket needs_me shows is flagged. Base rows
// have no Ticket, so they are on neither side.
func TestAttentionAndNeedsMeAgreeOnEveryTicket(t *testing.T) {
	f := agreementWorld(t)

	var reasons []core.NeedsMeReason
	shown := map[string]bool{}
	for _, row := range f.needsMe(t) {
		reasons = append(reasons, row.Reason)
		if row.ID != "" {
			shown[row.Project+" "+row.ID] = true
		}
		if row.CIItem != nil {
			for _, id := range listed(row.CIItem) {
				shown[row.Project+" "+id] = true
			}
		}
	}
	if !slices.Equal(reasons, allReasons) {
		t.Fatalf("needs_me reasons %v, want one item for each, in group order: %v", reasons, allReasons)
	}
	if it := f.theCIItem(t, core.ReasonCIFailed); !slices.Equal(listed(it), []string{"acme-5", "acme-8"}) {
		t.Fatalf("CI failed item lists %v, want acme-5 and acme-8", listed(it))
	}

	flagged := map[string]bool{}
	for _, a := range f.core.Attention(ctx) {
		in := inNeedsMe(t, f, a.Project, a.ID)
		if a.NeedsMe {
			flagged[a.Project+" "+a.ID] = true
		}
		var want core.NeedsMeReason
		if len(in) > 0 {
			want = in[0]
		}
		wantEverywhere(t, f, a.Project, a.ID, want)
	}
	if len(flagged) != len(shown) {
		t.Errorf("Attention flags %d Tickets and needs_me shows %d", len(flagged), len(shown))
	}
	for k := range shown {
		if !flagged[k] {
			t.Errorf("needs_me shows %s, which Attention doesn't flag", k)
		}
	}
	if len(flagged) != 16 {
		t.Errorf("Attention flags %v, want the 16 Tickets that wait", flagged)
	}
}

// Focus Kinds mark Focus Items and filter nothing: turning every reason
// off changes neither Needs Me nor its count, nor the reason each Ticket
// and Spec shows (the needs-you dots), nor list_projects' counts, nor
// what Notifications read, nor the watch.
func TestTurningFocusKindsOffChangesNoReader(t *testing.T) {
	f := agreementWorld(t)
	type reads struct {
		NeedsMe   core.NeedsMeView
		Attention []core.AttentionTicket
		Reasons   map[string]core.NeedsMeReason
		Counts    map[string]int
		Specs     []core.SpecRow
	}
	read := func() reads {
		t.Helper()
		v, err := f.core.NeedsMe(human)
		if err != nil {
			t.Fatal(err)
		}
		// focus_item is what Focus Kinds change; the rest must not move.
		for _, g := range v.Groups {
			for i := range g.Tickets {
				g.Tickets[i].FocusItem = true
			}
		}
		out := reads{NeedsMe: v, Attention: f.core.Attention(ctx), Reasons: map[string]core.NeedsMeReason{}, Counts: map[string]int{}}
		for _, a := range out.Attention {
			tk, err := f.core.GetTicket(ctx, a.Project, core.GetTicketInput{Ticket: core.Ref(a.ID)})
			if err != nil {
				t.Fatal(err)
			}
			out.Reasons[a.ID] = tk.NeedsMeReason
		}
		rows, err := f.core.ListProjects(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			out.Counts[row.Key] = row.NeedsMe
		}
		if out.Specs, err = f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	tick := watch(t, f)
	tick()
	before := read()

	for _, key := range []string{"acme", "web"} {
		if _, err := f.core.SetFocusKinds(human, key, core.SetFocusKindsInput{Off: allReasons}); err != nil {
			t.Fatal(err)
		}
	}
	if after := read(); !reflect.DeepEqual(after, before) {
		t.Errorf("with every Focus Kind off the readers read\n%+v\nwant as before\n%+v", after, before)
	}
	if got := tick(); len(got) != 0 {
		t.Errorf("the watch sent %v, want nothing", got)
	}
}

// Tickets sharing a pushed commit (code-central-418) get the same CI
// item and reason from every reader, though a later reason applies to
// one of them, and a response on the Project to the commit
// (code-central-432) clears the item for both alike.
func TestTicketsSharingAPushedCommitShareItsCIItemForEveryReader(t *testing.T) {
	f := itemWorld(t)
	f.cancelledBlocker(t, "acme-8", "acme-7")
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	f.clock.Advance(core.NotLandedAfter + time.Minute)
	it := f.theItem(t, core.ReasonCIFailed).CIItem
	if !slices.Equal(listed(it), []string{"acme-5", "acme-8"}) {
		t.Fatalf("item lists %v, want acme-5 and acme-8", listed(it))
	}
	for _, id := range []string{"acme-5", "acme-8"} {
		wantEverywhere(t, f, "acme", id, core.ReasonCIFailed)
		if v := f.getTicket(t, id); v.CIItem == nil || !reflect.DeepEqual(*v.CIItem, *it) {
			t.Errorf("%s: item %+v, want needs_me's %+v", id, v.CIItem, *it)
		}
		if a, _ := f.core.AttentionOf(ctx, "acme", id); a.CIFailedCommit != "abc123" {
			t.Errorf("%s: Attention CI failed at %q, want abc123", id, a.CIFailedCommit)
		}
	}

	if _, err := f.core.LandAnyway(human, "acme", core.TicketInput{Ticket: "acme-8"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"acme-5", "acme-8"} {
		wantEverywhere(t, f, "acme", id, core.ReasonAcceptedNotLanded)
		if v := f.getTicket(t, id); v.CIItem != nil || !v.LandAnyway {
			t.Errorf("%s: item %+v, land_anyway %v: want no item, landing anyway", id, v.CIItem, v.LandAnyway)
		}
	}
}

// theCIItem is the one CI item for reason in needs_me.
func (f fixture) theCIItem(t *testing.T, reason core.NeedsMeReason) *core.CIItemView {
	t.Helper()
	var out *core.CIItemView
	for _, row := range f.needsMe(t) {
		if row.Reason == reason && row.CIItem != nil {
			if out != nil {
				t.Fatalf("two %s items", reason)
			}
			out = row.CIItem
		}
	}
	if out == nil {
		t.Fatalf("no %s item", reason)
	}
	return out
}

// A Ticket a CI item lists carries the item's reason in the watch too:
// time passing gives it no timed reason, so the watch sends nothing.
func TestTheWatchSendsNothingForATicketACIItemListsAsTimePasses(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	tick := watch(t, f)
	tick()
	f.clock.Advance(core.NotLandedAfter + time.Minute)
	if got := tick(); len(got) != 0 {
		t.Fatalf("got %v, want no notice: acme-5 and acme-8 stay CI failed", got)
	}
}
