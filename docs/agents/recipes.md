# Recipes

Recipes for changes that touch many packages. Each one starts with a single edit and then lets the tests name what's missing: make the first change, run the tests listed, and fill in each gap they report. That's cheaper than tracing an existing operation through every layer by reading.

## Add or change an operation

1. Add it to `core.Operations` in `core/operations.go` with its `Audience` and `Write`, then write the Core method.
2. Run `go test ./core ./server`. The guard tests name each missing piece:
   - `TestHTTPExposesEveryOperation`: a route in `httpapi.Routes` (`httpapi/httpapi.go`), served through the shared handlers.
   - `TestMCPExposesExactlyTheAgentOperations` and `TestCLIExposesExactlyTheAgentOperations`, for `AgentAndHuman` only: a tool in `tools` (`mcp/mcp.go`, with a one-line description) and a kebab-case command in `cli/cli.go`, which calls the service through a `client/` method.
   - `TestEveryOperationHasACase`: a cross-door case. Put a feature's cases in their own `server/<feature>_doors_test.go`, registered from `init()` (see `server/modelchoice_doors_test.go`).
   - `TestAnArchivedProjectRefusesEveryWrite`, for a write: an entry in `writesOnAcme` in `core/archive_test.go`.
3. When the UI uses it: types, action, contract case. Response types come from `make wiretypes` (add a root to the generator's list in `internal/wiretypes` for a new top-level view); the staleness test fails if you forget to regenerate, and a typed string the UI reaches needs a value list registered in the generator. Hand-write only the request body in `web/src/api/types.ts`, the action in `web/src/api/actions/` (its body and result, merged into `TicketActions`), and a case for its behaviour in the feature's file under `web/src/api/contract/` (`contract/index.ts` registers them all; the `Driver` they use is in `web/src/api/driver.ts`, and `defaultLanding` and `defaultCI` in `scenario.ts` build a new Project's rows). `make vitest F=src/api/fake.test.ts` runs the contract's WASM half: `createHttpApi` on the WASM Core, the real Core built to WebAssembly (`make corewasm` builds it first).

Examples to copy: `02bd5d9` (`keep_as_draft`: a human operation with UI) and `8964c77` (`get_model_choice`: Factory-only, with a `client/` method).

## Change a store file

Records and their enums live in `domain` (`domain/domain.go`); the file shapes that persist them live in `store`. Change the record in `domain` and its file struct in `store`. Any change to a store file struct (`store/file.go`), its yaml tags or a checked value set bumps the store format, even an additive one. File structs and their encode/decode stay explicit on purpose, so write them by hand and don't propose a generator: see [ADR 0003](../adr/0003-the-store-is-yaml-files-loaded-into-memory.md). Run `go test ./core -run TestTheStoreSchemaMatchesItsSnapshot`: when it fails, it prints the four steps and the snapshot to diff against. The example to copy is `8964c77`'s `store/migrate_claim_model_test.go`.

Parallel Tickets each bump the format, so they collide at merge. When merging the base brings in a format your branch also claimed, your migration moves to the next number. Renumber its `From` in `store/migrate.go` and `currentFormat`, keep the base's snapshot for the format it took, and write yours for the next format by running the snapshot test with `-write-store-schema`. Merge commit `8093159` shows this.

## Adding or changing a landing rule

Every landing rule is a method of `domain.LandingPolicy` (`domain/landingpolicy.go`), and no caller reads the Landing Method, Push, Land per Spec or CI Tracking to decide, not even one of them alone (ADR 0039).

1. Add a method to `domain.LandingPolicy`, or extend one, with a doc comment quoting the glossary rule it answers.
2. Add or change its column in `TestLandingPolicyMatrix` (`domain/landingpolicy_test.go`), which pins every Method × Push × Land per Spec × CI Tracking combination. How the Factory lands one Ticket is `Route`, pinned by `TestLandingPolicyRoute`.
3. Call it on the policy of the Repository in question: from core through `landingPolicyIn(p, name)` (or `landingPolicies(p).In(name)`), and from the Factory through `ProjectView.LandingPolicies().In(name)`. A Project-wide rule asks across the Repositories' policies, as a named method of `core.LandingPolicies` (such as `RunsPullRequestWatcher`, "while any Repository is under Pull Request"); a rule about one Ticket asks across its Changed Repositories', `ticketLandingPolicies(p, t)`.
4. Run `go test ./core -run TestNoLandingSettingIsReadOutsideThePolicy`. It type-checks `domain/`, `core/` and `factory/` and fails on any read of `Method`, `Push`, `LandPerSpec` or `Tracking` on `domain.Landing`, `domain.CI` or `core.LandingView`, naming each by file and line. Its allowlist, `landingGuardAllowlist` (`core/landingguard_test.go`), names the only code that may read them: the policy's file, which builds the policy, and the code that saves the settings and builds their views. Ask the policy rather than add an entry.

## Add or change a UI component

A component used across areas lives in `web/src/controls/`; one that belongs to an area lives beside it (`ticket/`, `board/`, ...). It is presentational: data and callbacks come in as props. A view reads through the named hooks in `web/src/live.tsx`, never `useFetch` or `api` directly, and those hooks follow the live version themselves; a component that acts takes the client from `useApi()` and runs the action with `useAct`. Adding a read means adding a named hook there. It keeps the global CSS class names it renders.

1. Write the component, and a vitest test through its rendered output (role, accessible name, variant class). Where it needs data, build it with the scenario builder (`api/scenario.ts`) on a WASM Core world (`newWorld` in `api/core/core.ts`), through the real operations; see `api/core/clock.test.tsx`. Render a view that reads with `renderLive`.
2. Every component in `controls/` and every exported presentational component gets a `<Component>.stories.tsx` beside it (a controls story is titled `Controls/...`), one story per variant, using `Meta` and `StoryObj` from `@storybook/react-vite` (see `controls/ToggleGroup.stories.tsx`). Where it needs a Ticket, build its world with the scenario builder through `storyWorld` (`api/core/story.tsx`, which also mounts `<Live>` over the world; see `options/ModelsSection.stories.tsx`). Area CSS the story needs is imported in `web/.storybook/preview.ts`.
3. Run `make serve-components` to look at it, and `make build-components` to check Storybook still builds; both build `core.wasm` first (`make corewasm`). Storybook is a devDependency only: it is not in the embedded `dist` and `make test` doesn't run it, but CI builds it as its own step in the `test` job, so a broken story fails the run.
