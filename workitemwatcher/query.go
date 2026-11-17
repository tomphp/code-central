// Running the query: a Saved query by its ID, or a WIQL filter wrapped in
// a whole WIQL query, and each way it can fail as a problem with its fix.

package workitemwatcher

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strings"

	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/azdourl"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/ratelimit"
	"github.com/tomoram/code-central/store"
)

// MaxWorkItems is the most work items a query may match: more is an
// error that asks the human to narrow it, so a careless filter doesn't
// flood Code Central.
const MaxWorkItems = 1000

// scope is the PAT scope the Work Item Watcher needs, as Azure DevOps
// names it.
const scope = "Work Items: Read & write"

// wiqlQuery is the whole WIQL query a WIQL filter's WHERE clause runs as.
func wiqlQuery(where string) string { return "SELECT [System.Id] FROM workitems WHERE " + where }

// queryRun is a query that ran: the client that ran it, the organization
// its work items are in, as Link IDs carry it, and the IDs it matched.
type queryRun struct {
	c   *azdo.Client
	org string
	ids []int
}

// runQuery runs in's query with the PAT in its variable. A query that
// can't run, or matches more than MaxWorkItems work items, is a problem
// with its fix; Azure DevOps rate-limiting it is the error, a
// *ratelimit.Error, and not a problem.
func runQuery(ctx context.Context, env gh.Env, in core.AzureBoardsCheckInput) (queryRun, core.WatcherCheckResult, error) {
	projectURL, id := in.ProjectURL, ""
	if in.Mode != store.AzureBoardsWIQL {
		var err error
		if projectURL, id, err = azdo.SplitQueryURL(in.QueryURL); err != nil {
			return queryRun{}, core.WatcherCheckResult{Problem: core.ProblemQueryUnusable, Reason: err.Error(),
				Fix: "Paste the saved query's URL from the browser, with the query open, in the Azure Boards settings."}, nil
		}
	}
	c, err := azdo.New(env.Environ, in.PATEnv, projectURL, nil)
	switch {
	case errors.Is(err, azdo.ErrNoPAT):
		return queryRun{}, core.WatcherCheckResult{Problem: core.ProblemPATMissing,
			Reason: fmt.Sprintf("The environment variable %s is not set, or is empty, in the Work Item Watcher's environment.", in.PATEnv),
			Fix:    azdo.PATMissingFix(runtime.GOOS, in.PATEnv, scope)}, nil
	case err != nil:
		return queryRun{}, projectUnreadable(projectURL, err), nil
	}
	var res azdo.QueryResult
	if id != "" {
		res, err = c.SavedQuery(ctx, id)
	} else {
		res, err = c.WIQL(ctx, wiqlQuery(in.Where))
	}
	var limit *ratelimit.Error
	switch {
	case errors.As(err, &limit):
		return queryRun{}, core.WatcherCheckResult{}, limit
	case err != nil:
		return queryRun{}, problemOf(in, projectURL, err), nil
	}
	if id != "" && res.QueryType != "flat" {
		return queryRun{}, core.WatcherCheckResult{Problem: core.ProblemQueryUnusable,
			Reason: fmt.Sprintf("The saved query %s is a %s query, which Code Central can't take work items from.", in.QueryURL, queryTypeName(res.QueryType)),
			Fix:    "Make it a flat list of work items in Azure DevOps' query editor, or save a flat copy and paste its URL."}, nil
	}
	ids := res.IDs()
	if len(ids) > MaxWorkItems {
		return queryRun{}, core.WatcherCheckResult{Problem: core.ProblemTooManyWorkItems,
			Reason: fmt.Sprintf("The Azure Boards query matches %d work items, more than the %d Code Central takes in, so none were brought in or closed.", len(ids), MaxWorkItems),
			Fix:    "Narrow the query, such as by State, assignee or area path, so it matches the work items to bring in."}, nil
	}
	org, _ := azdourl.Names(projectURL)
	return queryRun{c: c, org: strings.ToLower(org), ids: ids}, core.WatcherCheckResult{}, nil
}

// queryTypeName is a query type as the human knows it.
func queryTypeName(queryType string) string {
	if queryType == "oneHop" {
		return "links (one-hop)"
	}
	return queryType
}

// problemOf is the problem err, from running in's query in projectURL, is.
func problemOf(in core.AzureBoardsCheckInput, projectURL string, err error) core.WatcherCheckResult {
	var refusal *azdo.RefusedError
	saved := in.Mode != store.AzureBoardsWIQL
	switch {
	case errors.Is(err, azdo.ErrPATRejected):
		return core.WatcherCheckResult{Problem: core.ProblemPATRejected,
			Reason: fmt.Sprintf("Azure DevOps rejected the PAT in %s: it may have expired or been revoked.", in.PATEnv),
			Fix:    fmt.Sprintf("Make a new PAT with %s, and set it in %s for the service.", scope, in.PATEnv)}
	case errors.Is(err, azdo.ErrMissingScope):
		return core.WatcherCheckResult{Problem: core.ProblemPATScope,
			Reason: fmt.Sprintf("The PAT in %s can't read work items in %s.", in.PATEnv, projectURL),
			Fix:    fmt.Sprintf("The PAT needs %s. Make a new PAT with that scope, and set it for the service.", scope)}
	case errors.Is(err, azdo.ErrNotFound) && saved:
		return core.WatcherCheckResult{Problem: core.ProblemQueryUnusable,
			Reason: fmt.Sprintf("Azure DevOps can't find the saved query %s.", in.QueryURL),
			Fix:    "Check the saved query's URL is the one the browser shows with the query open, and that the PAT's account can see the query."}
	case errors.Is(err, azdo.ErrNotFound):
		return projectUnreadable(projectURL, err)
	case errors.As(err, &refusal) && saved:
		return core.WatcherCheckResult{Problem: core.ProblemQueryUnusable,
			Reason: fmt.Sprintf("Azure DevOps can't run the saved query %s: %s", in.QueryURL, refusal.Message),
			Fix:    "Fix the query in Azure DevOps' query editor, keeping it a flat list of work items."}
	case errors.As(err, &refusal):
		return core.WatcherCheckResult{Problem: core.ProblemQueryUnusable,
			Reason: fmt.Sprintf("Azure DevOps refused the WIQL filter: %s", refusal.Message),
			Fix:    "Fix the WHERE clause in the Azure Boards settings: Code Central runs it as " + wiqlQuery("<clause>") + "."}
	}
	return core.WatcherCheckResult{Problem: core.ProblemCheckFailed,
		Reason: fmt.Sprintf("Code Central can't run the Azure Boards query: %v", err),
		Fix:    fmt.Sprintf("Check Azure DevOps' status, and that the service can reach %s; the Work Item Watcher tries again every few minutes.", hostOf(projectURL))}
}

// projectUnreadable is the project not found, or its host not reached.
func projectUnreadable(projectURL string, err error) core.WatcherCheckResult {
	return core.WatcherCheckResult{Problem: core.ProblemProjectUnreadable,
		Reason: fmt.Sprintf("Code Central can't find the Azure DevOps project %s: %v", projectURL, err),
		Fix:    "Check the project URL is the one the browser shows, such as https://dev.azure.com/acme/web, and that the PAT's organization can see it."}
}

// hostOf is the host of rawURL.
func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		return u.Host
	}
	return rawURL
}
