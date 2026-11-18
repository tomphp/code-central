# The UI's fake is the real Core built to WebAssembly

The UI's tests and Storybook need a world to run against, and the hand-written `FakeApi` was a second implementation of Core that grew back every time a state was awkward to reach. So we decided that the fake is the WASM Core: the real Core and `httpapi`, over `store.NewMemory()` and a fake clock, built to `js/wasm`. The UI's client is the real `createHttpApi` over a WebAssembly `fetch`. The Driver (the one module that plays agents, integrations and the Factory through operations over a `fetch`) and the Scenario builder (the Driver plus helpers for whole flows) are the only way a test or story builds a world. The same contract cases run on the WASM Core in `make test` and on `code-central serve` under `make e2e`.

## Considered Options

- **Seeded data.** Rejected: it is a second implementation of Core, so it drifts from Core and hides coupling to it. A seeded path is how that second implementation grows back.
- **Replay of recorded responses.** Rejected: it hides the coupling to Core and goes stale on every Core change.

## Consequences

- **No seeding.** Every state is reached through a real operation. There is no seeded escape hatch.
- **World controls stand only for what lies outside Core:** the clock, what the gh checks answer, a Watcher crash report, dropping the network's open streams, and the world's lifetime. Each calls the entry point `serve`'s own code calls, never a rule.
- **One control is for tests alone:** `settled`, which reports what keeps the world busy so a UI test waits for the world rather than for DOM text. [ADR 0044](0044-ui-tests-wait-for-the-world-to-settle-not-for-dom-text.md) defines it.
- **`httpapi` and the MCP front door stay in the bridge.** The client is the real `createHttpApi`, and agent MCP calls are how Core records call Usage, so the contract's cases need both.
- **The cost, measured by the Prototype:** the binary is about 15 MB, 3.8 MB gzipped; a cold start is about 200 ms per worker; each test costs 3 to 20 ms more; the suite takes about 1 s longer. We accept it.
- **`core.wasm` is never committed or embedded.** It is built locally by `make corewasm`, git-ignored, and not part of the embedded `dist`. Only vitest and Storybook load it.
- **ADR 0004 is untouched.** The WASM Core is reached only through the HTTP API and MCP, as the real service is.
