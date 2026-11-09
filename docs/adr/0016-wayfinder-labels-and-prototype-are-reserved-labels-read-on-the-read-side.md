# Wayfinder Labels and prototype are reserved Labels, read on the read side

This extends [ADR 0007](0007-a-stage-is-a-ticket-marked-by-a-reserved-label.md) to Maps and Prototypes. A Map is a Spec labelled `wayfinder:map`, a sibling of a Requirement, and each of its Tickets carries a Wayfinder Label for its type: `wayfinder:chart`, `wayfinder:grilling`, `wayfinder:research` or `wayfinder:task`. A Prototype is any Ticket labelled `prototype`, on any Spec. We decided that `wayfinder:` is a reserved prefix limited to those five names, and `prototype` a reserved Label, and that Code Central reads them on the read side only, as it reads `stage:`. There is no new record type, no new State, no Ticket kind field and no stored flag, and no workflow rule changes by Label. Label validation, on every write that sets Labels (create, add_labels, Import) and so for Integrations too, rejects an unknown `wayfinder:` name, keeps `wayfinder:map` to Specs and the Ticket types and `prototype` to Tickets, and lets a Ticket carry at most one reserved type Label (`stage:*`, `wayfinder:*` or `prototype`).

Code Central serves a Guide for each type (`map-chart`, `map-grilling`, `map-research`, `map-task` and `prototype`), beside the Stage Guides and the implementation Guide, which now covers only a Ticket with no reserved type Label. Each says *what* the Ticket produces, in Code Central operation names only. The Factory's table of skills by Guide name says *how*: which upstream skill starts the agent. That table is the only place naming upstream skills, so a change upstream changes only it.

## Considered Options

- **A Map record type, or a Ticket kind field.** Rejected for the reasons ADR 0007 rejected a Stage field: every store file, door, Integration and Import would have to learn it, when a reserved Label already carries it.
- **One Guide for the grilling Stage and a Map's grilling.** Rejected: the grilling Stage ends in a resolution comment for the spec Stage, a Map's grilling in an entry in the Map's Decisions so far, so their Guides differ.
- **Skip landing for Prototypes and research by Label.** Rejected, as ADR 0006 refused to decide landing by Label. Their Guides say to `finish` with no branch, so accept moves them straight to Done.
- **Guided progression in the service.** Rejected: the service has no scheduler ([ADR 0005](0005-the-local-factory-is-plain-code-starting-one-session-per-claim.md)). Approving a guided Map's next Ticket lives in the Factory's poll.

## Consequences

A Prototype or research Ticket skips landing only because `finish` names no branch. The `guided`, `prototype`, `research` and `map` Tags carry meaning only for the Factory and the read side: the Factory reads `guided` on a Map, and the read side shows the others; Code Central's workflow never reads them. A Ticket that was labelled `prototype` or `wayfinder:*` before this change is no longer run as an implementation Ticket.
