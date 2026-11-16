# Coding standards

Read during review. `make test` already enforces what tooling can (go vet, the TypeScript type check, ESLint, the suite's 60s budget); these are the judgement calls past reviews kept finding. Also check the diff against `GLOSSARY.md` and any ADR in `docs/adr/` that touches the area.

## Names are glossary terms

Name types, functions, props, CSS classes and test descriptions with the `GLOSSARY.md` noun for the concept (Claim, Round, Focus Item, Session Event, Transcript), not a synonym. A new concept in the diff gets a glossary entry in the same change. A name says what the thing is (`isSkipped`), not how it's computed.

## Every door behaves the same, and the UI's tests run on the real Core

An operation lives in `core`; the front doors (MCP, the HTTP API, the CLI) only translate. When the diff adds or changes an operation:

- every door that exposes it is covered in the cross-door suite, `frontdoor/doors_test.go`: one successful and one rejected call each;
- a new HTTP route joins the shared handlers in `httpapi/httpapi.go` rather than getting its own decoding and error mapping;
- the UI's tests run on the real Core, built to WebAssembly (the WASM Core), and the contract, `web/src/api/contract/` (a file per feature), pins `createHttpApi` against it and against `serve`. A UI test reaches its state through the scenario builder (`web/src/api/scenario.ts`), never by seeding one.

## One helper per setup

When a second test needs the same setup (a test server, an SSE opener, a world fixture, a link-click check), extract one shared helper and have both tests use it. Look for an existing helper in the package, or in `codecentraltest` / `factory/factorytest`, before writing one.

## A test settles the world before it asserts

In a changed `*.test.tsx`, a component whose data arrives through a fetch or a subscription is there once the WASM Core world has settled ([ADR 0044](docs/adr/0044-ui-tests-wait-for-the-world-to-settle-not-for-dom-text.md)), not before:

- Rendering (`renderApp`, `renderLive`) and every Driver and scenario step settle the world themselves, so a test asserts with `getBy*` straight after awaiting them.
- After a `user-event` interaction or a direct `w.api` call, `await settle(w)`, then query with `getBy*`. Build a world through the shared world testing helpers, whose `scenario` settles with `act`; the API folder's bare `scenario(w)` doesn't flush React.
- After a settle, query an element fresh rather than reuse one found before it: a block that remounts when its data arrives, or a popup another of the same name takes over from, has replaced it.
- A `findBy*`, `findAllBy*` or `waitFor` is only for a real UI timer (the Focus Mode pulse, session and note timers, the ShortId copy feedback, the feed's retry) and carries a `// waits: <reason>` comment on its line or the line before. A test that seems to need any other wait has found something *settled* doesn't count: fix the world control, not the test.
- A `waitFor` wraps one assertion with no side effects inside it, and a test never sleeps: ESLint enforces both (a deliberate pause takes `// eslint-disable-next-line no-restricted-syntax -- <reason>`). A passing test never waits a timeout out either (`web/src/test-setup.ts` says why).
- Fix a flaky test by settling before the thing it raced, never by a longer timeout.

## Comments match their neighbours

Each file opens with a comment on what it's for, and each exported or non-obvious declaration gets a short one stating what it is or why. Match the density and voice of the surrounding code. When the diff changes behaviour, the comments, ADRs and glossary entries that describe it change in the same commit.

## Commit messages are conventional commits

Every commit on the branch since the latest tag follows the convention in `CLAUDE.md`'s Commits section: `type(scope)!: subject`, with the type that says what the change does to a user. A user-facing feature is `feat` and a bug fix is `fix`, because they release; docs, tests, refactors, build and CI changes don't, so they must not be `feat` or `fix`. A breaking change is marked with `!` or a `BREAKING CHANGE:` footer. Merge commits are exempt. The hook and CI's `cog check` catch a malformed message; review checks that the type is the right one.
