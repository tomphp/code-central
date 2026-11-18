# UI tests wait for the world to settle, not for DOM text

The UI's tests run on the WASM Core ([ADR 0060](0060-the-uis-fake-is-the-real-core-built-to-webassembly.md)), but nothing told a test when the world had finished its work, so each test waited on some DOM text with `findBy*` or `waitFor`. Those waits hid races rather than removing them: a write made before a feed had opened missed its notice, and raised timeouts covered it up, so failing tests were slow to fail and slow machines flaked. So we decided that a test waits for the world to settle, and asserts synchronously once it has.

A WASM Core world is **settled** when all three of these hold:

- No plain request is in flight. A plain request is any request that is not a stream. It is in flight from the moment the client calls the world's `fetch` until its handler has returned, which happens only after the client has read its whole body.
- Every feed the client has subscribed (the change feed, a Transcript stream, a Chat stream) has an open stream: its status and opening frame have been read. A subscribed feed with no open stream is busy, whether it hasn't connected yet or is waiting out its retry after `dropStreams()`, and so is a stream still closing.
- No open stream has a frame written but not yet read by the client.

The word is *settled* because *idle* is the Guided Map's. It lives here, not in `GLOSSARY.md`, because the world, the Driver and the scenario builder are test machinery.

**How it is measured.** Go reports, JavaScript counts, JavaScript polls.

- The Go world's `Settled` control takes how many feeds of each stream kind the client has subscribed and answers what keeps the world busy: the plain requests in flight, each kind's open streams against its subscribed feeds where they differ, and the open streams with an unread write. An empty report means settled. The world already tracks every request it serves and the stream it follows, and its response writer counts what it has written that the reader has yet to read, since an `io.Pipe` write waits for its reader. `cmd/corewasm` binds it as `settled(id, events, transcripts, chats)`, answering the report as JSON.
- A synchronous check is exact because under `js/wasm` Go hands control back to JavaScript only when every goroutine is blocked. Whenever JavaScript can ask, a stream handler holding a queued notice has already written it and is blocked on its reader, so "no unread write" also means "no queued notice", and no feed needs hooks of its own. Go's reader is `cmd/corewasm`'s pull into the JavaScript `Response`'s stream, which runs at most one chunk ahead of the client's own read. The client takes that chunk in microtasks, and the JavaScript side checks only between macrotasks, so by then the client has it.
- The JavaScript `World` counts the subscriptions open on its `api` for all three kinds, the fetches it has started whose `core.fetch` hasn't settled yet (Go may see a request only on its next scheduling pass), and a running total of fetches started. Its `settled()` checks at once, then once per macrotask on a real timer saved when the module loads, so a test that fakes timers doesn't stall it. It resolves when Go's report is empty and no fetch is pending, and rejects after about 2 s naming what was still busy.

**The fixed point, `settle(w)`.** The shared world testing helpers' `settle(w)` runs `await act(() => w.settled())` in a loop until a pass starts no new fetch, and fails after about 10 passes that did, saying the UI keeps fetching. The loop is needed because a notice makes `<Live>` refetch, the refetch renders, and the render can subscribe or fetch again. `act` is the only thing that flushes React's effects, so this helper, not the world, holds the React part. An async `act` itself renders until React has nothing queued, so a UI that refetches as fast as the world answers never lets a pass end: while a pass's `act` renders after the world has settled, each poll that finds a fetch started since counts as a round too, and settle fails after about 10 rounds in all.

**The Driver's injected settle.** `worldDriver(w, settle?)` and `scenario(w, settle?)` take an optional settle function, which the Connection carries, and the Driver awaits it after every operation: each write and each world control it calls. A world's default is the bare `w.settled()`. The shared world testing helpers build `scenario(w, () => settle(w))`, so component tests settle through `act` without passing it, and `renderApp` and `renderLive` await `settle(w)` after rendering. A test therefore asserts with `getBy*` straight after its setup, and awaits `settle(w)` itself only after a `user-event` interaction or a direct `w.api` call.

**WASM-only.** The control exists only on the WASM Core. The serve adapter of the contract suite passes a no-op settle, and contract cases that wait for a notice or a stream frame keep `expect.poll`. Nothing is added to `code-central serve`.

**The `// waits:` escape and the scan.** A `findBy*`, `findAllBy*` or `waitFor` in a `*.test.tsx` stays only where it waits on a real UI timer (Focus Mode's pulse, session and note timers, the ShortId copy feedback, the feed's retry), and carries a `// waits: <reason>` comment on its line or the line before. A vitest test beside the shared test setup's own test scans every `*.test.tsx` and fails, naming the file and line, on any wait without one or with an empty reason. A wait that seems to need any other reason points to a gap in *settled*: fix the world control, not the test.

## Considered Options

- **Longer timeouts.** Rejected: they hide races instead of removing them, make a failing test slow to fail, and still flake on a slow machine. Once every test settles, the timeouts go back to Testing Library's 1000 ms and vitest's 5000 ms.
- **Waiting on DOM text.** Rejected: each test has to work out by hand what it is racing, and a wait on text that is already there passes before the race is over.
- **A settled or idle endpoint on `serve`.** Rejected: it adds test-only behaviour to the real service. The contract cases against `serve` keep `expect.poll` for the few waits they need.
- **An ESLint rule instead of the scan, for now.** Rejected for now: `web/` has no ESLint, and adding a toolchain for one rule is out of scope. Requirement code-central-801 adds ESLint to the web UI and moves the rule into it.

## Consequences

- The world reports on itself through `Settled`, which reads what the world already tracks; no feed or Core rule changes.
- A test that faked timers still settles, since `settled()` polls on a real timer.
- A UI that keeps fetching fails `settle(w)` with a message rather than hanging a test.
- Storybook stories don't settle; they need no change.
- A Go test of `Settled` on native Go synchronises on its reader, as goroutines there don't yield only when all are blocked; the WASM Core's own tests cover the `js/wasm` behaviour.
