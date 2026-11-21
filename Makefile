VERSION := $(or $(shell git describe --tags --always --dirty),dev)
LDFLAGS := -X github.com/tomoram/code-central/cli.Version=$(VERSION)
# The fast suite's wall-clock budget, in seconds.
TEST_BUDGET := 60
# npm ci writes this; it is newer than the lockfile while node_modules is current.
WEB_DEPS := web/node_modules/.package-lock.json

.PHONY: test test-verbose typecheck lint vitest vitest-related vitest-stress e2e e2e-run scale build web clean hooks context-report ci-failures serve-components build-components corewasm wiretypes demo-store demo

# Every recipe is one command that runs the same under cmd.exe and POSIX
# sh, so a Windows developer needs only GNU make. Binaries get .exe there.
ifeq ($(OS),Windows_NT)
EXE := .exe
endif

# Runs the fast suite and fails if it takes more than $(TEST_BUDGET)
# seconds of wall-clock time: internal/testrun runs `go test ./...`
# (against the committed placeholder page, so no Vite build), `vitest run`
# the TypeScript type check and ESLint in parallel. Installing web dependencies
# (first run, or after the lockfile changes) and building core.wasm
# happen before the clock starts. It prints one line per passing job and
# the output of a failing one; test-verbose prints every job's whole
# output.
#
# Heavy suites (make test, make e2e, make vitest without F=) take a
# machine-wide lock first (internal/heavylock), so sessions that overlap
# run them one at a time. The budget's clock starts once the lock is held.
test: $(WEB_DEPS) hooks corewasm
	@go run ./internal/heavylock -name "make test" -- go run ./internal/testrun -budget $(TEST_BUDGET)s

test-verbose: $(WEB_DEPS) hooks corewasm
	@go run ./internal/heavylock -name "make test-verbose" -- go run ./internal/testrun -budget $(TEST_BUDGET)s -v

# The inner loop while working on the UI: the TypeScript type check, and
# vitest on one file (F=src/...) or on all of them. Both install web
# dependencies first, so they work in a fresh worktree. The tools run
# through node rather than node_modules/.bin, whose shims differ by OS.
typecheck: $(WEB_DEPS) hooks
	cd web && node node_modules/typescript/bin/tsc --noEmit

# ESLint on one file (F=src/...) or on all of web/. It installs web
# dependencies first, and needs no core.wasm.
lint: $(WEB_DEPS) hooks
	cd web && node node_modules/eslint/bin/eslint.js $(or $(F:web/%=%),.)

vitest: $(WEB_DEPS) hooks corewasm
ifeq ($(F),)
	@go run ./internal/heavylock -name "make vitest" -dir web -- node node_modules/vitest/vitest.mjs run
else
	cd web && node node_modules/vitest/vitest.mjs run $(F:web/%=%)
endif

# vitest on every test that imports the given source files, through any
# chain (F=src/a.tsx src/b.ts), or with no F= on the web files changed
# against main, committed and uncommitted. An inner-loop step before
# make test, which still runs every file.
vitest-related: $(WEB_DEPS) hooks corewasm
	cd web && node node_modules/vitest/vitest.mjs $(if $(F),related --run $(F:web/%=%),run --changed $(shell git merge-base HEAD main)) --passWithNoTests

# Shows a flaky web test: vitest on one file (F=) N times (default 20)
# while LOAD busy workers (default 4, 0 for none) take the CPU, the way a
# loaded CI runner does. It prints a line per run and "N runs, F failures",
# and fails if any run failed. Go, so the workers stop however it exits.
# WITH="go test ./factory" also runs that command over and over, the Go
# suite load of make test.
vitest-stress: $(WEB_DEPS) hooks corewasm
	go run ./internal/vitestress -n $(or $(N),20) -load $(or $(LOAD),4) $(if $(WITH),-with "$(WITH)") $(F)

# Turns on the committed commit-msg hook in .githooks/, which refuses a
# message that isn't a conventional commit (see CLAUDE.md's Commits). The
# setting is in the clone's shared config, so every worktree gets it; the
# path is relative, so each runs its own checked-out copy.
hooks:
	@git config core.hooksPath .githooks

# The WASM Core the UI's tests and stories run on: cmd/corewasm built to
# js/wasm, with the same Go's wasm_exec.js, both into web/src/api/core/
# (never committed, never in web/dist). Go's build cache makes a rebuild
# with no change cheap. A Go helper, so it runs the same under cmd.exe.
corewasm:
	@go run ./internal/corewasm web/src/api/core

# The UI's wire types: web/src/api/wire.gen.ts, generated from the
# Core's Go view types (ADR 0042) and committed. Run it after changing a
# view the UI reads.
wiretypes:
	@go run ./internal/wiretypes web/src/api/wire.gen.ts

$(WEB_DEPS): web/package.json web/package-lock.json
	$(if $(shell npm --version),,$(error npm not found; the UI needs Node (go test ./... alone runs without it)))
	npm --prefix web ci

# Builds the UI with Vite into web/dist (never committed).
web: $(WEB_DEPS)
	npm --prefix web run build

# Builds the binary with the built UI embedded (-tags webdist), and the
# local Factory, code-central-factory. In development, run
# `code-central serve` and `npm --prefix web run dev`: Vite serves the UI
# on :5173 and proxies /api to the service.
build: web hooks
	go build -tags webdist -ldflags "$(LDFLAGS)" -o bin/code-central$(EXE) ./cmd/code-central
	go build -o bin/code-central-factory$(EXE) ./cmd/code-central-factory

# Builds the binaries, then runs the end-to-end tier against them (real
# binaries, real store in a temporary folder, never port 7733): the Go
# end-to-end tests and the real half of the UI's API contract suite. The
# binaries' paths reach both through make's environment, not the shell.
# The UI's service finds the fake gh, built here, on its PATH. The UI's
# e2e config shares the test setup, so it needs core.wasm too. With
# F=web/src/e2e/<file> only that web e2e file runs (web's dependencies are
# installed first); the Go end-to-end tests are skipped.
e2e:
	@go run ./internal/heavylock -name "make e2e" -- $(MAKE) --no-print-directory e2e-run $(if $(F),F=$(F))

e2e-run: export CODE_CENTRAL_BIN := $(CURDIR)/bin/code-central$(EXE)
e2e-run: export CODE_CENTRAL_FACTORY_BIN := $(CURDIR)/bin/code-central-factory$(EXE)
e2e-run: export CODE_CENTRAL_FAKE_GH_BIN := $(CURDIR)/bin/fake-gh$(EXE)
e2e-run: build corewasm $(if $(F),$(WEB_DEPS))
	go build -o bin/fake-gh$(EXE) ./internal/fakegh/gh
ifeq ($(F),)
	go test -tags e2e -count=1 ./e2e/
	cd web && npm run test:e2e
else
	cd web && npm run test:e2e -- $(F:web/%=%)
endif

# The Scale tier: the tests named TestScale…, which `make test` and
# `make e2e` leave out for their size. Builds what e2e builds, then runs
# the core one and the end-to-end one against the binaries.
scale: export CODE_CENTRAL_BIN := $(CURDIR)/bin/code-central$(EXE)
scale: export CODE_CENTRAL_FACTORY_BIN := $(CURDIR)/bin/code-central-factory$(EXE)
scale: export CODE_CENTRAL_FAKE_GH_BIN := $(CURDIR)/bin/fake-gh$(EXE)
scale: build corewasm
	go build -o bin/fake-gh$(EXE) ./internal/fakegh/gh
	go test -tags scale -count=1 -run "^TestScale" ./...
	go test -tags "e2e scale" -count=1 -run "^TestScale" ./e2e/

# A scratch service on PORT (default 7801, never 7733) filled with a demo
# Project, to look at the UI against; internal/demostore explains it.
# Stays in the foreground, so start it with run_in_background.
# docs/agents/running-the-ui.md says how to use it.
demo-store demo: build
	go run ./internal/demostore -bin bin/code-central$(EXE) -port $(or $(PORT),7801) $(ARGS)

clean:
	go run ./internal/clean bin web/dist web/src/api/core/core.wasm web/src/api/core/wasm_exec.js

# Prints the Context Report from Claude Code's session logs of this
# repository; flags go in ARGS, for example
# ARGS="-baseline internal/contextreport/baseline.json -since <time>".
# docs/context-report.md explains it.
context-report:
	go run ./internal/contextreport $(ARGS)

# What failed in a CI run (RUN=<id>, from `gh run list`): gh's failed
# steps' log without the setup, the passing tests or the timestamps,
# capped at 120 lines.
ci-failures:
	gh run view $(RUN) --log-failed 2>&1 | go run ./internal/cifailures

# The UI's component browser, a dev-only Storybook (web/.storybook/).
# Storybook is a devDependency of web/ and nothing in the embedded dist or in
# `make test` uses it. serve-components serves it on http://localhost:6006 with
# hot reload; build-components builds it statically into web/storybook-static,
# which checks it builds.
serve-components: $(WEB_DEPS) corewasm
	cd web && npm exec -- storybook dev -p 6006 --no-open

build-components: $(WEB_DEPS) corewasm
	cd web && npm exec -- storybook build
