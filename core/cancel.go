package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Cancel is the agent's cancel(ticket, replacements, reason?) on a Draft
// or Ready Ticket, and the human's cancel(replacements?) on any
// unfinished one. Anything the Ticket was blocking becomes blocked by its
// Replacements when read, so only this Ticket's file is written.
func (c *Core) Cancel(ctx context.Context, key string, in api.CancelInput) (api.Written[api.TicketView], error) {
	others := c.closedSpecOthers(ctx, key, in.Ticket)
	out, err := c.cancelOne(ctx, key, in)
	if err != nil {
		return out, err
	}
	// Cancelling a Spec's Pull request closed item cancels the Spec's
	// other waiting Tickets with it, and leaves the Spec Branch.
	for _, n := range others {
		if _, err := c.cancelOne(ctx, key, api.CancelInput{Ticket: api.Ref(domain.DisplayID(key, n)), Reason: in.Reason}); err != nil {
			return out, err
		}
	}
	// Cancelling an auto fix Ticket hands its failure back to the human.
	if err := c.sweepAutoFix(ctx, key); err != nil {
		return out, err
	}
	return out, nil
}

// closedSpecOthers are the Spec's other waiting Tickets when the human
// cancels the lead Ticket of a Spec whose pull request is closed.
func (c *Core) closedSpecOthers(ctx context.Context, key string, ref api.Ref) []int {
	if api.ActorOf(ctx) != api.Human {
		return nil
	}
	var others []int
	_ = c.store.View(func(r store.Reader) error {
		t, err := foundation.ResolveTicket(r, key, "cancel", "ticket", ref)
		if err != nil {
			return nil
		}
		if lead, _ := specPullRequestLead(r, key, t); !lead || !anyPullRequest(t, domain.PRClosed) {
			return nil
		}
		for _, o := range specWork(r, key, t) {
			if o.Number != t.Number {
				others = append(others, o.Number)
			}
		}
		return nil
	})
	return others
}

func (c *Core) cancelOne(ctx context.Context, key string, in api.CancelInput) (api.Written[api.TicketView], error) {
	actor := api.ActorOf(ctx)
	return c.updateTicketIn(key, "cancel", in.Ticket, in.Full, func(r store.Reader, t *domain.Ticket) (bool, error) {
		claim := 0
		if live, ok := liveClaim(*t); ok {
			claim = live.Number
		}
		before := *t
		now := c.now()
		if partlyLanded(key, *t) {
			// Its landed work stays where it landed: the Factory never
			// reverts (ADR 0049).
			return false, &api.Error{Code: api.ActionNotAllowed, AllowedActions: slices.DeleteFunc(allowedActions(t.State, actor), func(a string) bool { return a == "cancel" }), Message: fmt.Sprintf(
				"%s is Partly landed, its work already in the base of %s, so it can't be cancelled: land it in the rest of its Changed Repositories, or send it back",
				domain.DisplayID(key, t.Number), strings.Join(landedNames(key, *t), ", "))}
		}
		if err := transition(ctx, t, key, "cancel", "", now); err != nil {
			return false, err
		}
		if err := refuseCancelOfMerged(r, key, before); err != nil {
			return false, err
		}
		if actor != api.Human && len(in.Replacements) == 0 {
			return false, api.InvalidField("replacements", "an agent cancels a Ticket only by naming at least one Replacement that takes over its work")
		}
		reps, err := ticketRefs(r, key, "cancel", "replacements", in.Replacements)
		if err != nil {
			return false, err
		}
		if err := checkReplacements(r, key, before, reps); err != nil {
			return false, err
		}
		t.Replacements = reps
		if in.Reason != "" {
			addComment(t, actor, now, claim, in.Reason)
		}
		return true, nil
	})
}

// refuseCancelOfMerged refuses cancelling t once any of its pull requests
// has merged: that work is already in a base, and cancelling would lose
// track of it (ADR 0050).
func refuseCancelOfMerged(r store.Reader, key string, t domain.Ticket) error {
	var merged []string
	p, _ := r.Project(key)
	for _, pr := range t.PullRequests {
		if pr.State == domain.PRMerged {
			merged = append(merged, recordRepository(key, pr.Repository))
		}
	}
	if len(merged) == 0 {
		return nil
	}
	where := "its pull request has merged"
	if severalRepositories(p) {
		where = fmt.Sprintf("its pull request in %s has merged", strings.Join(namesInLandingOrder(p, merged), ", "))
	}
	return &api.Error{Code: api.ActionNotAllowed, Message: fmt.Sprintf("%s can't be cancelled: %s, so its work is already in a base. Land the rest of it, or send it back to redo it", domain.DisplayID(key, t.Number), where)}
}

// checkReplacements rejects Replacements for t that are t itself, that
// lead back to t through other Replacements, or that would make a Ticket
// t was blocking wait on itself.
func checkReplacements(r store.Reader, key string, t domain.Ticket, reps []int) error {
	var from []int
	for _, n := range reps {
		if n == t.Number {
			return api.InvalidField("replacements", "%s can't replace itself", domain.DisplayID(key, n))
		}
		followed := store.Follow(r, key, n)
		if slices.Contains(followed, t.Number) {
			return cycleError(key, []int{t.Number, n, t.Number})
		}
		from = append(from, followed...)
	}
	dependants := waitingOn(r, key, t.Number)
	if len(dependants) == 0 {
		return nil
	}
	cancelled := t
	cancelled.State, cancelled.Replacements = domain.Cancelled, reps
	ov := overlay{Reader: r, key: key, t: cancelled}
	for _, d := range dependants {
		if path := waitPath(ov, key, from, func(n int) bool { return n == d }); path != nil {
			return cycleError(key, append([]int{d}, path...))
		}
	}
	return nil
}
