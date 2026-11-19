# The wire contract is its own package and no client links the Core

Every client (`client`, the Factory, the Watchers, `gh`, `azdo`, `ratelimit`, `watchers`, `notify`) imported `core` for the types that cross the HTTP boundary, and `core` imports `store`. So each of them linked the Store and its database drivers, and nothing but discipline kept a client from calling a Core method in process. The leaves `gh`, `azdo` and `ratelimit` imported `core` for one result type.

So the wire contract is its own package, `api`: the inputs, views, rows and results, the error codes, `Ref`, `Actor`, `Decode`, and the Watcher status contract between the Watchers and the service (`WatcherCheckResult`, the `Problem*` codes, `WatcherKind`, `WatcherCrash`, `WatcherSteadyAfter`). The work is done in slices; the first moves the Watcher status contract, and each later slice moves more until `core` names `api.X` directly with no aliases.

`api` imports `domain` and `internal/fieldwalk` and no other Code Central package. `Decode` needs `fieldwalk`, which uses only the standard library and is already shared by the Core, the MCP door and `wiretypes`. A type that holds `store` or `events` values stays in `core`: it isn't wire.

`ops` is split the same way. The definitions (name, Audience, route, input and output types) stay in `ops` and import `api`, not `core`; the bindings, each operation's `Call` against `*core.Core` and the `Serve` and dispatch side, move to a service-only package that `server` composes with the definitions. `client`, `cli`, `mcp` and the UI generator import only the definitions. The cross-door suite checks that every definition has exactly one binding.

A guard in `internal/importguard` runs `go list -deps` on each client and fails when `core` or `store` is among the results, naming the client and the package. A sibling test checks that `api` depends on no Code Central package but `domain` and `internal/fieldwalk`. The existing direct-import test stays for `httpapi`, `mcp`, `server` and `internal/wiretypes`, which legitimately reach `core`.

It builds on [ADR 0063](0063-an-operation-is-defined-once-in-ops-and-every-door-derives-from-it.md) (operations are defined once), [ADR 0064](0064-the-domain-model-is-its-own-package-apart-from-persistence.md) (the domain model is its own package) and [ADR 0065](0065-the-doors-reach-core-through-a-narrow-interface-and-one-middleware-sets-the-actor-and-records-usage.md) (the doors reach Core through a narrow interface).

## Considered Options

- **Keep the types in `core` and guard direct imports only.** Rejected: that is the status quo, and it hides that every client links the Store through `core`.
- **Put the wire types in `domain`.** Rejected: `domain` is the persisted records and rules; the wire contract changes for other reasons and may name views that no record holds.
- **A package of its own for the Watcher status contract.** Rejected: it is the part of the wire contract between the Watchers and the service, so it lives beside the rest in `api`.

## Consequences

- The wire format doesn't change: JSON names and tags, routes, MCP tools, CLI commands, the store format and `wire.gen.ts` stay as they are.
- A change to the wire is a change to `api`, which a diff shows; `internal/wiretypes` generates from one package.
- `ratelimit`, `gh`, `azdo` and `watchers` no longer link `core` or `store`.
