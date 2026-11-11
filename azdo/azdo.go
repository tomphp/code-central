// Package azdo is how the CI Watcher reaches Azure DevOps: its REST API
// directly, with a Personal Access Token (PAT) read from the environment
// by the variable's name, so Code Central stores no PAT. It parses project
// URLs, makes the API's calls, and turns refusals into errors: a missing
// or rejected PAT, a missing scope, a project not found, and a rate limit.
package azdo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tomoram/code-central/watchers"
)

// APIVersion is the REST API version every call pins.
const APIVersion = "7.1"

var (
	// ErrNoPAT is the PAT's variable unset or empty.
	ErrNoPAT = errors.New("no PAT")
	// ErrPATRejected is Azure DevOps refusing the PAT (401), or sending
	// the caller to sign in.
	ErrPATRejected = errors.New("the PAT was rejected")
	// ErrMissingScope is Azure DevOps refusing the PAT its scope (403).
	ErrMissingScope = errors.New("the PAT lacks a scope")
	// ErrNotFound is Azure DevOps not finding what was asked (404), for a
	// project, the project not found.
	ErrNotFound = errors.New("not found")
)

// Client calls one Azure DevOps project's API.
type Client struct {
	projectURL string
	pat        string
	http       *http.Client
	baseURL    string
}

// New is a client for projectURL, as ParseProjectURL stores it, reading
// the PAT from environ (KEY=value pairs) by the name patEnv. An unset or
// empty variable is an error wrapping ErrNoPAT. httpClient may be nil.
func New(environ []string, patEnv, projectURL string, httpClient *http.Client) (*Client, error) {
	u, err := ParseProjectURL(projectURL)
	if err != nil {
		return nil, err
	}
	pat := getenv(environ, patEnv)
	if strings.TrimSpace(pat) == "" {
		return nil, fmt.Errorf("%w: %s is not set, or is empty", ErrNoPAT, patEnv)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	// A redirect is Azure DevOps sending a caller without a valid PAT to
	// sign in; it is not followed.
	hc := *httpClient
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{projectURL: u, pat: pat, http: &hc, baseURL: u}, nil
}

func getenv(environ []string, key string) string {
	for i := len(environ) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(environ[i], "="); ok && k == key {
			return v
		}
	}
	return ""
}

// ProjectURL is the project URL the client calls.
func (c *Client) ProjectURL() string { return c.projectURL }

// Project is a project's resource.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Build is one build, Azure DevOps' name for a pipeline run.
type Build struct {
	ID            int       `json:"id"`
	BuildNumber   string    `json:"buildNumber"`
	Status        string    `json:"status"`
	Result        string    `json:"result"`
	SourceBranch  string    `json:"sourceBranch"`
	SourceVersion string    `json:"sourceVersion"`
	QueueTime     time.Time `json:"queueTime"`
	StartTime     time.Time `json:"startTime"`
	FinishTime    time.Time `json:"finishTime"`
	LastChanged   time.Time `json:"lastChangedDate"`
	Definition    struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"definition"`
	Links struct {
		Web struct {
			Href string `json:"href"`
		} `json:"web"`
	} `json:"_links"`
}

// BuildQuery is what a list of builds asks for. Zero fields are left out.
type BuildQuery struct {
	// Top is how many builds at most.
	Top int
	// Order is "queueTimeDescending" or "finishTimeDescending".
	Order string
	// Branch is a full ref name, such as refs/heads/main.
	Branch string
	// Status is a status filter, such as "completed".
	Status string
	// MinTime is the earliest finish time (with Status completed).
	MinTime time.Time
}

// TimelineRecord is one record of a build's timeline: a stage, a job, a
// task and so on, by Type.
type TimelineRecord struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	State    string `json:"state"`
	Result   string `json:"result"`
	Attempt  int    `json:"attempt"`
	Log      *struct {
		URL string `json:"url"`
	} `json:"log"`
}

// Project reads the project: the project resource under the collection.
func (c *Client) Project(ctx context.Context) (Project, error) {
	i := strings.LastIndex(c.projectURL, "/")
	collection, name := c.projectURL[:i], c.projectURL[i+1:]
	var p Project
	err := c.do(ctx, http.MethodGet, collection+"/_apis/projects/"+name, nil, nil, &p)
	return p, err
}

// Builds lists the project's builds as q asks.
func (c *Client) Builds(ctx context.Context, q BuildQuery) ([]Build, error) {
	v := url.Values{}
	if q.Top > 0 {
		v.Set("$top", strconv.Itoa(q.Top))
	}
	if q.Order != "" {
		v.Set("queryOrder", q.Order)
	}
	if q.Branch != "" {
		v.Set("branchName", q.Branch)
	}
	if q.Status != "" {
		v.Set("statusFilter", q.Status)
	}
	if !q.MinTime.IsZero() {
		v.Set("minTime", q.MinTime.UTC().Format(time.RFC3339))
	}
	var list struct {
		Value []Build `json:"value"`
	}
	if err := c.do(ctx, http.MethodGet, c.baseURL+"/_apis/build/builds", v, nil, &list); err != nil {
		return nil, err
	}
	return list.Value, nil
}

// Timeline reads a build's timeline records.
func (c *Client) Timeline(ctx context.Context, buildID int) ([]TimelineRecord, error) {
	var t struct {
		Records []TimelineRecord `json:"records"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s/_apis/build/builds/%d/timeline", c.baseURL, buildID), nil, nil, &t); err != nil {
		return nil, err
	}
	return t.Records, nil
}

// Retry asks Azure DevOps to retry a build's failed jobs, as a new attempt.
func (c *Client) Retry(ctx context.Context, buildID int) error {
	return c.do(ctx, http.MethodPatch, fmt.Sprintf("%s/_apis/build/builds/%d", c.baseURL, buildID),
		url.Values{"retry": {"true"}}, map[string]any{"retry": true}, nil)
}

// do calls method on rawURL with query, sending body as JSON when it isn't
// nil, and decodes the JSON answer into out when it isn't nil.
func (c *Client) do(ctx context.Context, method, rawURL string, query url.Values, body, out any) error {
	if query == nil {
		query = url.Values{}
	}
	query.Set("api-version", APIVersion)
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL+"?"+query.Encode(), rd)
	if err != nil {
		return err
	}
	req.SetBasicAuth("", c.pat)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// The error's URL has no PAT, which only goes in a header.
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err := c.refusal(resp, data, method, rawURL); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("Azure DevOps answered %s %s with something that isn't JSON", method, rawURL)
	}
	return nil
}

// refusal is the error an answer is, or nil when it succeeded.
func (c *Client) refusal(resp *http.Response, data []byte, method, rawURL string) error {
	status := resp.StatusCode
	what := method + " " + rawURL
	switch {
	case status == http.StatusTooManyRequests,
		status == http.StatusServiceUnavailable && resp.Header.Get("Retry-After") != "":
		limit := &watchers.RateLimitError{System: "Azure DevOps", Message: fmt.Sprintf("%s answered %d", what, status)}
		if secs, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && secs > 0 {
			limit.RetryAfter = time.Duration(secs) * time.Second
		}
		if reset, err := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")), 10, 64); err == nil && reset > 0 {
			limit.Reset = time.Unix(reset, 0).UTC()
		}
		return limit
	case status == http.StatusUnauthorized, status == http.StatusNonAuthoritativeInfo, status >= 300 && status < 400:
		return fmt.Errorf("%w: %s answered %d", ErrPATRejected, what, status)
	case status == http.StatusForbidden:
		return fmt.Errorf("%w: %s answered 403", ErrMissingScope, what)
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: %s answered 404", ErrNotFound, what)
	case status >= 200 && status < 300:
		return nil
	}
	return fmt.Errorf("Azure DevOps answered %s with %d: %s", what, status, watchers.FirstLine(string(data)))
}
