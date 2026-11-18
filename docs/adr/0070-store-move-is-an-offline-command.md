# Store Move is an offline command

Status: accepted. A recorded exception to [ADR 0002](0002-the-service-is-the-only-writer.md)'s "every command is a client of the service"; builds on [ADR 0067](0067-stores-are-pluggable-and-a-database-store-keeps-document-rows.md), [ADR 0068](0068-several-services-may-share-a-postgres-store.md) and [ADR 0069](0069-usage-is-part-of-the-store.md).

ADR 0067 lets a service keep its records in Files, SQLite or Postgres, chosen by the `store:` block of `settings.yaml`. Someone who has run on Files for months needs a way to take every record, with its History, the Options, the CI Runs and the Usage, into a new Store of another Kind, and to point the service at it, without losing a write. A service opens one Store and holds it for its whole run, so the copy can't be one of its operations: it would read one Store and write another the service never opened, while the service went on writing the first.

So **Store Move**, `code-central store move --to <kind>[:<path>] [--url-env <name>]`, is a command that opens the Stores itself and never talks to the service:

- **The source is the Store `serve` would open**: the one `--store`, `CODE_CENTRAL_STORE` or the `store:` block names, resolved as `serve` resolves it. There is no `--from`. It opens the source as `serve` does, Migration included, and holding it is what makes the copy safe. A File or SQLite Store is locked while it is open, so a running service makes the move refuse, saying to stop it, and a service started during the move refuses to start. A Postgres Store takes no lock, so the move refuses while any live Service Registration exists, naming each host.
- **The target must be empty**: no Project, and the default Options. A target holding anything is refused by name, so two Stores are never merged.
- **The copy goes through the Store interface.** For each Project, `store.Copy` reads it, its Specs and Tickets with every History part (the full-record iterator `Records`), and its CI Runs in one View. It writes the Project, then its records as one `PutImport`, then its CI Runs, then its Usage day by day, and the Options last. So every pair of Kinds moves the same way, and the Migration exporter of ADR 0067 stays the database Stores' own concern.
- **The counts are checked before anything changes.** `store.TallyOf` counts each Project's Specs, Tickets, CI Runs and Usage days, and reads the Options, in the source and in the target. Any difference stops the move and names it, such as `acme: 4 Tickets in the source, 3 in the target`.
- **Only then is `settings.yaml` rewritten**, and only its `store:` block: the other keys and their comments stay. A `files` or `sqlite` target is written with the path given, made absolute, or none for the Kind's default. A `postgres` target is written with its `url_env`, never the URL.
- **The source is left as it was**, as the backup. The command prints what it copied and where the source is, and says when `--store` or `CODE_CENTRAL_STORE` would still open the source.

## Considered Options

- **An operation of the running service** (an online move). Rejected: the service would have to stop every write for the length of the copy and then switch its Store under the open requests, its Watchers and its Factories' waits. Stopping the service for a move that runs once is simpler, and the source's lock or the Registrations prove it is stopped.
- **Copying through the Migration exporter**, as a File Store layout. Rejected: it reads only the database Kinds, and writes only into an empty database in one transaction. The Store interface already reads and writes every Kind, and the contract suite runs `store.Copy` against each.
- **A `--from` flag.** Rejected: the source is always the Store the service opens, so the move can't be aimed at the wrong one, and the settings it rewrites are the ones that named it.
- **Deleting or renaming the source after the copy.** Rejected: the source is the only backup a move has. The person deletes it once the new Store serves them.
- **One transaction around the whole copy.** Rejected: the File Store has none across Projects, and a Kind-specific path would undo the one way every pair moves. A failed or mismatched copy leaves `settings.yaml` and the source as they were, and says the target holds part of the copy and must be removed before moving again.

## Consequences

Store Move is the one command besides `serve` that opens a Store, so `docs/agents/real-service.md` lists it among the commands an agent never runs against the human's Store. Usage under a key with no Project record isn't moved, since a File Store couldn't load it, and neither is a Usage line no read could parse; neither is counted. Moving into Postgres from a Store on this machine needs the database's URL in the variable `--url-env` names (`CODE_CENTRAL_DATABASE_URL` by default), in the move's environment and then in the service's.
