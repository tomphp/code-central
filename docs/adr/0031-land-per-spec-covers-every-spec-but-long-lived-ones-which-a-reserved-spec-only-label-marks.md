# Land per Spec covers every Spec but Long-lived ones, which a reserved Spec-only Label marks

[ADR 0028](0028-under-land-per-spec-a-specs-work-gathers-on-a-spec-branch-and-lands-together-and-blockers-within-the-spec-clear-on-accept.md) gathered each Requirement Spec's work on a Spec Branch. Other Specs, such as the Spec behind a Map or one written by hand, kept landing Ticket by Ticket, and so did every Spec that never finishes, a CI Rule's Spec or a Project's Issues Spec, where waiting for the Spec to be done would hold its Tickets for good.

Land per Spec now covers the implementation Tickets of every Spec except those of a **Long-lived Spec**: a Spec labelled `long-lived`. The Label is reserved and Spec-only: adding it to a Ticket, on create, edit or `add_labels`, is refused as invalid, with a message saying it goes on a Spec. On a Spec anyone, the human or an agent, may add or remove it. Creating a CI Rule's Spec or a Project's Issues Spec gives it the Label, and a one-time store backfill, recorded as applied by the store format, labels the ones that already exist, so a Label the human later removes isn't added back.

A Spec that isn't a Requirement has no Stages to finish, so it is ready to land once every implementation Ticket on it is Accepted, Done or Cancelled. A Ticket added after the Spec has landed is Accepted and waits on its own, and goes in a second batch on the Spec Branch.

Gaining the Label lands the Spec's waiting Tickets at once, since the Spec no longer waits for the rest of its Tickets. Losing it affects only Tickets accepted afterwards, which then wait for the Spec.

It amends [ADR 0028](0028-under-land-per-spec-a-specs-work-gathers-on-a-spec-branch-and-lands-together-and-blockers-within-the-spec-clear-on-accept.md): which Specs Land per Spec covers.

## Considered Options

- **Inferring "long-lived" from how a Spec was made.** Rejected: a hand-written Spec can be as long-lived as a CI Rule's, and a Label the human can change is a plain way to say so.
- **A "land this Spec" action from the human.** Rejected: it makes every Spec's landing a chore, and a Long-lived Spec would need it forever.
- **A per-Spec setting.** Rejected: a Label already does the job, and it shows in the Spec's reads without a new field.

## Consequences

The Label and the refusal on Tickets come first; Land per Spec reads the Label in a later change, so until then landing is unchanged.
