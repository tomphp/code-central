# Under Unblock on push, a Blocker clears once its work is pushed, while landing still waits for green

Superseded by [ADR 0032](0032-blockers-clear-only-on-land-again-and-unblock-on-push-is-removed.md).

Under the CI gate ([ADR 0019](0019-under-merge-and-push-with-ci-tracking-a-ticket-lands-only-on-a-passed-ci-result.md)) a pushed Ticket waits Accepted until its CI result is passed, and Blockers clear only on land, so a Ticket it blocks waits as long as CI takes, though the base already holds the work. With slow CI and a chain of dependent Tickets, the Factory spends most of its time waiting. We decided on a per-Project CI Tracking option, **Unblock on push**, off by default. With it on, while the CI gate holds an Accepted Ticket with a pushed commit, the Ticket stops blocking while its latest pushed commit's CI result is running or passed. Running includes no CI Runs reported yet and a pending Re-run request, so the start of CI doesn't hold dependants up; CI unavailable still surfaces a Ticket whose CI never reports. This amends ADR 0019's "Blockers still clear only on land, now only on green" for Projects with the option on. Landing doesn't change: the Ticket still lands only on a passed CI result, so Done still means on the remote and green.

Blocking stays computed on read, with no stored flag: the blocked check takes the Project's settings and the current time, and reads the Project's CI Runs only for a Blocker Accepted with a pushed commit in a Project with the option on. Only the latest pushed commit counts, so a Ticket sent back and pushed again clears only on its new push's CI result, and a push from before it was last Ready doesn't count: between its re-accept and its new push it keeps blocking, even if its older push has gone green. An In Review Ticket merged by hand, which the gate also holds, keeps blocking until it lands. Under Branch + Push, or once the gate lifts, the option has no effect.

A failed CI result makes the Ticket block again, and so does no CI result at all, once its pushed commit's runs are pruned: no new work starts on a red base until the human deals with the failure through its options. A dependant already claimed carries on, since blocking only ever stops a claim; moving or interrupting it would strand work in progress, and a fix lands on top of the base either way.

## Considered Options

- **Land on push, and track CI after Done.** Rejected: a CI failure would then hit a Done Ticket, which can't be sent back, and Done would no longer mean green.

## Consequences

Dependent Tickets start from the pushed base while CI runs, so a chain of Tickets no longer waits on CI at each link. A dependant can be claimed and then find its Blocker CI failed; it carries on, and the human deals with the failure as for any other. When a CI Run report changes a Ticket's CI result, the Tickets it blocks get a change notice too, so their rows show them blocked or claimable live.
