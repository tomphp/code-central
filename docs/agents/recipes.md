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

Any change to a store file struct (`store/file.go`), its yaml tags or a checked value set bumps the store format, even an additive one. File structs and their encode/decode stay explicit on purpose, so write them by hand and don't propose a generator: see [ADR 0003](../adr/0003-the-store-is-yaml-files-loaded-into-memory.md). Run `go test ./core -run TestTheStoreSchemaMatchesItsSnapshot`: when it fails, it prints the four steps and the snapshot to diff against. The example to copy is `8964c77`'s `store/migrate_claim_model_test.go`.

Parallel Tickets each bump the format, so they collide at merge. When merging the base brings in a format your branch also claimed, your migration moves to the next number. Renumber its `From` in `store/migrate.go` and `currentFormat`, keep the base's snapshot for the format it took, and write yours for the next format by running the snapshot test with `-write-store-schema`. Merge commit `8093159` shows this.

## Adding or changing a landing rule

Every landing rule is a method of `core.LandingPolicy` (`core/landingpolicy.go`), and no caller reads the Landing Method, Push, Land per Spec or CI Tracking to decide, not even one of them alone (ADR 0039).

1. Add a method to `core.LandingPolicy`, or extend one, with a doc comment quoting the glossary rule it answers.
2. Add or change its column in `TestLandingPolicyMatrix` (`core/landingpolicy_test.go`), which pins every Method × Push × Land per Spec × CI Tracking combination. How the Factory lands one Ticket is `Route`, pinned by `TestLandingPolicyRoute`.
3. Call it on the policy of the Repository in question: from core through `landingPolicyIn(p, name)` (or `landingPolicies(p).In(name)`), and from the Factory through `ProjectView.LandingPolicies().In(name)`. A Project-wide rule asks across the Repositories' policies, as a named method of `core.LandingPolicies` (such as `RunsPullRequestWatcher`, "while any Repository is under Pull Request"); a rule about one Ticket asks across its Changed Repositories', `ticketLandingPolicies(p, t)`.
4. Run `go test ./core -run TestNoLandingSettingIsReadOutsideThePolicy`. It type-checks `core/` and `factory/` and fails on any read of `Method`, `Push`, `LandPerSpec` or `Tracking` on `store.Landing`, `store.CI` or `core.LandingView`, naming each by file and line. Its allowlist, `landingGuardAllowlist` (`core/landingguard_test.go`), names the only code that may read them: the policy's file, which builds the policy, and the code that saves the settings and builds their views. Ask the policy rather than add an entry.
5. When the Options form must follow the rule, expose it as a Landing capability on `core.LandingView` (built in `landingViewWith`): in `methods`, `LandingMethodCapabilities`, if it depends on the method the human drafts, otherwise as a field of its own such as `runs_pull_request_watcher`. Run `make wiretypes`, and read that field in the UI. The UI's counterpart of step 4 is the landing scan in `web/src/test-setup.test.ts`: it fails on a comparison with a Landing Method or Repository Landing value, a `.method ===` or `!==`, or a `"_push"` test, naming file and line, unless the line or the one before carries `// landing: <reason>`.

## Add or change a UI component

A component used across areas lives in `web/src/controls/`; one that belongs to an area lives beside it (`ticket/`, `board/`, ...). It is presentational: data and callbacks come in as props. A view reads through the named hooks in `web/src/live.tsx`, never `useFetch` or `api` directly, and those hooks follow the live version themselves; a component that acts takes the client from `useApi()` and runs the action with `useAct`. Adding a read means adding a named hook there. It keeps the global CSS class names it renders.

1. Write the component, and a vitest test through its rendered output (role, accessible name, variant class). Where it needs data, build it with the scenario builder (`api/scenario.ts`) on a WASM Core world (`newWorld` in `api/core/core.ts`), through the real operations; see `api/core/clock.test.tsx`. Render a view that reads with `renderLive`.
2. Every component in `controls/` and every exported presentational component gets a `<Component>.stories.tsx` beside it (a controls story is titled `Controls/...`), one story per variant, using `Meta` and `StoryObj` from `@storybook/react-vite` (see `controls/ToggleGroup.stories.tsx`). Where it needs a Ticket, build its world with the scenario builder through `storyWorld` (`api/core/story.tsx`, which also mounts `<Live>` over the world; see `options/ModelsSection.stories.tsx`). Area CSS the story needs is imported in `web/.storybook/preview.ts`.
3. Run `make serve-components` to look at it, and `make build-components` to check Storybook still builds; both build `core.wasm` first (`make corewasm`). Storybook is a devDependency only: it is not in the embedded `dist` and `make test` doesn't run it, but CI builds it as its own step in the `test` job, so a broken story fails the run.

## Renaming across files

No `gopls`, `gorename` or `eg` is installed, and none is needed.

- **An expression or call**: `gofmt -w -r 'x.m -> x.repos.m' *.go` rewrites every match in the files you name. Single-letter names in the pattern are wildcards, so it can't rename a variable or a receiver such as `f` to `l`.
- **An identifier, method, field or receiver**: `go run golang.org/x/tools/gopls@v0.24.0 rename -w <file>:#<byte offset> <new name>`, which fetches gopls on first use. The offset is of the name in the file: `grep -b 'func (f \*factory) run' factory/x.go` gives the line's offset, and the name is a few bytes in. It follows the type checker, so it renames every use across the package and leaves look-alikes alone. A receiver is renamed per method, so repeat it for each method. Run `go build ./...` afterwards. It was checked on a receiver rename in a scratch copy of `factory/`.
