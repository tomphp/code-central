// Command vitestress runs one vitest file many times while the CPU is
// busy, for `make vitest-stress`: a flaky web test that passes alone
// fails under the load of a busy CI runner, and "0 failures in 20 runs
// under load" is the evidence that a fix worked. It prints one line per
// run and a final count, and fails if any run failed. The load is busy
// goroutines in this process, so it stops when the tool exits, however
// it exits, and runs the same under cmd.exe and POSIX sh. It is a
// developer tool for the Makefile, not a shipped command, so it lives
// under internal/ rather than cmd/.
//
// Run it from the repository root, after the web dependencies are
// installed:
//
//	go run ./internal/vitestress -n 20 -load 4 src/activity/Activity.test.tsx
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

func main() {
	n := flag.Int("n", 20, "how many times to run the file")
	load := flag.Int("load", 4, "how many CPU-busy workers to run meanwhile; 0 for none")
	verbose := flag.Bool("v", false, "print the output of a failed run")
	flag.Parse()
	if flag.NArg() != 1 || *n < 1 || *load < 0 {
		fmt.Fprintln(os.Stderr, "usage: vitestress [-n runs] [-load workers] [-v] <test file>")
		os.Exit(2)
	}
	file := strings.TrimPrefix(flag.Arg(0), "web/")
	args := []string{"node", "node_modules/vitest/vitest.mjs", "run", file}
	if stress(args, "web", *n, *load, *verbose, os.Stdout) > 0 {
		os.Exit(1)
	}
}

// stress runs the command n times in dir with load busy workers running,
// prints a line per run and a total to out, and returns the failures.
// With verbose, a failed run's output is printed under its line.
func stress(args []string, dir string, n, load int, verbose bool, out io.Writer) int {
	stop := startLoad(load)
	defer stop()
	failures := 0
	for i := 1; i <= n; i++ {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err == nil {
			fmt.Fprintf(out, "run %d: ok\n", i)
			continue
		}
		failures++
		fmt.Fprintf(out, "run %d: FAIL\n", i)
		if verbose || cmd.ProcessState == nil {
			out.Write(output)
			fmt.Fprintln(out, err)
		}
	}
	fmt.Fprintf(out, "%d runs, %d failures\n", n, failures)
	return failures
}

// startLoad starts n goroutines that spin until the returned function is
// called, which waits for them to stop.
func startLoad(n int) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			x := 0
			for {
				select {
				case <-done:
					return
				default:
				}
				for range 1_000_000 {
					x++
				}
			}
		})
	}
	return func() {
		close(done)
		wg.Wait()
	}
}
