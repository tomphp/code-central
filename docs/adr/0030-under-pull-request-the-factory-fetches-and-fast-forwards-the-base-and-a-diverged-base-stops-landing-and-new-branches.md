# Under Pull Request the Factory fetches and fast-forwards the base, and a diverged base stops landing and new branches

Under Pull Request the host, not the Factory, changes the base: a merged pull request lands there, with a merge commit the local base doesn't hold. [ADR 0013](0013-how-accepted-work-lands-is-a-project-setting-that-the-factory-carries-out.md) said the Factory never pulls, rebases or force-pushes, which would leave new branches cut from a base that lacks what the host merged and a Ticket's merge commit forever missing locally.

Under Pull Request, before it lands anything and before it cuts any new branch, Spec Branches included, the Factory fetches the Push remote and **fast-forwards** the local base to the remote base. It still never merges into, rebases or force-pushes the base. If the base isn't checked out, the ref is moved; if it is checked out with uncommitted changes in the way, the poll waits, as landing does today. A Ticket whose pull request is reported merged lands at the reported merge commit once that commit is in the fetched base, and otherwise waits for a later poll.

A local base that holds commits the remote base doesn't can't be fast-forwarded: it is **Base diverged**. The Factory reports it with `report_base`, giving `current` or `diverged` and the local and remote commits, which Code Central keeps in memory per Project like the heartbeat ([ADR 0025](0025-a-factory-announces-itself-with-a-heartbeat-that-lives-only-in-memory.md)) and drops when the heartbeat lapses. While it is diverged the Factory neither lands nor claims work that would cut a branch, and the human sees one Needs Me item per Project with the fix: reconcile the local base with the remote by hand, for example by resetting or rebasing it. A later poll that fast-forwards cleanly reports the base current and the Factory carries on by itself. A failed fetch, such as the network being down, is a warning line and a retry on the next poll, not Base diverged.

It amends [ADR 0013](0013-how-accepted-work-lands-is-a-project-setting-that-the-factory-carries-out.md)'s "never pulls, rebases or force-pushes" for this Landing Method only: it still never rebases or force-pushes, and fetches and fast-forwards only under Pull Request.

## Considered Options

- **Cutting branches from the remote-tracking base.** Rejected: the local base would stay behind, so the human's own checkout and every later comparison would disagree with what new work started from.
- **Detecting merges by git ancestry.** Rejected: a squash or rebase merge leaves the branch's commits out of the base, so the Ticket would never land; the host's merge commit is the reliable signal ([ADR 0029](0029-pull-request-is-a-third-landing-method-carried-out-by-a-pull-request-watcher-and-lands-at-the-code-hosts-merge-commit.md)).

## Consequences

A human who commits to the local base while using Pull Request can stop the Factory with Base diverged until they reconcile it.
