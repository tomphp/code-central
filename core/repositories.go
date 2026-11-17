package core

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/tomoram/code-central/store"
)

// A Project's Repositories (Spec code-central-848, ADR 0046): the Home
// Repository, where the Factory runs, is implicit, and the human adds,
// edits, removes and reorders the others with the HumanOnly
// add_repository, edit_repository, remove_repository and
// set_landing_order. The list's order is the landing order.

// RepositoryView is one of a Project's Repositories as every Project read
// returns it, in landing order.
type RepositoryView struct {
	Name string `json:"name"`
	// Home marks the Home Repository, named by the Project Key.
	Home bool `json:"home"`
	// CloneURL is the remote the Repository is cloned from and pushed to:
	// the Push remote URL the Factory reported for the Home Repository,
	// empty until it has.
	CloneURL string `json:"clone_url"`
	// Base is the branch its work is cut from and lands on: the Project's
	// Base for the Home Repository.
	Base string `json:"base"`
	// LocalPath is where the Factory finds its clone, empty for beside
	// the Home Repository.
	LocalPath string `json:"local_path"`
	// CodeHost is the Code Host its Clone URL names, empty for none
	// Code Central supports.
	CodeHost CodeHost `json:"code_host"`
	// ChangedBy are the unfinished Tickets whose Changed Repositories
	// include it, which keep it from being removed. Only get_project and
	// list_projects fill them in.
	ChangedBy []string `json:"changed_by"`
	// Clone is its clone as the Factory last reported it, absent while no
	// Factory is reporting.
	Clone *CloneView `json:"clone,omitempty"`
}

// LocalOnly reports whether r is a Repository other than the Home with no
// Clone URL. The Home's is empty only until the Factory reports it, so it
// is never local-only.
func (r RepositoryView) LocalOnly() bool { return !r.Home && r.CloneURL == "" }

// repositoriesView is p's Repositories in landing order.
func repositoriesView(p store.Project) []RepositoryView {
	var out []RepositoryView
	for _, r := range p.RepositoryList() {
		v := RepositoryView{Name: r.Name, Home: r.Home, CloneURL: r.CloneURL, Base: r.Base, LocalPath: r.LocalPath, ChangedBy: []string{}}
		if r.Home {
			v.CloneURL, v.Base = p.CI.PushRemoteURL, p.Base()
		}
		v.CodeHost, _ = codeHostOf(v.CloneURL)
		out = append(out, v)
	}
	return out
}

// fillChangedBy fills in each of v's Repositories' ChangedBy from the
// Project's Tickets.
func fillChangedBy(r store.Reader, v *ProjectView) {
	for i := range v.Repositories {
		v.Repositories[i].ChangedBy = changedBy(r, v.Key, v.Repositories[i].Name)
	}
}

// changedBy is the unfinished Tickets whose Changed Repositories include
// the Repository named name.
func changedBy(r store.Reader, key, name string) []string {
	by := []string{}
	for _, t := range r.Tickets(key) {
		if !finished(t.State) && slices.Contains(changedRepositoryNames(key, t), name) {
			by = append(by, DisplayID(key, t.Number))
		}
	}
	return by
}

// repositoryNames is rs's Repository Names, in order.
func repositoryNames(rs []store.Repository) []string {
	var names []string
	for _, r := range rs {
		names = append(names, r.Name)
	}
	return names
}

// AddRepositoryInput is the human's add_repository(name, clone_url?,
// base?, local_path?, full?): it needs clone_url, local_path or both.
type AddRepositoryInput struct {
	// Name is the Repository Name, fixed once added.
	Name string `json:"name"`
	// CloneURL left out makes the Repository local-only: the Factory
	// never clones or pushes it, and adopts the clone at LocalPath.
	CloneURL string `json:"clone_url,omitempty"`
	// Base is main when left out.
	Base string `json:"base,omitempty"`
	// LocalPath is an absolute path to an existing clone; left out, the
	// Factory clones it beside the Home Repository.
	LocalPath string `json:"local_path,omitempty"`
	// CI is the Repository's CI entry; left out, it is filled in from the
	// Clone URL, Not watched for none or a host no CI Provider is on.
	CI *RepositoryCI `json:"ci,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// EditRepositoryInput is the human's edit_repository(name, clone_url?,
// base?, local_path?, full?): each field left out keeps its value. The
// name only finds the Repository: a Repository Name never changes.
type EditRepositoryInput struct {
	Name string `json:"name"`
	// CloneURL, set empty, clears it, making the Repository local-only.
	CloneURL *string `json:"clone_url,omitempty"`
	Base     *string `json:"base,omitempty"`
	// LocalPath, set empty, puts the clone back beside the Home
	// Repository. A Repository keeps a Clone URL or a local path.
	LocalPath *string `json:"local_path,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// RemoveRepositoryInput is the human's remove_repository(name, full?).
type RemoveRepositoryInput struct {
	Name string `json:"name"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SetLandingOrderInput is the human's set_landing_order(names, full?):
// every Repository Name once, in the new landing order.
type SetLandingOrderInput struct {
	Names []string `json:"names"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// AddRepository is the human's add_repository: the Repository joins the
// end of the landing order. Under Pull Request, the Pull Request
// Watcher's check must pass for it first.
func (c *Core) AddRepository(ctx context.Context, key string, in AddRepositoryInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "add_repository"); err != nil {
		return Written[ProjectView]{}, err
	}
	r := store.Repository{Name: strings.TrimSpace(in.Name), CloneURL: strings.TrimSpace(in.CloneURL), Base: strings.TrimSpace(in.Base), LocalPath: strings.TrimSpace(in.LocalPath)}
	if r.Base == "" {
		r.Base = store.DefaultRepositoryBase
	}
	if !store.ValidRepositoryName(r.Name) {
		return Written[ProjectView]{}, &Error{Code: Invalid, Field: "name", Message: fmt.Sprintf("%s isn't a Repository Name", quote(r.Name)),
			Fix: fmt.Sprintf("name it with lowercase letters, digits, -, _ and ., starting with a letter or digit, in at most %d characters, such as web", store.MaxRepositoryName)}
	}
	if err := checkRepositoryFields(r); err != nil {
		return Written[ProjectView]{}, err
	}
	entry := ciEntryFromCloneURL(r.Name, r.CloneURL)
	if in.CI != nil {
		var err error
		if entry, err = ciEntry(r.Name, *in.CI, "ci."); err != nil {
			return Written[ProjectView]{}, err
		}
	}
	add := func(rs []store.Repository) ([]store.Repository, error) {
		if i := slices.IndexFunc(rs, func(o store.Repository) bool { return o.Name == r.Name }); i >= 0 {
			what := "a Repository"
			if rs[i].Home {
				what = "the Home Repository"
			}
			return nil, &Error{Code: Invalid, Field: "name", Message: fmt.Sprintf("%s is already named %s", what, r.Name),
				Fix: "choose another Repository Name: each names one Repository, and never changes once added"}
		}
		return append(rs, r), nil
	}
	return c.updateRepositories(ctx, key, in.Full, r, true, &entry, add)
}

// EditRepository is the human's edit_repository: a Repository's Clone
// URL, Base and local path. The Home Repository's are the Factory's
// report, the Landing section's Base and where the Factory runs, so none
// is edited here.
func (c *Core) EditRepository(ctx context.Context, key string, in EditRepositoryInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "edit_repository"); err != nil {
		return Written[ProjectView]{}, err
	}
	name := strings.TrimSpace(in.Name)
	var r store.Repository
	err := c.store.View(func(rd store.Reader) error {
		if err := writable(rd, key); err != nil {
			return err
		}
		p, _ := rd.Project(key)
		rs := p.RepositoryList()
		i, err := repositoryNamed(key, rs, name)
		if err != nil {
			return err
		}
		if r = rs[i]; r.Home {
			return editHomeRefusal(in)
		}
		return nil
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	was := r.CloneURL
	if in.CloneURL != nil {
		r.CloneURL = strings.TrimSpace(*in.CloneURL)
	}
	if in.Base != nil {
		r.Base = strings.TrimSpace(*in.Base)
	}
	if in.LocalPath != nil {
		r.LocalPath = strings.TrimSpace(*in.LocalPath)
	}
	if err := checkRepositoryFields(r); err != nil {
		return Written[ProjectView]{}, err
	}
	edit := func(rs []store.Repository) ([]store.Repository, error) {
		i, err := repositoryNamed(key, rs, name)
		if err != nil {
			return nil, err
		}
		rs[i] = r
		return rs, nil
	}
	// Only a Clone URL naming another repository needs the Pull Request
	// check again.
	return c.updateRepositories(ctx, key, in.Full, r, NormalCloneURL(was) != NormalCloneURL(r.CloneURL), nil, edit)
}

// editHomeRefusal is why in can't edit the Home Repository.
func editHomeRefusal(in EditRepositoryInput) error {
	switch {
	case in.Base != nil:
		return &Error{Code: Invalid, Field: "base", Message: "the Home Repository's Base is the Project's Base",
			Fix: "change it in the Landing section of Project Options"}
	case in.CloneURL != nil:
		return &Error{Code: Invalid, Field: "clone_url", Message: "the Home Repository's Clone URL is the Push remote URL the Factory reports",
			Fix: "point the Push remote at the new URL in the Home Repository's clone, and the Factory reports it when it next starts"}
	case in.LocalPath != nil:
		return &Error{Code: Invalid, Field: "local_path", Message: "the Home Repository is the clone the Factory runs in, so it has no local path",
			Fix: "run the Factory in the clone you want as the Home Repository"}
	}
	return nil
}

// RemoveRepository is the human's remove_repository. The Factory leaves
// its clone and any worktrees on disk. Neither the Home Repository nor a
// Repository an unfinished Ticket has changed can be removed.
func (c *Core) RemoveRepository(ctx context.Context, key string, in RemoveRepositoryInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "remove_repository"); err != nil {
		return Written[ProjectView]{}, err
	}
	name := strings.TrimSpace(in.Name)
	return c.changeRepositories(key, in.Full, nil, func(r store.Reader, p store.Project) ([]store.Repository, error) {
		rs := p.RepositoryList()
		i, err := repositoryNamed(key, rs, name)
		if err != nil {
			return nil, err
		}
		if rs[i].Home {
			return nil, &Error{Code: Invalid, Field: "name", Message: fmt.Sprintf("%s is the Home Repository, where the Factory runs and the Project's settings live, so it can't be removed", name),
				Fix: "remove the other Repositories you no longer want; the Home Repository stays"}
		}
		if by := changedBy(r, key, name); len(by) > 0 {
			return nil, &Error{Code: Invalid, Field: "name", Message: fmt.Sprintf("%s can't be removed while unfinished Tickets have changed it: %s", name, strings.Join(by, ", ")),
				Fix: "finish or cancel those Tickets first, then remove it"}
		}
		return slices.Delete(rs, i, i+1), nil
	})
}

// SetLandingOrder is the human's set_landing_order: the Repositories in
// a new landing order, which must name each of them once.
func (c *Core) SetLandingOrder(ctx context.Context, key string, in SetLandingOrderInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "set_landing_order"); err != nil {
		return Written[ProjectView]{}, err
	}
	return c.changeRepositories(key, in.Full, nil, func(_ store.Reader, p store.Project) ([]store.Repository, error) {
		rs := p.RepositoryList()
		names := repositoryNames(rs)
		var out []store.Repository
		var problems []string
		for _, n := range in.Names {
			i := slices.Index(names, n)
			switch {
			case i < 0:
				problems = append(problems, fmt.Sprintf("there is no Repository %s", n))
			case slices.ContainsFunc(out, func(r store.Repository) bool { return r.Name == n }):
				problems = append(problems, fmt.Sprintf("%s is named twice", n))
			default:
				out = append(out, rs[i])
			}
		}
		for _, n := range names {
			if !slices.Contains(in.Names, n) {
				problems = append(problems, fmt.Sprintf("%s is left out", n))
			}
		}
		if len(problems) > 0 {
			return nil, &Error{Code: Invalid, Field: "names", Message: "the landing order must name every Repository once: " + strings.Join(problems, "; "),
				Fix: "list each of " + strings.Join(names, ", ") + " once, in the order their work should land"}
		}
		return out, nil
	})
}

// checkRepositoryFields refuses a Repository whose Clone URL, Base or
// local path can't be one, with how to put it right. It needs a Clone
// URL, a local path or both: with no Clone URL it is local-only.
func checkRepositoryFields(r store.Repository) error {
	switch {
	case r.CloneURL == "" && r.LocalPath == "":
		return &Error{Code: Invalid, Field: "clone_url", Message: "a Repository needs a Clone URL to clone from, or the local path of a clone it already has",
			Fix: "give its Clone URL, such as git@github.com:acme/web.git, or the local path of its clone, such as /home/you/src/web"}
	case strings.ContainsFunc(r.CloneURL, unicode.IsSpace):
		return &Error{Code: Invalid, Field: "clone_url", Message: fmt.Sprintf("%s isn't a Clone URL: it holds a space", quote(r.CloneURL)),
			Fix: "give the URL you'd pass to git clone, such as git@github.com:acme/web.git"}
	case r.Base == "":
		return &Error{Code: Invalid, Field: "base", Message: "a Repository needs a Base, the branch its work is cut from and lands on",
			Fix: "name its Base, such as " + store.DefaultRepositoryBase}
	}
	if err := checkBranchName(r.Base); err != nil {
		return &Error{Code: Invalid, Field: "base", Message: err.Error(), Fix: "name a branch git accepts, such as " + store.DefaultRepositoryBase}
	}
	if r.LocalPath != "" && !filepath.IsAbs(r.LocalPath) {
		return &Error{Code: Invalid, Field: "local_path", Message: fmt.Sprintf("the local path %s isn't absolute", quote(r.LocalPath)),
			Fix: "give the clone's full path, such as /home/you/src/web, or leave it out to clone it beside the Home Repository"}
	}
	return nil
}

// refuseListed refuses r when another Repository has its Clone URL, as
// normalised, or its local path. The Home Repository's Clone URL is the
// reported Push remote URL, so it can clash only once that is reported.
// An empty Clone URL clashes with none, so local-only Repositories sit
// side by side.
//
// The path check compares resolved clone paths: a Repository's local
// path, else beside the Home Repository's clone, at home, the clone path
// the Factory last reported, empty without one. Without it, only
// explicit local paths are compared, and the Home Repository's clone
// holds none.
func refuseListed(p store.Project, r store.Repository, home string) error {
	mine := resolvedClonePath(r, home)
	for _, o := range p.RepositoryList() {
		if o.Name == r.Name {
			continue
		}
		url := o.CloneURL
		if o.Home {
			url = p.CI.PushRemoteURL
		}
		if url != "" && r.CloneURL != "" && NormalCloneURL(url) == NormalCloneURL(r.CloneURL) {
			return &Error{Code: Invalid, Field: "clone_url", Message: fmt.Sprintf("%s is already listed as Repository %s, at %s", r.CloneURL, o.Name, url),
				Fix: "a repository is listed once: edit " + o.Name + " instead, or give this one's own Clone URL"}
		}
		theirs := resolvedClonePath(o, home)
		if o.Home {
			theirs = home
		}
		if mine == "" || theirs != mine {
			continue
		}
		who := "Repository " + o.Name
		if o.Home {
			who = "the Home Repository"
		}
		if r.LocalPath == "" {
			return &Error{Code: Invalid, Field: "local_path", Message: fmt.Sprintf("Repository %s's clone would be at %s, beside the Home Repository, where %s's clone already is", r.Name, mine, who),
				Fix: "give this Repository the local path of its own clone"}
		}
		return &Error{Code: Invalid, Field: "local_path", Message: fmt.Sprintf("%s's clone is already at %s", who, mine),
			Fix: "give this Repository's own clone, or leave the local path out to clone it beside the Home Repository"}
	}
	return nil
}

// resolvedClonePath is where r's clone is: its local path, else beside
// the Home Repository's clone at home, or empty when home is unknown.
func resolvedClonePath(r store.Repository, home string) string {
	switch {
	case r.LocalPath != "":
		return filepath.Clean(r.LocalPath)
	case home != "":
		return filepath.Join(filepath.Dir(home), r.Name)
	}
	return ""
}

// reportedHomeClone is the path of p's Home Repository's clone as the
// Factory's latest Repositories report says, empty while no Factory is
// reporting or none has.
func (c *Core) reportedHomeClone(p store.Project) string {
	hs := c.heartbeats
	hs.mu.Lock()
	defer hs.mu.Unlock()
	for _, r := range p.RepositoryList() {
		if r.Home {
			if v := hs.cloneView(p.Key, r.Name, c.clock.Now()); v != nil {
				return filepath.Clean(v.Path)
			}
		}
	}
	return ""
}

// repositoryNamed is the index of the Repository named name in rs.
func repositoryNamed(key string, rs []store.Repository, name string) (int, error) {
	i := slices.IndexFunc(rs, func(r store.Repository) bool { return r.Name == name })
	if i < 0 {
		return 0, &Error{Code: NotFound, Field: "name", Message: fmt.Sprintf("Project %s has no Repository %s", key, quote(name)),
			Fix: "name one of its Repositories: " + strings.Join(repositoryNames(rs), ", ")}
	}
	return i, nil
}

// updateRepositories writes the Repositories change makes, once r's Clone
// URL and resolved clone path are listed nowhere else and, under Pull
// Request, r has a Clone URL that, when checkHost, passes the Pull Request
// Watcher's check. With ci, r's new CI entry, it is written with them, once, while
// CI Tracking is on, it passes the CI Watcher's check. The checks call
// the Code Host, so they run outside the store's lock, between a read
// that refuses what the store alone can and the write, which checks
// again.
func (c *Core) updateRepositories(ctx context.Context, key string, full bool, r store.Repository, checkHost bool, ci *store.CIRepository, change func([]store.Repository) ([]store.Repository, error)) (Written[ProjectView], error) {
	apply := func(p store.Project) ([]store.Repository, error) {
		rs, err := change(p.RepositoryList())
		if err != nil {
			return nil, err
		}
		// Before the Code Host check, so an empty Clone URL never reads
		// as a host Pull Request doesn't support.
		if landingPolicyOf(p).EveryRepositoryNeedsCloneURL() && r.LocalOnly() {
			return nil, &Error{Code: Invalid, Field: "clone_url", Message: needsCloneURL, Fix: needsCloneURLFix}
		}
		return rs, refuseListed(p, r, c.reportedHomeClone(p))
	}
	var check *PullRequestCheckInput
	var checkCI func() error
	err := c.store.View(func(rd store.Reader) error {
		if err := writable(rd, key); err != nil {
			return err
		}
		p, _ := rd.Project(key)
		if _, err := apply(p); err != nil {
			return err
		}
		if checkHost && landingPolicyOf(p).RunsPullRequestWatcher() {
			check = &PullRequestCheckInput{GHPath: rd.Options().GHPath, PATEnv: patEnvOf(store.CI{PATEnv: p.Landing.PATEnv})}
		}
		if opts := rd.Options(); ci != nil && landingPolicyOf(p).RunsCIWatcher() {
			checkCI = func() error { return c.checkCIEntry(ctx, p, opts, *ci, "ci.") }
		}
		return nil
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	if check != nil {
		if err := c.checkRepositoryPullRequests(ctx, r.Name, r.CloneURL, *check); err != nil {
			return Written[ProjectView]{}, err
		}
	}
	if checkCI != nil {
		if err := checkCI(); err != nil {
			return Written[ProjectView]{}, err
		}
	}
	return c.changeRepositories(key, full, ci, func(_ store.Reader, p store.Project) ([]store.Repository, error) { return apply(p) })
}

// checkRepositoryPullRequests refuses a Repository whose Clone URL names
// no Code Host Pull Request supports, or fails the Pull Request
// Watcher's check, with the reason and fix.
func (c *Core) checkRepositoryPullRequests(ctx context.Context, name, url string, in PullRequestCheckInput) error {
	if in.CodeHost, in.Repository = codeHostOf(url); in.CodeHost == CodeHostNone {
		return &Error{Code: Invalid, Field: "clone_url", Message: url + " is on a host Pull Request doesn't support; it supports GitHub (github.com) and Azure Repos (dev.azure.com, ssh.dev.azure.com, *.visualstudio.com)",
			Fix: "give its GitHub or Azure Repos Clone URL, or land by Merge or Branch"}
	}
	if c.pullRequestCheck == nil {
		return nil
	}
	in.Repositories = []PullRequestCheckRepository{{Name: name, CodeHost: in.CodeHost, Repository: in.Repository}}
	res := c.pullRequestCheck(ctx, in)
	if len(res.Repositories) > 0 {
		res.Reason, res.Fix = res.Repositories[0].Message, res.Repositories[0].Fix
	}
	if res.Problem != "" {
		return &Error{Code: Invalid, Field: "clone_url", Message: res.Reason, Fix: res.Fix}
	}
	return nil
}

// changeRepositories writes the Repositories change makes of the
// Project's, with ci, when given, as the CI entry of its Repository,
// acknowledging the Project. A Repository no longer listed loses its CI
// entry.
func (c *Core) changeRepositories(key string, full bool, ci *store.CIRepository, change func(store.Reader, store.Project) ([]store.Repository, error)) (Written[ProjectView], error) {
	var before, after ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		before = c.projectView(p)
		rs, err := change(w, p)
		if err != nil {
			return err
		}
		was, wasCI := p.Repositories, p.CI
		p.SetRepositories(rs)
		names := repositoryNames(p.RepositoryList())
		p.CI.Repositories = slices.DeleteFunc(slices.Clone(p.CI.Repositories), func(e store.CIRepository) bool { return !slices.Contains(names, e.Name) })
		if ci != nil {
			p.CI.Repositories = withCIEntry(p.CI.Repositories, *ci)
		}
		after = c.projectView(p)
		if slices.Equal(was, p.Repositories) && p.CI.Equal(wasCI) {
			return nil
		}
		return w.PutProject(p)
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	return acknowledgeProject(before, after, full, c.now())
}

// NormalCloneURL is url as Clone URLs are compared: two that name one
// repository normalise the same, whether ssh, scp-like or https, with or
// without a user, a port, a trailing .git or a trailing slash, and with
// the host and path in any case. A GitHub or Azure Repos URL normalises
// to its Code Host's repository, so Azure's ssh and https forms match.
func NormalCloneURL(url string) string {
	url = strings.ToLower(strings.TrimSpace(url))
	if host, repo := codeHostOf(url); host != CodeHostNone {
		return string(host) + ":" + repo
	}
	// withoutUser is a host with any user@ dropped.
	withoutUser := func(host string) string { return host[strings.LastIndex(host, "@")+1:] }
	var host, path string
	scheme, rest, isURL := strings.Cut(url, "://")
	switch h, p, scp := strings.Cut(url, ":"); {
	case isURL && scheme == "file":
		path = rest
	case isURL:
		host, path, _ = strings.Cut(rest, "/")
		host, _, _ = strings.Cut(withoutUser(host), ":")
	case scp && !strings.ContainsAny(h, `/\`):
		host, path = withoutUser(h), p
	default:
		path = url
	}
	path = strings.TrimRight(strings.TrimSuffix(strings.TrimRight(path, "/"), ".git"), "/")
	return host + "/" + strings.TrimLeft(path, "/")
}
