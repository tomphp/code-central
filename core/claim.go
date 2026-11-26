package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// stopResult is the result of action name, which ends the agent's work:
// the Acknowledgement, or with full the Ticket, and the next step, "stop
// working this Ticket", from the write's result v.
func stopResult(v api.Written[api.TicketView], name string) api.FinishResult {
	if v.Record != nil {
		return api.FinishResult{Ticket: v.Record, NextStep: nextStep(name)}
	}
	return api.FinishResult{Acknowledgement: &v.Acknowledgement, NextStep: nextStep(name)}
}

// stopWorking is the next step after an action that ends the agent's work.
const stopWorking = "stop working this Ticket"

// Claim is claim(spec?, ticket?, session_id?, working_dir?, resume_command?): the next claimable Ticket
// in claim order (or the named one) moves to In Progress under a new
// Claim, in one atomic step under the store's write lock. A named Ticket
// whose live Claim has the caller's non-empty session ID returns that
// Claim instead, writing nothing.
func (c *Core) Claim(ctx context.Context, key string, in api.ClaimInput) (api.ClaimResult, error) {
	if in.Spec != "" && in.Ticket != "" {
		return api.ClaimResult{}, api.InvalidField("ticket", "pass spec or ticket, not both")
	}
	var res api.ClaimResult
	// Made once, and kept by a retried Update, so the ID committed is the
	// one returned; claiming again takes none.
	var id string
	err := c.store.Update(func(w store.Writer) error {
		if err := foundation.Writable(w, key); err != nil {
			return err
		}
		rule := c.blockRule(w, key)
		target, found, err := claimTarget(rule, in)
		if err != nil || !found {
			return err
		}
		t, _ := w.Ticket(key, target.Number, whole...)
		if live, ok := liveClaim(t); ok && in.Ticket != "" {
			if in.SessionID == "" || live.SessionID != in.SessionID {
				return heldError(key, t, live, in.SessionID, api.ActorOf(ctx))
			}
			// The holding session claims again: its Claim, nothing written
			// (ADR 0010).
			res = c.claimResult(w, key, t, live.ID)
			return nil
		}
		if t.State == domain.Ready && rule.blocked(t) {
			return rule.blockedError(t, api.ActorOf(ctx))
		}
		now := c.now()
		cl := domain.Claim{Number: len(t.Claims) + 1, SessionID: in.SessionID, Factory: in.Factory, Host: in.Host, Started: now}
		setResume(&cl, in.WorkingDir, in.ResumeCommand)
		t.Claims = append(t.Claims, cl)
		if err := transition(ctx, &t, key, "claim", "", now); err != nil {
			return err
		}
		if id == "" {
			id = c.ids.ClaimID()
		}
		t.Claims[len(t.Claims)-1].ID = id
		if err := whole.put(w, key, &t); err != nil {
			return err
		}
		res = c.claimResult(w, key, t, id)
		return nil
	})
	return res, err
}

// claimResult is claim's result for Claim id on t.
func (c *Core) claimResult(r store.Reader, key string, t domain.Ticket, id string) api.ClaimResult {
	v := defaultView(c.ticketView(r, key, t))
	p, _ := r.Project(key)
	g := stageGuide(api.GuideName(t.Labels), p)
	mc := modelChoice(p, t)
	return api.ClaimResult{ClaimID: id, Ticket: &v, StageGuide: &g, ModelChoice: &mc}
}

// heldError is claim's action_not_allowed on a named Ticket whose live
// Claim isn't the caller's, saying who holds it and what to do.
func heldError(key string, t domain.Ticket, live *domain.Claim, sessionID string, actor api.Actor) error {
	msg := fmt.Sprintf("%s is held by Claim %d of another session; claim another Ticket, or ask the human to release it",
		domain.DisplayID(key, t.Number), live.Number)
	if sessionID == "" {
		msg += "; if this session holds it, call claim again with your session_id"
	}
	return &api.Error{Code: api.ActionNotAllowed, Message: msg, AllowedActions: allowedActions(t.State, actor)}
}

// claimTarget is the named Ticket, or the first claimable one (in the
// Spec, if given) in claim order. found is false when nothing is claimable.
func claimTarget(rule blockRule, in api.ClaimInput) (t domain.Ticket, found bool, err error) {
	r, key := rule.r, rule.p.Key
	if in.Ticket != "" {
		t, err := foundation.ResolveTicket(r, key, "claim", "ticket", in.Ticket)
		if err != nil {
			return t, false, err
		}
		// A Ticket that isn't Ready is rejected by the transition with
		// its State as the reason.
		return t, true, nil
	}
	specN := 0
	if in.Spec != "" {
		sp, err := foundation.ResolveSpec(r, key, "claim", "spec", in.Spec)
		if err != nil {
			return t, false, err
		}
		specN = sp.Number
	}
	f := store.TicketFilter{States: []domain.State{domain.Ready}, Order: store.ByClaim}
	if specN != 0 {
		f.Spec = []int{specN}
	}
	for _, t := range r.FindTickets(key, f) {
		if !rule.blocked(t) {
			return t, true, nil
		}
	}
	return t, false, nil
}

// Finish is finish(ticket, claim_id, summary): the holder moves In
// Progress to In Review, with the summary stored as a comment. A spec
// Stage's finish also gives its Spec recommendation. Then, in the same
// write, an auto-accept eligible Ticket is accepted on the human's
// behalf (ADR 0037), unless the finish dropped one of its Changed
// Repositories; the result is the same either way.
func (c *Core) Finish(ctx context.Context, key string, in api.FinishInput) (api.FinishResult, error) {
	if strings.TrimSpace(in.Summary) == "" {
		return api.FinishResult{}, api.InvalidField("summary", "finish needs a summary: what was done, where (branch or commit) and what the reviewer should check")
	}
	rec := domain.Recommendation(strings.TrimSpace(string(in.Recommendation)))
	if rec != "" && !rec.Valid() {
		return api.FinishResult{}, api.InvalidField("recommendation", "recommendation %q is neither %q nor %q", in.Recommendation, domain.RecommendImplement, domain.RecommendSplit)
	}
	var v api.Written[api.TicketView]
	err := c.store.Update(func(w store.Writer) error {
		if err := foundation.Writable(w, key); err != nil {
			return err
		}
		t, h, err := readForWrite(w, key, "finish", "ticket", in.Ticket, whole)
		if err != nil {
			return err
		}
		// With full the before view isn't compared, so it isn't built.
		var before api.TicketView
		if !in.Full {
			before = c.ticketView(w, key, t)
		}
		now := c.now()
		if err := c.actOn(ctx, &t, key, "finish", in.ClaimID, in.Summary, now); err != nil {
			return err
		}
		spec := domain.StageOf(t.Labels) == "spec"
		switch {
		case spec && rec == "":
			return api.InvalidField("recommendation", "finishing a spec Stage needs a recommendation: implement or split, with a one-line reason in the summary")
		case !spec && rec != "":
			return api.InvalidField("recommendation", "only a spec Stage takes a recommendation: finish again without it")
		}
		t.Recommendation = rec
		p, _ := w.Project(key)
		was := changedRepositoryNames(key, t)
		if err := settleFinish(p, &t, in); err != nil {
			return err
		}
		t.Updated = now
		// The result is read here, before any auto-accept, so the agent
		// sees the same whether or not one follows.
		if v, err = acknowledge(before, c.ticketView(w, key, t), in.Full); err != nil {
			return err
		}
		if dropsRepositories(key, t, was) {
			// The Factory's finish check comes first.
			return h.put(w, key, &t)
		}
		return c.autoAccept(ctx, w, key, t, now)
	})
	if err != nil {
		return api.FinishResult{}, err
	}
	return stopResult(v, "finish"), nil
}

// Accept is the human's accept(ticket): In Review to Accepted when the
// work's finish named a branch, so it waits to land (ADR 0006), and to
// Done otherwise.
func (c *Core) Accept(ctx context.Context, key string, in api.TicketInput) (api.Written[api.TicketView], error) {
	return c.updateTicket(key, "accept", in.Ticket, in.Full, func(t *domain.Ticket) (bool, error) {
		return true, acceptStep(ctx, t, key, "", c.now())
	})
}

// acceptStep is the accept transition on t, the step the human's accept
// and an auto-accept share: In Review to Accepted when the work's finish
// named a branch in any of its Changed Repositories, and to Done
// otherwise. The Move records cause, empty for the human's own accept.
func acceptStep(ctx context.Context, t *domain.Ticket, key string, cause domain.AutoAcceptCause, now time.Time) error {
	to := domain.Done
	if hasBranchToLand(key, *t) {
		to = domain.Accepted
	}
	if err := transitionTo(ctx, t, key, "accept", "", to, now); err != nil {
		return err
	}
	t.Moves[len(t.Moves)-1].Cause = cause
	return nil
}

// Land is land(ticket, commit, repository?): human or agent, once the
// work is in the base of one of the Ticket's Changed Repositories, the
// Home Repository's unless it names another (ADR 0049). It records that
// Repository's landed commit. Once every Changed Repository has landed it
// moves the Ticket from Accepted or In Review to Done, with the commit on
// the Move, Via merge; until then the Ticket stays where it is, Partly
// landed once Accepted.
func (c *Core) Land(ctx context.Context, key string, in api.LandInput) (api.Written[api.TicketView], error) {
	commit := strings.TrimSpace(in.Commit)
	if commit == "" {
		return api.Written[api.TicketView]{}, api.InvalidField("commit", "land needs the commit that put the work into the base")
	}
	return c.updateTicketIn(key, "land", in.Ticket, in.Full, func(r store.Reader, t *domain.Ticket) (bool, error) {
		p, _ := r.Project(key)
		name, err := landingRepository(p, *t, in.Repository)
		if err != nil {
			return false, err
		}
		now := c.now()
		i := slices.IndexFunc(t.Repositories, func(cr domain.ChangedRepository) bool { return cr.Name == name })
		if !landsLast(changedRepositories(key, *t), name) {
			if _, err := check("land", t.State, "", api.ActorOf(ctx), domain.DisplayID(key, t.Number)); err != nil {
				return false, err
			}
			if l := t.Repositories[i].Landed; l != nil && l.Commit == commit {
				return false, nil
			}
			t.Repositories[i].Landed = &domain.Landed{Commit: commit, Time: now}
			return true, nil
		}
		if err := c.actOn(ctx, t, key, "land", "", "", now); err != nil {
			return false, err
		}
		m := &t.Moves[len(t.Moves)-1]
		m.Via, m.Commit = "merge", commit
		if i >= 0 {
			t.Repositories[i].Landed = &domain.Landed{Commit: commit, Time: now}
		}
		return true, nil
	})
}

// Reject is the human's reject(ticket, comment): In Review back to Ready,
// with the comment required. The Ticket keeps its created time, so it
// keeps its place near the front of the claim order.
func (c *Core) Reject(ctx context.Context, key string, in api.RejectInput) (api.Written[api.TicketView], error) {
	if strings.TrimSpace(in.Comment) == "" {
		return api.Written[api.TicketView]{}, api.InvalidField("comment", "reject needs a comment saying what to change")
	}
	return c.act(ctx, key, in.Ticket, in.Full, "reject", "", in.Comment)
}

// act runs one action from the workflow table on a Ticket in one write:
// the transition, then the action's note (if any) as a comment under the
// Claim it was made on. Its result is the Acknowledgement, or with full
// the Ticket.
func (c *Core) act(ctx context.Context, key string, ref api.Ref, full bool, name, claimID, note string) (api.Written[api.TicketView], error) {
	return c.actThen(ctx, key, ref, full, name, claimID, note, nil)
}

// actThen is act with then applied to the Ticket after the transition,
// in the same write. An error from then abandons the write.
func (c *Core) actThen(ctx context.Context, key string, ref api.Ref, full bool, name, claimID, note string, then func(*domain.Ticket) error) (api.Written[api.TicketView], error) {
	return c.updateTicket(key, name, ref, full, func(t *domain.Ticket) (bool, error) {
		if err := c.actOn(ctx, t, key, name, claimID, note, c.now()); err != nil {
			return false, err
		}
		if then != nil {
			if err := then(t); err != nil {
				return false, err
			}
		}
		return true, nil
	})
}

// actOn is one action from the workflow table on t at now: the
// transition, then the action's note (if any) as a comment under the
// Claim it was made on.
func (c *Core) actOn(ctx context.Context, t *domain.Ticket, key, name, claimID, note string, now time.Time) error {
	claim := 0
	if live, ok := liveClaim(*t); ok {
		claim = live.Number
	}
	if err := transition(ctx, t, key, name, claimID, now); err != nil {
		return err
	}
	if note != "" {
		addComment(t, api.ActorOf(ctx), now, claim, note)
	}
	return nil
}
