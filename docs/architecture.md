# Architecture

A high-level map of Code Central's modules, what each owns, the contracts between them, and where those contracts are weakest. It is a review of the code, not a spec: first written for code-central-912 and revised on 2026-11-19 against the import graph (`go list`) of that day. The ADRs in `docs/adr/` hold the decisions behind it.

## The shape in one paragraph

One **service** (`code-central serve`) holds all state, and it is the only writer (ADR 0002). Everything else is a **client of its HTTP API**: the React UI, the CLI, the Factory and the Watchers (ADR 0004, 0005, 0020). Agents reach the same operations through a second door, **MCP**. Both doors dispatch through **`ops`**, the one list of operations, to the **Core**. The Core does no I/O apart from the Store (and reading its own memory size for the Storage tab), but it does hold runtime state in memory: Heartbeats, Transcripts, chats, activity, Factory wakes and Watcher status. Several services may share one Postgres Store (ADR 0068).

## Diagram

```mermaid
flowchart TB
  subgraph Clients["Clients (separate processes, HTTP only at run time)"]
    UI["Frontend<br/>web/ (React, embedded)"]
    CLI["CLI<br/>cli/ + cmd/code-central"]
    FAC["Factory<br/>factory/ + cmd/code-central-factory"]
    subgraph W["Watchers (supervised subprocesses of the service binary)"]
      CIW["ciwatcher"]
      ISW["issuewatcher"]
      PRW["pullrequestwatcher"]
      WIW["workitemwatcher"]
    end
    AGENT["Claude Code sessions"]
  end

  CL["client<br/>Go HTTP client"]

  subgraph Service["Service process (code-central serve)"]
    APP["app<br/>app.Run: the composition root"]
    FD["server<br/>handler + Guard + Calls + UI"]
    API["httpapi<br/>REST /api"]
    MCP["mcp<br/>/p/key/mcp"]
    OPS["ops<br/>ops.All(): every operation, bound to *core.Core"]
    CORE["core<br/>*Core: workflow, landing, CI, chat, read models"]
    WIRE["api<br/>the wire contract: inputs, views, rows, results, errors, Ref, Actor, Decode"]
    DOMAIN["domain<br/>records, enums, Landing Policy"]
    STORE["store<br/>Files · SQLite · Postgres"]
    SUP["watchers.Supervise"]
    SIDE["clock · events · ids · home · usage · notify"]
  end

  EXT["GitHub (gh CLI) · Azure DevOps (REST)<br/>via gh, azdo, azdourl, codehost"]
  GIT["git repositories / worktrees"]

  UI -->|REST + change feed| FD
  CLI & FAC & CIW & ISW & PRW & WIW --> CL -->|HTTP| FD
  AGENT -->|MCP tools| FD
  APP -.assembles.-> FD & CORE & STORE & SUP
  FD --> API & MCP --> OPS --> CORE
  CORE --> STORE --> DOMAIN
  CORE --> DOMAIN
  CORE --> SIDE
  CORE --> WIRE --> DOMAIN
  SUP -.imports.-> WIRE
  SUP -.starts.-> W
  FAC --> GIT
  FAC -.spawns.-> AGENT
  CIW & ISW & PRW & WIW --> EXT
  CL & FAC & CIW & ISW & PRW & WIW -.import the wire types from.-> WIRE
  CL -.imports ops, which imports core, from.-> OPS
  FAC & ISW & PRW & WIW -.call a few core rules in process.-> CORE
```

The dotted edges are the ones the run-time picture hides. The wire types are in `api` (weakness 1, fixed), so no client needs `core` for them. But `client` imports `ops` (weakness 4), and the Factory and three of the Watchers still call a few `core` functions (weakness 3), so they link `core` and `store` all the same.

A second, test-only composition exists: `world/` + `cmd/corewasm` build the real Core, `httpapi` and `mcp` over an in-memory store to WebAssembly (built by `go run ./internal/corewasm`), and the UI's tests and stories run against that instead of a hand-written fake (ADR 0060, 0044).

## Modules

| Module | Owns | Its contract |
|---|---|---|
| `core` | The workflow and every rule over it: Claims, Rounds, triage, Requirements, landing, CI, pull requests, issue and work-item links, chat, usage, the read models. It names the wire types (`*Input`, `*View`, `*Row`, `*Result`, the error codes, `Ref`, `Actor`, `Decode`) as `api.X`; it declares none, and has no alias for one. | `*core.Core`'s 150 exported methods; no interface. |
| `api` | The wire contract (ADR 0072): every input, view, row and result of an operation, the error codes, `Ref`, `Actor`, `Decode`, the Watcher status contract, and the rules that belong to one of those types (`TicketView.Brief`, `ProjectView.LandingPolicies`, `FindTicketsInput.EncodeQuery`). It imports only `domain` and `internal/fieldwalk`. | The exported types; `internal/wiretypes` generates `wire.gen.ts` from them. |
| `ops` | The one list of operations (`ops.All()`, ADR 0063): name, audience, route, input and output types, and the `Call` that binds each to a `*core.Core` method. `ops.Serve` adapts the Core to `ops.Service`, which `httpapi` needs, and `ops.Dispatcher`, which `mcp` needs (ADR 0065). | `ops.Service`, `ops.Dispatcher`, `ops.Definitions()`. |
| `httpapi`, `mcp` | The two doors: routes and tools derived from `ops`, plus the HTTP API's reads and streams that aren't operations (`Wait`, transcripts, chat feed, usage, Factory posts). | HTTP routes and MCP tools; checked by the cross-door suite in `server/*_test.go`. |
| `server` | The service's one handler: MCP, HTTP API and the embedded UI behind the Host/Origin Guard, with `server.Calls` wrapping each door's dispatch to set the Actor and record Usage. | `server.Handler`. |
| `app` | The composition root. `app.Run` opens the Store, builds the Core with its Watcher check modes and Usage, registers the service and renews it, serves `server`'s handler, supervises the Watchers and starts `notify`. | `app.Run`. |
| `domain` | The records (`Ticket`, `State`, `Landing`, `CI`…), the enums and `LandingPolicy` (ADR 0064). Imports nothing of Code Central. | Plain types. |
| `store` | Persistence: file shapes, migrations, locking, the Store Kinds (Files, SQLite, Postgres; ADR 0067, 0068), Service Registrations and Watcher Leases, and the Store's Usage (ADR 0069). | `store.Store`, with the `storetest` contract suite. See [The Store](#the-store). |
| `client` | The Go HTTP client, its calls derived from `ops`. Used by the CLI, the Factory and the Watchers. | `client.Client`. |
| `cli`, `cmd/*` | The command surface, `init` scaffolding, service install, the Watchers' subcommands, and `serve`'s flags and local settings (it calls `app.Run`). | Commands; most built from `ops.All()`. |
| `factory`, `cmd/code-central-factory` | A separate process that runs Claims as Claude Code sessions (`*factory`) and lands work (`lander`), over a shared registry of repositories. | Talks to the service only through `client`. |
| `ciwatcher`, `pullrequestwatcher`, `issuewatcher`, `workitemwatcher` | One Watcher each: CI runs, pull requests, GitHub Issues (inbound, ADR 0047) and Azure Boards (ADR 0053). `ciwatcher` has a `Provider` and `pullrequestwatcher` a `Host`, each with a GitHub and an Azure implementation. | Talk to the service through `client`; to the code host through `gh` and `azdo`. |
| `watchers` | Generic supervision: restart with backoff, and running a Project's Watchers only while this service holds its Watcher Lease. | `watchers.Supervise` over a `Central` interface. |
| `gh`, `azdo`, `azdourl`, `codehost`, `ratelimit` | Reaching a code host: `gh.Ready` and `azdo.Access` check access, `codehost` and `azdourl` parse remote URLs and repository names, `ratelimit` holds `RateLimitError` and `FirstLine`. | Functions; leaves that import only `api` for the Watcher status contract (ADR 0072). |
| `notify` | OS notifications for what needs the human. | Reads through a four-method `Source` interface. |
| `clock`, `events`, `ids`, `home`, `usage` | Small injectable leaves: time, the in-memory change bus, IDs, XDG paths (ADR 0056), Usage records and the Recorder. | Small types. |
| `web/` | The React UI, embedded by `web/web.go`. All its reads, actions and the live feed go through one `Api` (`web/src/api/api.ts`); the production `fetch` is in `web/src/api/http.ts` alone. | `Api`; wire types generated from Go into `wire.gen.ts` (ADR 0042); `web/src/api/contract/` pins `createHttpApi` on the WASM Core and on a real service. |
| Test and wasm tooling | `world`, `cmd/corewasm`, `codecentraltest`, `store/storetest`, `factory/factorytest`, `watchers/watchertest`, `internal/*` (fakes, guards, wire-type generation, test runners). | |

There is no separate issue-tracker module: Specs and Tickets live in `core`, and the inbound trackers are `issuewatcher` and `workitemwatcher`.

### What `core` contains

`core` is about 18.1k non-test lines in 95 files, with a `*Core` of 150 exported methods and 6 exported types (the wire types went to `api`). By file grouping:

- workflow and claims (claim, round, failure, blockers, triage, requirement, map, prototype, retro, small change, auto-accept, auto-fix…)
- landing (landing policy, land per spec, spec merge, repositories, changed repositories, base, branch format)
- CI and code host (ci*, pull request, GitHub issues, Azure Boards, links, write-back, watcher status)
- agent runtime (chat, transcript, heartbeat, models, plan limits, usage, stage guides, Factory wake)
- read models (`*View` types, needs-me, attention)
- the builders of the wire contract's views (`*View`, `*Row` and `*Result` are in `api`)

## Dependency rules (as intended and as found)

Intended: `clients → client → HTTP → server → httpapi/mcp → ops → core → store → domain → leaves`; only `core` and the composition roots (`app`, `world`, `codecentraltest`) link persistence; the code-host packages are leaves.

Found:

- **No import cycles**, and `core` never imports a door, `ops`, `factory`, `gh` or `azdo`.
- **Direct imports hold, transitive ones don't.** `internal/importguard` keeps `client`, `httpapi`, `mcp`, `factory` and the Watchers from importing `store` directly, and they don't. But `client` imports `ops`, and the Factory, `issuewatcher`, `pullrequestwatcher` and `workitemwatcher` import `core` (for a few rules, no longer for the wire types), and `core` imports `store`, so `client`, those three Watchers and the Factory binary all link `store` and its 16 database driver packages. `ciwatcher`, `gh`, `azdo`, `ratelimit`, `watchers` and `notify` link neither `core` nor `store`. That the Factory and the Watchers never touch the Store is kept by discipline, not by the compiler.
- **The leaves are leaves.** `gh`, `azdo` and `ratelimit` import `api` (the Watcher status contract) and not `core`.
- **`api` is a leaf over `domain`.** It imports only `domain` and `internal/fieldwalk`, and `core`, `ops`, the doors and the clients import it.

| Package | Imports of Code Central (non-test) |
|---|---|
| `client` | api, domain, ops, internal/fieldwalk |
| `factory` | api, client, clock, core, domain, home, usage |
| `ciwatcher` | api, azdo, clock, domain, gh, ratelimit, watchers |
| `pullrequestwatcher` | api, azdo, clock, core, domain, gh, ratelimit, watchers |
| `issuewatcher` | api, clock, core, domain, gh, ratelimit, watchers |
| `workitemwatcher` | api, azdo, azdourl, clock, core, domain, gh, ratelimit, watchers |
| `usage` | api, clock |
| `api` | domain, internal/fieldwalk |
| `gh` | api, ratelimit |
| `azdo` | api, azdourl, domain, ratelimit |
| `ratelimit` | api |
| `watchers` | api, clock, events, ratelimit |
| `ops` | api, core, domain, events, internal/fieldwalk |
| `httpapi` | api, core, domain, ops, internal/fieldwalk |
| `mcp` | api, core, ops, internal/fieldwalk |
| `server` | api, core, httpapi, mcp, ops, usage |
| `core` | api, azdourl, clock, codehost, domain, events, ids, store, usage, internal/fieldwalk |
| `store` | domain, events, home, usage |
| `cli` | api, app, ciwatcher, client, clock, core, domain, gh, home, issuewatcher, notify, ops, pullrequestwatcher, store, workitemwatcher, internal/fieldwalk |

### The Store

`store.Store` (ADR 0067, 0068, 0069) is what `core` persists through, and every Store Kind (Files, SQLite, Postgres) implements it:

- **Queries, not a map.** `View` hands a `Reader` whose reads say what they want: `Ticket`, `FindTickets` with a `TicketFilter`, `FindSpecs`, `StateCounts`, `SpecCount`, `ClaimBySession`, `TicketCommits`, `NextNumber`. A database Store answers them in SQL over its document rows, a File Store from memory. `Records` reads a Project whole and is for Export and Store Move alone.
- **History in parts.** A Ticket is a head plus its History, in the parts `store.History` lists: claims, rounds, comments, moves, edits and failure reports (ADR 0015). A read loads only the `Part`s it names, and `PutTicket` writes only the parts it names, so a Claim doesn't rewrite the Ticket's comments.
- **Transactions.** `Update` takes a `Writer` and writes at most one record (a second write is `ErrSecondWrite`; `PutImport` is the one bulk write). A File or SQLite Store runs it under one store-wide mutex. A Postgres Store runs each attempt in a `SERIALIZABLE` transaction (and a `View` in a `REPEATABLE READ READ ONLY` one), tries a collision again up to five times, then fails with `ErrConflict`, so an Update's function must be safe to run more than once. `Announce` sends the Update's `Change` to the other services only when it commits.
- **Services and the Lease.** `Services()` holds each running service's Registration and each Project's Watcher Lease, outside any View or Update. `HoldLease` renews or takes a Lease in one statement, and `watchers` runs a Project's Watchers only while this service holds it (ADR 0068). `Changes` delivers other services' Changes, `Usage()` is the Store's Usage (ADR 0069), and `Location()` and `Size()` feed the Storage tab.

## Contracts that are well defined

- **The doors.** `httpapi` and `mcp` depend on `ops.Service` and `ops.Dispatcher`, not on `*core.Core`, each with a test against a recording fake (ADR 0065). `server.Calls` is the one place the Actor is set and Usage recorded; `server.Guard` checks only Host and Origin.
- **The operations.** Each operation is defined once in `ops` (ADR 0063); `httpapi` routes, `mcp` tools and schemas, `client` calls and most `cli` commands derive from it, and the cross-door suite checks each door against `ops.All()`.
- **The Store.** Small, with a shared contract suite (`storetest`) run on the fake, Files, SQLite and, in CI, Postgres.
- **The domain model.** `domain` imports nothing of Code Central, and the wire views (`TicketView`…) are their own types rather than the records, so the Store's shapes don't leak onto the wire.
- **The UI.** Wire types are generated from Go (ADR 0042) and `TestTheCommittedWireTypesAreCurrent` fails when they drift. The views carry the derived rules (`editable`, `retroable`, Spec kind, the Stage strip, Landing capabilities), so the UI only formats them (ADR 0063).
- **Watchers' code hosts.** `ciwatcher.Provider` and `pullrequestwatcher.Host` each have a GitHub and an Azure implementation chosen once, and access checks are shared through `gh.Ready` and `azdo.Access`.
- **`notify` and `watchers`** depend on small interfaces (`Source`, `Central`), not on `*Core`.

## Where the contracts are weak

Ordered by how much they cost. Each recommendation would be its own Ticket.

### 1. The wire contract lives in `core` (fixed)

About 200 of `core`'s 318 exported types were the wire contract: inputs, views, rows, results, `Ref`, `Actor`, the error codes and `Decode`, and every client imported `core` for them. They are now in `api` (Spec code-central-1111, ADR 0072), which imports only `domain` and `internal/fieldwalk`. `core` names them as `api.X`, with no alias left, and `internal/wiretypes` generates `wire.gen.ts` from `api` alone. A change to the wire is now a change to `api`, and one to an internal type in `core` is not.

What is left of the problem is that nothing yet *stops* a client linking `core`: `client` imports `ops` (weakness 4), and the Factory and three Watchers call `core` rules (weakness 3), so each still links the whole Core, the Store and the database drivers. The next slice of the Spec extends `internal/importguard` so that no client imports `core` or `store`, checked on the transitive imports (`go list -deps`) rather than the direct ones.

### 2. The code-host leaves import `core` (fixed)

`gh`, `azdo`, `ratelimit` and `watchers` used to import `core` for the Watcher status contract. It is now in `api` (ADR 0072), and none of the four links `core` or `store`. The rest of the wire contract has moved there too (weakness 1).

### 3. Clients run Core logic, not just Core types (partly fixed)

The shared rules now live where a client can use them without `core`: `domain` has the Spec Branch name (`SpecBranch`, `BranchTag`), the issue markers (`IssueMarker`, `OpaqueMarker`), the status label prefix and the reserved labels and tags; `codehost` has `NormalCloneURL`; `api` has `SumRepositoryProblems` and `LandingPolicies` (with `ProjectView.LandingPolicies()`). The Factory's Stage names are `domain.Stages()`.

What is left is a few `core` names the Factory, the Watchers and the CLI still call, so they still link `core` and `store`:

- `factory`: `core.GuideName`, `core.ImplementationStage`, `core.StageLabel`, `core.StageOf`, `core.CantLandComment`, `core.SilentAfter`, `core.DefaultChatModel`, `core.WaitTimeout`.
- `issuewatcher`, `workitemwatcher`: `core.IntegrationGitHub`, `core.IntegrationAzureBoards`, `core.CommentContext`, `core.CommentWontFix`.
- `cli`: `core.TeachingFiles()`, `core.DefaultTicketParts()`.

Provider knowledge is left in `core` too: `core/ci.go`'s `withoutLogin` duplicates `azdourl`'s, and `core/cirepositories.go`'s `ciEntryFromCloneURL` builds an Azure DevOps project URL by hand, which `azdourl` could do.

**Recommendation:** move each remaining shared rule to `domain` (names, labels, Stages) or `codehost` (URLs). Where the rule is only the service's (Stage guide names), have the service send the answer in a view rather than the client compute it.

### 4. `ops` binds every operation to the concrete `*core.Core`

Each operation's `Call` is a `func(c *core.Core, …)` (124 references across `ops`), and `client` imports `ops` to derive its calls, so it gets the bindings as well as the definitions. The doors see only `ops.Service`, so they are safe from a split of `*Core`, but `ops` isn't: the split rewrites every operation's `Call`.

**Recommendation:** split `ops` into the definitions (name, audience, route, input and output types; what `client`, `cli`, `mcp` and the UI generator need) and the bindings (`Call`, `Serve`; what only the service needs). The bindings can then call per-area services as `core` is split (weakness 6) without touching the definitions.

### 5. The UI's HTTP client is written by hand

`web/src/api/http.ts` writes every route, method and request body by hand (`call("POST", \`${p(project)}/set_azure_boards\`, { on, mode, … })`). The types are generated, but not which operation lives at which path or which fields it takes, so a renamed route or a new input field is caught only by the cases in `web/src/api/contract/`, and only for operations those cases cover. The Go client derives the same thing from `ops`.

**Recommendation:** generate an operation table (name, method, path, input type) from `ops.Definitions()` into `wire.gen.ts` beside the types, and have `http.ts` build its calls from it, as `client` does. The `Api` interface stays hand-written: it is the UI's own vocabulary.

### 6. `*core.Core` is a god object

One type with 150 methods and 20 fields, mixing the persisted workflow with in-memory runtime (`heartbeats`, `transcripts`, `chats`, `activity`, `factoryWakes`, `watchers`, `latestNotices`), the Watcher check modes and Usage. Every file is a method on `*Core`, so nothing inside `core` has a boundary, and any rule can reach any other's state.

**Recommendation:** once weaknesses 1 and 4 are done, split along the file groupings above, starting where the seam is already clean: the agent runtime (heartbeats, transcripts, chat, plan limits, Factory wake) holds its own state and barely touches the Store, and the CI and code-host area talks to the rest through Tickets and Watcher status. Each becomes a type with its own narrow interface that the bindings in `ops` call, with `*Core` shrinking to the workflow.

### 7. Runtime state is per service, but the Store can be shared

Heartbeats, Transcripts, chats and Watcher status live in one service's memory ("in memory only, never in the store", `core/heartbeat.go`; ADR 0008). Since ADR 0068, several services can share a Postgres Store and hear each other's Changes, but not each other's runtime state: a Factory reporting to one service is Offline in the UI of another, and its Transcript is blank there. Nothing in the docs says whether that is intended.

**Recommendation:** decide it and record the decision in an ADR, either "a Factory and the UI that watches it use the same service" (and say so where a shared Store is set up), or move the state that must be seen everywhere (at least Heartbeats) into the Store's `Services()` side, outside any View or Update, as Registrations and Leases already are.

### 8. `cli` is still wide

`cli` imports 15 Code Central packages, `store` and all four Watchers among them: `serve`'s Store location (`cli/serve.go`), the Watchers' subcommands and Store Move live beside the commands that are only HTTP clients. That is why `factory/factorytest` and `watchers/watchertest`, which re-run `cli.Run` as a fake binary, link everything too.

**Recommendation:** move the Watchers' subcommands and Store location into `app` (or a `cmd/code-central` wiring file), leaving `cli` as commands over `client` and `ops`.
