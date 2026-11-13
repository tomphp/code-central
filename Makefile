VERSION := $(or $(shell git describe --tags --always --dirty),dev)
LDFLAGS := -X github.com/tomoram/code-central/cli.Version=$(VERSION)
# The fast suite's wall-clock budget, in seconds.
TEST_BUDGET := 10
# npm ci writes this; it is newer than the lockfile while node_modules is current.
WEB_DEPS := web/node_modules/.package-lock.json

.PHONY: test test-verbose typecheck vitest vitest-stress e2e build web clean hooks context-report ci-failures serve-components build-components

# Every recipe is one command that runs the same under cmd.exe and POSIX
# sh, so a Windows developer needs only GNU make. Binaries get .exe there.
ifeq ($(OS),Windows_NT)
EXE := .exe
endif

# Runs the fast suite and fails if it takes more than $(TEST_BUDGET)
# seconds of wall-clock time: internal/testrun runs `go test ./...`
# (against the committed placeholder page, so no Vite build), `vitest run`
# and the TypeScript type check in parallel. Installing web dependencies
# (first run, or after the lockfile changes) happens before the clock
# starts. It prints one line per passing job and the output of a failing
# one; test-verbose prints every job's whole output.
test: $(WEB_DEPS) hooks
	@go run ./internal/testrun -budget $(TEST_BUDGET)s

test-verbose: $(WEB_DEPS) hooks
	@go run ./internal/testrun -budget $(TEST_BUDGET)s -v

# The inner loop while working on the UI: the TypeScript type check, and
# vitest on one file (F=src/...) or on all of them. Both install web
# dependencies first, so they work in a fresh worktree. The tools run
# through node rather than node_modules/.bin, whose shims differ by OS.
typecheck: $(WEB_DEPS) hooks
	cd web && node node_modules/typescript/bin/tsc --noEmit

vitest: $(WEB_DEPS) hooks
	cd web && node node_modules/vitest/vitest.mjs run $(F:web/%=%)

# Shows a flaky web test: vitest on one file (F=) N times (default 20)
# while LOAD busy workers (default 4, 0 for none) take the CPU, the way a
# loaded CI runner does. It prints a line per run and "N runs, F failures",
# and fails if any run failed. Go, so the workers stop however it exits.
vitest-stress: $(WEB_DEPS) hooks
	go run ./internal/vitestress -n $(or $(N),20) -load $(or $(LOAD),4) $(F)

# Turns on the committed commit-msg hook in .githooks/, which refuses a
# message that isn't a conventional commit (see CLAUDE.md's Commits). The
# setting is in the clone's shared config, so every worktree gets it; the
# path is relative, so each runs its own checked-out copy.
hooks:
	@git config core.hooksPath .githooks

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
# The UI's service finds the fake gh, built here, on its PATH.
e2e: export CODE_CENTRAL_BIN := $(CURDIR)/bin/code-central$(EXE)
e2e: export CODE_CENTRAL_FACTORY_BIN := $(CURDIR)/bin/code-central-factory$(EXE)
e2e: export CODE_CENTRAL_FAKE_GH_BIN := $(CURDIR)/bin/fake-gh$(EXE)
e2e: build
	go build -o bin/fake-gh$(EXE) ./internal/fakegh/gh
	go test -tags e2e -count=1 ./e2e/
	cd web && npm run test:e2e

clean:
	go run ./internal/clean bin web/dist

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
serve-components: $(WEB_DEPS)
	cd web && npm exec -- storybook dev -p 6006 --no-open

build-components: $(WEB_DEPS)
	cd web && npm exec -- storybook build
