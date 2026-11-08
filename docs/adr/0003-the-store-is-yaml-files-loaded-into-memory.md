# The store is YAML files loaded whole into memory

The store has to stay readable and fixable by hand, so we decided to keep it as plain files rather than a database: one strict YAML file per Ticket, holding its whole history, and Markdown with YAML frontmatter per Spec, under `projects/<key>/`. The running service loads every file into memory at startup and serves all reads from memory. Each operation writes exactly one file, using a temp file, fsync and rename, under one store-wide mutex, and memory is updated only once the file is on disk. This is only safe because the service is the only writer ([ADR 0002](0002-the-service-is-the-only-writer.md)).

## Considered Options

- **SQLite.** Rejected because the store would no longer be readable or greppable, or fixable with an editor. A single user's data fits in memory, so the query power isn't needed.
- **Reading files for each query, or an index on disk.** Rejected because memory is simpler and faster, and the only writer always knows what's on disk.
- **Writes that touch several files.** Rejected because a crash could leave them half-applied without a journal. Data is shaped so each write touches one file instead: Replacements are resolved when read, and the ID counter is derived at startup.

## Consequences

Hand edits are supported only while the service is stopped. A file that fails strict validation stops startup and names the file and field, rather than being skipped. Edits made while the service is running are overwritten. Last activity from pings lives in memory only, so after a restart it counts from the time the service started. An Import or a submitted Requirement, the bulk writes, stage their new files, write a manifest, then rename each into place. A manifest found at startup is completed, so neither is ever left half-applied. A `store.yaml` records the format version, so a later version can migrate it.
