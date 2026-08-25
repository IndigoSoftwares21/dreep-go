package dreep

import (
	"context"
	"net/http"
)

// GetUsage returns current billing-cycle counters: storage bytes, image/file/
// folder counts. A 402 on other endpoints (*IsPaymentRequired*) usually means
// one of these limits was exceeded.
func (c *Client) GetUsage(ctx context.Context) (*Usage, error) {
	var out Usage
	if err := c.doJSON(ctx, http.MethodGet, "/usage", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
