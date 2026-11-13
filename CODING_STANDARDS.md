# Coding standards

Read during review. `make test` already enforces what tooling can (go vet, the TypeScript type check, the suite's 10s budget); these are the judgement calls past reviews kept finding. Also check the diff against `GLOSSARY.md` and any ADR in `docs/adr/` that touches the area.

## Names are glossary terms

Name types, functions, props, CSS classes and test descriptions with the `GLOSSARY.md` noun for the concept (Claim, Round, Focus Item, Session Event, Transcript), not a synonym. A new concept in the diff gets a glossary entry in the same change. A name says what the thing is (`isSkipped`), not how it's computed.

## Every door, and the fake, behave the same

An operation lives in `core`; the front doors (MCP, the HTTP API, the CLI) only translate. When the diff adds or changes an operation:

- every door that exposes it is covered in the cross-door suite, `frontdoor/doors_test.go`: one successful and one rejected call each;
- a new HTTP route joins the shared handlers in `httpapi/httpapi.go` rather than getting its own decoding and error mapping;
- the UI's `web/src/api/fake.ts` matches the service, and the behaviour is pinned in `web/src/api/contract.ts`, which runs against both the fake and the real service.

## One helper per setup

When a second test needs the same setup (a test server, an SSE opener, a world fixture, a link-click check), extract one shared helper and have both tests use it. Look for an existing helper in the package, or in `codecentraltest` / `factory/factorytest`, before writing one.

## A test waits for the loaded state before it interacts

In a changed `*.test.tsx`, a component whose data arrives through a fetch or a subscription is not there yet when `render` returns:

- Wait for its loaded state before interacting or asserting: a `findBy*` on the element that arrives last, or the file's `loaded()` helper (`loadedDashboard` for the Activity Dashboard), never a `getBy*` straight after `render`.
- A click on an element inside a block that remounts when its data arrives waits for that data first (the settled count or text), then queries the link fresh.
- A `waitFor` wraps one assertion, and a passing test never waits a timeout out (`web/src/test-setup.ts` says why).
- Fix a flaky test by waiting for the thing it raced, never by a sleep or a longer timeout.

## Comments match their neighbours

Each file opens with a comment on what it's for, and each exported or non-obvious declaration gets a short one stating what it is or why. Match the density and voice of the surrounding code. When the diff changes behaviour, the comments, ADRs and glossary entries that describe it change in the same commit.

## Commit messages are conventional commits

Every commit on the branch since the latest tag follows the convention in `CLAUDE.md`'s Commits section: `type(scope)!: subject`, with the type that says what the change does to a user. A user-facing feature is `feat` and a bug fix is `fix`, because they release; docs, tests, refactors, build and CI changes don't, so they must not be `feat` or `fix`. A breaking change is marked with `!` or a `BREAKING CHANGE:` footer. Merge commits are exempt. The hook and CI's `cog check` catch a malformed message; review checks that the type is the right one.
