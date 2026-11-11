# A Stage is a Ticket marked by a reserved Label

A Requirement is defined through three Stages (grilling, spec and tickets), and each Stage is an ordinary Ticket on the Requirement's Spec. We decided that a Ticket's Stage is a Label with the reserved `stage:` prefix, not a field or kind of Ticket, and that a Spec is a Requirement only because it has `stage:*` Tickets: nothing on the Spec stores it. Code Central reads Stage Labels on the read side only. It serves the Stage Guide for a Ticket's Stage, and one for implementation Tickets, and it folds Stage Tickets into a Requirement for the UI. It never changes the workflow by them: every State and action works on a Stage Ticket as it does on any other. So Labels still have no workflow meaning, and [ADR 0006](0006-accepted-work-lands-before-it-is-done.md), which refused to decide by Label which Tickets land, still holds. The Stage Guide says what a Stage must produce, where it lands and how to finish it, written in operation names so every Factory and front door can use it. How to run the agent, such as which skill or command starts it, belongs to the Factory.

## Considered Options

- **A Stage field or Ticket kind.** Rejected because every store file, front door, Integration and Import would have to learn a new field to carry what a Label already carries, and a Factory would still need to recognise the Stage. A reserved prefix gives the same recognition with no change to the Ticket's shape.
- **A stored Requirement flag or Label on the Spec.** Rejected because it could disagree with the Spec's Tickets. Deriving it from the Tickets can't drift.
- **Each Factory owns its own Stage instructions.** Rejected because sandcastle and the local Factory would then define what a Stage produces differently. Code Central owns *what* and each Factory owns *how*, which keeps the model Factory-agnostic.

## Consequences

`stage:` is a reserved Label prefix, and only the names Code Central knows may use it. Because Stage Labels don't drive the workflow, the In Review gate is the same for a Stage as for any Ticket, and Stage Tickets accept straight to Done because `finish` named no branch, not because of their Label.

[ADR 0026](0026-implement-skips-the-tickets-stage.md) makes one named exception: `implement` is valid only on an In Review `stage:spec` Ticket.
