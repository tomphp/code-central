# The testing strategy by seam

Type: grilling
Status: resolved
Blocked by: 10, 11, 12, 14

## Question

Given the final seams, what is tested where: which contract tests, fakes and end-to-end tests exist, how is the UI tested, and how is the 10-second budget enforced?

[Process shape and web stack](10-process-and-stack.md) sets the package structure: a deep `core` with a clock and ID source as inputs, a `store` with a file-backed version and an in-memory fake under one contract test suite, `events`, thin `mcp` and `httpapi` front doors, a `client` that the CLI uses, and `web`, with a committed placeholder page so Go tests run without Node.

[The MCP and HTTP surface](12-api-surface.md) gives one operation list exposed by three front doors with one error shape. Can one contract suite run every operation through MCP, the HTTP API and the CLI alike?

[How the Scratch Import behaves](13-scratch-import.md) adds the one bulk write: staged files, a manifest and renames rolled forward at startup. How is recovery from a crash at each step tested, and are the payload validator and its `field` paths tested once in `core`, rather than through every front door?

[The Integration plugin contract](14-integration-contract.md) removed the `integration` package: an Integration is an HTTP client, and none ships in v1. Where are the `integration` Actor's rules tested (only it writes definitions on linked items, it can't move State), along with the "closed externally" reason and `unlink`? Is the HTTP API contract suite enough to keep a future Integration's fake of Code Central honest?

[The attention-first UI](15-attention-ui.md) fixes the UI's shape for v1: one triage inbox, with one action panel per Needs Me reason that later views will share. Are the panels tested once against a fake API, with the inbox's layout tested separately? Which UI behaviours, if any, need an end-to-end test through the real binary?

## Answer

Each behaviour is tested once, at the seam that owns it. The only fakes are the in-memory store, the clock, the ID source and the UI's fake API, and each fake runs under a contract suite with the real thing it stands in for. There is no fake of `core` and no fake of Code Central.

**`core`.** Every domain rule is tested here and nowhere above it, against the in-memory store with a fake clock and ID source:
- States and moves, Claims and Claim IDs, Rounds, Failure Reports, Blockers and Replacements, Silent Claims, and cycle rejection.
- The Actor rules, including that only the `integration` Actor writes definitions on linked items and that it can't move State, plus "closed externally" and `unlink`.
- The Import validator and its `field` paths.
- Needs Me grouping and order. `core` computes the reason order and longest-waiting-first, `/api/needs-me` returns them already ordered, and the UI renders groups as it receives them, so the later views get the same order.

**Time.** The injected clock reaches `core`, `events` (the Silent Claim ticker) and the long-poll's 60-second timeout, so the 30-minute Silent Claim, the 60-second ping throttle and `wait` are all tested without sleeping. A test that sleeps on real time counts as a bug.

**The store.** One contract suite runs against the file-backed store and the in-memory fake: reads after writes, the derived ID counter, Replacements followed on read, and Import's all-or-nothing result. File-backed tests use `t.TempDir()`. File-only concerns are tested only on the file-backed store: strict decoding that stops startup and names the file and field, refusing an unknown `format`, the exclusive lock, one file per write, and fixed field order.

**Import crash recovery.** The file-backed Import write has a test-only "stop after step k" hook, where the steps are each staged file, the manifest, each rename and the manifest delete. For every k, a test runs a real Import that stops after step k, runs startup recovery on what's left, and checks that either nothing or everything was imported. A new step is covered automatically. Two named cases sit beside it: a staging folder with no manifest is deleted, and a manifest with some renames done is rolled forward.

**The front doors.** `mcp`, `httpapi` and the CLI are tested against the real `core` with the in-memory store, all in-process. One table-driven suite, built from `core`'s operation list, runs each operation through every door that exposes it:
- MCP through the Go MCP client over streamable HTTP.
- The HTTP API directly.
- The CLI with its commands run in-process and `client` pointed at an `httptest` server.

Each operation gets one successful call and one rejected call, which checks the error shape for that door: `isError` with JSON text for MCP, a 404, 409 or 422 status for HTTP, and JSON on stderr with exit `2` for the CLI. The suite fails if a door is missing an operation it should expose, or exposes a human-only one over MCP or the CLI. It tests translation only, never rules. The shared middleware gets its own unit tests in `httpapi`, run once: the `Host` and `Origin` allowlist, the Actor header, no CORS headers, and `GET` never writing.

**CLI-only behaviour.** Commands outside the operation list are tested once in the CLI package against `codecentraltest`, with a fake clock:
- `wait`'s exit codes `0`, `3` and `1`, and its retries through connection errors.
- `ping` giving up after 300 ms and always exiting `0`.
- The version-mismatch warning on stderr.
- How `init` derives the Project Key.
- The `service_unavailable` message naming `serve` and `service install`.

**HTTP clients.** `codecentraltest.NewServer(t)` starts the real `core`, the in-memory store and a fake clock behind the real `httpapi` on an `httptest` port. The `client` package tests against it, and so can a future Integration or Factory written in Go, so a client's tests hit the real API and can't drift. A client in another language runs `code-central serve --store <tmpdir>` instead.

**The UI.** The UI reaches the HTTP API and SSE through one TypeScript API module, with an in-memory fake of it.
- **The contract suite** covers each read, each human action, the error shape and SSE event delivery. It runs against the fake in the fast suite, and against the real module talking to `code-central serve --store <tmpdir>` (an already-built binary on a free port, reset between tests) in the end-to-end tier.
- **Action panels** get component tests once each, with Vitest and Testing Library against the fake API: the Round, Resume-then-Release, `Failed`, `In Review`, Blocked and closed externally panels. They check what each panel shows and which action it calls with what payload, such as the pre-filled recommendation, Accept all recommended, Reject needing a comment, and the Release warning.
- **Inbox layout tests** use the same fake API with real panels, with no panel mocks, and check only layout. Groups render in the order the server returns them, the Drafts group is collapsed, a State filter gives a flat list with dots, the Project and View filters combine, the view is in the URL, and an SSE notice triggers a refetch.

**End-to-end.** Only these run through the real binary:
1. **The agent loop:** `init`, then an MCP `claim` and `post_round`, the human answers over HTTP, `code-central wait` exits `0`, then `pick_up_answers`, `finish` and `accept`.
2. **Restart:** the service is stopped during a `wait` and started again. `wait` resumes and the state survives, and a second `serve` on the same store refuses to start.
3. **The UI in a real browser:** Playwright against the embedded, built UI. An agent posts a Round, the inbox updates over SSE, and the human answers it.

The Docker and `sbx` paths, `service install`, and a check against real Claude Code are a manual checklist run when v1 is built, not automated.

**The 10-second budget.** `make test` runs `go test ./...` and `vitest run` in parallel against the committed placeholder page, so no Vite build is needed, and fails hard if the run takes more than 10 seconds of wall-clock time. `make e2e` builds the real binary and UI, then runs the three end-to-end tests and the real half of the UI contract suite. End-to-end Go tests sit behind an `e2e` build tag, so `go test ./...` never runs them. Both targets run in CI once CI exists.

Considered and rejected:
- **Testing rules through each front door:** the same rule would be tested three times.
- **A fake `core` for the front doors:** `core` is deep, fast and in-process, so a fake would only add something to drift.
- **A hand-written fake of Code Central for HTTP clients:** `codecentraltest` runs the real thing just as cheaply.
- **Hand-built fixture folders for crash recovery:** stepping through every k covers new steps without new fixtures.
- **Needs Me order computed in the UI:** each view would have to repeat it.
- **Running every UI test against the real Go service:** it needs a Go build, which breaks the 10-second budget.
- **Mocking panels in layout tests:** the fake API is already cheap, and one fake is simpler than two.
- **Automating the Docker, `sbx` and real Claude Code checks:** they depend on the machine and are run once per release.
