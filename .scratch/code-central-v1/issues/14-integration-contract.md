# The Integration plugin contract

Type: grilling
Status: resolved
Blocked by: 04, 05, 11

## Question

What is the Integration interface, and how are plugins packaged given a single binary? For a Ticket linked to an external system, who owns it, what flows each way, and what happens when both sides change it? How does the contract handle what How external trackers model work found: polling, Tickets with no parent, trees deeper than two levels, external workflows that don't map onto State, and work spanning several repositories? Does Atlassian's API token policy rule out a Jira Integration of this kind?

## Answer

v1 fixes only what is hard to reverse: where an Integration runs, who owns what on a linked item, and how a link is stored. No Integration ships, and operation signatures wait for the first one. These parts were checked on paper against GitHub Issues and Azure DevOps, the likely first two. See [ADR 0004](../../../docs/adr/0004-an-integration-is-a-client-of-the-http-api.md).

**Where it runs.** An Integration is a separate program and a client of the HTTP API, like a Factory. It runs its own polling loop and holds its own credentials. Nothing is loaded into the binary, and the service never sees an external credential. This amends [Process shape and web stack](10-process-and-stack.md): there is no `integration` package, only the `client` package and the linking operations below.

**Configuration.** Each Integration keeps its own configuration: the query per Project, the Spec level, the starting State and credentials. Code Central stores none of it and has no registry of Integrations. `link.integration` is just a name the Integration writes.

**Ownership.** The external system owns the *definition* of a linked Spec or Ticket: title, description and parent. Code Central owns its *delivery*: State, Claims, Rounds, Failure Reports, Labels, Tags and Blockers. On a linked item the definition fields are read-only to the human and agents, so no field can be changed on both sides. Definitions flow in. A summary of delivery flows out.

**The link.** Specs and Tickets gain an optional typed `link` field: `{integration, id, url, external_state, detached}`. `external_state` is the external status as last seen (`open`, `closed`, `deleted`, or `out_of_scope` when the item has left the query). Each item has at most one link. Tags aren't used for links, because a link changes what can be edited and Tags have no workflow meaning.

**A third Actor.** `integration` joins `human` and `agent`. Only an Integration can create linked Specs and Tickets, update their definition fields, and set `external_state`. It can create items in `Draft` or `Ready`, set per Integration, and the first Integration built will pull into `Draft`. It can't make any other State move. Like the other Actors, this guards against mistakes, not misuse.

**Hierarchy.** An external item with no parent becomes a one-Ticket Spec. In deeper trees, the Integration chooses which level counts as the Spec, and everything below it becomes a Ticket under its nearest Spec-level ancestor. The original parent can be kept as a plain Tag. Code Central never models trees deeper than two levels.

**Several repositories.** A Project is still one repository and is selected by its own query. An external parent whose children span several Projects becomes one linked Spec in each of those Projects. There is no Spec that spans Projects.

**Changes on the external side.** The Integration only updates `external_state` and never moves State. A linked Ticket whose `external_state` isn't `open`, and whose own State is neither `Done` nor `Cancelled`, appears in Needs Me with the computed reason "closed externally". The human then cancels it, finishes it, or unlinks it. Reopening the item clears the reason. This is computed, not a State, so [ADR 0001](../../../docs/adr/0001-one-state-value-on-tickets.md) stands. This is a starting point and is expected to change.

**Unlinking.** `unlink` is a human-only action. It marks the link `detached`, which makes the definition editable and stops the link counting for Needs Me. Lookups by link still find detached links, so an Integration sees the item is already known and won't pull it again. There's no re-link in v1.

**Outward.** The Integration subscribes to the same SSE events as the UI and refetches. What it pushes out is up to the Integration: a mapping from State to external status where one exists (a GitHub Projects status field, an Azure DevOps state), and closing the item on `Done`. Rounds, Claims and Failure Reports stay in Code Central. Tickets created locally under a linked Spec are ordinary unlinked Tickets and aren't pushed out in v1.

**Jira.** Atlassian's API token policy doesn't block v1, because credentials belong to the Integration. Jira, and Trello, are months away and out of scope for this effort.

Considered and rejected:
- **A full Go interface now:** it would lock in method signatures before anything uses them.
- **A Go package compiled into the binary:** every Integration would need a new build of Code Central, and polling and credentials would live inside the service.
- **A plugin process managed by the service:** this adds process management for something any HTTP client can already do.
- **Code Central owning everything after the first pull, or ownership per field with last write wins:** the first loses the external definition, and the second causes sync bugs.
- **Links stored as Tags:** Tags have no workflow meaning.
- **Moving State when the external item closes:** Code Central owns State, so the human decides.
