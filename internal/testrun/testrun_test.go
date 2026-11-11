package main

import (
	"fmt"
	"os"
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
	ok := run(jobs, budget, verbose, &out)
	return ok, out.String()
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
	ok, out := runSuite(50*time.Millisecond,
		fakeJob(t, "go test", "sleep", "200ms"),
		fakeJob(t, "vitest"),
	)
	if ok {
		t.Fatalf("the suite passed:\n%s", out)
	}
	if !strings.Contains(out, "over the 0.05s budget") {
		t.Errorf("output doesn't report the budget:\n%s", out)
	}
}
