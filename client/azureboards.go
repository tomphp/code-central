package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// Azure Boards (Spec code-central-894).

// SetAzureBoards is the human's set_azure_boards; set Actor to human.
func (c *Client) SetAzureBoards(ctx context.Context, key string, in core.SetAzureBoardsInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/set_azure_boards", in, in.Full)
}
