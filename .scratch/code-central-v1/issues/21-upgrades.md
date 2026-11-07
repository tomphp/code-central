# How Code Central is upgraded

Type: grilling
Status: resolved

## Question

What happens when a new version of the binary replaces the old one? How is the store's format migrated, keyed on `store.yaml`'s `format` ([The store's file layout](11-store-format.md)), and what if a migration fails partway? A restart must stay under about 31 seconds, or MCP sessions mark the server failed ([Process shape and web stack](10-process-and-stack.md)). How is a mismatch handled between the service and an older CLI inside a sandbox, where the CLI only prints a warning ([The MCP and HTTP surface](12-api-surface.md))? How do generated files and hook commands from an older `init` keep working, given that running `init` again refreshes them ([How agent sessions are taught to use Code Central](17-teaching-agent-sessions.md))? What does `service install` do when the binary is replaced?

## Answer

An upgrade means replacing the binary and restarting the service. The service migrates the store itself at startup, and the parts other copies depend on stay fixed.

**Upgrading.** v1 is built from source, so an upgrade means `make build`, copying the binary onto `PATH`, and `code-central service restart`. The running service never watches for its binary being replaced.

**`service install`** records the binary's absolute path as invoked, without resolving symlinks, so a symlinked install keeps working. It is idempotent: running it again rewrites the launchd or systemd unit and restarts the service. `service restart` is new.

**Migrating the store.** `serve` migrates automatically when `store.yaml` holds an older `format`, so a login item never crash-loops after an upgrade.

- It runs the chained steps (1→2→3) into a staging folder next to the store.
- It then renames the old store to `code-central.format-<n>.bak` and renames the staging folder into place.
- If any step fails, the original store is untouched, and the service refuses to start, naming the step and the file.
- A newer `format` is still refused, as [The store's file layout](11-store-format.md) says.

**Backups and downgrading.** The service keeps one backup per format it has migrated from and never deletes them itself. To downgrade, stop the service and put the `.bak` folder back, which loses every write made since the upgrade. This is documented and manual.

**What bumps `format`.** Every change to the schema does, even a purely additive one, because files are decoded strictly and an older binary would otherwise fail on an "unknown field" instead of the clear "unknown format". An additive change gets a migration that only rewrites the number. A `core` test compares the schema against a checked-in snapshot and fails if the schema changed without a bump.

**Startup order.**
1. Roll forward any pending Import manifest. The manifest only lists file renames from staging into place, so its shape is frozen and never versioned.
2. Migrate the format.
3. Load the store.
4. Bind the port.

**Restart time.** Migrating runs before the port is bound, so it counts against the roughly 31 seconds Claude Code allows. An end-to-end test migrates and loads a synthetic store of 10,000 Tickets with long histories and fails over 10 seconds. Loading without a migration gets the same budget.

**Compatibility between versions.**
- Within v1, the HTTP API and the MCP tools change additively only: new operations and new optional fields. Nothing is renamed or removed, and no meaning changes. A breaking change is the point at which `/api/v2` is introduced, not before.
- The service still decodes requests strictly, so a newer CLI calling an older service gets `invalid` or `not_found`.
- The Go `client` tolerates unknown fields in responses, so an older CLI, such as a copy inside a sandbox, keeps working against a newer service.
- The CLI's version warning names the older side when it can: "run `code-central service restart`" when the service is older, and "this CLI is older than the service" when the CLI is.

**Version.** `make build` stamps `git describe --tags --always --dirty` through `-ldflags`. That value appears in the response header, in `code-central --version` and in the header comment of generated files. Versions are compared for equality only, and which side is older is reported only when both are clean tags. Otherwise the warning just says the versions differ.

**A permanent contract.** These don't change across versions, so files from an older `init` keep working:
- the `.mcp.json` URL shape `/p/<key>/mcp`, the port 7733 and the MCP server name `code-central`
- the hook subcommands `code-central hook session-id` and `code-central ping`, which always exit without blocking, even if a later binary no longer knows the hook
- `store.yaml`'s `format` key and the Import manifest's shape

The generated `docs/agents/` files name operations only, so the additive rule keeps them valid. Their header comment records the version that wrote them. Nothing detects stale files in v1, and the upgrade docs say when to run `init` again.

Considered and rejected:
- **An explicit `migrate` command:** a login item would crash-loop until someone ran it.
- **Migrating in place, file by file:** a failure partway leaves a mixed store.
- **Additive changes without a `format` bump:** strict decoding makes an older binary fail with an unclear error.
- **Binding the port first and answering `service_unavailable` while migrating:** nobody knows whether Claude Code retries that.
- **Watching the binary for replacement:** a restart is one command.
- **Detecting stale generated files:** the service can't see repositories, and the files only hold pointers.

Not verified: how long a migration of a real-sized store takes. The 10-second budget is checked when v1 is built.
