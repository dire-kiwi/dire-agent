package extensions

import (
	"context"
	"fmt"
)

type Status struct {
	Level   string `json:"level"`
	Message string `json:"message,omitempty"`
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	if c.closed.Load() {
		return Status{}, ErrClosed
	}
	requestCtx, cancel := withTimeout(ctx, c.limits.CallTimeout)
	defer cancel()
	var result Status
	if err := c.connection.Call(requestCtx, "get_status", struct{}{}, &result); err != nil {
		return Status{}, fmt.Errorf("extensions: get status for %s: %w", c.id, err)
	}
	result.Message = truncate(result.Message, c.limits.MaxOutputBytes)
	return result, nil
}
