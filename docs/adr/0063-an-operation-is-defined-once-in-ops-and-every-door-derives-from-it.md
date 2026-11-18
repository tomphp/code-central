# An operation is defined once in `ops` and every door derives its route, tool, schema, client call and command from that definition

Code Central has one list of operations, and three doors onto them: the HTTP API, MCP and the CLI through the Go client. Until now the list, `core.Operations`, held only a name, an Audience and whether the operation writes, and each door kept its own hand-written translation: a route and handler in `httpapi`, a tool with a hand-built JSON schema in `mcp`, a path-building method in `client`. Adding an operation meant writing the same facts four times, and the cross-door suite could only check that each door served every name, not that the four tellings agreed. The MCP schema and the HTTP query decoding, in particular, drifted from the input struct they described.

So each operation is defined once, in the `ops` package, which sits between `core` and the doors and imports only `core`. A definition is a generic `ops.Definition[In, Out]` holding the name, Audience and write flag, the HTTP method and path pattern in ServeMux syntax, the Core method to call, a one-line description, per-field description overrides and a hook for the HTTP door's defaults. Everything else is on the input struct's tags: `desc` and `required` for the MCP schema, `enum` for a fixed string set, `path:"n"` for the field the path's `{n}` binds, `query` for a parameter whose name differs from its JSON name, and `mcp:"-"` to keep a field out of the tool. One field walker, `internal/fieldwalk`, reads those tags for every reader of a struct's wire shape, including the UI's generated wire types. `ops.All()` replaces `core.Operations`, and the doors derive from `ops.Definitions()`: the HTTP door one handler that binds the path, decodes the query or body and applies the defaults; the MCP door one tool whose schema is derived from the input type, checking the Project itself for an operation whose path has no `{key}`; the client one call that fills the path and encodes a GET's query by the same tags the handler decodes with. The derivation fails at start-up, naming the operation and the field, for a shape it can't express, so a mistake never reaches a request. A query shape the tags can't carry, such as a Tag filter's `tag=<key>=<value>`, is decoded and encoded by small methods on the input type.

A definition stays transport-free: each door's cross-cutting work, the Actor it acts as, the usage log and the one error shape, sits on its generic dispatch, around every derived call, not in the definitions.

The doors were converted one group of operations at a time; every operation is now defined in `ops`, and no legacy route, handler or tool map remains. The CLI derives a command from its definition and the input struct's `cli` tags only where the result equals the hand-written command it replaces, so its usage stays the same; a command with `--file` or stdin input, a default from the environment or more than one call stays hand-written against the client's unchanged method names, and takes only its operation name and `--help` text from the definition.

## Considered Options

- **Keep the three hand-written translations and check them with more tests.** Rejected: the cross-door suite already checked names, and the drift was in the shapes, which a test per operation per door would have to spell out a fourth time.
- **Put the handler on `core.Operation` as a closure over the Core.** Rejected: core would then know HTTP and MCP, where the coding standards keep an operation in core and the doors translating only, and a closure can't be reflected on to derive a schema or a client call.
- **Generate the doors' code from the input structs.** Rejected: a generator adds a build step and a second copy to keep in sync; reflection at start-up gives the same single source with the same start-up failure for a bad shape.

## Consequences

- Adding an operation is a Core method, one `ops.Definition` and the input struct's tags; the three snapshots in `server/testdata` show what each door now serves.
- An input struct's `desc` tags are the agent-facing wording, so a change there changes the MCP tools/list snapshot.
- The ADR was numbered 0056 in the Spec that decided it; 0056 to 0062 were taken by the time it was written, so it is 0063.
- `httpapi.Routes` is a function, deriving the routes from the definitions.
