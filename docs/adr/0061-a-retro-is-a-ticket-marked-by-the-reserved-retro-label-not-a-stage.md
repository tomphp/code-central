# A Retro is a Ticket marked by the reserved `retro` Label, not a Stage

This extends [ADR 0057](0057-wayfinder-labels-and-prototype-are-reserved-labels-read-on-the-read-side.md) to Retros. A Retro looks back over the sessions that delivered a Finished Ticket or Spec, and opens a Ticket for each improvement the human picks. We decided it is a Ticket labelled `retro`, a reserved type Label that goes only on a Ticket and counts toward the one reserved type Label a Ticket may carry (`stage:*`, `wayfinder:*`, `prototype` or `retro`). Code Central reads it on the read side only, as it reads `prototype`: it chooses the Ticket's Guide, `retro`, and changes no workflow rule. There is no new State, Ticket kind field or stored flag. Because model and effort settings are kept per Guide name, Retros get their own row in Options with no further work.

Stages are the fixed steps that turn a Requirement into Tickets. A Retro comes after delivery and can sit on any work, so it is not a step of anything.

## Considered Options

- **A fourth Stage.** Rejected: a Stage belongs to a Requirement and runs before its Tickets, while a Retro runs after, on work of any kind.
- **A Ticket kind field.** Rejected for the reasons ADR 0007 rejected a Stage field: every store file, door, Integration and Import would have to learn it, when a reserved Label already carries it.

## Consequences

A Retro never lands only because its Guide says to `finish` with no branch, so accepting it makes it Done; landing is still not decided by Label. Land per Spec leaves Retros out as it leaves out Prototypes. A Ticket that was labelled `retro` before this change becomes a Retro.
