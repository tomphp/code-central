// Command testrun runs the fast suite for `make test`: `go test ./...`,
// `gofmt -l`, `vitest run`, the TypeScript type check and ESLint, in parallel. It is quiet
// by default: a job that passes prints one line, and a job that fails
// prints its output without the packages that passed. With -v every
// job's whole output is printed. When a job fails, every job's whole output
// (as -v would print it) is also written to a file in the temporary folder,
// $TMPDIR/code-central-test-<worktree>-<hash>.log, and its path is the last
// line printed, after one "FAIL <suite> <file or test>" line per failure
// (ANSI codes stripped), so a failure that was piped through tail can still be read
// without rerunning. The output is printed when all jobs are
// done, so they don't interleave. It fails if any job fails, or if the
// suite takes longer than the budget of wall-clock time, unless the load
// average is above the core count, when the overrun is only a warning.
// A job that fails only the way load makes tests fail (a Vitest file, a Go
// test killed or one whose rig "didn't stop") gets one serial rerun of
// just what failed; if that passes the job is reported as flaky under load
// and doesn't fail the suite. The load average is printed beside failures.
// With -e2e [-f <web file>] it runs `make e2e`'s suites instead, one after
// the other, with no budget, the same way and with its log named
// code-central-e2e-<worktree>-<hash>.log; a FAIL line also carries the first
// error under a failed Playwright file.
// It is Go rather than a shell recipe so `make test`
// runs the same under cmd.exe and POSIX sh. It is a developer tool for
// the Makefile, not a shipped command, so it lives under internal/
// rather than cmd/.
//
// Run it from the repository root, after the web dependencies are
// installed, so installing them isn't timed:
//
//	go run ./internal/testrun -budget 60s [-v]
package main

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// job is one command of the suite: its name in the report, the folder
// it runs in (the current one when empty) and its program and arguments.
// Rerun, when set, is given the output of a failed run and returns the
// command that reruns only what failed and what that is (files or packages),
// or no command when the failure isn't one that load explains.
type job struct {
	Name  string
	Dir   string
	Args  []string
	Rerun func(output []byte) (args, items []string)
	// FailOnOutput fails the job when it prints anything, for a tool like
	// gofmt -l that lists what is wrong and still exits zero.
	FailOnOutput bool
	// Failures, when set, is given the ANSI-stripped output of a failed run
	// and returns what failed (files, tests or packages), for the FAIL lines.
	Failures func(output []byte) []string
}

// command names the run in its report, kind is the word in its log's file
// name, and serial runs the jobs one after another rather than together
// (`make e2e`: its two suites are heavy and time out when they share the
// machine). main sets them for -e2e; they are variables so tests can too.
var (
	command = "make test"
	kind    = "test"
	serial  = false
)

// loadAverage and cores are variables so tests can fake them.
var (
	loadAverage = systemLoad
	cores       = runtime.NumCPU
)

var loadPattern = regexp.MustCompile(`load averages?:?\s*\{?\s*([0-9]+[.,]?[0-9]*)`)

// systemLoad is the one-minute load average from /proc or uptime, or -1
// when it can't be read (on Windows, say).
func systemLoad() float64 {
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f, err := strconv.ParseFloat(strings.Fields(string(b))[0], 64); err == nil {
			return f
		}
	}
	out, err := exec.Command("uptime").Output()
	if err != nil {
		return -1
	}
	m := loadPattern.FindSubmatch(out)
	if m == nil {
		return -1
	}
	f, err := strconv.ParseFloat(strings.Replace(string(m[1]), ",", ".", 1), 64)
	if err != nil {
		return -1
	}
	return f
}

// vitestFail matches the line Vitest prints for a failed test file or test.
var vitestFail = regexp.MustCompile(`(?m)^\s*FAIL\s+(\S+\.(?:test|spec)\.\w+)`)

// rerunVitest reruns the files that failed, one at a time.
func rerunVitest(output []byte) (args, items []string) {
	seen := map[string]bool{}
	for _, m := range vitestFail.FindAllSubmatch(output, -1) {
		if f := string(m[1]); !seen[f] {
			seen[f] = true
			items = append(items, f)
		}
	}
	if len(items) == 0 {
		return nil, nil
	}
	args = append([]string{"node", "node_modules/vitest/vitest.mjs", "run", "--no-file-parallelism"}, items...)
	return args, items
}

// loadFailure matches what a Go test run prints when load killed the test
// binary or the Factory rig's process didn't stop in its grace period.
var loadFailure = regexp.MustCompile(`signal: killed|didn't stop`)

// rerunGo reruns the packages that failed with a load failure, one at a
// time. A package that failed any other way means the run isn't flaky.
func rerunGo(output []byte) (args, items []string) {
	var block []byte
	for _, line := range bytes.SplitAfter(output, []byte("\n")) {
		switch {
		case bytes.HasPrefix(line, []byte("FAIL\t")):
			fields := strings.Fields(string(line))
			if len(fields) < 2 || !loadFailure.Match(block) {
				return nil, nil
			}
			items = append(items, fields[1])
			block = nil
		case bytes.HasPrefix(line, []byte("ok  \t")), bytes.HasPrefix(line, []byte("?   \t")):
			block = nil
		default:
			block = append(block, line...)
		}
	}
	if len(items) == 0 {
		return nil, nil
	}
	return append([]string{"go", "test", "-p", "1", "-count=1"}, items...), items
}

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")

// failuresVitest names each failed test file once.
func failuresVitest(output []byte) []string {
	var items []string
	seen := map[string]bool{}
	for _, m := range vitestFail.FindAllSubmatch(output, -1) {
		if f := string(m[1]); !seen[f] {
			seen[f] = true
			items = append(items, f)
		}
	}
	return items
}

var errorLine = regexp.MustCompile(`^\s*(?:\w*Error|Error)\b`)

// failuresVitestErrors names each failed test file once with the first error
// printed under it: a failed test's assertion, or for a file that failed with
// no failed test (a setup or import error) the error that failed the file.
func failuresVitestErrors(output []byte) []string {
	var items []string
	index := map[string]int{}
	current := -1
	for _, line := range strings.Split(string(output), "\n") {
		if m := vitestFail.FindStringSubmatch(line); m != nil {
			i, ok := index[m[1]]
			if !ok {
				i = len(items)
				index[m[1]] = i
				items = append(items, m[1])
			}
			current = i
			continue
		}
		if current >= 0 && errorLine.MatchString(line) {
			if !strings.Contains(items[current], " - ") {
				items[current] += " - " + strings.TrimSpace(line)
			}
			current = -1
		}
	}
	return items
}

var goTestFail = regexp.MustCompile(`^\s*--- FAIL: (\S+)`)

// failuresGo names each failed test as "<package> <test>", or the package
// alone when it failed without a failing test (a build failure, a panic).
func failuresGo(output []byte) []string {
	var items, tests []string
	for _, line := range strings.Split(string(output), "\n") {
		if m := goTestFail.FindStringSubmatch(line); m != nil {
			if !strings.Contains(m[1], "/") {
				tests = append(tests, m[1])
			}
			continue
		}
		if fields := strings.Fields(line); strings.HasPrefix(line, "FAIL\t") && len(fields) >= 2 {
			if len(tests) == 0 {
				items = append(items, fields[1])
			}
			for _, t := range tests {
				items = append(items, fields[1]+" "+t)
			}
			tests = nil
		}
	}
	return items
}

var tscError = regexp.MustCompile(`(?m)^(\S+?)\(\d+,\d+\): error`)

// failuresTsc names each file with a type error once.
func failuresTsc(output []byte) []string {
	var items []string
	seen := map[string]bool{}
	for _, m := range tscError.FindAllSubmatch(output, -1) {
		if f := string(m[1]); !seen[f] {
			seen[f] = true
			items = append(items, f)
		}
	}
	return items
}

// failuresLines names each non-empty line, for gofmt -l's file list.
func failuresLines(output []byte) []string {
	var items []string
	for _, l := range strings.Split(string(output), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "^ ") {
			items = append(items, l)
		}
	}
	return items
}

// goFiles lists the .go files of the module at root, as go test ./... sees
// them: it skips any directory below root with its own go.mod (a nested
// module such as an agent worktree), and .git and node_modules.
func goFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// suite is the fast suite. The web jobs run their tools through node
// rather than node_modules/.bin, whose shims differ by OS.
var suite = []job{
	{Name: "go test", Args: []string{"go", "test", "./..."}, Rerun: rerunGo, Failures: failuresGo},
	{Name: "gofmt", Args: []string{"gofmt", "-l"}, FailOnOutput: true, Failures: failuresLines},
	{Name: "vitest", Dir: "web", Args: []string{"node", "node_modules/vitest/vitest.mjs", "run"}, Rerun: rerunVitest, Failures: failuresVitest},
	{Name: "tsc", Dir: "web", Args: []string{"node", "node_modules/typescript/bin/tsc", "--noEmit"}, Failures: failuresTsc},
	{Name: "eslint", Dir: "web", Args: []string{"node", "node_modules/eslint/bin/eslint.js", "."}},
	{Name: "hooks", Args: []string{"python3", "-I", ".claude/hooks/prefer-edit_test.py"}},
}

// e2eSuite is `make e2e`: the Go end-to-end tests, then the UI's. The
// Makefile has built the binaries and set their paths in the environment.
// file, when not empty, is one web e2e file, and the Go tests are skipped.
func e2eSuite(file string) []job {
	web := job{Name: "playwright", Dir: "web", Args: []string{"node", "node_modules/vitest/vitest.mjs", "run", "--config", "vitest.e2e.config.ts"}, Failures: failuresVitestErrors}
	if file != "" {
		web.Args = append(web.Args, file)
		return []job{web}
	}
	goE2E := job{Name: "go e2e", Args: []string{"go", "test", "-tags", "e2e", "-count=1", "./e2e/"}, Failures: failuresGo}
	return []job{goE2E, web}
}

func main() {
	budget := flag.Duration("budget", 60*time.Second, "the most wall-clock time the suite may take")
	verbose := flag.Bool("v", false, "print every job's whole output, not only the failures'")
	e2e := flag.Bool("e2e", false, "run `make e2e`'s suites instead of the fast suite")
	file := flag.String("f", "", "with -e2e, run only this web e2e file")
	flag.Parse()
	wd, _ := os.Getwd()
	if *e2e {
		command, kind, serial = "make e2e", "e2e", true
		if !run(e2eSuite(strings.TrimPrefix(filepath.ToSlash(*file), "web/")), 0, *verbose, logPath(wd), os.Stdout) {
			os.Exit(1)
		}
		return
	}
	files, err := goFiles(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for i := range suite {
		if suite[i].Name == "gofmt" && len(files) > 0 {
			suite[i].Args = append(suite[i].Args, files...)
		}
	}
	if !run(suite, *budget, *verbose, logPath(wd), os.Stdout) {
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

// logPath is where a failed run keeps its whole output for the worktree at
// dir: the folder's name and a hash of its whole path, so worktrees don't
// share a file.
func logPath(dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return filepath.Join(os.TempDir(), fmt.Sprintf("code-central-%s-%s-%x.log", kind, filepath.Base(dir), sum[:4]))
}

// run runs the jobs in parallel, prints each one's output under its name
// and then a summary to out, and reports whether every job passed inside
// the budget. Unless verbose, a job that passed prints only its name and
// a failed one's output leaves out the packages that passed. When a job
// failed and log isn't empty, every job's whole output is written to that
// file and its path is the last line.
func run(jobs []job, budget time.Duration, verbose bool, log string, out io.Writer) bool {
	start := time.Now()
	outputs := make([]bytes.Buffer, len(jobs))
	codes := make([]int, len(jobs))
	skipped := make([]bool, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		do := func() {
			codes[i] = runJob(j, j.Args, &outputs[i])
			if codes[i] == 0 && j.FailOnOutput && outputs[i].Len() > 0 {
				fmt.Fprintln(&outputs[i], "^ these files aren't gofmt'd: run gofmt -w on them")
				codes[i] = 1
			}
		}
		if serial {
			// A serial run stops at the first failure, so a Go failure doesn't
			// wait on the slower UI suite.
			if failedBefore(codes[:i]) {
				skipped[i] = true
				continue
			}
			do()
		} else {
			wg.Go(do)
		}
	}
	wg.Wait()

	// What failed gets one serial rerun, after the parallel jobs are done so
	// the machine is as quiet as it will be.
	flaky := make([][]string, len(jobs))
	for i, j := range jobs {
		if codes[i] == 0 || j.Rerun == nil {
			continue
		}
		args, items := j.Rerun(outputs[i].Bytes())
		if args == nil {
			continue
		}
		fmt.Fprintf(&outputs[i], "== serial rerun of %s\n", strings.Join(items, " "))
		if codes[i] = runJob(j, args, &outputs[i]); codes[i] == 0 {
			flaky[i] = items
		}
	}
	elapsed := time.Since(start).Seconds()
	load := loadAverage()
	loadText := ""
	if load >= 0 {
		loadText = fmt.Sprintf("%.1f", load)
	}

	failed := false
	statuses := make([]string, len(jobs))
	for i, j := range jobs {
		switch {
		case verbose:
			fmt.Fprintf(out, "== %s\n", j.Name)
			out.Write(outputs[i].Bytes())
		case flaky[i] != nil:
		case skipped[i]:
			fmt.Fprintf(out, "skipped %s\n", j.Name)
		case codes[i] == 0:
			fmt.Fprintf(out, "ok %s\n", j.Name)
		default:
			fmt.Fprintf(out, "== %s\n", j.Name)
			out.Write(quiet(outputs[i].Bytes()))
		}
		if flaky[i] != nil {
			fmt.Fprintf(out, "%s: passed on serial rerun: flaky under load (load %s): %s\n", j.Name, loadText, strings.Join(flaky[i], ", "))
		}
		statuses[i] = fmt.Sprintf("%s: %d", j.Name, codes[i])
		failed = failed || codes[i] != 0
	}
	overBudget := budget > 0 && elapsed > budget.Seconds()
	switch {
	case failed:
		fmt.Fprintf(out, "%s failed (%s) in %.2fs", command, strings.Join(statuses, ", "), elapsed)
		if loadText != "" {
			fmt.Fprintf(out, ", load %s on %d cores", loadText, cores())
		}
		fmt.Fprintln(out)
		for i, j := range jobs {
			if codes[i] == 0 {
				continue
			}
			var items []string
			if j.Failures != nil {
				items = j.Failures(ansi.ReplaceAll(outputs[i].Bytes(), nil))
			}
			if len(items) == 0 {
				fmt.Fprintf(out, "FAIL %s\n", j.Name)
			}
			for _, item := range items {
				fmt.Fprintf(out, "FAIL %s %s\n", j.Name, item)
			}
		}
		if log != "" {
			var all bytes.Buffer
			for i, j := range jobs {
				fmt.Fprintf(&all, "== %s\n%s", j.Name, ansi.ReplaceAll(outputs[i].Bytes(), nil))
			}
			if err := os.WriteFile(log, all.Bytes(), 0o644); err != nil {
				fmt.Fprintf(out, "the whole output couldn't be saved: %v\n", err)
			} else {
				fmt.Fprintf(out, "whole output: %s\n", log)
			}
		}
		return false
	case overBudget && load >= 0 && load > float64(cores()):
		fmt.Fprintf(out, "%s: warning: all suites passed, but took %.2fs, over the %gs budget; the load (%s) is above the %d cores, so this isn't a failure.\n", command, elapsed, budget.Seconds(), loadText, cores())
		return true
	case overBudget:
		fmt.Fprintf(out, "%s: all suites passed, but took %.2fs, over the %gs budget.\n", command, elapsed, budget.Seconds())
		fmt.Fprintln(out, "A cold cache (after npm ci or a fresh worktree) or another process (a dev server, a stress run) inflates this; rerun once before treating it as a failure.")
		return false
	}
	if budget <= 0 {
		fmt.Fprintf(out, "%s: %.2fs\n", command, elapsed)
		return true
	}
	fmt.Fprintf(out, "%s: %.2fs (budget %gs)\n", command, elapsed, budget.Seconds())
	return true
}

func failedBefore(codes []int) bool {
	for _, c := range codes {
		if c != 0 {
			return true
		}
	}
	return false
}

// runJob runs the job with its output, stdout and stderr together, going
// to w, and returns its exit code: -1 when it couldn't be started.
func runJob(j job, args []string, w io.Writer) int {
	cmd := exec.Command(args[0], args[1:]...)
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
