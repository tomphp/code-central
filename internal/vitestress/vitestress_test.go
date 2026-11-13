package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestMain runs the test binary as a fake vitest when VITESTRESS_FAKE is
// set: it exits 1.
func TestMain(m *testing.M) {
	if os.Getenv("VITESTRESS_FAKE") == "1" {
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestStressCountsFailures(t *testing.T) {
	t.Setenv("VITESTRESS_FAKE", "1")
	var out strings.Builder
	got := stress([]string{os.Args[0]}, "", 3, 0, false, &out)
	want := "run 1: FAIL\nrun 2: FAIL\nrun 3: FAIL\n3 runs, 3 failures\n"
	if got != 3 || out.String() != want {
		t.Errorf("got %d failures, output %q", got, out.String())
	}
}

func TestStressPassingRuns(t *testing.T) {
	var out strings.Builder
	got := stress([]string{os.Args[0], "-test.run=^$"}, "", 2, 1, false, &out)
	want := "run 1: ok\nrun 2: ok\n2 runs, 0 failures\n"
	if got != 0 || out.String() != want {
		t.Errorf("got %d failures, output %q", got, out.String())
	}
}

func TestLoadStops(t *testing.T) {
	before := runtime.NumGoroutine()
	stop := startLoad(3)
	if runtime.NumGoroutine() < before+3 {
		t.Fatal("load workers didn't start")
	}
	stop()
	time.Sleep(10 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("%d goroutines after stop, %d before", after, before)
	}
}
