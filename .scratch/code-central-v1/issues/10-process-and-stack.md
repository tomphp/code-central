# Process shape and web stack

Type: grilling
Status: resolved
Blocked by: 03

## Question

How does the service run (daemon, login item, or started by hand) given that agents need it available? Which frontend framework and build are embedded in the binary? SSE or WebSockets? How is the Go code divided into modules around its seams? Which address does it bind to, and how are `Host` and `Origin` validated, given what Serving MCP from a Go binary found about Docker access? How a Project is identified commits a default address into every repository's `.mcp.json`, so which port can stay stable across upgrades and restarts?

## Answer

Amended by [How Code Central is upgraded](21-upgrades.md), which added `service restart` and made `service install` idempotent, recording the binary path as invoked.

Amended by [The Integration plugin contract](14-integration-contract.md): an Integration is a separate HTTP client, so there is no `integration` package.

The service is one long-running process that is the only writer to the store, serving MCP, the HTTP API and the UI on one fixed port that only loopback can reach.

**How it runs.** `code-central serve` runs the service in the foreground. `code-central service install` is an optional wrapper that runs it as a login item: a launchd agent on macOS or a systemd user unit on Linux, started at login and restarted after a crash. This is the usual setup. The CLI never starts the service. If it can't connect, it fails with a message naming both commands.

**The only writer.** Every `code-central` command, including `init` and Import, is an HTTP client of the running service, and nothing else opens the store for writing. That makes a claim a single in-process step. The service locks the store exclusively at startup, so a second copy on another port refuses to start. See [ADR 0002](../../../docs/adr/0002-the-service-is-the-only-writer.md).

**Address and port.** By default the service binds to `127.0.0.1` and `::1`, because `.mcp.json` says `localhost`, which can resolve to either. `--bind` adds one specific address, such as the Docker bridge IP on Linux, and the docs warn against using it with a LAN address. The port is 7733 (unassigned at IANA) and is permanent across versions. If the port is taken, the service refuses to start and names the conflict, with no fallback to another port. To work around a real clash, a local setting changes the port and `CODE_CENTRAL_URL` points agents at it, so committed files never change.

**`Host` and `Origin`.** One middleware sits in front of MCP and the HTTP API, and the MCP library's own checks are turned off.
- **`Host`:** must be `localhost`, `127.0.0.1`, `[::1]` or `host.docker.internal`, plus any address added through `--bind`.
- **`Origin`:** a missing `Origin` is allowed. If one is present, it must be one of those hosts.
- **No CORS headers:** the service doesn't send any.
- **`GET` requests:** these never write.

**Reaching it from a sandbox.** Docker Sandboxes (`sbx`) reach a loopback-only service through their proxy once `sbx policy allow network localhost:7733` has been run. Plain Linux Docker with `host-gateway` can't reach it without `--bind`. Docker Desktop for Mac gets tested when v1 is built, not assumed.

**Surviving a restart.** MCP is served statelessly, and Claims, Rounds and all other state live only in the store, never in memory. On shutdown, the service finishes in-flight writes and closes long-polls. `code-central wait` retries through connection errors until the service is back. Claude Code retries a dropped HTTP MCP server five times over about 31 seconds and then marks it failed until someone reconnects it, so a restart must stay short. If the server is down when a session starts, Claude Code retries three times and then marks it failed.

**Live updates.** The UI receives Server-Sent Events. Each event only names what changed, such as a Ticket ID and a type, and the UI then refetches over the HTTP API. After a reconnect the UI refetches everything, so there's no event replay.

**Web stack.** The UI is React with TypeScript, built by Vite as a static single-page app with no meta-framework, and embedded with `go:embed`. The built files aren't committed. `make build` runs Vite and then `go build`, and release binaries are built the same way. Node is needed only to build. In development, Vite's dev server proxies to the Go service. A small placeholder page stays committed, so `go build` and Go tests work without Node.

**Go structure.** There is one Go module, with these packages:
- **`core`:** the one deep module. It holds the domain rules (States and moves, Claims and Claim IDs, Rounds, Failure Reports, Blockers, Silent Claims) behind a small interface. It takes a clock and an ID source as inputs.
- **`store`:** the interface `core` persists through, with a file-backed version and an in-memory fake, both checked by one contract test suite.
- **`events`:** change notices from `core`, which SSE and the long-poll behind `code-central wait` both subscribe to.
- **`mcp` and `httpapi`:** the two front doors, thin translators onto `core` that share the middleware and one `ServeMux`.
- **`client`:** the Go client for the HTTP API. Every `code-central` command uses it.
- ~~`integration`~~: removed by [The Integration plugin contract](14-integration-contract.md); an Integration is an HTTP client.
- **`web`:** the embedded UI.
- **`cmd/code-central`:** the program's entry point, which wires these together.

Considered and rejected:
- **Binding `0.0.0.0`:** it exposes an unauthenticated service to the network.
- **WebSockets:** they add two-way traffic the UI doesn't need, plus a library and reconnect logic.
- **Committing the built UI:** it makes `go install` work, but puts generated files in every diff.
- **Svelte, or plain TypeScript with htmx:** React is what agents know best.
- **The CLI starting the service:** MCP sessions never go through the CLI, so it wouldn't help them.
- **Cross-process file locks:** see ADR 0002.

Not verified: whether a container on Docker Desktop for Mac reaches a service bound only to loopback (only a forum post says so), whether the `sbx` proxy rewrites `Host` (the allowlist covers both cases), and what an MCP library does with a lost server-side session after a restart (stateless serving makes this moot). No Docker path was run.
