// Command testrun runs the fast suite for `make test`: `go test ./...`,
// `vitest run` and the TypeScript type check, in parallel. It is quiet
// by default: a job that passes prints one line, and a job that fails
// prints its output without the packages that passed. With -v every
// job's whole output is printed. The output is printed when all jobs are
// done, so they don't interleave. It fails if any job fails, or if the
// suite takes longer than the budget of wall-clock time. It is Go rather than a shell recipe so `make test`
// runs the same under cmd.exe and POSIX sh. It is a developer tool for
// the Makefile, not a shipped command, so it lives under internal/
// rather than cmd/.
//
// Run it from the repository root, after the web dependencies are
// installed, so installing them isn't timed:
//
//	go run ./internal/testrun -budget 10s [-v]
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// job is one command of the suite: its name in the report, the folder
// it runs in (the current one when empty) and its program and arguments.
type job struct {
	Name string
	Dir  string
	Args []string
}

// suite is the fast suite. The web jobs run their tools through node
// rather than node_modules/.bin, whose shims differ by OS.
var suite = []job{
	{Name: "go test", Args: []string{"go", "test", "./..."}},
	{Name: "vitest", Dir: "web", Args: []string{"node", "node_modules/vitest/vitest.mjs", "run"}},
	{Name: "tsc", Dir: "web", Args: []string{"node", "node_modules/typescript/bin/tsc", "--noEmit"}},
}

func main() {
	budget := flag.Duration("budget", 10*time.Second, "the most wall-clock time the suite may take")
	verbose := flag.Bool("v", false, "print every job's whole output, not only the failures'")
	flag.Parse()
	if !run(suite, *budget, *verbose, os.Stdout) {
		os.Exit(1)
	}
}

// passed matches the lines of a job's output that say a package passed or
// has no tests, which a failure's output is better without.
var passed = regexp.MustCompile(`^(ok  \t|\?   \t.*\[no test files\])`)

// quiet is output without the lines that say a package passed or has no tests.
func quiet(output []byte) []byte {
	var kept bytes.Buffer
	for _, line := range bytes.SplitAfter(output, []byte("\n")) {
		if !passed.Match(line) {
			kept.Write(line)
		}
	}
	return kept.Bytes()
}

// run runs the jobs in parallel, prints each one's output under its name
// and then a summary to out, and reports whether every job passed inside
// the budget. Unless verbose, a job that passed prints only its name and
// a failed one's output leaves out the packages that passed.
func run(jobs []job, budget time.Duration, verbose bool, out io.Writer) bool {
	start := time.Now()
	outputs := make([]bytes.Buffer, len(jobs))
	codes := make([]int, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Go(func() { codes[i] = runJob(j, &outputs[i]) })
	}
	wg.Wait()
	elapsed := time.Since(start).Seconds()

	failed := false
	statuses := make([]string, len(jobs))
	for i, j := range jobs {
		switch {
		case verbose:
			fmt.Fprintf(out, "== %s\n", j.Name)
			out.Write(outputs[i].Bytes())
		case codes[i] == 0:
			fmt.Fprintf(out, "ok %s\n", j.Name)
		default:
			fmt.Fprintf(out, "== %s\n", j.Name)
			out.Write(quiet(outputs[i].Bytes()))
		}
		statuses[i] = fmt.Sprintf("%s: %d", j.Name, codes[i])
		failed = failed || codes[i] != 0
	}
	switch {
	case failed:
		fmt.Fprintf(out, "make test failed (%s) in %.2fs\n", strings.Join(statuses, ", "), elapsed)
		return false
	case elapsed > budget.Seconds():
		fmt.Fprintf(out, "make test took %.2fs, over the %gs budget\n", elapsed, budget.Seconds())
		return false
	}
	fmt.Fprintf(out, "make test: %.2fs (budget %gs)\n", elapsed, budget.Seconds())
	return true
}

// runJob runs the job with its output, stdout and stderr together, going
// to w, and returns its exit code: -1 when it couldn't be started.
func runJob(j job, w io.Writer) int {
	cmd := exec.Command(j.Args[0], j.Args[1:]...)
	cmd.Dir = j.Dir
	cmd.Stdout = w
	cmd.Stderr = w
	err := cmd.Run()
	if err == nil {
		return 0
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode()
	}
	fmt.Fprintln(w, err)
	return -1
}
