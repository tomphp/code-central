# Under Merge + Push with CI Tracking, a Ticket lands only on a passed CI result

[ADR 0013](0013-how-accepted-work-lands-is-a-project-setting-that-the-factory-carries-out.md) made Done mean pushed under Merge + Push: the Ticket lands once the push succeeds. With CI Tracking on, a pushed base can still be red, and a Ticket it blocks would then be claimed and started from a broken base. We decided that while Merge, Push and CI Tracking all hold, Done means on the remote and green. The Factory merges, pushes, reports the pushed commit and leaves the Ticket Accepted; on a later poll it lands the Ticket at its pushed commit once that commit's CI result is passed. This amends ADR 0013's "Done means pushed" for that case.

There is no new State: the Ticket waits as Accepted (or In Review, for a hand merge), so Blockers still clear only on land, now only on green. The Factory reads the CI result from the Ticket it already reads, so it needs no new read, and it is still the only one that lands. The gate doesn't serialise landing: other accepted Tickets keep merging and pushing while one waits, and each lands on its own pushed commit's CI result, never on a later Ticket's push.

The gate lifts when any of the three is turned off: with CI Tracking off, Push off or the Branch Landing Method, the Factory lands waiting Tickets on its next poll. Under Branch + Push the CI result is informational: the human's merge is the decision, so the Factory lands on seeing it, whatever the CI result.

A failed CI result leaves the Ticket Accepted, in Needs Me as CI failed, with four options for the human:

- **Re-run failed jobs**: the CI Watcher re-runs them, and the Ticket waits again.
- **Land anyway**: the human accepts the failure, say an unrelated flake, and the Factory lands the Ticket.
- **New fix Ticket**: a Ready Ticket on the same Spec carries the failure, and the failed Ticket lands as with Land anyway.
- **Send back to fix**: a new human row in the workflow, Accepted → Ready, beside the Factory's conflict row, with a note linking the failed run and naming the failed jobs. The next Claim fixes the work on the same branch, and the human reviews it again.

## Considered Options

- **CI as information only.** Rejected: Done would still mean pushed, so a red base could unblock dependent work, and the human would have to watch CI to catch it, which is what CI Tracking is for.

## Consequences

Landing under Merge + Push with CI Tracking takes as long as CI does. The base already holds the work while the Ticket waits, so later merges build on it; a Ticket sent back to fix is fixed on top. A Ticket waiting for 10 minutes with no CI Runs reported, or while the CI Watcher is in error, shows in Needs Me as CI unavailable, so a broken CI Watcher doesn't hold up work silently, and turning CI Tracking off lets it land.
