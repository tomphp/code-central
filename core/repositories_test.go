package core_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// srcPath is a clone's absolute path on this OS: "/src/<name>" isn't
// absolute on Windows.
func srcPath(name string) string { return filepath.ToSlash(filepath.Join(os.TempDir(), "src", name)) }

// A Project's Repositories (Spec code-central-848): the Home Repository
// is implicit, and the human adds, edits, removes and reorders the
// others with the HumanOnly Repository operations.

// repositoryNames is the Project's Repository Names in landing order.
func repositoryNames(p core.ProjectView) []string {
	var out []string
	for _, r := range p.Repositories {
		out = append(out, r.Name)
	}
	return out
}

// repository is the Project's Repository named name.
func repository(t *testing.T, p core.ProjectView, name string) core.RepositoryView {
	t.Helper()
	i := slices.IndexFunc(p.Repositories, func(r core.RepositoryView) bool { return r.Name == name })
	if i < 0 {
		t.Fatalf("no Repository %s in %v", name, repositoryNames(p))
	}
	return p.Repositories[i]
}

func (f fixture) addRepository(t *testing.T, in core.AddRepositoryInput) core.ProjectView {
	t.Helper()
	in.Full = true
	v, err := f.core.AddRepository(human, "acme", in)
	if err != nil {
		t.Fatalf("add_repository %s: %v", in.Name, err)
	}
	return *v.Record
}

// wantFix is wantCode for a refusal that says how to put it right.
func wantFix(t *testing.T, err error, code core.Code, field string) {
	t.Helper()
	wantCode(t, err, code, field)
	var e *core.Error
	errors.As(err, &e)
	if e.Fix == "" {
		t.Fatalf("%s has no fix", e.Message)
	}
}

func TestAProjectListsItsHomeRepositoryNamedByItsKey(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	p := f.projectView(t, "acme")
	if got := repositoryNames(p); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("repositories = %v, want the Home Repository acme alone", got)
	}
	if home := p.Repositories[0]; !home.Home || home.CloneURL != "" || home.Base != "main" || home.CodeHost != core.CodeHostNone {
		t.Fatalf("home = %+v, want Home on main with no Clone URL until the Factory reports one", home)
	}

	// The Home Repository's Clone URL is the reported Push remote URL, and
	// its Base the Project's.
	if _, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/acme.git"}); err != nil {
		t.Fatal(err)
	}
	base := "trunk"
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingMerge, Base: &base}); err != nil {
		t.Fatal(err)
	}
	home := f.projectView(t, "acme").Repositories[0]
	if home.CloneURL != "git@github.com:acme/acme.git" || home.Base != "trunk" || home.CodeHost != core.CodeHostGitHub {
		t.Fatalf("home = %+v, want the Push remote URL on GitHub and the Project's Base trunk", home)
	}
}

func TestAddRepositoryListsItAfterTheOthersAndItReadsBack(t *testing.T) {
	f := reloaded(t, func(f fixture) {
		f.project(t, "acme")
		p := f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: " git@github.com:acme/web.git "})
		if web := repository(t, p, "web"); web.Home || web.CloneURL != "git@github.com:acme/web.git" || web.Base != "main" || web.LocalPath != "" || web.CodeHost != core.CodeHostGitHub {
			t.Fatalf("web = %+v, want its trimmed Clone URL on GitHub, with Base main", web)
		}
		f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "https://dev.azure.com/acme/shop/_git/api", Base: "develop", LocalPath: srcPath("api")})
	})
	p := f.projectView(t, "acme")
	if got := repositoryNames(p); !slices.Equal(got, []string{"acme", "web", "api"}) {
		t.Fatalf("repositories = %v, want acme, web, api in the order they were added", got)
	}
	if api := repository(t, p, "api"); api.Base != "develop" || api.LocalPath != srcPath("api") || api.CodeHost != core.CodeHostAzureRepos {
		t.Fatalf("api = %+v, want Base develop at /src/api on Azure Repos", api)
	}
}

func TestAddRepositoryAcknowledgesTheRepositories(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	w, err := f.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	if err != nil {
		t.Fatal(err)
	}
	// It brings its CI entry, filled in from its Clone URL.
	if w.Record != nil || w.ID != "acme" || !slices.Equal(w.Changed, []string{"ci", "repositories"}) {
		t.Fatalf("acknowledgement = %+v, want acme with ci and repositories changed", w)
	}
}

func TestAddRepositoryRefusesWhatBreaksItsRules(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if _, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/acme.git"}); err != nil {
		t.Fatal(err)
	}
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git", LocalPath: srcPath("web")})
	for name, tc := range map[string]struct {
		in    core.AddRepositoryInput
		field string
	}{
		"a name with capitals":            {core.AddRepositoryInput{Name: "Api", CloneURL: "git@github.com:acme/api.git"}, "name"},
		"no name":                         {core.AddRepositoryInput{CloneURL: "git@github.com:acme/api.git"}, "name"},
		"a name already listed":           {core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/api.git"}, "name"},
		"the Home Repository's name":      {core.AddRepositoryInput{Name: "acme", CloneURL: "git@github.com:acme/api.git"}, "name"},
		"no Clone URL nor local path":     {core.AddRepositoryInput{Name: "api"}, "clone_url"},
		"a Clone URL already listed":      {core.AddRepositoryInput{Name: "api", CloneURL: "https://github.com/Acme/Web"}, "clone_url"},
		"the Home Repository's Clone URL": {core.AddRepositoryInput{Name: "api", CloneURL: "https://github.com/acme/acme.git/"}, "clone_url"},
		"a bad Base":                      {core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git", Base: "a..b"}, "base"},
		"a relative local path":           {core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git", LocalPath: "src/api"}, "local_path"},
		"a local path another one is at":  {core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git", LocalPath: srcPath("web") + "/"}, "local_path"},
		"a Clone URL with a space in it":  {core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/my api.git"}, "clone_url"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.core.AddRepository(human, "acme", tc.in)
			wantFix(t, err, core.Invalid, tc.field)
		})
	}
	if got := repositoryNames(f.projectView(t, "acme")); !slices.Equal(got, []string{"acme", "web"}) {
		t.Fatalf("repositories = %v after refusals, want acme, web", got)
	}
}

func TestTheRepositoryOperationsAreTheHumansAndRefuseAnArchivedProject(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	_, err := f.core.AddRepository(ctx, "acme", core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	wantCode(t, err, core.ActionNotAllowed, "")
	if _, err := f.core.ArchiveProject(human, "acme"); err != nil {
		t.Fatal(err)
	}
	_, err = f.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	wantCode(t, err, core.ActionNotAllowed, "")
}

func TestEditRepositoryChangesItsCloneURLBaseAndLocalPath(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git", LocalPath: srcPath("web")})
	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	url, base := "https://dev.azure.com/acme/shop/_git/web", "develop"
	v, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", CloneURL: &url, Base: &base, Full: true})
	if err != nil {
		t.Fatal(err)
	}
	web := repository(t, *v.Record, "web")
	if web.CloneURL != url || web.Base != "develop" || web.LocalPath != srcPath("web") || web.CodeHost != core.CodeHostAzureRepos {
		t.Fatalf("web = %+v, want the new Clone URL and Base, and its local path kept", web)
	}
	// An empty local path puts it back beside the Home Repository.
	none := ""
	v, err = f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", LocalPath: &none, Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if web := repository(t, *v.Record, "web"); web.LocalPath != "" || web.Base != "develop" {
		t.Fatalf("web = %+v, want no local path", web)
	}
	if got := repositoryNames(*v.Record); !slices.Equal(got, []string{"acme", "web", "api"}) {
		t.Fatalf("repositories = %v, want the order kept", got)
	}
}

func TestEditRepositoryRefusesTheHomeRepositoryAndWhatBreaksTheRules(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	s := func(v string) *string { return &v }
	for name, tc := range map[string]struct {
		in    core.EditRepositoryInput
		field string
	}{
		"the Home Repository's Base":       {core.EditRepositoryInput{Name: "acme", Base: s("trunk")}, "base"},
		"the Home Repository's Clone URL":  {core.EditRepositoryInput{Name: "acme", CloneURL: s("git@github.com:acme/x.git")}, "clone_url"},
		"the Home Repository's local path": {core.EditRepositoryInput{Name: "acme", LocalPath: s(srcPath("acme"))}, "local_path"},
		"another's Clone URL":              {core.EditRepositoryInput{Name: "web", CloneURL: s("ssh://git@github.com/acme/api")}, "clone_url"},
		"no Clone URL nor local path":      {core.EditRepositoryInput{Name: "web", CloneURL: s(" ")}, "clone_url"},
		"a bad Base":                       {core.EditRepositoryInput{Name: "web", Base: s("-x")}, "base"},
		"no Base":                          {core.EditRepositoryInput{Name: "web", Base: s("")}, "base"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.core.EditRepository(human, "acme", tc.in)
			wantFix(t, err, core.Invalid, tc.field)
		})
	}
	_, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "shop", Base: s("main")})
	wantFix(t, err, core.NotFound, "name")
	// Its own Clone URL, written another way, is no clash.
	if _, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", CloneURL: s("https://github.com/acme/web")}); err != nil {
		t.Fatalf("edit web to its own Clone URL: %v", err)
	}
}

// A Repository other than the Home needs a Clone URL, a local path or
// both (Spec code-central-879): one with no Clone URL is local-only.

func TestAddRepositoryWithOnlyALocalPathIsLocalOnly(t *testing.T) {
	f := reloaded(t, func(f fixture) {
		f.project(t, "acme")
		f.addRepository(t, core.AddRepositoryInput{Name: "web", LocalPath: srcPath("web")})
		// The duplicate check skips an empty Clone URL, so two local-only
		// Repositories sit side by side.
		f.addRepository(t, core.AddRepositoryInput{Name: "api", LocalPath: srcPath("api")})
	})
	p := f.projectView(t, "acme")
	for _, name := range []string{"web", "api"} {
		if r := repository(t, p, name); r.CloneURL != "" || r.CodeHost != core.CodeHostNone || r.LocalPath != srcPath(name) || r.Base != "main" {
			t.Fatalf("%s = %+v, want no Clone URL nor Code Host, at its local path with Base main", name, r)
		}
	}
}

func TestARepositoryWithNeitherCloneURLNorLocalPathIsRefusedWithTheFix(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	_, err := f.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "web", CloneURL: " ", LocalPath: " "})
	wantFix(t, err, core.Invalid, "clone_url")
	var e *core.Error
	errors.As(err, &e)
	if !strings.Contains(e.Message, "a Clone URL to clone from, or the local path of a clone it already has") {
		t.Fatalf("message %q, want it to say a Repository needs a Clone URL or the local path of its clone", e.Message)
	}
	if !strings.Contains(e.Fix, "Clone URL") || !strings.Contains(e.Fix, "local path") {
		t.Fatalf("fix %q, want it to say to give one or the other", e.Fix)
	}
}

func TestEditRepositoryClearsACloneURLAndGivesItOneAgain(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git", LocalPath: srcPath("web")})
	none, url := "", "git@github.com:acme/web.git"
	v, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", CloneURL: &none, Full: true})
	if err != nil {
		t.Fatalf("clear web's Clone URL: %v", err)
	}
	if web := repository(t, *v.Record, "web"); web.CloneURL != "" || web.CodeHost != core.CodeHostNone || web.LocalPath != srcPath("web") {
		t.Fatalf("web = %+v, want no Clone URL nor Code Host, and its local path kept", web)
	}
	// Now local-only, its local path can't be cleared too.
	_, err = f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", LocalPath: &none})
	wantFix(t, err, core.Invalid, "clone_url")
	v, err = f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", CloneURL: &url, Full: true})
	if err != nil {
		t.Fatalf("give web its Clone URL again: %v", err)
	}
	if web := repository(t, *v.Record, "web"); web.CloneURL != url || web.CodeHost != core.CodeHostGitHub {
		t.Fatalf("web = %+v, want its Clone URL back on GitHub", web)
	}
	// With a Clone URL, its local path can be cleared, and then its Clone
	// URL can't.
	if _, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", LocalPath: &none}); err != nil {
		t.Fatalf("clear web's local path: %v", err)
	}
	_, err = f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", CloneURL: &none})
	wantFix(t, err, core.Invalid, "clone_url")
	if web := repository(t, f.projectView(t, "acme"), "web"); web.CloneURL != url || web.LocalPath != "" {
		t.Fatalf("web = %+v after refusals, want its Clone URL and no local path", web)
	}
}

func TestRemoveRepositoryTakesItOffTheListButNeverTheHomeRepository(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})

	_, err := f.core.RemoveRepository(human, "acme", core.RemoveRepositoryInput{Name: "acme"})
	wantFix(t, err, core.Invalid, "name")
	_, err = f.core.RemoveRepository(human, "acme", core.RemoveRepositoryInput{Name: "shop"})
	wantFix(t, err, core.NotFound, "name")

	v, err := f.core.RemoveRepository(human, "acme", core.RemoveRepositoryInput{Name: "web", Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := repositoryNames(*v.Record); !slices.Equal(got, []string{"acme", "api"}) {
		t.Fatalf("repositories = %v, want acme, api", got)
	}
	if _, err := f.core.RemoveRepository(human, "acme", core.RemoveRepositoryInput{Name: "api"}); err != nil {
		t.Fatal(err)
	}
	if got := repositoryNames(f.projectView(t, "acme")); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("repositories = %v, want the Home Repository alone", got)
	}
	// A removed Repository's name and Clone URL are free to add again.
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
}

func TestSetLandingOrderTakesAPermutationOfTheRepositories(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})

	v, err := f.core.SetLandingOrder(human, "acme", core.SetLandingOrderInput{Names: []string{"api", "acme", "web"}, Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := repositoryNames(*v.Record); !slices.Equal(got, []string{"api", "acme", "web"}) {
		t.Fatalf("repositories = %v, want api, acme, web", got)
	}
	for name, names := range map[string][]string{
		"one left out":  {"api", "acme"},
		"an unknown":    {"api", "acme", "web", "shop"},
		"one twice":     {"api", "acme", "web", "web"},
		"none":          nil,
		"one for other": {"api", "acme", "shop"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.core.SetLandingOrder(human, "acme", core.SetLandingOrderInput{Names: names})
			wantFix(t, err, core.Invalid, "names")
		})
	}
	if got := repositoryNames(f.projectView(t, "acme")); !slices.Equal(got, []string{"api", "acme", "web"}) {
		t.Fatalf("repositories = %v after refusals, want api, acme, web", got)
	}
}

func TestAddRepositoryUnderPullRequestRunsTheWatchersCheckForIt(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if _, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/acme.git"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest}); err != nil {
		t.Fatal(err)
	}
	var checked []core.PullRequestCheckInput
	f.core.SetPullRequestCheck(func(_ context.Context, in core.PullRequestCheckInput) core.WatcherCheckResult {
		checked = append(checked, in)
		if in.Repository == "acme/private" {
			return core.WatcherCheckResult{Problem: core.ProblemRepositoryUnreadable, Reason: "gh can't see acme/private", Fix: "grant gh access to acme/private"}
		}
		return core.WatcherCheckResult{}
	})

	_, err := f.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "web", CloneURL: "https://gitlab.com/acme/web.git"})
	wantFix(t, err, core.Invalid, "clone_url")

	_, err = f.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "private", CloneURL: "git@github.com:acme/private.git"})
	wantFix(t, err, core.Invalid, "clone_url")
	if e := (*core.Error)(nil); errors.As(err, &e) && (e.Message != "gh can't see acme/private" || e.Fix != "grant gh access to acme/private") {
		t.Fatalf("refusal = %q, fix %q, want the check's reason and fix", e.Message, e.Fix)
	}

	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "https://dev.azure.com/acme/shop/_git/web"})
	if last := checked[len(checked)-1]; last.CodeHost != core.CodeHostAzureRepos || last.Repository != "acme/shop/web" || last.PATEnv != store.DefaultPATEnv {
		t.Fatalf("checked %+v, want Azure Repos acme/shop/web with the PAT variable", last)
	}

	// An edit checks again only when its Clone URL names another repository.
	n := len(checked)
	base, same, private := "develop", "git@ssh.dev.azure.com:v3/acme/shop/web", "git@github.com:acme/private.git"
	if _, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", Base: &base, CloneURL: &same}); err != nil {
		t.Fatalf("edit web's Base: %v", err)
	}
	if len(checked) != n {
		t.Fatalf("editing the Base checked %+v again", checked[n:])
	}
	_, err = f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", CloneURL: &private})
	wantFix(t, err, core.Invalid, "clone_url")
}

func TestUnderPullRequestAddingOrEditingALocalOnlyRepositoryIsRefusedBeforeTheCodeHostCheck(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if _, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/acme.git"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest}); err != nil {
		t.Fatal(err)
	}
	checked := 0
	f.core.SetPullRequestCheck(func(context.Context, core.PullRequestCheckInput) core.WatcherCheckResult {
		checked++
		return core.WatcherCheckResult{}
	})
	wantNeedsCloneURL := func(t *testing.T, err error) {
		t.Helper()
		wantFix(t, err, core.Invalid, "clone_url")
		if e := (*core.Error)(nil); errors.As(err, &e) && (e.Message != "Pull Request needs a Clone URL in every Repository" || e.Fix != "give its Clone URL, or land by Merge or Branch") {
			t.Fatalf("refusal = %q, fix %q, want Pull Request's need for a Clone URL", e.Message, e.Fix)
		}
	}

	_, err := f.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "notes", LocalPath: srcPath("notes")})
	wantNeedsCloneURL(t, err)

	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git", LocalPath: srcPath("web")})
	empty := ""
	_, err = f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", CloneURL: &empty})
	wantNeedsCloneURL(t, err)
	if checked != 1 {
		t.Errorf("the Pull Request check ran %d times, want once, for web's add alone", checked)
	}
	v := f.projectView(t, "acme")
	if got := repositoryNames(v); !slices.Equal(got, []string{"acme", "web"}) {
		t.Fatalf("repositories = %v, want acme, web", got)
	}
	if got := repository(t, v, "web").CloneURL; got != "git@github.com:acme/web.git" {
		t.Fatalf("web's Clone URL = %q after the refusal, want it kept", got)
	}
}

func TestCloneURLsNormaliseAcrossSchemesAndSuffixes(t *testing.T) {
	same := [][]string{
		{"git@github.com:acme/web.git", "https://github.com/acme/web", "ssh://git@github.com/Acme/Web.git/", "https://user@GitHub.com/acme/web.git"},
		{"git@ssh.dev.azure.com:v3/acme/shop/web", "https://acme@dev.azure.com/acme/shop/_git/web", "https://acme.visualstudio.com/shop/_git/web"},
		{"git@gitlab.example.com:team/web.git", "https://gitlab.example.com/team/web", "ssh://git@gitlab.example.com:2222/team/web"},
	}
	for _, group := range same {
		for _, u := range group[1:] {
			if a, b := core.NormalCloneURL(group[0]), core.NormalCloneURL(u); a != b {
				t.Errorf("%s normalises to %q and %s to %q, want the same", group[0], a, u, b)
			}
		}
	}
	for _, pair := range [][2]string{
		{"git@github.com:acme/web.git", "git@github.com:acme/api.git"},
		{"git@github.com:acme/web.git", "git@gitlab.com:acme/web.git"},
	} {
		if core.NormalCloneURL(pair[0]) == core.NormalCloneURL(pair[1]) {
			t.Errorf("%s and %s normalise the same", pair[0], pair[1])
		}
	}
	if got := core.NormalCloneURL(" /srv/git/web.git/ "); !strings.HasSuffix(got, "/srv/git/web") {
		t.Errorf("a local path normalises to %q", got)
	}
}
