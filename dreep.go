// Package dreep provides a client for the Dreep media API.
//
// Dreep (https://docs.dreep.cloud) is a universal media API: upload, store,
// process, and deliver images, videos, and documents. This package wraps every
// REST endpoint with zero dependencies beyond the Go standard library.
//
// Basic usage:
//
//	client, err := dreep.New(os.Getenv("DREEP_API_KEY"))
//	if err != nil { ... }
//
//	f, _ := os.Open("hero.jpg")
//	asset, err := client.Upload(ctx, dreep.UploadOptions{
//		File:        f,
//		Filename:    "hero.jpg",
//		Destination: dreep.Destination{Folder: "marketing/2026"},
//		Transform:   &dreep.Transform{Width: 1200, Format: dreep.FormatWebP},
//	})
package dreep

import (
	"errors"
	"net/http"
	"time"
)

const (
	// DefaultAPIBaseURL is the root of the Dreep REST API.
	DefaultAPIBaseURL = "https://api.dreep.cloud/api/v1"

	// DefaultCDNBaseURL is the root of Dreep's public asset delivery URL.
	DefaultCDNBaseURL = "https://cdn.dreep.cloud/api/v1/fetch"

	defaultMaxRetries     = 2
	defaultRetryBaseDelay = 500 * time.Millisecond
)

// Client is a Dreep API client. It is safe for concurrent use by multiple
// goroutines once constructed.
type Client struct {
	apiKey        string
	signingSecret string

	// APIBaseURL is the prefix for all REST calls (default DefaultAPIBaseURL).
	APIBaseURL string

	// CDNBaseURL is the prefix used to build delivery URLs (default
	// DefaultCDNBaseURL).
	CDNBaseURL string

	// HTTP is the underlying HTTP client. The default client applies no
	// global timeout — uploads of large files can legitimately take minutes.
	// Bound requests with context deadlines instead.
	HTTP *http.Client

	// maxRetries is how many times a replayable request is retried on a
	// 429 or 5xx response, with exponential backoff. 0 disables retries.
	maxRetries int

	// retryBaseDelay is the backoff for the first retry; each subsequent
	// retry doubles it (capped by any Retry-After header).
	retryBaseDelay time.Duration

	// now is injectable for deterministic signed-URL tests.
	now func() time.Time
}

// Option configures a Client.
type Option func(*Client) error

// New returns a new Dreep client. The API key ("drp_live_…") is required and
// must be kept server-side; anyone holding it can read, upload, and delete
// every asset in the project.
func New(apiKey string, opts ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, errors.New("dreep: apiKey is required")
	}
	c := &Client{
		apiKey:         apiKey,
		APIBaseURL:     DefaultAPIBaseURL,
		CDNBaseURL:     DefaultCDNBaseURL,
		HTTP:           &http.Client{},
		maxRetries:     defaultMaxRetries,
		retryBaseDelay: defaultRetryBaseDelay,
		now:            time.Now,
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// WithSigningSecret sets the project URL Signing Secret, required only for
// SignedURL and signed-folder access.
func WithSigningSecret(secret string) Option {
	return func(c *Client) error {
		c.signingSecret = secret
		return nil
	}
}

// WithAPIBaseURL overrides the REST base URL (useful for tests and proxies).
func WithAPIBaseURL(u string) Option {
	return func(c *Client) error {
		if u == "" {
			return errors.New("dreep: API base URL must not be empty")
		}
		c.APIBaseURL = u
		return nil
	}
}

// WithCDNBaseURL overrides the CDN fetch URL prefix (useful for tests).
func WithCDNBaseURL(u string) Option {
	return func(c *Client) error {
		if u == "" {
			return errors.New("dreep: CDN base URL must not be empty")
		}
		c.CDNBaseURL = u
		return nil
	}
}

// WithHTTPClient replaces the underlying http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) error {
		if hc == nil {
			return errors.New("dreep: http.Client must not be nil")
		}
		c.HTTP = hc
		return nil
	}
}

// WithMaxRetries sets how many times a request with a replayable body is
// retried after a 429 or 5xx response, using exponential backoff. The default
// is 2 (three attempts in total); 0 disables retries. Streaming bodies — such
// as multipart uploads — cannot be replayed and are never retried.
func WithMaxRetries(n int) Option {
	return func(c *Client) error {
		if n < 0 {
			return errors.New("dreep: max retries must not be negative")
		}
		c.maxRetries = n
		return nil
	}
}

// WithRetryBaseDelay sets the wait before the first retry; every further
// retry doubles it. A Retry-After response header overrides the computed
// delay. Defaults to 500ms.
func WithRetryBaseDelay(d time.Duration) Option {
	return func(c *Client) error {
		if d < 0 {
			return errors.New("dreep: retry base delay must not be negative")
		}
		c.retryBaseDelay = d
		return nil
	}
}

// Ptr returns a pointer to v — a convenience for optional fields such as
// Transform.TrimStart or UploadOptions.AutoCreateFolders.
func Ptr[T any](v T) *T { return &v }
