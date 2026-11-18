# Usage is part of the Store

Status: accepted. Amends [ADR 0016](0016-usage-records-are-append-only-json-lines-files-per-project-per-day-kept-90-days.md).

ADR 0016 kept Usage as JSON Lines files beside the store, behind a recorder that opened them itself. That made the files the only form Usage could take, and the store knew nothing of it. Spec code-central-1032 lets a service keep its records in a database as well as in files, and Usage must follow its records: a Store Move copies Usage with them, and a service on a shared database has no directory of its own to write beside.

So Usage is part of the Store. `store.Store` gains `Usage()`, which returns a `usage.Store` with three operations: append a record to a Project's day, read a Project's days (or every Project's), and delete every day before a cutoff. Each Store Kind implements it in its own form. `usage.Recorder` keeps its API (`RecordCall`, `RecordResponses`, `Read`) and keeps ADR 0016's rules, over whichever `usage.Store` it is given: it works out each record's local day and today from its clock and zone, and it asks the Store to delete the days older than `KeepDays` (90) at startup and on the first append of each new day.

- **The File Store** keeps ADR 0016's layout exactly: `projects/<key>/usage/<YYYY-MM-DD>.jsonl`, one JSON object per line with a `type`, no fsync, a line that doesn't parse skipped and counted, and a name that isn't a dated `.jsonl` file never deleted. The in-memory fake keeps the same records in memory.
- **The database Stores** keep a Usage table keyed by Project and local day. They insert on their own connection, outside any Update, and a daily timer deletes the days older than `KeepDays`, since a database has no first-append-of-a-day habit to piggyback on. They are added by later Tickets of the Spec.

The rules of ADR 0016 hold for every Kind. Recording never slows or fails an agent's call: an append that fails is logged and dropped, and the Recorder never returns it. A Store's Usage takes its own lock, apart from the store-wide one, so an append never waits on an Update and an Update never waits on an append; in a database that is a separate connection, outside every transaction. Only the check that a call's Project exists, which is a store read like the call's own, comes before an append, so nothing is filed under a Project that doesn't exist.

## Considered Options

- **Leave Usage outside the Store.** Rejected: a database Store would need a second place for Usage, set apart from its records, and Store Move couldn't carry it.
- **Append Usage inside an Update.** Rejected: ADR 0016 made appends never wait on a store write, and an Update allows one write. On a shared database it would also put every agent call in a serializable transaction that can conflict and retry.
- **Aggregate Usage in the database.** Rejected again for ADR 0016's reason: the per-session detail is what Context Size and the session chart need.

## Consequences

`usage.Store` is where a Kind's contract is tested: `store/storetest` runs append, read, prune, reopening and a failing append that must not fail the caller against the fake and Files, and against each later Kind. The recorder's own tests run on the fake and Files too. The on-disk layout is pinned by a test of the exact bytes written. Reading still scans the days in its range on each request, with no index.
