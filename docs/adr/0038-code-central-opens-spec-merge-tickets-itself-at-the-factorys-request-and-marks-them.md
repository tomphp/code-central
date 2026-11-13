# Code Central opens Spec merge Tickets itself, at the Factory's request, and marks them

When a Spec Branch conflicts with the base, the Factory used to find, comment on or create the Ticket for merging the base into it. That sequence now lives in Code Central: the Factory calls one operation with the Spec, the conflicting files and the two tips, and Code Central writes the title and body. It is idempotent: it comments on the Spec's open marked Ticket (writing nothing if the same body is already there), else on an open unmarked Ticket with the same title, else creates a Ready marked Ticket. The mark is a stored field only this operation sets, and it gives the Ticket the Merge results Auto-accept kind. This amends [ADR 0028](0028-under-land-per-spec-a-specs-work-gathers-on-a-spec-branch-and-lands-together-and-blockers-within-the-spec-clear-on-accept.md), where the Factory opened the conflict Ticket.

Tickets opened before the mark existed stay unmarked and manual: they are not Merge results and are never auto-accepted, though the operation still comments on one.

## Considered Options

- **Matching the title.** Rejected: a human can rename a Ticket, and a title would give Auto-accept a meaning by accident.
- **A reserved Label the Factory sets.** Rejected: Auto-accept eligibility must not rest on a mark an agent-side client can write.

## Consequences

The operation has no MCP tool and no CLI command, so agents' tool lists don't grow. The Factory's conflict handling becomes one call.
