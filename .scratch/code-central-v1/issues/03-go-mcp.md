# Serving MCP from a Go binary

Type: research
Status: resolved

## Question

What is the current state of MCP server libraries for Go and of the MCP transports? How does Claude Code connect to a long-running local HTTP MCP server, and how would a session in a Docker sandbox reach one on the host? Can MCP and the HTTP API share one port?

## Answer

Both `modelcontextprotocol/go-sdk` (v1.8.0) and `mark3labs/mcp-go` (v1.5.0) are actively maintained and implement the current spec, 2026-07-28, whose Streamable HTTP is stateless. Claude Code connects by URL with `type: http`. A Docker session reaches the host through `host.docker.internal`, with setup that varies by environment. MCP and the HTTP API can share one port, because both libraries provide an `http.Handler`; this worked on one `ServeMux` in a local probe.

Findings: `research/go-mcp.md` on branch `research/go-mcp` (commit `0f365d3`).

Carry forward: both libraries reject `Host: host.docker.internal` with a 403 by default (seen with a simulated header); neither validates `Origin` by default, though the spec requires it; and a server bound only to 127.0.0.1 is probably unreachable through Linux `host-gateway`, which conflicts with the spec's advice to bind to localhost.

Not verified: no Docker path was tested, as Docker and `sbx` aren't installed here, and Linux wasn't tested. Claude Code was checked only with `claude mcp list`, so no real tool call ran. No browser DNS-rebinding attack was run.
