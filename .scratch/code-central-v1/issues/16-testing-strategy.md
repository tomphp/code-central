# The testing strategy by seam

Type: grilling
Blocked by: 10, 11, 12, 14

## Question

Given the final seams, what is tested where: which contract tests, fakes and end-to-end tests exist, how is the UI tested, and how is the 10-second budget enforced?

[Process shape and web stack](10-process-and-stack.md) sets the package structure: a deep `core` with a clock and ID source as inputs, a `store` with a file-backed version and an in-memory fake under one contract test suite, `events`, thin `mcp` and `httpapi` front doors, a `client` that the CLI uses, and `web`, with a committed placeholder page so Go tests run without Node.

[The MCP and HTTP surface](12-api-surface.md) gives one operation list exposed by three front doors with one error shape. Can one contract suite run every operation through MCP, the HTTP API and the CLI alike?

[How the Scratch Import behaves](13-scratch-import.md) adds the one bulk write: staged files, a manifest and renames rolled forward at startup. How is recovery from a crash at each step tested, and are the payload validator and its `field` paths tested once in `core`, rather than through every front door?

[The Integration plugin contract](14-integration-contract.md) removed the `integration` package: an Integration is an HTTP client, and none ships in v1. Where are the `integration` Actor's rules tested (only it writes definitions on linked items, it can't move State), along with the "closed externally" reason and `unlink`? Is the HTTP API contract suite enough to keep a future Integration's fake of Code Central honest?
