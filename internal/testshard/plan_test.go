package main

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"testing"
)

// ran returns, per package, the names in the listing that the invocations
// run, by matching each invocation's -run pattern against its package's
// names as `go test` would.
func ran(t *testing.T, tests map[string][]string, invs []invocation) map[string][]string {
	t.Helper()
	got := map[string][]string{}
	for _, inv := range invs {
		re, err := regexp.Compile(inv.Run)
		if err != nil {
			t.Fatalf("the -run pattern %q of %s doesn't compile: %v", inv.Run, inv.Package, err)
		}
		for _, name := range tests[inv.Package] {
			if re.MatchString(name) {
				got[inv.Package] = append(got[inv.Package], name)
			}
		}
	}
	for _, names := range got {
		slices.Sort(names)
	}
	return got
}

// realisticListing is a listing shaped like this repository's: packages of
// different sizes, with names that are prefixes of others.
func realisticListing() map[string][]string {
	tests := map[string][]string{}
	for p, size := range []int{1, 3, 12, 40, 150} {
		pkg := fmt.Sprintf("example.com/m/pkg%d", p)
		for i := range size {
			tests[pkg] = append(tests[pkg], fmt.Sprintf("TestBehaviour%d", i))
		}
		tests[pkg] = append(tests[pkg], "TestFoo", "TestFooBar", "ExampleFoo", "FuzzFoo")
	}
	return tests
}

// sortedNames is the listing with each package's names sorted, as ran
// returns them.
func sortedNames(tests map[string][]string) map[string][]string {
	out := map[string][]string{}
	for pkg, names := range tests {
		out[pkg] = slices.Sorted(slices.Values(names))
	}
	return out
}

func TestOneShardRunsEveryTest(t *testing.T) {
	t.Parallel()
	tests := realisticListing()
	got := ran(t, tests, plan(tests, 1, 1, defaultLimit))
	if want := sortedNames(tests); !maps.EqualFunc(got, want, slices.Equal[[]string]) {
		t.Errorf("-of 1 ran\n%v\nwant\n%v", got, want)
	}
}

func TestEveryTestRunsInExactlyOneShard(t *testing.T) {
	t.Parallel()
	tests := realisticListing()
	for of := 1; of <= 5; of++ {
		var all []invocation
		for shard := 1; shard <= of; shard++ {
			all = append(all, plan(tests, shard, of, defaultLimit)...)
		}
		// A test in two Shards is run twice, so it can't equal the listing.
		if got, want := ran(t, tests, all), sortedNames(tests); !maps.EqualFunc(got, want, slices.Equal[[]string]) {
			t.Errorf("-of %d: the Shards together ran\n%v\nwant each test once:\n%v", of, got, want)
		}
	}
}

func TestAPackageWithNoTestsInTheShardIsNotRun(t *testing.T) {
	t.Parallel()
	tests := realisticListing()
	// pkg0 has 5 tests, so at least one of 7 Shards has none of them.
	const of = 7
	skipped := 0
	for shard := 1; shard <= of; shard++ {
		invs := plan(tests, shard, of, defaultLimit)
		for _, inv := range invs {
			if len(ran(t, tests, []invocation{inv})[inv.Package]) == 0 {
				t.Errorf("shard %d/%d runs %s with %q, which matches none of its tests", shard, of, inv.Package, inv.Run)
			}
		}
		if !slices.ContainsFunc(invs, func(inv invocation) bool { return inv.Package == "example.com/m/pkg0" }) {
			skipped++
		}
	}
	if skipped == 0 {
		t.Errorf("every one of %d Shards runs pkg0, which has 5 tests", of)
	}
}

func TestANameRunsOnlyThatTestNotOnesItPrefixes(t *testing.T) {
	t.Parallel()
	tests := map[string][]string{"example.com/m/p": {"TestFoo", "TestFooBar", "TestFo"}}
	for of := 2; of <= 5; of++ {
		for shard := 1; shard <= of; shard++ {
			for _, inv := range plan(tests, shard, of, defaultLimit) {
				re := regexp.MustCompile(inv.Run)
				for _, name := range tests[inv.Package] {
					if re.MatchString(name) != (shardOf(inv.Package, name, of) == shard) {
						t.Errorf("shard %d/%d: %q matching %s is %v", shard, of, inv.Run, name, re.MatchString(name))
					}
				}
			}
		}
	}
}

func TestAPatternOverTheLimitIsSplitIntoSeveralRunsUnderIt(t *testing.T) {
	t.Parallel()
	const pkg = "example.com/m/big"
	tests := map[string][]string{pkg: nil}
	for i := range 2000 {
		tests[pkg] = append(tests[pkg], fmt.Sprintf("TestARatherLongBehaviourName%d", i))
	}
	for _, limit := range []int{defaultLimit, 1000, 100} {
		invs := plan(tests, 1, 1, limit)
		if len(invs) < 2 {
			t.Errorf("limit %d: %d invocations for %d names of about 32 characters", limit, len(invs), len(tests[pkg]))
		}
		for _, inv := range invs {
			if len(inv.Run) > limit {
				t.Errorf("limit %d: a -run pattern is %d characters", limit, len(inv.Run))
			}
		}
		if got, want := ran(t, tests, invs), sortedNames(tests); !maps.EqualFunc(got, want, slices.Equal[[]string]) {
			t.Errorf("limit %d: the invocations together don't run each test once", limit)
		}
	}
}

// TestTheAssignmentIsStable fixes which Shard each test of a listing runs
// in, worked out by an FNV-1a written apart from the tool's, so a change to
// the hash is a deliberate edit here.
func TestTheAssignmentIsStable(t *testing.T) {
	t.Parallel()
	const core, factory = "github.com/tomoram/code-central/core", "github.com/tomoram/code-central/factory"
	tests := map[string][]string{
		core:    {"TestA", "TestB", "TestC", "ExampleD", "FuzzE"},
		factory: {"TestLand", "TestRebase", "TestClaim", "TestFinish"},
	}
	want := []map[string][]string{
		{factory: {"TestRebase"}},
		{core: {"FuzzE", "TestA", "TestB"}, factory: {"TestFinish"}},
		{core: {"ExampleD", "TestC"}, factory: {"TestClaim", "TestLand"}},
	}
	for i, w := range want {
		if got := ran(t, tests, plan(tests, i+1, len(want), defaultLimit)); !maps.EqualFunc(got, w, slices.Equal[[]string]) {
			t.Errorf("shard %d/%d runs\n%v\nwant\n%v", i+1, len(want), got, w)
		}
	}
}
