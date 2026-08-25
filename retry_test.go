package dreep

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func retryTestClient(t *testing.T, srv *httptest.Server, opts ...Option) *Client {
	t.Helper()
	o := append([]Option{WithAPIBaseURL(srv.URL), WithRetryBaseDelay(time.Millisecond)}, opts...)
	c, err := New("k", o...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestRetryOn429ThenSuccess(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"imageCount": 1})
	}))
	defer srv.Close()

	c := retryTestClient(t, srv)
	u, err := c.GetUsage(context.Background())
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if u.ImageCount != 1 || attempts != 3 {
		t.Errorf("attempts=%d usage=%+v", attempts, u)
	}
}

func TestRetryExhaustionReturnsLastError(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":true,"message":"upstream","code":"bad_gateway"}`))
	}))
	defer srv.Close()

	c := retryTestClient(t, srv) // default maxRetries = 2 → 3 attempts
	_, err := c.GetUsage(context.Background())
	if !hasStatus(err, 502) {
		t.Fatalf("want 502 *Error, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (initial + 2 retries)", attempts)
	}
}

func TestRetryDisabledWithZeroRetries(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := retryTestClient(t, srv, WithMaxRetries(0))
	if _, err := c.GetUsage(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 with retries disabled", attempts)
	}
}

func TestNoRetryOn4xxOtherThan429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":true,"message":"bad key","code":"unauthorized"}`))
	}))
	defer srv.Close()

	c := retryTestClient(t, srv)
	if _, err := c.GetUsage(context.Background()); !IsUnauthorized(err) {
		t.Fatalf("want 401 error, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 — 401 must not be retried", attempts)
	}
}

func TestRetryReplaysJSONBody(t *testing.T) {
	var bodies []string
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "f1", "name": "q1"})
	}))
	defer srv.Close()

	c := retryTestClient(t, srv)
	f, err := c.CreateFolder(context.Background(), CreateFolderOptions{Path: "invoices/2024/q1"})
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if f.ID != "f1" {
		t.Errorf("folder = %+v", f)
	}
	if len(bodies) != 2 || bodies[0] != bodies[1] ||
		!strings.Contains(bodies[1], `"path":"invoices/2024/q1"`) {
		t.Errorf("bodies = %q — body must be replayed identically on retry", bodies)
	}
}

func TestStreamingUploadIsNeverRetried(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":true,"message":"boom","code":"internal"}`))
	}))
	defer srv.Close()

	c := retryTestClient(t, srv)
	_, err := c.Upload(context.Background(), UploadOptions{
		File:     strings.NewReader("JPEGDATA"),
		Filename: "hero.jpg",
	})
	if !hasStatus(err, 500) {
		t.Fatalf("want 500 error, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 — streaming upload bodies are not replayable", attempts)
	}
}

func TestRetryDelayHonorsRetryAfterAndBackoff(t *testing.T) {
	cases := []struct {
		retryAfter string
		attempt    int
		want       time.Duration
	}{
		{"3", 0, 3 * time.Second},          // Retry-After wins
		{"0", 5, 0},                        // immediate retry allowed
		{"", 0, time.Millisecond},          // base
		{"", 2, 4 * time.Millisecond},      // base << attempt
		{"garbage", 1, 2 * time.Millisecond},
	}
	base := time.Millisecond
	for _, tc := range cases {
		if got := retryDelay(base, tc.attempt, tc.retryAfter); got != tc.want {
			t.Errorf("retryDelay(%v, %d, %q) = %v, want %v", base, tc.attempt, tc.retryAfter, got, tc.want)
		}
	}
	if got := retryDelay(time.Hour, 64, ""); got != time.Minute {
		t.Errorf("overflow guard failed: got %v", got)
	}
}
