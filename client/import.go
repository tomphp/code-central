package client

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// Scratch Import (ticket 40).

// GetImportGuide is get_import_guide.
func (c *Client) GetImportGuide(ctx context.Context, key string) (api.ImportGuideView, error) {
	return call(ctx, c, ops.GetImportGuide, key, struct{}{})
}

// Import is import.
func (c *Client) Import(ctx context.Context, key string, in api.ImportInput) (api.ImportResult, error) {
	return call(ctx, c, ops.Import, key, in)
}
