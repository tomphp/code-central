package client

import (
	"context"
	"net/url"
	"sort"
	"strconv"

	"github.com/tomoram/code-central/core"
)

// Blockers, Replacements and finding work (ticket 28).

// AddBlocker is add_blocker.
func (c *Client) AddBlocker(ctx context.Context, key string, in core.BlockerInput) (core.TicketView, error) {
	var out core.TicketView
	return out, c.do(ctx, "POST", item(key, "tickets", in.Ticket, "add_blocker"), in, &out)
}

// RemoveBlocker is remove_blocker.
func (c *Client) RemoveBlocker(ctx context.Context, key string, in core.BlockerInput) (core.TicketView, error) {
	var out core.TicketView
	return out, c.do(ctx, "POST", item(key, "tickets", in.Ticket, "remove_blocker"), in, &out)
}

// Cancel is cancel.
func (c *Client) Cancel(ctx context.Context, key string, in core.CancelInput) (core.TicketView, error) {
	var out core.TicketView
	return out, c.do(ctx, "POST", item(key, "tickets", in.Ticket, "cancel"), in, &out)
}

// GetSpec is get_spec.
func (c *Client) GetSpec(ctx context.Context, key string, in core.GetSpecInput) (core.SpecDetail, error) {
	q := url.Values{"part": in.Parts, "section": in.Sections}
	path := scoped(key, "specs/"+url.PathEscape(string(in.Spec)))
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out core.SpecDetail
	return out, c.do(ctx, "GET", path, nil, &out)
}

// FindSpecs is find_specs.
func (c *Client) FindSpecs(ctx context.Context, key string, in core.FindSpecsInput) ([]core.SpecRow, error) {
	q := url.Values{}
	if in.Title != "" {
		q.Set("title", in.Title)
	}
	for _, l := range in.Labels {
		q.Add("label", l)
	}
	addTags(q, in.Tags)
	if in.Finished {
		q.Set("finished", strconv.FormatBool(true))
	}
	if in.Requirements {
		q.Set("requirements", strconv.FormatBool(true))
	}
	path := scoped(key, "specs")
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []core.SpecRow
	return out, c.do(ctx, "GET", path, nil, &out)
}

// addTags adds each Tag as a tag=<key>=<value> query parameter.
func addTags(q url.Values, tags map[string]string) {
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		q.Add("tag", k+"="+tags[k])
	}
}
