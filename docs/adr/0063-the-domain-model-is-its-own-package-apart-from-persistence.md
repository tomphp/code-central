# The domain model is its own package, apart from persistence

The records Code Central keeps (Projects, Specs, Tickets, CI Runs, Options and the rest), with their enumerations, defaults and invariants, were declared in `store`, beside the persistence that writes them. Every client of the service that needed a `Ticket` or a `State` imported `store`, so the package that holds the files on disk was reachable from code that must never write them ([ADR 0002](0002-the-service-is-the-only-writer.md), [ADR 0004](0004-an-integration-is-a-client-of-the-http-api.md)).

So the domain model is its own package, `domain`, at the module root, importing nothing of Code Central. It holds the records, the enumerations with their value lists, name functions and `Valid()` methods, the constants and defaults, and the invariants that belong to the records (`Landing.Normalized` and the `Link` predicates). `store` keeps persistence: the `Store`, `Reader` and `Writer` interfaces, `Import`, `FileStore`, `NewMemory`, the migrations, `Schema`, and the hand-written file structs with their encode and decode, which now read into and write from `domain`'s records. Helpers only persistence uses, such as deep copies, stay unexported in `store`.

- Clients of the service import `domain` and never `store`.
- Only `core` and the composition roots (`cli/serve.go`, `cli/service.go`, `world`, `codecentraltest`) import `store`.
- The Landing Policy value lives in `domain` beside `Landing.Normalized`, and `core` keeps the builders that make it from a `store.Project` or a view.

The package is named `domain` because the Glossary's "Model" already means a Claude model; ADR 0003 already calls these the domain struct and the domain shape.

This leaves [ADR 0003](0003-the-store-is-yaml-files-loaded-into-memory.md) unchanged: file shapes stay explicit and live in `store`, and nothing on disk changes. It amends only the placement wording of [ADR 0039](0039-every-landing-rule-is-a-landing-policy-method-and-no-caller-reads-the-landing-settings-to-decide.md), which put the policy in `core` because `store` couldn't import `core`; the rule that every landing rule is a policy method stands.

## Considered Options

- **Naming the package `model`.** Rejected: `model.Model…` names would be ambiguous with Models, Model Choice and the `model` Tag.
- **Leaving the records in `store`.** Rejected: clients keep importing the persistence package, and nothing stops them reaching its writers.

## Consequences

- The move goes in steps (Spec code-central-925): `store` first aliases `domain`'s names (`type Ticket = domain.Ticket`), so callers move package by package, and the aliases go once none is left.
- The type checker names an aliased type by where it is declared, so a check keyed by type path, such as the landing-settings guard, names `domain`'s types.
- `domain` compiles for `js/wasm` with the rest of the Core the UI runs.
