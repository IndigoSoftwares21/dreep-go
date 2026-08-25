package dreep

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const maxErrBody = 1 << 20 // 1 MiB cap on buffered error bodies

// doJSON performs an API request carrying an optional JSON body and decodes a
// JSON response into out (which may be nil). path is appended to APIBaseURL.
func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, in, out any) error {
	var body io.Reader
	contentType := ""
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("dreep: encoding request body: %w", err)
		}
		body = bytes.NewReader(b)
		contentType = "application/json"
	}
	full := c.APIBaseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	req, err := c.newRequest(ctx, method, full, body, contentType)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

// do executes req, maps non-2xx responses to *Error, and decodes a JSON
// response into out (which may be nil).
func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("dreep: %w", err)
	}
	if resp.StatusCode >= 400 {
		return c.errorFromResponse(resp)
	}
	if out == nil {
		drainAndClose(resp)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		drainAndClose(resp)
		return fmt.Errorf("dreep: decoding response: %w", err)
	}
	drainAndClose(resp)
	return nil
}

// decodeJSONBody reads a successful response fully into out, tolerating an
// empty body (used where the API may return nothing on success).
func decodeJSONBody(resp *http.Response, out any) error {
	defer drainAndClose(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrBody))
	if err != nil {
		return fmt.Errorf("dreep: reading response: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("dreep: decoding response: %w", err)
	}
	return nil
}

func drainAndClose(resp *http.Response) {
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
}

// newRequest builds an authenticated request against the Dreep API.
func (c *Client) newRequest(ctx context.Context, method, rawURL string, body io.Reader, contentType string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("dreep: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req, nil
}

func formatFloat(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
