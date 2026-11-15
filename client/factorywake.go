package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// The Factory's wake (Spec code-central-743).

// NextFactoryWake is the Factory's next_factory_wake: it holds for up to
// core.WaitTimeout until a write the Factory's poll acts on reaches the
// Project, and returns woken then, or empty when none does.
func (c *Client) NextFactoryWake(ctx context.Context, key string, in core.NextFactoryWakeInput) (core.FactoryWake, error) {
	var out core.FactoryWake
	path := scoped(key, "factory/next-wake") + "?" + url.Values{"factory": {in.Factory}, "host": {in.Host}}.Encode()
	return out, c.do(ctx, "GET", path, nil, &out)
}
