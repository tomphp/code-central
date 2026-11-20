// Command testshard runs one Shard of the Go tests, so CI can split them
// across jobs: `go test -v` over the tests that hash to Shard i of N. It
// lists the top-level Test, Example and Fuzz names of each package with
// `go test -list . -json` (Benchmarks aren't run), puts each in Shard
// FNV-1a(import path, 0, name) mod N, and runs each package that has tests
// in this Shard as `go test -v -run '^(A|B|…)$' <package>`. A package with
// none is skipped, so its TestMain (which the listing runs too) isn't run
// a second time for nothing. A package whose -run pattern would be longer
// than the limit, which keeps the command line well under Windows' 32,767
// characters, is run in several invocations. Packages
// run concurrently, up to the CPU count, and each one's output is printed
// whole when it finishes, so it reads as plain `go test -v`. It is
// stateless: no list of tests or timings is kept, so a new test or package
// lands in some Shard by itself. It exits non-zero if the listing doesn't
// build or any `go test` fails. With -of 1 it runs every test.
//
// It is a developer and CI tool, not a shipped command, so it lives under
// internal/ rather than cmd/. Run it from the module's root:
//
//	go run ./internal/testshard -shard 2 -of 4 [-tags t] [-dry-run] [packages]
//
// The packages default to ./...; -dry-run (or -print) prints the `go test`
// commands instead of running them.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
)

// defaultLimit is the longest -run pattern one `go test` is given.
const defaultLimit = 24000

// invocation is one `go test` of a package, running the tests -run matches.
type invocation struct {
	Package string
	Run     string
}

// args are the invocation's `go test` arguments, with the build tags if any.
func (inv invocation) args(tags string) []string {
	args := []string{"test", "-v"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	return append(args, "-run", inv.Run, inv.Package)
}

// plan returns the invocations that run Shard shard (1..of) of the tests
// (import path → top-level names), none of whose -run patterns is longer
// than limit unless one name alone is.
func plan(tests map[string][]string, shard, of, limit int) []invocation {
	var invs []invocation
	for _, pkg := range slices.Sorted(maps.Keys(tests)) {
		// quoted are the names of the invocation being filled, and joined
		// is the length of strings.Join(quoted, "|").
		var quoted []string
		joined := 0
		flush := func() {
			if len(quoted) > 0 {
				invs = append(invs, invocation{Package: pkg, Run: "^(" + strings.Join(quoted, "|") + ")$"})
			}
			quoted, joined = nil, 0
		}
		for _, name := range slices.Sorted(slices.Values(tests[pkg])) {
			if shardOf(pkg, name, of) != shard {
				continue
			}
			q := regexp.QuoteMeta(name)
			if len(quoted) > 0 && len("^()$")+joined+len("|")+len(q) > limit {
				flush()
			}
			if len(quoted) > 0 {
				joined += len("|")
			}
			quoted = append(quoted, q)
			joined += len(q)
		}
		flush()
	}
	return invs
}

// shardOf is the Shard, 1..of, that a test belongs to: FNV-1a over its
// package's import path, a zero byte and its name, mod of. Changing it
// moves tests between Shards, so the golden test fails until it is updated.
func shardOf(pkg, name string, of int) int {
	h := fnv.New32a()
	h.Write([]byte(pkg))
	h.Write([]byte{0})
	h.Write([]byte(name))
	return int(h.Sum32()%uint32(of)) + 1
}

// listed matches a line of `go test -list` output that names a test the
// tool runs: a Test, Example or Fuzz, not a Benchmark.
var listed = regexp.MustCompile(`^(Test|Example|Fuzz)[\p{L}\p{N}_]*$`)

// list returns the top-level tests of the packages, by import path, as
// `go test -list` in dir lists them. A package without tests is left out.
// When the listing fails, the error holds what `go test` printed, such as
// the build error.
func list(dir, tags string, patterns []string) (map[string][]string, error) {
	args := []string{"test", "-list", ".", "-json"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	cmd := exec.Command("go", append(args, patterns...)...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, runErr := cmd.Output()
	tests := map[string][]string{}
	var printed strings.Builder
	var decodeErr error
	for dec := json.NewDecoder(bytes.NewReader(stdout)); ; {
		var e struct{ Action, Package, Output string }
		if err := dec.Decode(&e); err != nil {
			if err != io.EOF {
				decodeErr = err
			}
			break
		}
		printed.WriteString(e.Output)
		if name := strings.TrimSuffix(e.Output, "\n"); e.Action == "output" && listed.MatchString(name) {
			tests[e.Package] = append(tests[e.Package], name)
		}
	}
	if runErr != nil {
		return nil, fmt.Errorf("go test -list: %v\n%s%s", runErr, printed.String(), stderr.String())
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("go test -list: reading its JSON: %v", decodeErr)
	}
	return tests, nil
}

// run runs the invocations in dir, up to the CPU count at once, writing
// each one's output to out whole when it finishes, and reports whether
// they all passed.
func run(dir, tags string, invs []invocation, out io.Writer) bool {
	var (
		mu  sync.Mutex
		ok  = true
		wg  sync.WaitGroup
		sem = make(chan struct{}, runtime.NumCPU())
	)
	for _, inv := range invs {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			cmd := exec.Command("go", inv.args(tags)...)
			cmd.Dir = dir
			output, err := cmd.CombinedOutput()
			mu.Lock()
			defer mu.Unlock()
			out.Write(output)
			if err != nil {
				ok = false
				if _, exited := err.(*exec.ExitError); !exited {
					fmt.Fprintf(out, "FAIL\t%s: %v\n", inv.Package, err)
				}
			}
		})
	}
	wg.Wait()
	return ok
}

// command runs testshard with these arguments in dir (the current folder
// when empty) and returns its exit code.
func command(args []string, dir string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("testshard", flag.ContinueOnError)
	flags.SetOutput(stderr)
	shard := flags.Int("shard", 1, "the Shard to run, 1..of")
	of := flags.Int("of", 1, "the number of Shards")
	tags := flags.String("tags", "", "build tags, as go test -tags takes them")
	var dryRun bool
	flags.BoolVar(&dryRun, "dry-run", false, "print the go test commands instead of running them")
	flags.BoolVar(&dryRun, "print", false, "the same as -dry-run")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *of < 1 || *shard < 1 || *shard > *of {
		fmt.Fprintf(stderr, "testshard: -shard %d -of %d: want 1 <= shard <= of\n", *shard, *of)
		return 2
	}
	patterns := flags.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	tests, err := list(dir, *tags, patterns)
	if err != nil {
		fmt.Fprintf(stderr, "testshard: %v\n", err)
		return 1
	}
	invs := plan(tests, *shard, *of, defaultLimit)
	if dryRun {
		for _, inv := range invs {
			quoted := invocation{Package: inv.Package, Run: "'" + inv.Run + "'"}
			fmt.Fprintln(stdout, "go "+strings.Join(quoted.args(*tags), " "))
		}
		return 0
	}
	if !run(dir, *tags, invs, stdout) {
		return 1
	}
	return 0
}

func main() {
	os.Exit(command(os.Args[1:], "", os.Stdout, os.Stderr))
}
