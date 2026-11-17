# Check every Repository before landing any; no all-or-nothing landing across Repositories

A Ticket or Spec can change several Repositories, each with its own remote. No tool lands atomically across separate remotes (research in code-central-832): each push can fail on its own.

So under Merge the Factory works out the merge in every Changed Repository before it moves any base. On any conflict it lands nothing, and the Ticket goes back to Ready with a comment naming each conflicting Repository and its files. Then it makes the merge, advances the base and pushes in each Repository, in the landing order (the Home Repository first by default), so producers can land before consumers without per-Ticket work. A failed base move or push is retried on later polls. The Factory never force-resets and never reverts; a revert is the human's call. It records a landed commit per Repository, and the Ticket lands only once it has landed in every Changed Repository.

Under Branch, a Ticket lands once its branch is merged in every Changed Repository. Land per Spec follows the same rule: a Spec Branch per Changed Repository, all checked before any lands, one Spec merge Ticket per Spec naming each conflicting Repository, and the same "Merge spec" commit in each Repository in the landing order.

Base missing and Base diverged are per Repository, and only work touching that Repository waits to land. No landing setting becomes per Repository; only the landing order is new. Pull Request and the CI gate follow the "every Changed Repository" rule; their per-Repository mechanics are the next ADR's.

[ADR 0058](0058-the-base-is-a-project-setting-not-the-factorys.md)'s "No per-Ticket base is stored" still holds: a landed commit per Repository records where the work landed, not a base.

## Considered Options

- **True atomicity across Repositories.** Rejected: no remote offers it, and faking it needs rollback.
- **Landing each Repository unchecked.** Rejected: a conflict found late leaves a Ticket landed in part, the case checking first avoids.
- **A per-Ticket order.** Rejected: it needs work on every Ticket, and one Project-wide order covers producers before consumers.
- **A new State or Needs Me reason for partial landing.** Rejected: checking first makes it rare, and a status and the existing Accepted but not landed reason cover it.
- **One Spec merge Ticket per Repository.** Rejected: it splits one Spec's conflict across Tickets that must all be done before it lands.

## Consequences

- With checking first, a failed push or base move is the only way a Ticket lands in part. Such a Ticket stays Accepted and shows Partly landed until every Repository has landed, and Accepted but not landed names the missing Repositories.
- A broken base in a rarely used Repository holds up only the work that touches it.
- A single-repository Project is unchanged.
