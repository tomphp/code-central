# An Integration is a client of the HTTP API that owns definitions, not delivery

v1 has to leave room for Integrations without shipping one, and the binary is a single file. So we decided that an Integration is a separate program that talks to Code Central over the HTTP API, like a Factory. It runs its own polling and holds its own credentials and configuration. On a linked Spec or Ticket, the external system owns the definition (title, description, parent), and Code Central owns the delivery (State, Claims, Rounds, Failure Reports, Labels, Tags, Blockers). A typed `link` field records the external item. A third Actor, `integration`, is the only one that can write definitions on linked items, and it can't move State.

## Considered Options

- **A Go package compiled into the binary.** Rejected because every Integration would need a new build of Code Central, and polling and external credentials would live inside the service.
- **A plugin process managed by the service.** Rejected because it adds process management for something any HTTP client can already do, and the service is already the only writer ([ADR 0002](0002-the-service-is-the-only-writer.md)).
- **Code Central owning everything after the first pull.** Rejected because the external definition would be lost after the first pull.
- **Ownership per field with last write wins.** Rejected because changes on both sides cause sync bugs. Splitting ownership means no field is ever changed on both sides.
- **Links stored as Tags.** Rejected because a link makes fields read-only, and Tags have no workflow meaning.

## Consequences

When the external item closes, is deleted or leaves the query, the Ticket's State doesn't change. The Integration records the external status, and Needs Me shows a computed "closed externally" reason until the human acts. Unlinking keeps a detached link, so an Integration doesn't pull the same item in again. External trees flatten to Spec and Ticket. An external parent that spans repositories becomes one linked Spec in each Project. Operation signatures are left until the first Integration, which will probably be GitHub Issues.

[ADR 0020](0020-an-integration-runs-as-a-watcher-supervised-by-the-service.md) amends where an Integration runs, and [ADR 0021](0021-to-requirement-snapshots-an-issue-and-turns-its-link-outbound.md) who owns an outbound Link's definition.
