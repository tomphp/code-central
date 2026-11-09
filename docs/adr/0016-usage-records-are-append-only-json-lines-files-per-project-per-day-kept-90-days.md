# Usage records are append-only JSON Lines files per Project per day, kept 90 days

The human wants to see what agents consume: which Code Central operations they call and how much each loads, how full each session's context gets, and token usage day by day (Spec code-central-201). That needs history that survives a restart, which neither the store's YAML records nor memory give. So we decided that Code Central keeps **Usage** records as append-only JSON Lines files beside the store, one per Project per local day, at `projects/<key>/usage/<YYYY-MM-DD>.jsonl`, and deletes a file once its day is more than 90 days before today. Each line is one JSON object with a `type`: an `mcp_call`, which Code Central records itself for each agent MCP tool call, or a `model_response`, which the Factory posts. This is the store's first data that is neither YAML ([ADR 0003](0003-the-store-is-yaml-files-loaded-into-memory.md)) nor memory only ([ADR 0008](0008-session-events-are-posted-by-the-factory-and-live-only-in-memory.md)).

The Factory posts token usage to an endpoint of its own, `POST /api/usage`, not as a fifth Session Event kind. Session Events are opaque, memory-only and bounded per Claim (ADR 0008), while usage is structured numbers that must persist; carrying it as Session Events would make Code Central parse what ADR 0008 keeps opaque.

## Considered Options

- **Memory only.** Rejected because Usage would be lost on every restart, and its graphs cover up to 90 days.
- **Aggregates only**, such as running totals per operation and day. Rejected because they lose the per-session detail that Context Size and the session chart need.
- **SQLite.** Rejected for ADR 0003's reasons: a dependency and a binary file nobody can read or fix by hand, for one user's data, which fits in a few small files.

## Consequences

Appends fall outside the one-file-per-operation YAML rule and take the usage recorder's own lock, not the store-wide one, so an append never waits on a store write and a store write never waits on an append. Only the check that a call's Project exists, a store read like the call's own, comes before it, so nothing is ever filed under a Project that has no `project.yaml`. Recording must never slow or fail an agent's call, so an append that fails is logged and dropped, and there is no fsync: a crash may lose the last records. A line that doesn't parse is skipped and counted in the service log, never fatal, unlike the store's YAML. The store's loader reads only `specs/` and `tickets/`, so a `usage/` folder never affects startup. A read scans the files in its range on each request, with no index. The files can be read and grepped by hand. Data starts when Usage ships, with no backfill.
