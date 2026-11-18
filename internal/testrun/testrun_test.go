package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestMain runs the test binary as a fake job when TESTRUN_FAKE is set:
// its arguments are steps, "print <text>", "sleep <duration>" and
// "exit <code>", so the fakes need no shell.
func TestMain(m *testing.M) {
	if os.Getenv("TESTRUN_FAKE") == "1" {
		os.Exit(fake(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fake(steps []string) int {
	if len(steps)%2 != 0 {
		panic(fmt.Sprintf("fake job: steps %q aren't pairs", steps))
	}
	for i := 0; i < len(steps); i += 2 {
		switch arg := steps[i+1]; steps[i] {
		case "print":
			fmt.Println(arg)
		case "sleep":
			d, err := time.ParseDuration(arg)
			if err != nil {
				panic(err)
			}
			time.Sleep(d)
		case "exit":
			code, err := strconv.Atoi(arg)
			if err != nil {
				panic(err)
			}
			return code
		default:
			panic(fmt.Sprintf("fake job: unknown step %q", steps[i]))
		}
	}
	return 0
}

// fakeJob is a job that runs the fake with these steps.
func fakeJob(t *testing.T, name string, steps ...string) job {
	t.Setenv("TESTRUN_FAKE", "1")
	return job{Name: name, Args: append([]string{os.Args[0]}, steps...)}
}

// runSuite runs the jobs and returns whether they passed, and what was printed.
func runSuite(budget time.Duration, jobs ...job) (bool, string) {
	return runSuiteVerbosely(budget, false, jobs...)
}

func runSuiteVerbosely(budget time.Duration, verbose bool, jobs ...job) (bool, string) {
	var out strings.Builder
	ok := run(jobs, budget, verbose, "", &out)
	return ok, out.String()
}

func TestAFailedRunKeepsTheWholeOutputInAFileAndPrintsItsPathLast(t *testing.T) {
	log := filepath.Join(t.TempDir(), "test.log")
	var out strings.Builder
	ok := run([]job{
		fakeJob(t, "go test", "print", "ok  \tpkg/a\t0.1s", "print", "--- FAIL: TestC", "exit", "1"),
		fakeJob(t, "vitest", "print", "42 passed"),
	}, 10*time.Second, false, log, &out)
	if ok {
		t.Fatalf("the suite passed:\n%s", out.String())
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if last := lines[len(lines)-1]; !strings.HasSuffix(last, log) {
		t.Errorf("the last line doesn't give the log's path %q:\n%s", log, out.String())
	}
	saved, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"== go test\nok  \tpkg/a\t0.1s\n--- FAIL: TestC\n", "== vitest\n42 passed\n"} {
		if !strings.Contains(string(saved), want) {
			t.Errorf("the log lacks %q:\n%s", want, saved)
		}
	}
}

func TestAPassingRunWritesNoLog(t *testing.T) {
	log := filepath.Join(t.TempDir(), "test.log")
	var out strings.Builder
	if !run([]job{fakeJob(t, "go test")}, 10*time.Second, false, log, &out) {
		t.Fatalf("the suite failed:\n%s", out.String())
	}
	if _, err := os.Stat(log); err == nil || strings.Contains(out.String(), log) {
		t.Errorf("a passing run left a log or named one:\n%s", out.String())
	}
}

func TestEachWorktreeHasItsOwnLogFile(t *testing.T) {
	a := logPath("/work/a/code-central")
	b := logPath("/work/b/code-central")
	if a == b {
		t.Errorf("two worktrees share %s", a)
	}
	if logPath("/work/a/code-central") != a {
		t.Error("a worktree's log path isn't stable")
	}
	if !strings.HasPrefix(filepath.Base(a), "code-central-test-code-central-") {
		t.Errorf("the log's name doesn't say the worktree: %s", a)
	}
}

func TestTheSuitePassesWhenEveryJobPassesInsideTheBudget(t *testing.T) {
	ok, out := runSuite(10*time.Second,
		fakeJob(t, "go test", "print", "ok  \tpkg"),
		fakeJob(t, "vitest", "print", "42 passed"),
	)
	if !ok {
		t.Fatalf("the suite failed:\n%s", out)
	}
	if strings.Contains(out, "pkg") || strings.Contains(out, "42 passed") {
		t.Errorf("a passing job's output was printed:\n%s", out)
	}
	for _, want := range []string{"ok go test\n", "ok vitest\n", "make test: "} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestVerboseSuitePrintsEveryJobsWholeOutput(t *testing.T) {
	ok, out := runSuiteVerbosely(10*time.Second, true,
		fakeJob(t, "go test", "print", "ok  \tpkg"),
		fakeJob(t, "vitest", "print", "42 passed"),
	)
	if !ok {
		t.Fatalf("the suite failed:\n%s", out)
	}
	for _, want := range []string{"== go test\nok  \tpkg\n", "== vitest\n42 passed\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestAFailedJobsOutputLeavesOutThePackagesThatPassed(t *testing.T) {
	ok, out := runSuite(10*time.Second,
		fakeJob(t, "go test",
			"print", "ok  \tpkg/a\t0.1s",
			"print", "?   \tpkg/b\t[no test files]",
			"print", "--- FAIL: TestC",
			"exit", "1"),
	)
	if ok || strings.Contains(out, "pkg/a") || strings.Contains(out, "pkg/b") || !strings.Contains(out, "--- FAIL: TestC") {
		t.Errorf("the failure's output isn't just what failed:\n%s", out)
	}
}

func TestTheSuiteFailsAndNamesEachJobsFailure(t *testing.T) {
	ok, out := runSuite(10*time.Second,
		fakeJob(t, "go test", "print", "ok go"),
		fakeJob(t, "vitest", "print", "FAIL a test", "exit", "1"),
		fakeJob(t, "tsc", "print", "error TS2322", "exit", "2"),
	)
	if ok {
		t.Fatalf("the suite passed:\n%s", out)
	}
	for _, want := range []string{
		"== vitest\nFAIL a test\n",
		"== tsc\nerror TS2322\n",
		"make test failed (go test: 0, vitest: 1, tsc: 2)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestTheSuiteFailsWhenAJobCannotStart(t *testing.T) {
	ok, out := runSuite(10*time.Second,
		fakeJob(t, "go test"),
		job{Name: "vitest", Args: []string{"testrun-no-such-program"}},
	)
	if ok {
		t.Fatalf("the suite passed:\n%s", out)
	}
	if !strings.Contains(out, "== vitest\n") || !strings.Contains(out, "testrun-no-such-program") {
		t.Errorf("output doesn't say vitest couldn't start:\n%s", out)
	}
	if !strings.Contains(out, "make test failed (go test: 0, vitest: -1)") {
		t.Errorf("output doesn't report vitest's failure:\n%s", out)
	}
}

func TestTheSuiteFailsWhenItTakesLongerThanTheBudget(t *testing.T) {
	withLoad(t, 1, 8)
	ok, out := runSuite(50*time.Millisecond,
		fakeJob(t, "go test", "sleep", "200ms"),
		fakeJob(t, "vitest"),
	)
	if ok {
		t.Fatalf("the suite passed:\n%s", out)
	}
	if !strings.Contains(out, "all suites passed, but took") || !strings.Contains(out, "over the 0.05s budget.") {
		t.Errorf("output doesn't lead with the result and report the budget:\n%s", out)
	}
	if !strings.Contains(out, "rerun once before treating it as a failure") {
		t.Errorf("output doesn't name the usual causes:\n%s", out)
	}
}

// withLoad fakes the load average and the core count for the test.
func withLoad(t *testing.T, load float64, cpus int) {
	t.Helper()
	oldLoad, oldCores := loadAverage, cores
	t.Cleanup(func() { loadAverage, cores = oldLoad, oldCores })
	loadAverage = func() float64 { return load }
	cores = func() int { return cpus }
}

// rerunWith is a Rerun that reruns the fake job with these steps.
func rerunWith(items []string, steps ...string) func([]byte) ([]string, []string) {
	return func([]byte) ([]string, []string) {
		return append([]string{os.Args[0]}, steps...), items
	}
}

func TestAFailureThatPassesOnSerialRerunIsFlakyAndExitsZero(t *testing.T) {
	withLoad(t, 12.5, 8)
	j := fakeJob(t, "vitest", "print", "FAIL a.test.ts", "exit", "1")
	j.Rerun = rerunWith([]string{"a.test.ts", "b.test.ts"}, "print", "2 passed")
	ok, out := runSuite(10*time.Second, j)
	if !ok {
		t.Fatalf("the suite failed:\n%s", out)
	}
	want := "vitest: passed on serial rerun: flaky under load (load 12.5): a.test.ts, b.test.ts\n"
	if !strings.Contains(out, want) {
		t.Errorf("output lacks %q:\n%s", want, out)
	}
	if strings.Contains(out, "ok vitest") || strings.Contains(out, "FAIL a.test.ts") {
		t.Errorf("a flaky job was reported as plain ok or its failure printed:\n%s", out)
	}
}

func TestAFailureThatFailsAgainOnSerialRerunFailsWithTheLoad(t *testing.T) {
	withLoad(t, 12.5, 8)
	j := fakeJob(t, "vitest", "print", "FAIL a.test.ts", "exit", "1")
	j.Rerun = rerunWith([]string{"a.test.ts"}, "print", "still broken", "exit", "1")
	ok, out := runSuite(10*time.Second, j)
	if ok {
		t.Fatalf("the suite passed:\n%s", out)
	}
	for _, want := range []string{"still broken", "make test failed (vitest: 1)", "load 12.5 on 8 cores"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "flaky") {
		t.Errorf("a failure was called flaky:\n%s", out)
	}
}

func TestAFailureWithNothingToRerunIsNotRerun(t *testing.T) {
	withLoad(t, 1, 8)
	j := fakeJob(t, "go test", "print", "--- FAIL: TestC", "exit", "1")
	j.Rerun = func([]byte) ([]string, []string) { return nil, nil }
	ok, out := runSuite(10*time.Second, j)
	if ok || strings.Contains(out, "flaky") {
		t.Errorf("the suite passed or was called flaky:\n%s", out)
	}
}

func TestOverTheBudgetUnderLoadIsOnlyAWarning(t *testing.T) {
	withLoad(t, 20, 8)
	ok, out := runSuite(50*time.Millisecond, fakeJob(t, "go test", "sleep", "200ms"))
	if !ok {
		t.Fatalf("the suite failed:\n%s", out)
	}
	if !strings.Contains(out, "warning: all suites passed, but took") || !strings.Contains(out, "load (20.0) is above the 8 cores") {
		t.Errorf("output doesn't warn about the budget under load:\n%s", out)
	}
}

func TestOverTheBudgetWithoutLoadStillFails(t *testing.T) {
	withLoad(t, 2, 8)
	if ok, out := runSuite(50*time.Millisecond, fakeJob(t, "go test", "sleep", "200ms")); ok {
		t.Fatalf("the suite passed:\n%s", out)
	}
}

func TestRerunVitestNamesTheFailedFilesOnce(t *testing.T) {
	out := " FAIL  src/a.test.ts > a > works\n FAIL  src/a.test.ts > a > also\n FAIL  src/b.test.tsx [ timed out ]\n"
	args, items := rerunVitest([]byte(out))
	if strings.Join(items, " ") != "src/a.test.ts src/b.test.tsx" {
		t.Errorf("items = %q", items)
	}
	if got := strings.Join(args[len(args)-3:], " "); got != "--no-file-parallelism src/a.test.ts src/b.test.tsx" {
		t.Errorf("args = %q", args)
	}
	if args, _ := rerunVitest([]byte("tsc error\n")); args != nil {
		t.Errorf("rerun without a failed file: %q", args)
	}
}

func TestRerunGoRerunsOnlyLoadFailures(t *testing.T) {
	killed := "ok  \tpkg/a\t0.1s\nsignal: killed\nFAIL\tpkg/b\t9.1s\n--- FAIL: TestX (0.00s)\n    didn't stop within the kill grace\nFAIL\tpkg/c\t1.0s\nFAIL\n"
	args, items := rerunGo([]byte(killed))
	if strings.Join(items, " ") != "pkg/b pkg/c" || args[len(args)-1] != "pkg/c" {
		t.Errorf("args = %q, items = %q", args, items)
	}
	real := killed + "--- FAIL: TestReal\nFAIL\tpkg/d\t1.0s\n"
	if args, _ := rerunGo([]byte(real)); args != nil {
		t.Errorf("a real failure was rerun: %q", args)
	}
}
