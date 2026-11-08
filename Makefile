VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/tomoram/code-central/cli.Version=$(VERSION)
# The fast suite's wall-clock budget, in seconds.
TEST_BUDGET := 10
# npm ci writes this; it is newer than the lockfile while node_modules is current.
WEB_DEPS := web/node_modules/.package-lock.json

.PHONY: test e2e build web clean

# Runs the fast suite and fails if it takes more than $(TEST_BUDGET)
# seconds of wall-clock time. Three jobs run in parallel: `go test ./...`
# (against the committed placeholder page, so no Vite build), `vitest run`
# and the TypeScript type check. Each job's output is printed when all
# are done, so they don't interleave. Installing web dependencies (first
# run, or after the lockfile changes) happens before the clock starts.
test: $(WEB_DEPS)
	@out=$$(mktemp -d); \
	start=$$(perl -MTime::HiRes=time -e 'printf "%.3f", time'); \
	go test ./... > $$out/go 2>&1 & g=$$!; \
	(cd web && node_modules/.bin/vitest run) > $$out/vitest 2>&1 & v=$$!; \
	(cd web && node_modules/.bin/tsc --noEmit) > $$out/tsc 2>&1 & c=$$!; \
	wait $$g; gs=$$?; wait $$v; vs=$$?; wait $$c; cs=$$?; \
	end=$$(perl -MTime::HiRes=time -e 'printf "%.3f", time'); \
	echo "== go test"; cat $$out/go; \
	echo "== vitest"; cat $$out/vitest; \
	echo "== tsc"; cat $$out/tsc; rm -rf $$out; \
	elapsed=$$(perl -e "printf '%.2f', $$end - $$start"); \
	if [ $$gs -ne 0 ] || [ $$vs -ne 0 ] || [ $$cs -ne 0 ]; then \
		echo "make test failed (go test: $$gs, vitest: $$vs, tsc: $$cs) in $${elapsed}s"; exit 1; \
	fi; \
	if perl -e "exit !($$elapsed > $(TEST_BUDGET))"; then \
		echo "make test took $${elapsed}s, over the $(TEST_BUDGET)s budget"; exit 1; \
	fi; \
	echo "make test: $${elapsed}s (budget $(TEST_BUDGET)s)"

$(WEB_DEPS): web/package.json web/package-lock.json
	@command -v npm >/dev/null || { echo "make: npm not found; the UI needs Node (go test ./... alone runs without it)"; exit 1; }
	npm --prefix web ci

# Builds the UI with Vite into web/dist (never committed).
web: $(WEB_DEPS)
	npm --prefix web run build

# Builds the binary with the built UI embedded (-tags webdist), and the
# local Factory, code-central-factory. In development, run
# `code-central serve` and `npm --prefix web run dev`: Vite serves the UI
# on :5173 and proxies /api to the service.
build: web
	go build -tags webdist -ldflags "$(LDFLAGS)" -o bin/code-central ./cmd/code-central
	go build -o bin/code-central-factory ./cmd/code-central-factory

# Builds the binary, then runs the end-to-end tier against it (real
# binary, real store in a temporary folder, never port 7733): the Go
# end-to-end tests and the real half of the UI's API contract suite.
e2e: build
	CODE_CENTRAL_BIN=$(CURDIR)/bin/code-central go test -tags e2e -count=1 ./e2e/
	cd web && CODE_CENTRAL_BIN=$(CURDIR)/bin/code-central npm run test:e2e

clean:
	rm -rf bin web/dist
