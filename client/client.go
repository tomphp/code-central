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
	var out core.ProjectView
	return out, c.do(ctx, "POST", "/api/projects/create_project", in, &out)
}

// GetProject is get_project.
func (c *Client) GetProject(ctx context.Context, key string) (core.ProjectView, error) {
	var out core.ProjectView
	return out, c.do(ctx, "GET", "/api/projects/"+url.PathEscape(key), nil, &out)
}

// CreateSpec is create_spec.
func (c *Client) CreateSpec(ctx context.Context, key string, in core.CreateSpecInput) (core.Written[core.SpecView], error) {
	return write[core.SpecView](ctx, c, scoped(key, "create_spec"), in, in.Full)
}

// CreateTicket is create_ticket.
func (c *Client) CreateTicket(ctx context.Context, key string, in core.CreateTicketInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, scoped(key, "create_ticket"), in, in.Full)
}

// GetTicket is get_ticket with the parts named, or the whole Ticket with
// none, as the HTTP API returns it.
func (c *Client) GetTicket(ctx context.Context, key string, ref core.Ref, parts ...string) (core.TicketView, error) {
	q := url.Values{"part": parts}
	path := scoped(key, "tickets/"+url.PathEscape(string(ref)))
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out core.TicketView
	return out, c.do(ctx, "GET", path, nil, &out)
}

// FindTickets is find_tickets.
func (c *Client) FindTickets(ctx context.Context, key string, in core.FindTicketsInput) ([]core.TicketRow, error) {
	q := url.Values{}
	for _, s := range in.State {
		q.Add("state", string(s))
	}
	for _, l := range in.Labels {
		q.Add("label", l)
	}
	if in.Title != "" {
		q.Set("title", in.Title)
	}
	if in.Spec != "" {
		q.Set("spec", string(in.Spec))
	}
	addTags(q, in.Tags)
	if in.Claimable != nil {
		q.Set("claimable", strconv.FormatBool(*in.Claimable))
	}
	path := scoped(key, "tickets")
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []core.TicketRow
	return out, c.do(ctx, "GET", path, nil, &out)
}

// Claim is claim.
func (c *Client) Claim(ctx context.Context, key string, in core.ClaimInput) (core.ClaimResult, error) {
	var out core.ClaimResult
	return out, c.do(ctx, "POST", scoped(key, "claim"), in, &out)
}

// Finish is finish.
func (c *Client) Finish(ctx context.Context, key string, in core.FinishInput) (core.FinishResult, error) {
	var out core.FinishResult
	return out, c.do(ctx, "POST", item(key, "tickets", in.Ticket, "finish"), in, &out)
}

// Accept is the human's accept; set Actor to human.
func (c *Client) Accept(ctx context.Context, key string, in core.TicketInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "accept"), in, in.Full)
}

// Reject is the human's reject; set Actor to human.
func (c *Client) Reject(ctx context.Context, key string, in core.RejectInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "reject"), in, in.Full)
}

// SetSession is set_session.
func (c *Client) SetSession(ctx context.Context, key string, in core.SetSessionInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "set_session"), in, in.Full)
}

// Release is the human's release; set Actor to human.
func (c *Client) Release(ctx context.Context, key string, in core.ReleaseInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "release"), in, in.Full)
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

// Transcript is GET /api/projects/{key}/tickets/{n}/claims/{c}/transcript:
// the Session Events held for the Ticket's Claim number claim.
func (c *Client) Transcript(ctx context.Context, key string, ticket core.Ref, claim int) (core.TranscriptView, error) {
	var out core.TranscriptView
	path := "/api/projects/" + url.PathEscape(key) + "/tickets/" + url.PathEscape(string(ticket)) + "/claims/" + strconv.Itoa(claim) + "/transcript"
	return out, c.do(ctx, "GET", path, nil, &out)
}

// EditSpec is edit_spec.
func (c *Client) EditSpec(ctx context.Context, key string, in core.EditSpecInput) (core.Written[core.SpecView], error) {
	return write[core.SpecView](ctx, c, item(key, "specs", in.Spec, "edit_spec"), in, in.Full)
}

// write POSTs an acknowledging write and decodes its result.
func write[V any](ctx context.Context, c *Client, path string, in any, full bool) (core.Written[V], error) {
	var raw json.RawMessage
	if err := c.do(ctx, "POST", path, in, &raw); err != nil {
		return core.Written[V]{}, err
	}
	return decodeWritten[V](raw, full)
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
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "edit_ticket"), in, in.Full)
}

// AddLabels is add_labels.
func (c *Client) AddLabels(ctx context.Context, key string, in core.LabelsInput) (core.TargetView, error) {
	var out core.TargetView
	return out, c.do(ctx, "POST", scoped(key, "add_labels"), in, &out)
}

// RemoveLabels is remove_labels.
func (c *Client) RemoveLabels(ctx context.Context, key string, in core.LabelsInput) (core.TargetView, error) {
	var out core.TargetView
	return out, c.do(ctx, "POST", scoped(key, "remove_labels"), in, &out)
}

// SetTags is set_tags.
func (c *Client) SetTags(ctx context.Context, key string, in core.SetTagsInput) (core.TargetView, error) {
	var out core.TargetView
	return out, c.do(ctx, "POST", scoped(key, "set_tags"), in, &out)
}

// RemoveTags is remove_tags.
func (c *Client) RemoveTags(ctx context.Context, key string, in core.RemoveTagsInput) (core.TargetView, error) {
	var out core.TargetView
	return out, c.do(ctx, "POST", scoped(key, "remove_tags"), in, &out)
}

// Comment is comment.
func (c *Client) Comment(ctx context.Context, key string, in core.CommentInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "comment"), in, in.Full)
}

// Approve is approve.
func (c *Client) Approve(ctx context.Context, key string, in core.TicketInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "approve"), in, in.Full)
}

// Defer is defer.
func (c *Client) Defer(ctx context.Context, key string, in core.DeferInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "defer"), in, in.Full)
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
		return err
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
