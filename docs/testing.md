# Testing and CI

How the project tests, what each tier catches, and where a new test goes. The Makefile and `.github/workflows/ci.yml` are the source of truth for commands; this says why they are shaped as they are.

## The tiers

| Tier | Runs | Catches |
| --- | --- | --- |
| `make test` | `go test ./...`, gofmt, vitest, tsc, ESLint and the hooks' test, in parallel (`internal/testrun`), against a 60s wall-clock budget | Logic, type and style errors, and the guard tests. Run it once at the end of a change. |
| `make e2e` | Builds the binaries, then the Go end-to-end tests (`e2e/`, tag `e2e`) and the UI's Playwright tests (`web/src/e2e`) | Whole-system behaviour: real binaries, a real store in a temporary folder, never port 7733. |

`make test` runs Go against the committed placeholder page, so it needs no Vite build. Installing web dependencies and building `core.wasm` happen before the clock starts. A failing job's output is kept in a log file whose path prints last; failed vitest files and Go tests killed under load rerun once serially, and a pass there prints "passed on serial rerun: flaky under load".

CI does not call `make test`: its 60s budget would trip on a cold runner, and each step wants its own log. It runs the same steps directly.

## CI's jobs

| Job | Runs on | What |
| --- | --- | --- |
| `web` | Linux | vitest, tsc, ESLint, the Vite build, the Storybook build |
| `test (<os>)` | Linux, macOS, Windows | gofmt, `go test -v ./...`, the Vite build, the binaries, the Go e2e tests, the UI e2e tests |
| `test (postgres)` | Linux | `go test -v ./store/... ./cli/... ./core/... ./app/...` against a Postgres 17 service container |
| `conventional commits` | Linux | `cog check` of every commit since the latest tag, to catch a message that bypassed the commit-msg hook |

There is no scale or performance tier yet.

### Why each OS runs what it runs

- **Web checks run once, on Linux.** vitest runs in jsdom, and tsc and ESLint are pure analysis: none depends on the OS, so three copies add time and no signal.
- **Go tests and both e2e suites run on every OS.** Process handling, file locking, flushing and path rules differ (`store/lock_*.go`, `flush_*.go`, `e2e/process_*.go`), and only a real run finds a difference. Every step runs under bash so commands are identical and a red run on one OS is a real platform difference.
- **Postgres runs only on Linux**, the one runner that gives a service container. The other jobs skip those cases, and `-v` lists the skip with its reason.

### The Windows setup

Go tests that start git and write many small files ran 10-15x slower on Windows than on Linux. The Windows job therefore:

- excludes the workspace, TEMP and the Go caches from Defender's on-access scanning;
- puts TEMP, GOTMPDIR and the Go caches on a ReFS Dev Drive (`samypr100/setup-dev-drive`), which skips NTFS filter overhead; the drive is created before `setup-go` so its cache restores onto it;
- raises the Go e2e startup budget to 1m (`CODE_CENTRAL_E2E_STARTUP_BUDGET`), because migrating a 10,000-Ticket store takes ~25s there.

The Defender step is an experiment (code-central-1141): check the timings before treating it as settled.

## Contract suites

`store/storetest.Run` is the store's contract suite. It takes a `Factory` that opens a store of one kind, and each implementation runs it from its own test: memory, file, SQLite and Postgres (`store/*_test.go`). Optional factory hooks (`Reopen`, `BreakUsage`, `Second`) let a kind opt into the cases that need them; a kind without a hook skips that case.

Postgres runs only when `CODE_CENTRAL_TEST_POSTGRES_URL` is set (`storetest.NeedsPostgres`); otherwise it skips and names the variable. CI's `test (postgres)` job sets it.

This is the pattern for testing a port against more than one implementation: write the behaviour once as a suite over the interface, and give each implementation a thin test that supplies a factory. A new implementation passes the suite or is not an implementation of the port.

## The guard tests

Each fails when a rule about the code's shape is broken, rather than testing behaviour.

- **Landing guard** (`core/landingguard_test.go`, ADR 0039): no code in domain, api, core or the Factory reads a landing setting to decide, outside the policy and the code that saves and shows the settings.
- **Import guard** (`internal/importguard`): the clients of the domain model (httpapi, mcp, server, cli and others) do not import `store`; they name records through `domain`.
- **Doc guard** (`internal/docguard`): `docs/agents/` names no Go identifier that no longer exists.

The landing guard type-checks the packages once and shares the result across its tests, so adding a guard test to the package costs an assertion, not another type-check. Follow that when adding one.

## Where the time goes

Find a hotspot before changing a test.

- **One package's tests:** `go test -v ./<pkg>` prints each test's time. `go test -json` gives the same for sorting.
- **A CI run:** `make ci-failures RUN=<id>` (id from `gh run list`) shows what failed. For timings, `gh run view <id> --log` of a `go test -v` step lists `ok <pkg> <seconds>` per package and `--- PASS: <test> (<seconds>)` per test; sort those by seconds to rank packages and tests.
- **A flaky web test:** `make vitest-stress F=<file>` runs it N times under CPU load (`N=`, `LOAD=`); `WITH="go test ./factory"` adds the Go suite's load.
- **The fast suite as a whole:** `make test-verbose` prints each job's output, and `make test` names the budget overrun.

Known costs: tests that start git or write many files (Windows especially), the e2e suites' binary builds, and the Vite and Storybook builds.

## Adding a test

1. **Pick the tier.** A rule about one package's logic goes in that package's Go test or a vitest file, and runs in `make test`. Behaviour that needs the real binaries, a real process or a browser goes in `e2e/` or `web/src/e2e`. A rule about the code's shape is a guard test.
2. **Call `t.Parallel()`** unless the test changes process-wide state (environment, working directory, a fixed port).
3. **Share expensive fixtures per package.** `factorytest.Main` seeds one repository once for the package's tests; copy that shape rather than building the fixture in each test.
4. **Keep a scale test out of the per-push suite.** It belongs in a tier of its own once one exists; until then, flag it in the ticket instead of adding it to `make test`.
5. **Test a port through its contract suite** when it has more than one implementation.
6. **Keep the budget.** `make test` has 60s; a test that adds noticeable time needs a reason.
