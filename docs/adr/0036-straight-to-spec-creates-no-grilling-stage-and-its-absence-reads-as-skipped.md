# Straight to Spec creates no grilling Stage, and its absence reads as skipped

A human submitting a Requirement may choose **Straight to Spec** (`submit_requirement` with `skip_grilling`), so the spec Stage works from the Requirement's text alone. Code Central then creates only the spec and tickets Stage Tickets, as it does for a Requirement written from a Map. Nothing records the choice: a Requirement with no grilling Stage and no `map` Tag is read as one whose grilling was skipped, and the Stage strip shows its Grilling segment struck through, as it shows a Cancelled Stage. `skip_grilling` and `map` may not be given together. This amends [ADR 0007](0007-a-stage-is-a-ticket-marked-by-a-reserved-label.md), under which a Requirement's Stages are exactly its Stage Tickets, and differs from [ADR 0026](0026-implement-skips-the-tickets-stage.md), where skipping the tickets Stage Cancels a Ticket that exists.

## Considered Options

- **Create the grilling Stage Ticket already Cancelled, as Implement does.** Rejected: it creates a Ticket that never runs, only so the strip can show it, and the human asked for none.
- **A `grilling: skipped` Tag on the Spec.** Rejected: it stores what the Spec's Tickets and `map` Tag already tell, and could disagree with them.

## Consequences

A missing grilling Stage has two meanings, told apart by the `map` Tag. The spec Stage Guide covers a third source for the spec Stage: the Requirement's text alone.
