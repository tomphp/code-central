# Implement skips the tickets Stage

A Requirement's spec Stage can be accepted two ways: **Split into Tickets**, so the tickets Stage runs as usual, or **Implement**, which builds the Spec as one implementation Ticket. Implement accepts the spec Stage, creates the Ticket and Cancels the tickets Stage with that Ticket as its Replacement, in one operation. A Stage that is Done or Cancelled counts as passed, so the Requirement goes on to Build and `code-central:building` comes once the Stages are all Done or Cancelled. This is the one place where a Stage Label gates an operation: `implement` is valid only on an In Review `stage:spec` Ticket. Every other State and action still ignores Stage Labels. It amends [ADR 0007](0007-a-stage-is-a-ticket-marked-by-a-reserved-label.md).

## Considered Options

- **An `implement` option on `accept`.** Rejected: every caller would learn a field for one kind of Ticket.
- **Creating the tickets Stage lazily, on Split.** Rejected: it changes the shape of every Requirement, import and Stage strip.
- **The human creating and cancelling by hand.** Rejected: it isn't atomic, and the Factory can claim the tickets Stage in between.

## Consequences

ADR 0007's "never changes the workflow by them" now has this one named exception.
