# Under Merge + Push the Factory keeps the local base level with the remote

Under Merge with Push on, the Factory never fetched. When someone pushed to the remote base from another clone, its local base stayed behind, new Ticket branches were cut from a base lacking those commits, and its next push was rejected because it never forces. It then commented "can't land yet" on the Ticket and retried every poll until the human pulled the base by hand. [ADR 0013](0013-how-accepted-work-lands-is-a-project-setting-that-the-factory-carries-out.md) said the Factory never pulls, rebases or force-pushes, and [ADR 0030](0030-under-pull-request-the-factory-fetches-and-fast-forwards-the-base-and-a-diverged-base-stops-landing-and-new-branches.md) limited fetching and fast-forwarding to Pull Request.

Under Merge + Push, before it lands anything and before it cuts any branch, Spec Branches included, the Factory fetches the Push remote and brings the local base level with the remote base, whether or not CI Tracking is on:

- **Level, or the local base ahead**: nothing to do. Ahead is normal under Merge, since the Factory merges locally and then pushes.
- **Behind**: fast-forward, as under Pull Request. If the base is checked out with uncommitted changes in the way, the poll waits.
- **Both moved**: move the local base onto the remote tip, then carry on and push. The Factory's own unpushed merges are rebuilt on the new tip, each with its Ticket's branch (or the Spec Branch) untouched as the second parent, and the human's own unpushed commits are replayed on top. A plain `git rebase`, with or without `--rebase-merges`, rewrites the Ticket's own commits, and the Factory decides a Ticket's work is in the base by its head being an ancestor of the base, so the merges are rebuilt the way they were made: `merge-tree` of the new tip and the original second parent, then `commit-tree`. The move is all or nothing, and when a rebuilt merge or a replayed commit conflicts the local base is left as it was and the Factory reports **Base diverged**.

It still never force-pushes and never rewrites a pushed commit. A failed fetch is a warning line and a retry on the next poll, as under Pull Request. Merge without Push has no remote and is unchanged, as are Branch and Pull Request.

This amends [ADR 0013](0013-how-accepted-work-lands-is-a-project-setting-that-the-factory-carries-out.md)'s "never pulls, rebases or force-pushes" and [ADR 0030](0030-under-pull-request-the-factory-fetches-and-fast-forwards-the-base-and-a-diverged-base-stops-landing-and-new-branches.md)'s "fetches and fast-forwards only under Pull Request": under Merge + Push the Factory fetches, fast-forwards and rebases unpushed local commits onto the remote.

## Considered Options

- **Stopping with Base diverged whenever both sides moved.** Rejected: a push landing in the seconds between the Factory's fetch and its own push would need a hand fix every time.
- **A setting to choose between the two.** Rejected: the other value would only make the human do the same rebase by hand.

## Consequences

Base diverged now also shows in Needs Me and Focus Mode for a Project under Merge + Push, where it means the Factory couldn't move the base onto the remote's. Under Pull Request it is unchanged: a base with local commits still stops with Base diverged, because there the Factory never commits to the local base.
