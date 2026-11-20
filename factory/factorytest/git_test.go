package factorytest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// onEachGit runs test on real git and on the Fake git, in parallel.
func onEachGit(t *testing.T, test func(t *testing.T, opt Option)) {
	for _, g := range []struct {
		name string
		opt  Option
	}{{"real git", RealGit()}, {"Fake git", FakeGit()}} {
		t.Run(g.name, func(t *testing.T) {
			t.Parallel()
			test(t, g.opt)
		})
	}
}

func TestAnAcceptedTicketLandsAlikeOnRealGitAndTheFakeGit(t *testing.T) {
	t.Parallel()
	onEachGit(t, func(t *testing.T, opt Option) {
		h := New(t, opt)
		id := h.ReadyTicket("Add a feature")
		if h.Exists(id) {
			t.Fatalf("%s exists before CommitOn", id)
		}
		commit := h.CommitOn(id, "feature.txt", "feature\n")
		if !h.Exists(id) || h.Tip(id) != commit {
			t.Fatalf("%s isn't at the commit %s", id, commit)
		}
		if got, want := h.Log(id), []string{"Change feature.txt", "Set up Code Central"}; !slices.Equal(got, want) {
			t.Errorf("Log(%s) = %q, want %q", id, got, want)
		}
		h.Finished(id, id, commit)
		h.Accept(id)
		oldMain := h.Tip("main")

		h.Start()
		h.WantState(id, domain.Done)

		merge := h.Tip("main")
		if got, want := h.Parents(merge), []string{oldMain, commit}; !slices.Equal(got, want) {
			t.Errorf("main's tip has parents %q, want %q", got, want)
		}
		if got, want := h.Log("main"), []string{"Merge " + id + ": Add a feature", "Set up Code Central"}; !slices.Equal(got, want) {
			t.Errorf("Log(main) = %q, want %q", got, want)
		}
		if got := h.File("main", "feature.txt"); got != "feature\n" {
			t.Errorf("main's feature.txt is %q", got)
		}
		// main is checked out in the repo, so it was fast-forwarded there.
		if b, err := os.ReadFile(filepath.Join(h.Repo, "feature.txt")); err != nil || string(b) != "feature\n" {
			t.Errorf("feature.txt in the checkout: %q, %v", b, err)
		}
	})
}

func TestARefusedPushAndItsRetryAreAlikeOnRealGitAndTheFakeGit(t *testing.T) {
	t.Parallel()
	onEachGit(t, func(t *testing.T, opt Option) {
		h := New(t, opt)
		remote := h.AddRemote("origin")
		h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, Push: true, Remote: "origin"})
		id := h.ReadyTicket("Add a feature")
		commit := h.CommitOn(id, "feature.txt", "feature\n")
		h.Finished(id, id, commit)
		h.Accept(id)
		remote.RefusePushes()

		r := h.Start()
		r.WantOutput(id + ": can't land yet")
		if remote.Exists("main") {
			t.Errorf("the remote has main at %s though it refuses pushes", remote.Tip("main"))
		}

		remote.AcceptPushes()
		r.Poll()
		h.WantState(id, domain.Done)
		merge := h.Tip("main")
		if got := remote.Tip("main"); got != merge {
			t.Errorf("the remote's main is at %s, want the merge %s", got, merge)
		}
		if got := h.RemoteTip("origin", "main"); got != merge {
			t.Errorf("origin/main is at %s, want the merge %s", got, merge)
		}
		if got, want := remote.Parents(merge), h.Parents(merge); !slices.Equal(got, want) {
			t.Errorf("the remote's merge has parents %q, want %q", got, want)
		}
		if got, want := remote.Log("main"), h.Log("main"); !slices.Equal(got, want) {
			t.Errorf("the remote's Log(main) = %q, want %q", got, want)
		}
		if got := remote.File("main", "feature.txt"); got != "feature\n" {
			t.Errorf("the remote's feature.txt is %q", got)
		}
	})
}

func TestAMergeByHandIsAlikeOnRealGitAndTheFakeGit(t *testing.T) {
	t.Parallel()
	onEachGit(t, func(t *testing.T, opt Option) {
		h := New(t, opt)
		topic := h.CommitOn("topic", "topic.txt", "topic\n")
		base := h.CommitOn("main", "main.txt", "main\n")
		other := h.CommitOn("other", "other.txt", "other\n")

		merge := h.MergeByHand("topic", "main")
		if got, want := h.Parents(merge), []string{base, topic}; !slices.Equal(got, want) {
			t.Errorf("the merge has parents %q, want %q", got, want)
		}
		if got, want := h.Log("main"), []string{"Merge topic", "Change main.txt", "Set up Code Central"}; !slices.Equal(got, want) {
			t.Errorf("Log(main) = %q, want %q", got, want)
		}
		if b, err := os.ReadFile(filepath.Join(h.Repo, "topic.txt")); err != nil || string(b) != "topic\n" {
			t.Errorf("topic.txt in the checkout: %q, %v", b, err)
		}
		if again := h.MergeByHand("topic", "main"); again != merge {
			t.Errorf("merging topic again made %s: main has it already", again)
		}

		into := h.MergeByHand("main", "other")
		if got, want := h.Parents(into), []string{other, merge}; !slices.Equal(got, want) {
			t.Errorf("the merge into other has parents %q, want %q", got, want)
		}
		if h.Tip("other") != into || h.Tip("main") != merge {
			t.Errorf("other is at %s and main at %s, want %s and %s", h.Tip("other"), h.Tip("main"), into, merge)
		}
		if got := h.File("other", "topic.txt"); got != "topic\n" {
			t.Errorf("other's topic.txt is %q", got)
		}
	})
}

func TestASecondRepositoryHasTheTypedHelpersOnRealGitAndTheFakeGit(t *testing.T) {
	t.Parallel()
	onEachGit(t, func(t *testing.T, opt Option) {
		h := New(t, opt)
		repo := h.AddLocalRepository("api")
		commit := repo.CommitOn("feature", "api.txt", "api\n")
		if !repo.Exists("feature") || repo.Tip("feature") != commit {
			t.Fatalf("feature isn't at %s in api", commit)
		}
		if h.Exists("feature") {
			t.Errorf("CommitOn in api made feature in the Home Repository")
		}
		merge := repo.MergeByHand("feature", "main")
		if got := repo.Parents(merge); len(got) != 2 || got[1] != commit {
			t.Errorf("api's merge has parents %q", got)
		}
		if got, want := repo.Log("main"), []string{"Merge feature", "Start api"}; !slices.Equal(got, want) {
			t.Errorf("api's Log(main) = %q, want %q", got, want)
		}
		if got := repo.File("main", "api.txt"); got != "api\n" {
			t.Errorf("api's api.txt is %q", got)
		}

		made := h.MakeRepository("web")
		if got, want := made.Remote("origin").Log("main"), []string{"Start web"}; !slices.Equal(got, want) {
			t.Errorf("web's remote Log(main) = %q, want %q", got, want)
		}
	})
}

// fatalT is a testing.TB that records what fails the test and ends the
// goroutine there, as Fatal does.
type fatalT struct {
	testing.TB
	mu    sync.Mutex
	fatal string
}

func (f *fatalT) Helper() {}
func (f *fatalT) Fatalf(format string, args ...any) {
	f.mu.Lock()
	f.fatal = fmt.Sprintf(format, args...)
	f.mu.Unlock()
	runtime.Goexit()
}
func (f *fatalT) Fatal(args ...any) { f.Fatalf("%s", fmt.Sprint(args...)) }

// failure runs do on its own goroutine, as h's test, and returns what
// failed it.
func (f *fatalT) failure(do func()) string {
	done := make(chan struct{})
	go func() {
		defer close(done)
		do()
	}()
	<-done
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fatal
}

func TestRawGitFailsUnderTheFakeGitNamingTheTypedHelpersAndRealGit(t *testing.T) {
	t.Parallel()
	h := New(t, FakeGit())
	if h.fake == nil {
		t.Skip(gitEnv + "=real runs every Harness on real git")
	}
	remote := h.AddRemote("origin")
	for name, raw := range map[string]func(){
		"Git":        func() { h.Git("rev-parse", "main") },
		"GitIn":      func() { h.GitIn(h.Repo, "rev-parse", "main") },
		"Remote.Git": func() { remote.Git("rev-parse", "main") },
	} {
		ft := &fatalT{TB: t}
		h.t = ft
		got := ft.failure(raw)
		for _, want := range []string{"git rev-parse main", "Tip, Exists, Log, Parents, File, RemoteTip, CommitOn, MergeByHand", "RealGit()"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s under the Fake git failed with %q, want it to name %q", name, got, want)
			}
		}
	}
	h.t = t
}

func TestTheGitSwitchRunsATestThatChoseTheFakeGitOnRealGit(t *testing.T) {
	t.Setenv(gitEnv, "real")
	h := New(t, FakeGit())
	if h.fake != nil {
		t.Fatalf("%s=real, but the Harness is on the Fake git", gitEnv)
	}
	if got, want := h.Git("rev-parse", "main"), h.Tip("main"); got != want {
		t.Errorf("real git's main is %s, want %s", got, want)
	}
}

func TestTheGitSwitchTakesOnlyReal(t *testing.T) {
	t.Setenv(gitEnv, "fake")
	ft := &fatalT{TB: t}
	got := ft.failure(func() { useFakeGit(ft, []Option{FakeGit()}) })
	if !strings.Contains(got, gitEnv) || !strings.Contains(got, `"real"`) {
		t.Errorf("%s=fake failed with %q, want it to name the switch and real", gitEnv, got)
	}
}

func TestTheLastGitOptionWinsAndRealGitIsTheDefault(t *testing.T) {
	t.Setenv(gitEnv, "")
	for _, c := range []struct {
		opts []Option
		fake bool
	}{
		{nil, false},
		{[]Option{FakeGit()}, true},
		{[]Option{FakeGit(), RealGit()}, false},
		{[]Option{RealGit(), FakeGit()}, true},
	} {
		if got := useFakeGit(t, c.opts); got != c.fake {
			t.Errorf("%d options: on the Fake git %v, want %v", len(c.opts), got, c.fake)
		}
	}
}

func TestGitVersionSetsTheFakeGitsVersion(t *testing.T) {
	t.Parallel()
	h := New(t, FakeGit())
	if h.fake == nil {
		t.Skip(gitEnv + "=real runs every Harness on real git")
	}
	h.GitVersion("2.37.9")
	if v, err := h.fake.Version(); err != nil || v != "git version 2.37.9" {
		t.Errorf("the Fake git's version is %q, %v", v, err)
	}
}
