# The doors reach core through a narrow interface, and one middleware sets the Actor and records Usage

The two transports, `httpapi` and `mcp`, each took the concrete `*core.Core`, with its 140-odd exported methods, so neither could be tested against a fake and both could reach everything. Their policy was split too: `server.Guard` read the HTTP Actor header into every request's context, while `mcp` forced the agent Actor, recorded a Usage `mcp_call` for each tool call and wrote its log line itself.

So each door depends on an interface defined in `ops`, beside the operations: `ops.Dispatcher`, one method, `Dispatch(ctx, ops.Call)`, for MCP, and `ops.Service`, which adds the change feed and the reads, streams and Factory posts that aren't operations (wait, transcript, chat, usage, CI overview, ping), for the HTTP API. An `ops.Call` is the operation, the Project key, the input the door decoded or the raw arguments for `Dispatch` to decode strictly, and the Actor the request names. `ops.Serve(c)` is the one adapter over `*core.Core` (since [ADR 0072](0072-the-wire-contract-is-its-own-package-and-no-client-links-the-core.md), `bind.Serve(c)` in the service-only `ops/bind`). One middleware, `server.Calls`, wraps that Service for each door. It acts as the Actor the call names (the HTTP header, or `agent`, which MCP always names), and refuses an unknown one, naming the header. For a door that records Usage, it also records each call and writes its log line.

Only MCP calls are recorded as Usage, and only they are logged, as before. The HTTP API's door does not record them, by a flag on the door, not by where the code sits. Usage is what agents load into their context through their tools ([ADR 0016](0016-usage-records-are-append-only-json-lines-files-per-project-per-day-kept-90-days.md)). The HTTP API's callers are mostly the UI, the Factory and the watchers, which load nothing into an agent's context, and the UI's reads would drown the agents' calls. The CLI, which agents also run, reaches the HTTP API too, but its output is not the operation's JSON, so the response bytes the record measures wouldn't be what the agent read.

## Considered Options

- **Have `*core.Core` itself satisfy the interface.** Rejected: `Dispatch` takes an `ops.Operation`, and `core` can't import `ops`, which imports it. One small adapter in `ops` keeps the import direction of [ADR 0063](0063-an-operation-is-defined-once-in-ops-and-every-door-derives-from-it.md).
- **An HTTP middleware around both routes.** Rejected: for MCP it would have to parse the JSON-RPC body to find the tool, its arguments and the text it returns, all of which the dispatch already has.
- **Record the HTTP API's calls as Usage too.** Rejected for the reasons above; a door can turn it on with its flag if that changes.

## Consequences

- `grep -rn '\*core\.Core' httpapi mcp` finds only test code; each door has a test that runs it against a recording fake, and the cross-door suite still runs on the real Core.
- The Actor is set in one place, `server.Calls`, on each operation. A request that isn't an operation, such as a ping or the live feed, is made as no Actor, and its Actor header is not checked.
- A key given to an operation that isn't on one Project must still name a Project: `ops.Serve`'s (now `bind.Serve`'s) `Dispatch` checks it, so every MCP tool on an unknown key answers `unknown_project`, as before.
