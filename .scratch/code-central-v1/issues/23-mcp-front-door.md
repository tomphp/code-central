# 23: The MCP front door and the cross-door suite

**What to build:** An agent session connects to Code Central over MCP at the URL that names its Project and can create and read Specs and Tickets as tools, with no Project argument. Every operation is exposed through each front door from one list, and a single suite proves the three doors translate the same way. One middleware guards MCP and the HTTP API against requests from the wrong host or origin. See the [spec](../spec.md), sections "Shape of the system" and "Operations and front doors".

**Blocked by:** 22

**Status:** ready-for-agent

- [ ] MCP is served statelessly over Streamable HTTP at `/p/<key>/mcp`, sharing one `ServeMux` and port with the HTTP API
- [ ] Agent operations appear as snake_case MCP tools with one-line descriptions and no Project argument; every MCP write is made as `agent`
- [ ] For an unknown Project Key the MCP connection is accepted and every tool call returns `unknown_project`, naming the key and telling the agent to ask the human to run `code-central init`
- [ ] MCP and the CLI accept both `<key>-<n>` and a bare `<n>`; an ID with another Project's key is rejected with an error naming the connection's Project
- [ ] MCP errors are a tool result with `isError` set and the error JSON as text
- [ ] One middleware in front of MCP and the HTTP API allows only `Host` values `localhost`, `127.0.0.1`, `[::1]` and `host.docker.internal` (plus any `--bind` address later); a missing `Origin` is allowed and a present one must be from those hosts; no CORS headers are sent; the MCP library's own checks are off
- [ ] The middleware has its own unit tests, run once: the allowlist, no CORS headers, and `GET` never writing
- [ ] One table-driven suite, built from `core`'s operation list, runs each operation through MCP (the Go MCP client over Streamable HTTP), the HTTP API and the CLI (in-process, `client` pointed at a test server), with one successful and one rejected call each, checking each door's error shape
- [ ] The suite fails if a door is missing an operation it should expose, or if MCP or the CLI exposes a human-only operation; it tests translation only, never rules
