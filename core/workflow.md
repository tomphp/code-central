# Working a Ticket

The `states` and `actions` above are the rules Code Central enforces; anything they don't allow is rejected with `action_not_allowed` and the actions that are allowed. Who `agent` can take, the human can take too. `agent (holder)` actions need the live Claim ID as `claim_id`; reads, comments and the other triage writes don't.

1. `claim` takes the next claimable Ticket (Ready, no open Blockers, oldest first), or the one or the Spec you name. It returns a Claim ID and the full Ticket. `{"ticket": null}` means nothing can be claimed: stop.
2. Read the whole Ticket before starting: its description, its Spec, past Claims and how each ended, every Failure Report, comments and send-back notes. A Ticket that failed before failed for a reason; don't repeat the attempt that failed.
3. Work on a branch named after the Ticket ID (for example `acme-12`), and commit and push often, so the next session can pick the work up if yours ends. Code Central never checks this; it is the convention everyone relies on.
4. `finish` with a summary: what was done, where (the branch or commit), and what the reviewer should check. Then stop working this Ticket: the human accepts it to Done or rejects it to Ready.

Every write's response may carry a `next_step`. Do what it says, when it says.

If a write returns `stale_claim`, your Claim has ended (the human released or moved the Ticket, or another session holds it now). Stop working that Ticket at once and don't write to it again with that Claim ID. If you lose your Claim ID, for example after context compaction, stop the same way and tell the human, who can release the Ticket.

## Questions go to Rounds

While you hold a Claim, every question for the human is a Round posted with `post_round`, never printed in the terminal, even when the human is sitting at it: only the UI records answers. Put everything you need into one Round. Each Question has a title, a body and, when you have one, a recommended answer. A grilling skill's numbered questions become one Question each, with its recommendation.

After `post_round`, end your turn as its next step says. When the Ticket is Answered, `pick_up_answers` returns the answers, the dismissed Questions and the comments added since; carry on from there. A dismissed Question was deliberately left unanswered: decide without it. To grill in the terminal instead, don't claim.

## Three exits on trouble

When you can't carry on, take exactly one of these, by what would unblock you:

- **Ask a Question** (`post_round`) if the human can unblock this session: a decision, missing information, an access you lack.
- **Report a failure** (`report_failure`) if nothing can: something is broken that this Ticket can't fix, or the Ticket itself is wrong. All four fields are required: `what_broke` (a headline, then the symptom), `why`, `what_to_fix` and `where_work_stands`. The Ticket goes to Failed and your Claim ends; stop working this Ticket.
- **Give up cleanly** (`give_up`) if nothing is wrong but this session can't finish, for example it is running out of context or time. The note must say where the work stands (branch, what is done, what is left), so the next agent doesn't start cold. The Ticket goes back to Ready and your Claim ends; stop working this Ticket.

Never leave a claimed Ticket without one of these or `finish`.

## Sent back from Failed

The human sends a Failed Ticket back to Ready with an optional note. A send-back with no note means nothing has changed: the cause in the last Failure Report is still there, so check it before repeating the attempt, and report a failure again if it still stands.

## Defining work

Draft Tickets are still being defined and aren't claimable; `approve` moves one to Ready when it is fully defined, and `defer` (with a reason) moves a Ready one back. Record ordering with Blockers: only Done clears a Blocker. An agent cancels only Draft or Ready Tickets, and must name the Replacements that take the work over.

## Holding several Claims

You may hold any number of Claims, one per Ticket, and pass a Claim ID to a subagent; Code Central doesn't care which process sends it. Know the limits:

- Silence is tracked per session, because a ping can't say which Ticket it is for: a stuck subagent stays hidden while its main session is busy.
- Every Ticket claimed by one session shows the same resume command, and a resume restores only the main session.
- Code Central doesn't stop two Tickets touching the same files. Order such work with Blockers.
