// GitHub: the Pull Request Watcher reaches it through gh, the GitHub CLI,
// and its own login (package gh), so Code Central stores no GitHub token.

package pullrequestwatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/ratelimit"
)

// github is one GitHub repository's pull requests into one base, as gh
// reaches them.
type github struct {
	ghPath     string
	env        gh.Env
	repository string
	base       string
	// prog is the gh CheckAccess found.
	prog string
}

// CheckAccess finds gh, checks its login, reads the repository and checks
// the login has write permission there, which opening a pull request
// needs.
func (g *github) CheckAccess(ctx context.Context, _ bool) (core.WatcherCheckResult, error) {
	prog, r, err := gh.Ready(ctx, g.env, g.ghPath)
	if r.Problem != "" || err != nil {
		return r, err
	}
	g.prog = prog
	body, msg, err := gh.API(ctx, g.env, prog, "repos/"+g.repository)
	if err != nil {
		var limit *ratelimit.Error
		if errors.As(err, &limit) {
			return core.WatcherCheckResult{}, limit
		}
		if r, ran := gh.DidntRun(prog, err); !ran {
			return r, nil
		}
		return core.WatcherCheckResult{Problem: core.ProblemRepositoryUnreadable, Reason: fmt.Sprintf("gh can't read the repository %s: %s", g.repository, msg),
			Fix: "Check the repository is the owner/name GitHub shows, and that the account gh is logged in as can see it."}, nil
	}
	var repo struct {
		Permissions struct {
			Push bool `json:"push"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(body, &repo); err != nil || !repo.Permissions.Push {
		return core.WatcherCheckResult{Problem: core.ProblemNoWrite,
			Reason: fmt.Sprintf("the account gh is logged in as can't write to %s, which opening a pull request needs", g.repository),
			Fix:    "Log gh in as an account with write access to the repository: `gh auth login`, or ask for write access."}, nil
	}
	return core.WatcherCheckResult{}, nil
}

// list is every pull request from branch into the base, newest first.
func (g *github) list(ctx context.Context, branch string) ([]*pullRequest, error) {
	owner, _, _ := strings.Cut(g.repository, "/")
	path := fmt.Sprintf("repos/%s/pulls?state=all&sort=created&direction=desc&per_page=100&head=%s&base=%s",
		g.repository, url.QueryEscape(owner+":"+branch), url.QueryEscape(g.base))
	body, _, err := gh.API(ctx, g.env, g.prog, path)
	if err != nil {
		return nil, fmt.Errorf("listing pull requests from %s: %w", branch, err)
	}
	var prs []*pullRequest
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &prs); err != nil {
			return nil, fmt.Errorf("reading pull requests from %s: %w", branch, err)
		}
	}
	return prs, nil
}

// refresh is the open pull request read singly, as only that read
// tells whether GitHub finds it conflicting; a pull request GitHub hasn't
// worked the mergeability of yet reads as open.
func (g *github) refresh(ctx context.Context, p *pullRequest) (*pullRequest, error) {
	body, _, err := gh.API(ctx, g.env, g.prog, fmt.Sprintf("repos/%s/pulls/%d", g.repository, p.Number))
	if err != nil {
		return p, fmt.Errorf("reading pull request %d: %w", p.Number, err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return p, nil
	}
	var one pullRequest
	if err := json.Unmarshal(body, &one); err != nil {
		return p, fmt.Errorf("reading pull request %d: %w", p.Number, err)
	}
	if one.Number != p.Number {
		return p, nil
	}
	return &one, nil
}

// open opens a pull request from branch into the base, ready for review,
// with title and body.
func (g *github) open(ctx context.Context, branch, title, body string) (*pullRequest, error) {
	out, msg, err := gh.API(ctx, g.env, g.prog, "repos/"+g.repository+"/pulls", "--method", "POST",
		"-f", "title="+title, "-f", "body="+body, "-f", "head="+branch, "-f", "base="+g.base)
	if err != nil {
		var limit *ratelimit.Error
		if errors.As(err, &limit) {
			return nil, err
		}
		return nil, fmt.Errorf("opening a pull request from %s into %s: %w: %s", branch, g.base, err, msg)
	}
	var pr pullRequest
	if err := json.Unmarshal(out, &pr); err != nil || pr.Number == 0 {
		return nil, fmt.Errorf("opening a pull request from %s into %s: GitHub's answer has no pull request", branch, g.base)
	}
	return &pr, nil
}

// sameBody reports whether a listed body is body: GitHub keeps a body as
// it was sent.
func (g *github) sameBody(listed, body string) bool { return listed == body }

// patch sets field (name=value) on pull request number, with GitHub's
// answer; a refusal is wrapped with what, and a rate limit returned as
// it is.
func (g *github) patch(ctx context.Context, number int, what, field string) ([]byte, error) {
	out, msg, err := gh.API(ctx, g.env, g.prog, fmt.Sprintf("repos/%s/pulls/%d", g.repository, number), "--method", "PATCH", "-f", field)
	if err != nil {
		var limit *ratelimit.Error
		if errors.As(err, &limit) {
			return nil, err
		}
		return nil, fmt.Errorf(what+": %w: %s", number, err, msg)
	}
	return out, nil
}

// edit sets an open pull request's body.
func (g *github) edit(ctx context.Context, p *pullRequest, body string) error {
	_, err := g.patch(ctx, p.Number, "editing pull request %d's body", "body="+body)
	return err
}

// close closes pull request number without merging it.
func (g *github) close(ctx context.Context, number int) (*pullRequest, error) {
	out, err := g.patch(ctx, number, "closing pull request %d", "state=closed")
	if err != nil {
		return nil, err
	}
	var pr pullRequest
	if err := json.Unmarshal(out, &pr); err != nil || pr.Number == 0 {
		return nil, fmt.Errorf("closing pull request %d: GitHub's answer has no pull request", number)
	}
	return &pr, nil
}
