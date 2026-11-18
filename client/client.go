// Package client is the Go client for Code Central's HTTP API. Every
// code-central command reaches the service through it. It tolerates
// unknown response fields, so an older client works with a newer service.
package client

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

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// DefaultURL is used when CODE_CENTRAL_URL is unset.
const DefaultURL = "http://localhost:7733"

// Client talks to one Code Central service.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// Actor is sent with every request; empty means agent, which is how
	// every code-central command writes.
	Actor core.Actor
	// Version, when set, is this client's version, sent on every request
	// as ClientVersionHeader.
	Version string
	// OnVersion, when set, is called with the service's version header
	// on every response that carries one.
	OnVersion func(serviceVersion string)
}

// VersionHeader is the response header carrying the service's version.
const VersionHeader = "X-Code-Central-Version"

// ClientVersionHeader is the request header carrying the client's version.
const ClientVersionHeader = "X-Code-Central-Client-Version"

// New returns a Client for the service at baseURL.
func New(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: http.DefaultClient}
}

// CreateProject is create_project.
func (c *Client) CreateProject(ctx context.Context, in core.CreateProjectInput) (core.ProjectView, error) {
	return call(ctx, c, ops.CreateProject, "", in)
}

// GetProject is get_project.
func (c *Client) GetProject(ctx context.Context, key string) (core.ProjectView, error) {
	return call(ctx, c, ops.GetProject, key, struct{}{})
}

// CreateSpec is create_spec.
func (c *Client) CreateSpec(ctx context.Context, key string, in core.CreateSpecInput) (core.Written[core.SpecView], error) {
	return written(ctx, c, ops.CreateSpec, key, in, in.Full)
}

// CreateTicket is create_ticket.
func (c *Client) CreateTicket(ctx context.Context, key string, in core.CreateTicketInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.CreateTicket, key, in, in.Full)
}

// GetTicket is get_ticket with the parts named, or the whole Ticket with
// none, as the HTTP API returns it.
func (c *Client) GetTicket(ctx context.Context, key string, ref core.Ref, parts ...string) (core.TicketView, error) {
	return call(ctx, c, ops.GetTicket, key, core.GetTicketInput{Ticket: ref, Parts: parts})
}

// FindTickets is find_tickets.
func (c *Client) FindTickets(ctx context.Context, key string, in core.FindTicketsInput) ([]core.TicketRow, error) {
	in.Full, in.Parts = false, nil
	var out []core.TicketRow
	return out, request(ctx, c, ops.FindTickets, key, in, &out)
}

// FindTicketViews is find_tickets with full: each Ticket as GetTicket
// returns it, with the parts named, or whole with none, from one read.
func (c *Client) FindTicketViews(ctx context.Context, key string, in core.FindTicketsInput, parts ...string) ([]core.TicketView, error) {
	in.Full, in.Parts = true, parts
	var out []core.TicketView
	return out, request(ctx, c, ops.FindTickets, key, in, &out)
}

// Claim is claim.
func (c *Client) Claim(ctx context.Context, key string, in core.ClaimInput) (core.ClaimResult, error) {
	return call(ctx, c, ops.Claim, key, in)
}

// Finish is finish.
func (c *Client) Finish(ctx context.Context, key string, in core.FinishInput) (core.FinishResult, error) {
	return call(ctx, c, ops.Finish, key, in)
}

// Accept is the human's accept; set Actor to human.
func (c *Client) Accept(ctx context.Context, key string, in core.TicketInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.Accept, key, in, in.Full)
}

// Reject is the human's reject; set Actor to human.
func (c *Client) Reject(ctx context.Context, key string, in core.RejectInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.Reject, key, in, in.Full)
}

// SetSession is set_session.
func (c *Client) SetSession(ctx context.Context, key string, in core.SetSessionInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.SetSession, key, in, in.Full)
}

// Release is the human's release; set Actor to human.
func (c *Client) Release(ctx context.Context, key string, in core.ReleaseInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.Release, key, in, in.Full)
}

// Ping is POST /api/ping: it marks activity on every live Claim with this
// session ID. The service always answers 204.
func (c *Client) Ping(ctx context.Context, sessionID string) error {
	return c.do(ctx, "POST", "/api/ping", map[string]string{"session_id": sessionID}, nil)
}

// PostSessionEvents is POST /api/session-events: it adds a batch of
// Session Events to the newest Claim with this session ID. The service
// always answers 204.
func (c *Client) PostSessionEvents(ctx context.Context, sessionID string, events []core.SessionEvent) error {
	in := struct {
		SessionID string              `json:"session_id"`
		Events    []core.SessionEvent `json:"events"`
	}{sessionID, events}
	return c.do(ctx, "POST", "/api/session-events", in, nil)
}

// PostUsage is POST /api/usage: it records a batch of model responses'
// token usage under the newest Claim with this session ID. The service
// always answers 204; an older one, without the endpoint, 404.
func (c *Client) PostUsage(ctx context.Context, sessionID string, responses []core.UsageResponse) error {
	in := struct {
		SessionID string               `json:"session_id"`
		Responses []core.UsageResponse `json:"responses"`
	}{sessionID, responses}
	return c.do(ctx, "POST", "/api/usage", in, nil)
}

// Usage is GET /api/usage: Usage for one Project, or every Project when
// key is "", over the last days local days (0 for the default).
func (c *Client) Usage(ctx context.Context, key string, days int) (core.UsageView, error) {
	q := url.Values{}
	if key != "" {
		q.Set("project", key)
	}
	if days != 0 {
		q.Set("days", strconv.Itoa(days))
	}
	path := "/api/usage"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out core.UsageView
	return out, c.do(ctx, "GET", path, nil, &out)
}

// Transcript is GET /api/projects/{key}/tickets/{n}/claims/{c}/transcript:
// the Session Events held for the Ticket's Claim number claim.
func (c *Client) Transcript(ctx context.Context, key string, ticket core.Ref, claim int) (core.TranscriptView, error) {
	var out core.TranscriptView
	path := "/api/projects/" + url.PathEscape(key) + "/tickets/" + url.PathEscape(string(ticket)) + "/claims/" + strconv.Itoa(claim) + "/transcript"
	return out, c.do(ctx, "GET", path, nil, &out)
}

// EditSpec is edit_spec.
func (c *Client) EditSpec(ctx context.Context, key string, in core.EditSpecInput) (core.Written[core.SpecView], error) {
	return written(ctx, c, ops.EditSpec, key, in, in.Full)
}

// decodeWritten decodes an acknowledging write's result into the Record
// when full was asked for, else into the Acknowledgement.
func decodeWritten[V any](raw json.RawMessage, full bool) (core.Written[V], error) {
	var out core.Written[V]
	if full {
		out.Record = new(V)
		return out, json.Unmarshal(raw, out.Record)
	}
	return out, json.Unmarshal(raw, &out.Acknowledgement)
}

// EditTicket is edit_ticket.
func (c *Client) EditTicket(ctx context.Context, key string, in core.EditTicketInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.EditTicket, key, in, in.Full)
}

// AddLabels is add_labels.
func (c *Client) AddLabels(ctx context.Context, key string, in core.LabelsInput) (core.TargetView, error) {
	return call(ctx, c, ops.AddLabels, key, in)
}

// RemoveLabels is remove_labels.
func (c *Client) RemoveLabels(ctx context.Context, key string, in core.LabelsInput) (core.TargetView, error) {
	return call(ctx, c, ops.RemoveLabels, key, in)
}

// SetTags is set_tags.
func (c *Client) SetTags(ctx context.Context, key string, in core.SetTagsInput) (core.TargetView, error) {
	return call(ctx, c, ops.SetTags, key, in)
}

// RemoveTags is remove_tags.
func (c *Client) RemoveTags(ctx context.Context, key string, in core.RemoveTagsInput) (core.TargetView, error) {
	return call(ctx, c, ops.RemoveTags, key, in)
}

// Comment is comment.
func (c *Client) Comment(ctx context.Context, key string, in core.CommentInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.Comment, key, in, in.Full)
}

// Approve is approve.
func (c *Client) Approve(ctx context.Context, key string, in core.TicketInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.Approve, key, in, in.Full)
}

// Defer is defer.
func (c *Client) Defer(ctx context.Context, key string, in core.DeferInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.Defer, key, in, in.Full)
}

// GetWorkflow is get_workflow.
func (c *Client) GetWorkflow(ctx context.Context) (core.WorkflowView, error) {
	var out core.WorkflowView
	return out, c.do(ctx, "GET", "/api/workflow", nil, &out)
}

// item is the path of an action on one Spec or Ticket. The path names
// it, so the service ignores the ref in the body.
func item(key, kind string, ref core.Ref, action string) string {
	return scoped(key, kind+"/"+url.PathEscape(string(ref))+"/"+action)
}

func scoped(key, rest string) string { return "/api/p/" + url.PathEscape(key) + "/" + rest }

// do sends one request. API errors come back as *core.Error; a service
// that can't be reached is a *core.Error with code service_unavailable.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	refuseRealService(c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	actor := c.Actor
	if actor == "" {
		actor = core.Agent
	}
	req.Header.Set("X-Code-Central-Actor", string(actor))
	if c.Version != "" {
		req.Header.Set(ClientVersionHeader, c.Version)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &core.Error{
			Code: core.ServiceUnavailable,
			Message: fmt.Sprintf("cannot reach Code Central at %s: start it with `code-central serve`, "+
				"or run it as a login item with `code-central service install`; "+
				"from a sandbox or container, see `code-central help sandboxes`", c.BaseURL),
		}
	}
	defer resp.Body.Close()
	if v := resp.Header.Get(VersionHeader); v != "" && c.OnVersion != nil {
		c.OnVersion(v)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s %s: reading the answer: %w", method, path, err)
	}
	if resp.StatusCode >= 400 {
		var e core.Error
		if json.Unmarshal(data, &e) != nil || e.Code == "" {
			return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, bytes.TrimSpace(data))
		}
		return &e
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.Join(fmt.Errorf("%s %s: bad response", method, path), err)
	}
	return nil
}
