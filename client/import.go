package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Scratch Import (ticket 40).

// GetImportGuide is get_import_guide.
func (c *Client) GetImportGuide(ctx context.Context, key string) (core.ImportGuideView, error) {
	var out core.ImportGuideView
	return out, c.do(ctx, "GET", scoped(key, "import-guide"), nil, &out)
}

// Import is import.
func (c *Client) Import(ctx context.Context, key string, in core.ImportInput) (core.ImportResult, error) {
	var out core.ImportResult
	return out, c.do(ctx, "POST", scoped(key, "import"), in, &out)
}
