# A stopped session is a mark on its Claim, not a State or a move to Draft

The human needs to stop a Factory's session on a Ticket without losing it.

## Decision

Stop keeps the Claim. The Ticket stays In Progress or Answered, and the Claim is marked Stopped: its latest Stop, recorded with its time, has no Resume. Resume records the time of the Resume and clears the mark. Only a Factory Claim, one that named its Factory and host on `claim`, can be Stopped, so an old Factory, which never names itself, never offers Stop.

The Factory hears a Stop through its existing wake and its per-poll reconcile, not a new channel. Ending the Claim by any move ends the mark with it; the holder's own State changes (`post_round`, an answer) leave it Stopped.

## Alternatives considered

- **Move the Ticket to Draft**, as the Requirement first suggested. This ends the Claim and loses the session.
- **A new Stopped State.** It goes against [ADR 0001](0001-one-state-value-on-tickets.md)'s single State and would need moves in and out of every flow that reads In Progress.

## Consequences

- The session stays resumable ([ADR 0010](0010-a-claims-own-session-can-claim-it-again-and-gets-the-same-claim-back.md)) and the workflow's States are untouched.
- Every reader of a Claim that means "working" must exclude Stopped Claims: Silent and Long Claims, Needs Me and the Stage strip.
