# The UI's wire types are generated from Go's view structs

The UI's response types were copied by hand from the Core's Go view structs, and nothing compared the two, so they drifted: fields required in TypeScript that Go omits, fields TypeScript never had, a field list missing names a write can return. Since the UI's fake became the real Core ([ADR 0060](0060-the-uis-fake-is-the-real-core-built-to-webassembly.md)), these types were the last second copy of the Core's shapes. So we decided that Go's view structs are their one source. An in-repo generator, `internal/wiretypes`, run by `make wiretypes`, writes the committed `web/src/api/wire.gen.ts` from an explicit list of root types and every type they reach. Each typed string a root reaches is a union with its value array, taken from a Go value list registered beside the roots. The TypeScript side never hand-writes a response shape; `types.ts` keeps only request bodies, filters, UI-derived types and UI-only constants.

## Considered Options

- **Guard the hand-written types instead of generating them.** A test could compare the TypeScript against Go and fail on a difference. Rejected: every view change would still be made twice by hand, and the guard would need its own parser for the TypeScript it checks. Generating removes the second copy rather than policing it.
- **tygo.** Rejected: it maps `type State string` to `string`, which loses the unions the UI narrows on, and it would be the toolchain's first non-standard-library generator. The in-repo generator uses only the standard library: reflection for shapes, so JSON names follow the Core's own `jsonNames`, and `go/parser` with `go/doc` for the doc comments, which carry over so an editor shows what a field means.
- **Derive the roots from `core.Operations`.** Rejected: an operation doesn't declare its result type, and an explicit list keeps MCP- and Factory-only views out of the UI.

## Consequences

- **No runtime shape check.** `call<T>` keeps parsing and casting. Generation makes the static types right, and the contract suite already runs the real client against the real Core.
- **Go owns every enum.** A typed string with no registered value list fails the generator, naming the type and the field that reached it; there is no fallback to `string`. A value list that misses one of its type's declared constants fails it too, so a union can't be narrower than what the Core sends. Where the UI narrows on a value Go types as a plain `string`, Go gains a typed string and a value list.
- **A type with its own `MarshalJSON` fails the generator,** because its shape isn't its JSON. It stays out of the roots, and the UI describes it by hand over a generated type, as the acknowledgements do over `Acknowledgement`.
- **Enum-keyed maps are `Partial`,** because Go's type can't promise every key; a reader that relies on every key reads through the `Partial`.
- **The generated file is committed,** so a view change shows its TypeScript effect in the same diff, and a Go test fails when it is stale, naming `make wiretypes`.
