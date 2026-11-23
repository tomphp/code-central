# The Core is split into areas over a shared foundation, and a guard keeps them apart

`*Core` had 150 exported methods and one pool of state: every file was a method on it, so nothing inside `core` had a boundary and any rule could reach any other's fields. With the wire contract in `api` and the bindings in `ops/bind` (ADR 0072), the split can now be made area by area.

So each area is a package under `core`, with its own type, its own state and its own tests. The first is the agent runtime, `core/factories`: `factories.Factories` holds the Heartbeats, Transcripts, chats, activity, Plan Limits, Factory wakes and the shared-Store reports of the Factories, and `(*core.Core).Factories()` hands it out. The workflow stays in `core`.

What the areas share is in `core/foundation`: the service identity, the Store's archive check, ticket and Spec resolution, the error constructors and the effort rules. It is a leaf under both `core` and the areas.

The import rules:

- `core` may import an area. An area imports neither `core` nor another area, only `foundation`. `foundation` imports neither.
- No area or `foundation` imports `ops`, `ops/bind`, `server`, `httpapi` or `mcp`.
- Only `core`, `ops/bind`, `world`, `codecentraltest`, `core/coretest` and `app` (to set a setting) import an area.

`internal/importguard` checks them on the non-test imports and the transitive dependencies. It finds the areas by listing the packages under `core`, so a new area is guarded the moment it exists, with no list to update.

An operation of an area is bound in `ops/bind` with `bindFactories`, which calls `c.Factories()`; a Core operation is bound with `bind`.

The wake rules stay in `core`. They read the Store's writes to decide whether the Factories' poll should wake, and the notifying Store wrapper that applies them also sends the Project's notices, which are the workflow's. The area exposes only the wake itself, and `core` reaches it through a small `waker` interface. `ReportHeartbeat` is split the same way: the area records the Heartbeat, and the `core` wrapper builds the Project view and acknowledges the Project.

`core/coretest` builds a whole Core over a Store and a fake clock, with only the helpers an area's tests need. The area's tests can't import `core` themselves (the rule above), so they use it; `core`'s own tests use it too.

## Considered Options

- **Keep one package and split the type by embedding.** Rejected: the compiler would not stop one part reading another's fields, so there would be no boundary to keep.
- **Let an area import `core`.** Rejected: it would be a cycle, and the area would reach the whole workflow.
- **Move the wake rules into the area.** Rejected: they would need the Store's write path and the Project's notices, so the area would import what it must not. They stay in `core` behind an interface.
- **One shared package of helpers named `util`.** Rejected: `foundation` holds only what an area needs from the workflow's own rules, and the guard keeps it from importing them back.

## Consequences

- Behaviour, the wire and the store format are unchanged; only the binding of the agent runtime's operations moves.
- A later split (CI and code host, landing) adds a package under `core` and a `bindX` helper, and the guard covers it without a change.
- The agent runtime's tests live in `core/factories`; the wake rules' and the `ReportHeartbeat` wrapper's stay in `core`.
