VERSION := $(or $(shell git describe --tags --always --dirty),dev)
LDFLAGS := -X github.com/tomoram/code-central/cli.Version=$(VERSION)
# The fast suite's wall-clock budget, in seconds.
TEST_BUDGET := 10
# npm ci writes this; it is newer than the lockfile while node_modules is current.
WEB_DEPS := web/node_modules/.package-lock.json

.PHONY: test typecheck vitest e2e build web clean hooks context-report

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
# starts.
test: $(WEB_DEPS) hooks
	@go run ./internal/testrun -budget $(TEST_BUDGET)s

# The inner loop while working on the UI: the TypeScript type check, and
# vitest on one file (F=src/...) or on all of them. Both install web
# dependencies first, so they work in a fresh worktree. The tools run
# through node rather than node_modules/.bin, whose shims differ by OS.
typecheck: $(WEB_DEPS) hooks
	cd web && node node_modules/typescript/bin/tsc --noEmit

vitest: $(WEB_DEPS) hooks
	cd web && node node_modules/vitest/vitest.mjs run $(F:web/%=%)

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
e2e: export CODE_CENTRAL_BIN := $(CURDIR)/bin/code-central$(EXE)
e2e: export CODE_CENTRAL_FACTORY_BIN := $(CURDIR)/bin/code-central-factory$(EXE)
e2e: build
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
