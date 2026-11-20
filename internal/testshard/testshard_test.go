package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// module writes a Go module named example.com/fixture with these files
// (path → source) to a temporary folder and returns the folder.
func module(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files["go.mod"] = "module example.com/fixture\n\ngo 1.21\n"
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// twoPackages is a module of two packages with Tests, an Example, a Fuzz
// target and a Benchmark, and a package without tests.
func twoPackages() map[string]string {
	return map[string]string{
		"a/a.go": "package a\n",
		"a/a_test.go": `package a

import (
	"fmt"
	"testing"
)

func TestOne(t *testing.T)    {}
func TestOneMore(t *testing.T) {}
func TestTwo(t *testing.T)    { t.Run("sub", func(t *testing.T) {}) }

func Example() {
	fmt.Println("hello")
	// Output: hello
}
`,
		"b/b.go": "package b\n",
		"b/b_test.go": `package b

import "testing"

func TestThree(t *testing.T) {}
func TestFour(t *testing.T)  {}
func TestFive(t *testing.T)  {}

func FuzzSix(f *testing.F) {
	f.Add("seed")
	f.Fuzz(func(t *testing.T, s string) {})
}

func BenchmarkSeven(b *testing.B) {}
`,
		"c/c.go": "package c\n",
	}
}

// testshard runs the command in dir with these arguments and returns its
// exit code and what it printed.
func testshard(dir string, args ...string) (int, string) {
	var out strings.Builder
	code := command(args, dir, &out, &out)
	return code, out.String()
}

var passed = regexp.MustCompile(`(?m)^--- PASS: (\w+) `)

func TestAcrossTheShardsEachTestRunsOnce(t *testing.T) {
	t.Parallel()
	dir := module(t, twoPackages())
	const of = 3
	// The Shards run at once, as CI's jobs do.
	outs := make([]string, of)
	var wg sync.WaitGroup
	for shard := 1; shard <= of; shard++ {
		wg.Go(func() {
			code, out := testshard(dir, "-shard", strconv.Itoa(shard), "-of", strconv.Itoa(of))
			if code != 0 {
				t.Errorf("shard %d/%d exited %d:\n%s", shard, of, code, out)
			}
			outs[shard-1] = out
		})
	}
	wg.Wait()
	var ran []string
	for _, out := range outs {
		for _, m := range passed.FindAllStringSubmatch(out, -1) {
			ran = append(ran, m[1])
		}
	}
	slices.Sort(ran)
	want := []string{"Example", "FuzzSix", "TestFive", "TestFour", "TestOne", "TestOneMore", "TestThree", "TestTwo"}
	if !slices.Equal(ran, want) {
		t.Errorf("the Shards ran %v, want each of %v once", ran, want)
	}
}

func TestAFailingTestFailsTheShardWithGoTestsOutput(t *testing.T) {
	t.Parallel()
	files := twoPackages()
	files["b/b_test.go"] += "\nfunc TestBroken(t *testing.T) { t.Error(\"broken\") }\n"
	dir := module(t, files)
	code, out := testshard(dir, "-shard", "1", "-of", "1")
	if code == 0 {
		t.Fatalf("a run with a failing test exited 0:\n%s", out)
	}
	for _, want := range []string{"--- FAIL: TestBroken", "FAIL\texample.com/fixture/b", "ok  \texample.com/fixture/a"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output doesn't say %q:\n%s", want, out)
		}
	}
}

func TestAListingThatDoesNotBuildFailsWithTheBuildError(t *testing.T) {
	t.Parallel()
	files := twoPackages()
	files["b/b_test.go"] += "\nfunc TestUndefined(t *testing.T) { undefinedName() }\n"
	dir := module(t, files)
	code, out := testshard(dir, "-shard", "1", "-of", "2")
	if code == 0 {
		t.Fatalf("a listing that doesn't build exited 0:\n%s", out)
	}
	if !strings.Contains(out, "undefinedName") {
		t.Errorf("the output doesn't give the build error:\n%s", out)
	}
}

func TestDryRunPrintsTheCommandsWithoutRunningThem(t *testing.T) {
	t.Parallel()
	files := twoPackages()
	files["b/b_test.go"] += "\nfunc TestBroken(t *testing.T) { t.Error(\"broken\") }\n"
	dir := module(t, files)
	code, out := testshard(dir, "-shard", "1", "-of", "1", "-tags", "extra", "-dry-run")
	if code != 0 {
		t.Fatalf("a dry run exited %d:\n%s", code, out)
	}
	// Package c has no tests, so it isn't run.
	want := "go test -v -tags extra -run '^(Example|TestOne|TestOneMore|TestTwo)$' example.com/fixture/a\n" +
		"go test -v -tags extra -run '^(FuzzSix|TestBroken|TestFive|TestFour|TestThree)$' example.com/fixture/b\n"
	if out != want {
		t.Errorf("a dry run printed\n%s\nwant\n%s", out, want)
	}
}
