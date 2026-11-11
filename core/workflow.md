# Working a Ticket

The `states` and `actions` above are the rules Code Central enforces; anything they don't allow is rejected with `action_not_allowed` and the actions that are allowed. Who `agent` can take, the human can take too. `agent (holder)` actions need the live Claim ID as `claim_id`; reads, comments and the other triage writes don't.

1. `claim` takes the next claimable Ticket (Ready, no open Blockers, oldest first), or the one or the Spec you name. It returns a Claim ID, the Ticket and its `stage_guide`: what the Ticket must produce, where it lands and how to finish it. Follow the guide; `get_stage_guide` serves it again. `{"ticket": null}` means nothing can be claimed: stop. `claim` on a Ticket your session already holds, with your `session_id`, returns the same Claim and writes nothing.
2. Read the Ticket before starting: its description, its Spec, past Claims and how each ended, every Failure Report and every comment, send-back notes too. `claim` returns these; `get_ticket` with `parts` returns what `left_out` counts, such as `rounds` on a redo. A Ticket that failed before failed for a reason; don't repeat the attempt that failed. Read the Spec with `get_spec` and `parts: ["body"]`, or with `sections` for a large Spec (choose them from the `outline` it returns), and add `tickets` only when sibling Tickets matter.
3. Work on a branch named after the Ticket ID (for example `acme-12`), cut from the base or, under Land per Spec, the Spec Branch, and commit often, so the next session can pick the work up if yours ends. Don't push or open a pull request.
4. `finish` with a summary: what was done, where (the branch or commit), and what the reviewer should check. When the work is on a branch, also pass `branch` and its head `commit`. Then stop working this Ticket: the human accepts it or rejects it to Ready.

## Accepted work lands

Accepting a Ticket whose finish named a `branch` moves it to Accepted to wait to land; any other Ticket goes straight to Done. Whoever merges the branch calls `land` with the commit that put the work in the base, moving the Ticket to Done; `land` also works from In Review, for work already merged. Only Done clears a Blocker, so a Ticket blocked by Accepted work waits until it lands. Under Land per Spec, a Spec's work lands together; a same-Spec Blocker clears once on the Spec Branch. If the merge conflicts, `send_back` the Ticket to Ready with a note naming the base commit and the conflicting files. Under a Project's Branch Landing Method, an Accepted Ticket shows the human `to_merge` at once, and lands when the merge is seen; under Merge, one Accepted over 10 minutes shows `accepted_not_landed`.

The human accepts a Requirement's spec Stage In Review with `accept` (Split into Tickets: the tickets Stage runs) or with `implement`, which in one write accepts it, creates one Ready `Implement: <Spec title>` Ticket for the whole Spec and Cancels the tickets Stage with that Ticket as its Replacement. A Cancelled tickets Stage whose Replacement is an implementation Ticket was skipped this way: build the Spec from that Ticket, and write the glossary terms and ADRs it records.

A write returns an acknowledgement (`id`, `updated`, `changed`, …), not the record; `full: true` returns the record. Every write's response may carry a `next_step`. Do what it says, when it says.

## Questions go to Rounds

While you hold a Claim, every question for the human is a Round posted with `post_round`, never printed in the terminal, even when the human is sitting at it: only the UI records answers. Put everything you need into one Round. Each Question has a title, a body and, when you have one, a recommended answer. A grilling skill's numbered questions become one Question each, with its recommendation.

After `post_round`, end your turn as its next step says. When the Ticket is Answered, `pick_up_answers` returns the answers, the dismissed Questions and the comments added since; carry on from there. A dismissed Question was deliberately left unanswered: decide without it. To grill in the terminal instead, don't claim.

<!-- trouble.md -->

## Defining work

Draft Tickets are still being defined and aren't claimable; `approve` moves one to Ready when it is fully defined, and `defer` (with a reason) moves a Ready one back. Record ordering with Blockers: only Done clears a Blocker. An agent cancels only Draft or Ready Tickets, and must name the Replacements that take the work over.

## Holding several Claims

You may hold any number of Claims, one per Ticket, and pass a Claim ID to a subagent. Know the limits:

- Silence is tracked per session, because a ping can't say which Ticket it is for: a stuck subagent stays hidden while its main session is busy.
- Every Ticket claimed by one session shows the same resume command, and a resume restores only the main session.
- Code Central doesn't stop two Tickets touching the same files. Order such work with Blockers.
