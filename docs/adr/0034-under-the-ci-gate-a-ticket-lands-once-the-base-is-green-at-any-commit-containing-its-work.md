# Under the CI gate a Ticket lands once the base is green at any commit containing its work

[ADR 0019](0019-under-merge-and-push-with-ci-tracking-a-ticket-lands-only-on-a-passed-ci-result.md) made Done under the CI gate mean that the Ticket's own pushed commit passed CI. When that commit failed and later work fixed the failure, the Ticket stayed CI failed until the human dealt with it, although the base was green with its work in it: `code-central-416` sat like this after commit `8f8e74a`, which contained its commit, passed.

Under the CI gate (Merge + Push with CI Tracking on, a Spec's single push under Land per Spec included) a Ticket's CI result is now passed if any of its pushed commits pushed since it last moved back to Ready has passed; otherwise it is its latest pushed commit's. Each commit's result is worked out as before, from every Workflow that isn't ignored at its latest attempt, so one passing run beside a failing or running one doesn't pass it. The Factory lands the Ticket on its next poll, and the landing records the commit that passed. The CI Watcher watches every such pushed commit still running, once per commit, so an earlier commit that passes after a later one failed is seen.

Done under the CI gate therefore means the base was green at a commit containing the Ticket's work, not that the Ticket's own pushed commit was. This amends ADR 0019.

A commit pushed before the Ticket was last sent back never counts: the Ticket was sent back to change its work, and a green run of the old work says nothing about the new.

Under Branch + Push and Pull Request the CI result is unchanged: there each Ticket's pushed commit is its own branch, and the human's merge decides.

## Considered Options

- **Keep Done meaning the Ticket's own pushed commit passed.** Rejected: the human has to act on every Ticket a later fix already made green, and a Ticket waiting on CI holds its dependants up for nothing.

## Consequences

A later commit that reverts the Ticket's change, or deletes the test that failed, also lands the Ticket. We accept this: under Merge + Push the work is already in the base when CI runs, so the gate was never what kept a bad change out of the base; it keeps dependent work from starting on a red base, and a green base at a later commit does that.
