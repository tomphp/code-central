# 44: Upgrades and migration

**What to build:** After an upgrade, the service migrates an older store by itself at startup, safely and quickly, so a login item never crash-loops and Claude Code's reconnect window isn't missed. A schema change can't ship without bumping the format. See the [spec](../spec.md), section "The store".

**Blocked by:** 40, 43

**Status:** resolved

- [x] Startup runs in order: roll forward any Import manifest, migrate the format, load and validate the store, bind the port
- [x] When `store.yaml` holds an older `format`, `serve` runs the chained steps into a staging folder next to the store, renames the old store to `code-central.format-<n>.bak` and renames the staging folder into place
- [x] If any step fails, the original store is untouched and the service refuses to start, naming the step and the file; a newer `format` is still refused by name
- [x] One backup is kept per format migrated from, and the service never deletes them
- [x] A `core` test compares the schema against a checked-in snapshot and fails if it changed without a `format` bump; an additive change gets a migration that only rewrites the number
- [x] A test migration from `format: 1` to a test-only `format: 2` proves the staging, backup and failure paths on the file-backed store
- [x] An end-to-end test migrates and loads a synthetic store of 10,000 Tickets with long histories and fails over 10 seconds; loading without a migration gets the same budget
- [x] The upgrade steps (`make build`, copy onto `PATH`, `code-central service restart`), the manual downgrade from a `.bak` folder, and when to run `init` again are documented
