VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/tomoram/code-central/cli.Version=$(VERSION)
# The fast suite's wall-clock budget, in seconds.
TEST_BUDGET := 10

.PHONY: test build clean

# Runs the fast suite against the committed placeholder page (no Node
# needed) and fails if it takes more than $(TEST_BUDGET) seconds.
# vitest joins here, in parallel, once the UI exists.
test:
	@start=$$(perl -MTime::HiRes=time -e 'printf "%.3f", time'); \
	go test ./... || exit 1; \
	end=$$(perl -MTime::HiRes=time -e 'printf "%.3f", time'); \
	elapsed=$$(perl -e "printf '%.2f', $$end - $$start"); \
	if perl -e "exit !($$elapsed > $(TEST_BUDGET))"; then \
		echo "make test took $${elapsed}s, over the $(TEST_BUDGET)s budget"; exit 1; \
	fi; \
	echo "make test: $${elapsed}s (budget $(TEST_BUDGET)s)"

# Builds the binary. The Vite build of web/dist and -tags webdist join
# here once the UI exists.
build:
	go build -ldflags "$(LDFLAGS)" -o bin/code-central ./cmd/code-central

clean:
	rm -rf bin
