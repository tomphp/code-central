package core

import (
	"cmp"
	"context"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/azdourl"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// A Project's Azure Boards settings (Spec code-central-894): the human
// turns Azure Boards on and off with set_azure_boards, which first runs
// the Azure Boards check, and creates the Project's Azure Boards Spec the
// first time it is turned on. Its work items are those a Saved query or a
// WIQL filter returns.

func azureBoardsView(p domain.Project, watcher api.WatcherView) api.AzureBoardsView {
	a := p.AzureBoards
	v := api.AzureBoardsView{On: a.On, Mode: azureBoardsModeOf(a), QueryURL: a.QueryURL, ProjectURL: a.ProjectURL, Where: a.Where,
		PATEnv: boardsPATEnvOf(p.CI, a), NewIssuesNeedMe: a.NewIssuesNeedMe, HideCodeCentral: a.HideCodeCentral, BranchFormat: a.BranchFormat,
		WorkItemStates: workItemStatesView(a.WorkItemStates), Watcher: watcher}
	if a.Spec != 0 {
		v.Spec = domain.DisplayID(p.Key, a.Spec)
	}
	return v
}

// azureBoardsModeOf is a's Mode, a Saved query unless the human chooses a
// WIQL filter.
func azureBoardsModeOf(a domain.AzureBoards) domain.AzureBoardsMode {
	return cmp.Or(a.Mode, domain.AzureBoardsSavedQuery)
}

// boardsPATEnvOf is a's PAT environment variable: the one it names, else
// the CI settings ci's.
func boardsPATEnvOf(ci domain.CI, a domain.AzureBoards) string {
	return cmp.Or(a.PATEnv, patEnvOf(ci))
}

// SetAzureBoards is the human's set_azure_boards: Azure Boards on or off,
// with its Mode and each Mode's settings, the PAT environment variable,
// New issues need me, Hide Code Central, the Branch format and the Work
// Item States, which no check runs for. Turning it on needs the chosen
// Mode's settings, and turning it on, or changing what it runs while on,
// passes the check. A Branch format is checked whether on or off.
// The settings of the Mode not chosen are kept. The first time it is
// turned on, it creates the Azure Boards Spec, written before the
// settings so they never name a missing Spec. A change of the chosen
// Mode's organization or project retitles the Azure Boards Spec unless
// the human has changed its title. Turning it off touches no Link.
func (c *Core) SetAzureBoards(ctx context.Context, key string, in api.SetAzureBoardsInput) (api.Written[api.ProjectView], error) {
	if err := foundation.HumanOnly(ctx, "set_azure_boards"); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	a := domain.AzureBoards{On: in.On, Mode: in.Mode, QueryURL: strings.TrimSpace(in.QueryURL),
		ProjectURL: strings.TrimSpace(in.ProjectURL), Where: strings.TrimSpace(in.Where), PATEnv: strings.TrimSpace(in.PATEnv), NewIssuesNeedMe: in.NewIssuesNeedMe, HideCodeCentral: in.HideCodeCentral,
		BranchFormat: strings.TrimSpace(in.BranchFormat), WorkItemStates: namedWorkItemStates(in.WorkItemStates)}
	if err := checkAzureBoardsSettings(&a); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	// Checked before the Azure Boards check and the Azure Boards Spec, so
	// a refused format leaves nothing behind.
	if err := c.store.View(func(r store.Reader) error {
		p, _ := r.Project(key)
		return checkBranchFormat(p, "branch_format", a.BranchFormat)
	}); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	if err := c.checkAzureBoards(ctx, key, a); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	created := 0
	if a.On {
		var err error
		created, err = c.createIntegrationSpec(key, func(p domain.Project) int { return p.AzureBoards.Spec },
			domain.Spec{Title: azureBoardsSpecTitle(a), Body: azureBoardsSpecBody})
		if err != nil {
			return api.Written[api.ProjectView]{}, err
		}
	}
	var was domain.AzureBoards
	var before api.ProjectView
	var p domain.Project
	var resetWatcher bool
	given := a
	err := c.store.Update(func(w store.Writer) error {
		a, resetWatcher = given, false
		if err := foundation.Writable(w, key); err != nil {
			return err
		}
		p, _ = w.Project(key)
		before, was = c.projectView(p), p.AzureBoards
		a.Spec = cmp.Or(was.Spec, created)
		a.WorkItemStates = withSince(a.WorkItemStates, was.WorkItemStates, c.now())
		// Naming the variable the CI settings name keeps following them.
		if a.PATEnv == patEnvOf(p.CI) {
			a.PATEnv = ""
		}
		p.AzureBoards = a
		if a == was {
			return nil
		}
		// What the Work Item Watcher said was about the old settings.
		if !a.On || azureBoardsCheckInput(p.CI, a) != azureBoardsCheckInput(p.CI, was) || a.WorkItemStates != was.WorkItemStates {
			resetWatcher = true
		}
		return w.PutProject(p)
	})
	if err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	if resetWatcher {
		c.watchers.reset(watcherKey{api.WatcherWorkItems, key})
	}
	after := c.projectView(p)
	if title := azureBoardsSpecTitle(a); was.Spec != 0 && title != "" && title != azureBoardsSpecTitle(was) {
		if err := c.retitleIntegrationSpec(ctx, key, was.Spec, givenAzureBoardsSpecTitle, title); err != nil {
			return api.Written[api.ProjectView]{}, err
		}
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

// checkAzureBoardsSettings validates a's settings, every Mode's, and
// replaces each URL with the one it parses to. Turning Azure Boards on
// needs the chosen Mode's: a Saved query's URL, or a WIQL filter's
// project URL and WHERE clause.
func checkAzureBoardsSettings(a *domain.AzureBoards) error {
	mode := azureBoardsModeOf(*a)
	if !mode.Valid() {
		return api.InvalidField("mode", "Azure Boards mode %q isn't %q or %q", a.Mode, domain.AzureBoardsSavedQuery, domain.AzureBoardsWIQL)
	}
	// The default Mode is stored as none, so settings never set stay
	// unwritten.
	if mode == domain.AzureBoardsSavedQuery {
		a.Mode = ""
	}
	if a.QueryURL != "" {
		u, err := azdourl.ParseQueryURL(a.QueryURL)
		if err != nil {
			return api.InvalidField("query_url", "%v", err)
		}
		a.QueryURL = u
	}
	if a.ProjectURL != "" {
		u, err := azdourl.ParseProjectURL(a.ProjectURL)
		if err != nil {
			return api.InvalidField("project_url", "%v", err)
		}
		a.ProjectURL = u
	}
	if words := strings.Fields(a.Where); len(words) > 0 && (strings.EqualFold(words[0], "SELECT") || strings.EqualFold(words[0], "WHERE")) {
		return api.InvalidField("where", "Give only the WHERE clause, without SELECT or the WHERE keyword: Code Central wraps it, such as %s", exampleWhere)
	}
	if err := checkPATEnv(a.PATEnv); err != nil {
		return err
	}
	if !a.On {
		return nil
	}
	switch {
	case mode == domain.AzureBoardsSavedQuery && a.QueryURL == "":
		return api.InvalidField("query_url", "Azure Boards needs the saved query's URL, such as https://dev.azure.com/acme/web/_queries/query/<id>")
	case mode == domain.AzureBoardsWIQL && a.ProjectURL == "":
		return api.InvalidField("project_url", "A WIQL filter needs the Azure DevOps project it runs in, as its URL, such as https://dev.azure.com/acme/web")
	case mode == domain.AzureBoardsWIQL && a.Where == "":
		return api.InvalidField("where", "A WIQL filter needs a WHERE clause, such as %s", exampleWhere)
	}
	return nil
}

// exampleWhere is a WHERE clause for messages.
const exampleWhere = "[System.State] = 'Approved' AND [System.AssignedTo] = @Me"

// chosenAzureBoardsURL is the URL a's chosen Mode runs: the Saved
// query's or the WIQL filter's project.
func chosenAzureBoardsURL(a domain.AzureBoards) string {
	if azureBoardsModeOf(a) == domain.AzureBoardsWIQL {
		return a.ProjectURL
	}
	return a.QueryURL
}

// azureBoardsSpecTitle is the Azure Boards Spec's title for a, such as
// "Azure Boards: contoso/web", after the organization and project its
// chosen Mode runs in; empty while a names none.
func azureBoardsSpecTitle(a domain.AzureBoards) string {
	org, project := azdourl.Names(chosenAzureBoardsURL(a))
	if org == "" {
		return ""
	}
	return azureBoardsSpecName + ": " + org + "/" + project
}

// azureBoardsSpecName is what every Azure Boards Spec's title starts
// with.
const azureBoardsSpecName = "Azure Boards"

// givenAzureBoardsSpecTitle reports whether title is one Code Central gave
// an Azure Boards Spec, for some organization and project.
func givenAzureBoardsSpecTitle(title string) bool {
	named, given := strings.CutPrefix(title, azureBoardsSpecName+": ")
	org, project, ok := strings.Cut(named, "/")
	return given && ok && org != "" && project != ""
}

// azureBoardsSpecBody is the Azure Boards Spec's body. It names no query
// or project, which the title carries, so changing them leaves it as it
// is.
const azureBoardsSpecBody = "This Spec holds the Project's Azure Boards Incoming Issues: each work item its query returns arrives on it as a linked Draft Ticket, to triage with Keep as Draft, To Requirement or Won't fix. " +
	"Azure Boards is in the Azure Boards section of Project Options; turning it off leaves this Spec and its Tickets as they are.\n"

// azureBoardsCheckInput is what the check checks for a, in a Project
// whose CI settings are ci.
func azureBoardsCheckInput(ci domain.CI, a domain.AzureBoards) api.AzureBoardsCheckInput {
	in := api.AzureBoardsCheckInput{Mode: azureBoardsModeOf(a), PATEnv: boardsPATEnvOf(ci, a)}
	if in.Mode == domain.AzureBoardsWIQL {
		in.ProjectURL, in.Where = a.ProjectURL, a.Where
	} else {
		in.QueryURL = a.QueryURL
	}
	return in
}

// SetAzureBoardsCheck makes c run check when Azure Boards is turned on or
// what it runs changes while on. Call it before serving; without it,
// nothing is checked.
func (c *Core) SetAzureBoardsCheck(check api.AzureBoardsCheck) { c.azureBoardsCheck = check }

// checkAzureBoards runs the Azure Boards check when a turns Azure Boards
// on, or changes what it runs while on, and refuses with the reason and
// fix when it fails. It runs outside the store's lock, as the check calls
// Azure DevOps.
func (c *Core) checkAzureBoards(ctx context.Context, key string, a domain.AzureBoards) error {
	if !a.On || c.azureBoardsCheck == nil {
		return nil
	}
	var was, now api.AzureBoardsCheckInput
	var wasOn bool
	err := c.store.View(func(r store.Reader) error {
		if err := foundation.Writable(r, key); err != nil {
			return err
		}
		p, _ := r.Project(key)
		wasOn, was, now = p.AzureBoards.On, azureBoardsCheckInput(p.CI, p.AzureBoards), azureBoardsCheckInput(p.CI, a)
		return nil
	})
	if err != nil || wasOn && was == now {
		return err
	}
	res := c.azureBoardsCheck(ctx, now)
	if res.Problem == "" {
		return nil
	}
	return &api.Error{Code: api.Invalid, Field: "on", Message: res.Reason, Fix: res.Fix}
}
