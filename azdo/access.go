// Access: the one way a Watcher gets a ready client for its check, and the
// problems every Watcher's check shares, a PAT missing or rejected.

package azdo

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// Access is how one Watcher reaches Azure DevOps: the PAT it reads from
// its environment by the variable's name, and the scope it needs.
type Access struct {
	// Environ is the Watcher's environment, which holds the PAT.
	Environ []string
	// PATEnv names the environment variable holding the PAT; "" is
	// domain.DefaultPATEnv.
	PATEnv string
	// Watcher is the Watcher's name, as its problems say it, such as CI
	// Watcher.
	Watcher string
	// Scope is the PAT scope the Watcher needs, as Azure DevOps names it,
	// such as Build: Read & execute.
	Scope string
}

// PATVariable is the PAT's variable name, the default when none is named.
func (a Access) PATVariable() string {
	if a.PATEnv == "" {
		return domain.DefaultPATEnv
	}
	return a.PATEnv
}

// Ready is a client for projectURL, or the PAT missing problem when the
// PAT's variable is unset or empty. A projectURL that doesn't parse is
// the error, which each Watcher reports as what it couldn't read.
func (a Access) Ready(projectURL string) (*Client, api.WatcherCheckResult, error) {
	c, err := New(a.Environ, a.PATVariable(), projectURL, nil)
	switch {
	case errors.Is(err, ErrNoPAT):
		return nil, api.WatcherCheckResult{Problem: api.ProblemPATMissing,
			Reason: fmt.Sprintf("The environment variable %s is not set, or is empty, in the %s's environment.", a.PATVariable(), a.Watcher),
			Fix:    PATMissingFix(runtime.GOOS, a.PATVariable(), a.Scope)}, nil
	case err != nil:
		return nil, api.WatcherCheckResult{}, err
	}
	return c, api.WatcherCheckResult{}, nil
}

// Rejected is the problem of Azure DevOps refusing the PAT, an
// ErrPATRejected from a call.
func (a Access) Rejected() api.WatcherCheckResult {
	return api.WatcherCheckResult{Problem: api.ProblemPATRejected,
		Reason: fmt.Sprintf("Azure DevOps rejected the PAT in %s: it may have expired or been revoked.", a.PATVariable()),
		Fix:    fmt.Sprintf("Make a new PAT with %s, and set it in %s for the service.", a.Scope, a.PATVariable())}
}
