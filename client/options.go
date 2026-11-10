package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Options (Spec code-central-160).

// GetOptions is get_options.
func (c *Client) GetOptions(ctx context.Context) (core.OptionsView, error) {
	var out core.OptionsView
	return out, c.do(ctx, "GET", "/api/options", nil, &out)
}

// SetOptions is the human's set_options; set Actor to human.
func (c *Client) SetOptions(ctx context.Context, in core.SetOptionsInput) (core.OptionsView, error) {
	var out core.OptionsView
	return out, c.do(ctx, "POST", "/api/set_options", in, &out)
}
