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
	"strings"

	"github.com/tomoram/code-central/core"
)

// DefaultURL is used when CODE_CENTRAL_URL is unset.
const DefaultURL = "http://localhost:7733"

// Client talks to one Code Central service.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

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
func (c *Client) CreateSpec(ctx context.Context, key string, in core.CreateSpecInput) (core.SpecView, error) {
	var out core.SpecView
	return out, c.do(ctx, "POST", scoped(key, "create_spec"), in, &out)
}

// CreateTicket is create_ticket.
func (c *Client) CreateTicket(ctx context.Context, key string, in core.CreateTicketInput) (core.TicketView, error) {
	var out core.TicketView
	return out, c.do(ctx, "POST", scoped(key, "create_ticket"), in, &out)
}

// GetTicket is get_ticket.
func (c *Client) GetTicket(ctx context.Context, key string, ref core.Ref) (core.TicketView, error) {
	var out core.TicketView
	return out, c.do(ctx, "GET", scoped(key, "tickets/"+url.PathEscape(string(ref))), nil, &out)
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
	path := scoped(key, "tickets")
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []core.TicketRow
	return out, c.do(ctx, "GET", path, nil, &out)
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
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &core.Error{
			Code: core.ServiceUnavailable,
			Message: fmt.Sprintf("cannot reach Code Central at %s: start it with `code-central serve`, "+
				"or run it as a login item with `code-central service install`", c.BaseURL),
		}
	}
	defer resp.Body.Close()
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
