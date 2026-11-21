# Testing and CI

How the project tests, what each tier catches, and where a new test goes. The Makefile and `.github/workflows/ci.yml` are the source of truth for commands; this says why they are shaped as they are.

## The tiers

| Tier | Runs | Catches |
| --- | --- | --- |
| `make test` | `go test ./...`, gofmt, vitest, tsc, ESLint and the hooks' test, in parallel (`internal/testrun`), against a 60s wall-clock budget | Logic, type and style errors, and the guard tests. Run it once at the end of a change. |
| `make e2e` | Builds the binaries, then the Go end-to-end tests (`e2e/`, tag `e2e`) and the UI's Playwright tests (`web/src/e2e`) | Whole-system behaviour: real binaries, a real store in a temporary folder, never port 7733. |
| `make scale` | Builds what `make e2e` builds, then the **Scale tests** (`TestScale…`, tag `scale`): `go test -tags scale -run '^TestScale' ./...`, then the same with tags `e2e scale` over `e2e/` | Slowdowns only a large store shows, such as loading and serving 5,000 or 10,000 Tickets. Nightly in CI (`scale.yml`); run it by hand when you change what loads or serves a large store. |

The **Scale tier** is the tests too big for the per-push suite. `make test` and `make e2e` leave them out, because they build without the `scale` tag.

`make test` runs Go against the committed placeholder page, so it needs no Vite build. Installing web dependencies and building `core.wasm` happen before the clock starts. A failing job's output is kept in a log file whose path prints last; failed vitest files and Go tests killed under load rerun once serially, and a pass there prints "passed on serial rerun: flaky under load".

CI does not call `make test`: its 60s budget would trip on a cold runner, and each step wants its own log. It runs the same steps directly.

## CI's jobs

A **Shard** is one job's share of the Go tests. `internal/testshard` lists each package's tests, puts each in Shard `FNV-1a(import path, 0, name) mod N`, and runs that Shard's tests with `go test -v`. It keeps no list of tests or timings, so a new test or package lands in some Shard by itself. The Shard counts are matrix values in `ci.yml`, so they can be retuned from measured times without touching the tool; with one Shard it runs every test. To see what a Shard would run: `go run ./internal/testshard -shard 2 -of 4 -dry-run`.

| Job | Runs on | What |
| --- | --- | --- |
| `test (<os>, shard <i>/<n>)` | Linux (1 Shard), macOS (2), Windows (4) | One Shard of `go test -v`, untagged and against the committed placeholder page, so it needs no Node and no Vite build |
| `e2e (<os>)` | Linux, macOS, Windows | `core.wasm`, the Vite build, the binaries, the Go e2e tests, the UI e2e tests. It has 30 minutes, since a cold Windows runner installs and builds everything first. |
| `web` | Linux | vitest, tsc, ESLint, the Vite build, the Storybook build |
| `gofmt` | Linux | `gofmt -l .`, failing on any output |
| `test (postgres)` | Linux | `go test -v ./store/... ./cli/... ./core/... ./app/...` against a Postgres 17 service container |
| `conventional commits` | Linux | `cog check` of every commit since the latest tag, to catch a message that bypassed the commit-msg hook |

`scale.yml` is a separate workflow, nightly (and on `workflow_dispatch`), with one `scale (<os>)` job on Linux, macOS and Windows. It builds what the `e2e` job builds, then runs the steps `make scale` runs, with a 60-minute timeout. It is not part of `ci.yml`, so no pull request waits on it.

Two Project settings, which the human sets in Code Central and which live nowhere in the repository, go with it:

- **`scale.yml` in Ignored workflows** (CI Tracking, see [Watching CI](setting-up-a-project.md#watching-ci)), so a nightly run never counts towards a Ticket's CI result.
- **A CI Rule that opens a Ticket when `scale.yml` fails on `main`**, so a nightly failure is picked up like any other.

### Why each OS runs what it runs

- **Web checks and gofmt run once, on Linux.** vitest runs in jsdom, and tsc, ESLint and gofmt are pure analysis: none depends on the OS, so three copies add time and no signal.
- **Go tests and both e2e suites run on every OS.** Process handling, file locking, flushing and path rules differ (`store/lock_*.go`, `flush_*.go`, `e2e/process_*.go`), and only a real run finds a difference. Every step runs under bash so commands are identical and a red run on one OS is a real platform difference.
- **Postgres runs only on Linux**, the one runner that gives a service container. The other jobs skip those cases, and `-v` lists the skip with its reason.

### The Windows setup

Go tests that start git and write many small files ran 10-15x slower on Windows than on Linux. Every Windows job (the Shards, `e2e` and `scale`) therefore calls one local action, `.github/actions/windows-setup`, after checkout and before `setup-go`; its steps do nothing off Windows. It:

- excludes the workspace, TEMP and the Go caches from Defender's on-access scanning;
- puts TEMP, GOTMPDIR and the Go caches on a ReFS Dev Drive (`samypr100/setup-dev-drive`), which skips NTFS filter overhead; the drive is created before `setup-go` so its cache restores onto it;
- turns off CRLF conversion, so the git the tests start writes the bytes it is given.

The Go e2e startup budget is not in the action: `scale.yml` raises it to 1m on Windows (`CODE_CENTRAL_E2E_STARTUP_BUDGET`), because migrating a 10,000-Ticket store takes ~25s there. That test is a Scale test, so `ci.yml` has no override.

The Defender step is an experiment (code-central-1141): check the timings before treating it as settled.

## Contract suites

`store/storetest.Run` is the store's contract suite. It takes a `Factory` that opens a store of one kind, and each implementation runs it from its own test: memory, file, SQLite and Postgres (`store/*_test.go`). Optional factory hooks (`Reopen`, `BreakUsage`, `Second`) let a kind opt into the cases that need them; a kind without a hook skips that case.

Postgres runs only when `CODE_CENTRAL_TEST_POSTGRES_URL` is set (`storetest.NeedsPostgres`); otherwise it skips and names the variable. CI's `test (postgres)` job sets it.

`factory/git/gittest.Run` is the Git port's contract suite, the same shape. Its `Setup` gives it a Git of one kind, and two thin tests supply it: real git (`factory/git/real_test.go`) and the Fake git (`factory/git/fake_test.go`). See [The Git port and the Fake git](#the-git-port-and-the-fake-git).

This is the pattern for testing a port against more than one implementation: write the behaviour once as a suite over the interface, and give each implementation a thin test that supplies a factory. A new implementation passes the suite or is not an implementation of the port.

## The Git port and the Fake git

Four terms, defined here and not in `GLOSSARY.md` (ADR 0073):

- **Git port**: `factory/git`, the one interface through which the Factory runs git. The Factory's own files run no git (see the Git guard).
- **Fake git**: `git.Fake`, an in-memory implementation of the Git port that starts no process. A Factory test runs on it by default, which is what keeps the Factory's package fast, and it is the faster of the two ways to run the Factory's scenarios.
- **Git contract suite**: `gittest.Run`, the behaviour of the Git port written once over the interface. Real git and the Fake git each pass it on every push, so the Fake is held to real git, and a behaviour the Fake gets wrong fails the suite rather than a scenario test.
- **Real-git tests**: the Factory tests that name `factorytest.RealGit()`. They run real git on every push, for what the Fake git doesn't model: raw git (`Git`, `GitIn`, `Remote.Git`), revision syntax such as `HEAD` or `main^`, and a fake claude's Git Script steps beyond the Fake's.

`factorytest.New(t)` runs the harness, and its repositories, on the Fake git. A test reads and changes them only through the typed helpers (`Tip`, `Log`, `CommitOn`, `Publish` and the rest, listed in `factorytest/git.go`), which serve both implementations; raw git fails the test with a message naming them. A test that needs raw git passes `factorytest.RealGit()`. `factorytest.FakeGit()` is the default and only overrides an earlier `RealGit()`; the last option wins.

`CODE_CENTRAL_TEST_FACTORY_GIT=real` runs every harness on real git, `RealGit()` or not:

```
CODE_CENTRAL_TEST_FACTORY_GIT=real go test ./factory/...
```

Run it after changing the Fake git or a typed helper, to check the Fake against real git. The nightly `scale.yml` runs this as its `Factory on real git` step on Linux, macOS and Windows, the backstop for the contract suite. The variable takes no other value: anything else fails the test.

A new Fake behaviour goes into `gittest.Run` first, then into the Fake until real git and the Fake both pass it.

## The guard tests

Each fails when a rule about the code's shape is broken, rather than testing behaviour.

- **Landing guard** (`core/landingguard_test.go`, ADR 0039): no code in domain, api, core or the Factory reads a landing setting to decide, outside the policy and the code that saves and shows the settings.
- **Git guard** (`factory/gitguard_test.go`, Spec code-central-1148): the Factory's own files run no git; every git call goes through the Git port, `factory/git`. It allows only the processes it lists (claude and ps), and fails on `exec.Command` taken as a value and on `os.StartProcess`.
- **Import guard** (`internal/importguard`): the clients of the domain model (httpapi, mcp, server, cli and others) do not import `store`; they name records through `domain`.
- **Doc guard** (`internal/docguard`): `docs/agents/` names no Go identifier that no longer exists.

The landing guard type-checks the packages once and shares the result across its tests, so adding a guard test to the package costs an assertion, not another type-check. Follow that when adding one.

## Where the time goes

Find a hotspot before changing a test.

- **One package's tests:** `go test -v ./<pkg>` prints each test's time. `go test -json` gives the same for sorting.
- **A CI run:** `make ci-failures RUN=<id>` (id from `gh run list`) shows what failed, each failing job and step under a header such as `== test (windows-latest, shard 2/4) / go test`. For timings, `gh run view <id> --log` of a Shard's `go test` step lists `ok <pkg> <seconds>` per package and `--- PASS: <test> (<seconds>)` per test; sort those by seconds to rank tests. A package's timings are spread over the Shard jobs of an OS, since its tests hash to different Shards, so add them up across those jobs to rank packages.
- **A flaky web test:** `make vitest-stress F=<file>` runs it N times under CPU load (`N=`, `LOAD=`); `WITH="go test ./factory"` adds the Go suite's load.
- **The fast suite as a whole:** `make test-verbose` prints each job's output, and `make test` names the budget overrun.

Known costs: tests that start git or write many files (Windows especially), the e2e suites' binary builds, and the Vite and Storybook builds.

## Adding a test

1. **Pick the tier.** A rule about one package's logic goes in that package's Go test or a vitest file, and runs in `make test`. Behaviour that needs the real binaries, a real process or a browser goes in `e2e/` or `web/src/e2e`. A rule about the code's shape is a guard test.
2. **Call `t.Parallel()`** unless the test changes process-wide state (environment, working directory, a fixed port).
3. **Share expensive fixtures per package.** `factorytest.Main` seeds one repository once for the package's tests; copy that shape rather than building the fixture in each test.
4. **Keep a scale test out of the per-push suite.** A test is a scale test when its cost grows with the size of the data (thousands of Tickets, say) and it would add noticeable time to `make test`'s 60s. Mark it four ways, which `internal/scaleguard` checks: a `//go:build scale` constraint (`e2e && scale` for an end-to-end one), a file named `*_scale_test.go`, a name starting `TestScale`, and a doc comment saying what it guards and its budget. Run it with `make scale`; CI runs it nightly in `scale.yml`.
5. **Test a port through its contract suite** when it has more than one implementation.
6. **Keep the budget.** `make test` has 60s; a test that adds noticeable time needs a reason.
