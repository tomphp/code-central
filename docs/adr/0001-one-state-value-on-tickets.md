# One State value on Tickets, with waiting on the human as States

Code Central exists to show what needs the human, so a Ticket's position must be readable from one value. We decided that State is a single value on a Ticket, in one fixed workflow shared by every Project, and that waiting on the human is expressed as States (`Needs Answer`, `Answered`, `Failed`, `In Review`) rather than as flags beside a delivery State. A Spec has no stored State: its State is derived from its Tickets, and any work on a Spec, including grilling, is a Ticket on it.

## Considered Options

- **Delivery-only States plus flags for "has an open Question" and "has failed".** Rejected because a Ticket's position would then need several fields to read, and the UI, agents and Integrations would each have to combine them the same way.
- **A separate workflow on Specs.** Rejected because a grilling Ticket meets the same need, and Factories then only ever look for `Ready` Tickets. A Spec with no Tickets shows as `Draft`.
- **Per-Project workflows.** Rejected because every Factory, the UI and each Integration would have to cope with States they don't know about. A fixed workflow can become configurable later, but not the reverse.

## Consequences

Blocked is computed, not a State: a blocked `Ready` Ticket stays `Ready` but isn't claimable. A Silent Claim is the one exception to waiting on the human being a State: an `In Progress` Ticket with no recent activity stays `In Progress` and shows in Needs Me, because silence is a guess about a session rather than a fact about the Ticket, and a State would flip back and forth during a long build. An Integration has to map an external workflow onto this fixed one rather than extend it.
