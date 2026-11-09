# Options are kept by the service, apart from settings.yaml

The human wanted Focus Mode to show an Activity Dashboard instead of All clear, chosen from the web UI. Until then the only configuration was `settings.yaml`, which the human edits by hand and `serve` reads once at startup, so the UI couldn't change it. We decided such preferences are **Options**, kept in Code Central's store as one global file, `options.yaml` at the store's root beside `store.yaml`. It is the store's first record that belongs to no Project. The service writes it like every other record ([ADR 0002](0002-the-service-is-the-only-writer.md), [ADR 0003](0003-the-store-is-yaml-files-loaded-into-memory.md)): one file, through a temp file, fsync and rename, decoded strictly at startup, with a store format bump and a migration.

Only the human changes Options, over HTTP, and each change is announced on the live feed with `options.changed`, so every open browser applies it at once. There is no MCP tool and no CLI command for them. `settings.yaml` stays as it is: a hand-edited file, local to the machine, read once at startup.

The boundary between the two: anything the human changes while Code Central runs is an Option, and anything needed at startup or tied to the machine is a setting.

## Considered Options

- **The browser's localStorage.** Rejected: it is kept per browser, so every browser and machine would need setting up separately, and it is easily lost when the browser's storage is cleared.
- **A key in `settings.yaml`.** Rejected: the UI can't change it, and a change needs a restart of `serve`.

## Consequences

Options survive restarts and cleared browsers, and every browser and machine shows the same thing. A store without `options.yaml` reads every Option at its default, so a fresh or migrated store needs no setup; the file is written the first time the human changes an Option. A hand-edited `options.yaml` with an unknown key or a bad value stops startup, naming the file and field, as other store files do. Options have no history: a change adds no Move, Edit or Comment, and the file itself is the only record. As the first record outside any Project, Options pass through the archive guard untouched, and `options.changed` is the first notice with no Project, which listeners treat as not about any Project.
