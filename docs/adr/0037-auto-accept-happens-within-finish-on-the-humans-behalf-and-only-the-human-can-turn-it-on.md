# Auto-accept happens within `finish`, on the human's behalf, and only the human can turn it on

The human may let work they would accept anyway skip In Review. When a Ticket is auto-accept eligible, `finish` moves it to In Review and, in the same store write, carries out the human's accept: the same transition `accept` makes, or `implement` or Split into Tickets for a spec Stage, by its Spec recommendation. The Move's Actor is `human`, with one new field holding the cause (`project`, `ticket-label` or `spec-label`), so the record says why it was accepted without adding an Actor. What follows is unchanged: the Factory lands Accepted Tickets on its poll.

Only the human can turn it on, by two switches. One is a Project Option per Auto-accept kind, set over HTTP under the Project key with no MCP tool and no CLI command. The other is the reserved `auto-accept` Label on a Spec or Ticket, which a non-human Actor is refused. A setting applies only to later finishes, while adding the Label also accepts eligible work already In Review.

## Considered Options

- **A Factory poll that accepts eligible Tickets.** Rejected: it adds a delay, needs the Factory running, and makes the Factory a decider of what is accepted.
- **A "system" Actor.** Rejected: the accept is the human's, made ahead of time, and a new Actor would touch every place that reads Actors.
- **A Label that agents can set.** Rejected: an agent could accept its own work.
- **An opt-out Label.** Rejected: a Project's default would then be accept, and a forgotten Label would accept work the human never chose.

## Consequences

An accepted Ticket reads like one the human accepted, so its Move needs the cause to be told apart. Turning a kind on accepts nothing already In Review.
