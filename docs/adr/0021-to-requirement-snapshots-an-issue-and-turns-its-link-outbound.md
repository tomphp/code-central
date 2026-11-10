# To Requirement snapshots an issue and turns its Link outbound

[ADR 0004](0004-an-integration-is-a-client-of-the-http-api.md) says the external system owns the definition (title, description, parent) of every linked Spec or Ticket. An Incoming Issue follows that rule: while it waits for triage, edits to the GitHub issue update it. But when the human turns one into a Requirement, the spec Stage rewrites the Requirement's body, which it couldn't do if GitHub owned it. We decided **To Requirement** snapshots the issue: Code Central creates a Requirement, a Spec with its Stage Tickets as `submit_requirement` makes, whose text is the issue's title, then its body, then the human's optional extra context under a heading of its own. The issue's Link moves to the Requirement's Spec as an **Outbound Link**: Code Central owns the Spec's definition, later edits to the issue don't flow in, and Code Central only writes to the issue, posting the work's progress and keeping its status label. The Incoming Issue is Cancelled, with the grilling Stage Ticket as its Replacement, and keeps its Link, detached, for history.

This amends ADR 0004's rule that the external system always owns a linked item's definition: a Link is now owned, as ADR 0004 describes, or outbound.

## Considered Options

- **Keeping the Requirement's Spec GitHub-owned.** Rejected: the spec Stage rewrites the Requirement's body into a Spec, which `update_linked_spec` would overwrite on the next edit to the issue, and which the spec Stage couldn't write at all.
- **One Spec per issue from the start.** Rejected: most issues end as Won't fix or wait as Drafts, so the Spec list would grow by one per issue; see [ADR 0022](0022-incoming-issues-go-on-one-long-lived-issues-spec-per-project.md).

## Consequences

An Outbound Link never counts for closed externally, and `update_linked_spec` and `update_linked_ticket` refuse it as they refuse a detached one. GitHub comments added after To Requirement don't come into Code Central; the human can add them as context. The human's `unlink` still applies to an Outbound Link, and detaching it stops every write to the issue.
