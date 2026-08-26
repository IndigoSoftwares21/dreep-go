//go:build live

package dreep

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// debugTransport records successful response bodies when DREEP_DEBUG is set,
// so failures can show what the real API actually returned.
type debugTransport struct {
	next http.RoundTripper

	mu  sync.Mutex
	log []string
}

func (d *debugTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := d.next.RoundTrip(r)
	if err != nil || os.Getenv("DREEP_DEBUG") == "" || resp.StatusCode >= 400 {
		return resp, err
	}
	body, _ := io.ReadAll(resp.Body) //nolint:errcheck
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	d.mu.Lock()
	if dump, derr := httputil.DumpResponse(resp, true); derr == nil {
		d.log = append(d.log, string(dump))
	}
	d.mu.Unlock()
	return resp, nil
}

func (d *debugTransport) dump() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return strings.Join(d.log, "\n---\n")
}

func newLiveClient(t *testing.T) *Client {
	t.Helper()
	key := os.Getenv("DREEP_API_KEY")
	if key == "" {
		t.Skip("DREEP_API_KEY not set")
	}
	opts := []Option{WithHTTPClient(&http.Client{
		Transport: &debugTransport{next: http.DefaultTransport},
	})}
	if sec := os.Getenv("DREEP_SIGNING_SECRET"); sec != "" {
		opts = append(opts, WithSigningSecret(sec))
	}
	c, err := New(key, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// TestLiveSmoke runs a real end-to-end round trip against api.dreep.cloud:
//
//	upload → list → delivery URL → signed URL (optional) → usage → delete
//
// Run it explicitly:
//
//	DREEP_API_KEY=drp_live_... go test -tags live -run TestLiveSmoke -v
//
// Set DREEP_DEBUG=1 to log raw response bodies. The uploaded asset is deleted
// at the end; nothing remains except the auto-created "dreep-go-smoke" folder.
func TestLiveSmoke(t *testing.T) {
	c := newLiveClient(t)
	dt := c.HTTP.Transport.(*debugTransport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Upload a tiny valid PNG, re-encoded to WebP.
	const png = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n-\xb4\x00\x00\x00\x00IEND\xaeB`\x82"
	asset, err := c.Upload(ctx, UploadOptions{
		File:        strings.NewReader(png),
		Filename:    "smoke.png",
		ContentType: "image/png",
		Destination: Destination{Folder: "dreep-go-smoke"},
		Transform:   &Transform{Width: 64, Format: FormatWebP},
		KnownSize:   int64(len(png)),
	})
	if err != nil {
		t.Fatalf("Upload: %v\nraw responses:\n%s", err, dt.dump())
	}
	if asset.ID == "" {
		t.Fatalf("empty asset id; response body:\n%s", dt.dump())
	}
	t.Logf("uploaded %s (%s) → %s", asset.ID, asset.Status, asset.URL)

	defer func() {
		dctx, dcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dcancel()
		if err := c.DeleteMedia(dctx, asset.ID); err != nil && !IsNotFound(err) {
			t.Errorf("cleanup DeleteMedia(%s): %v", asset.ID, err)
		} else {
			t.Logf("deleted %s", asset.ID)
		}
	}()

	// 2. The asset should appear in a listing of its folder.
	var found bool
	err = c.ListAllMedia(ctx, &ListMediaOptions{Folder: "dreep-go-smoke"}, func(a *MediaAsset) error {
		if a.ID == asset.ID {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Errorf("ListAllMedia: %v", err)
	} else if !found {
		t.Errorf("uploaded asset %s missing from folder listing", asset.ID)
	}

	// 3. Delivery-time transform URL must be well-formed.
	u := c.URL(asset.ID, &Transform{Width: 400})
	t.Logf("delivery URL: %s", u)

	// 4. Optional signed URL.
	if c.signingSecret != "" {
		su, err := c.SignedURL(asset.ID, time.Hour, nil)
		if err != nil {
			t.Errorf("SignedURL: %v", err)
		} else {
			t.Logf("signed URL: %s", su)
		}
	}

	// 5. Usage endpoint answers.
	usage, err := c.GetUsage(ctx)
	if err != nil {
		t.Errorf("GetUsage: %v", err)
	} else {
		t.Logf("usage: %+v", usage)
	}

	if t.Failed() {
		t.Logf("raw responses:\n%s", dt.dump())
	}
}

// TestLivePresets round-trips a preset against the real API: create → list →
// delete. It is opt-in so routine smoke runs do not leave artifacts behind:
//
//	DREEP_API_KEY=... DREEP_LIVE_PRESETS=1 go test -tags live -run TestLivePresets -v
func TestLivePresets(t *testing.T) {
	if os.Getenv("DREEP_LIVE_PRESETS") == "" {
		t.Skip("DREEP_LIVE_PRESETS not set")
	}
	c := newLiveClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	p, err := c.CreatePreset(ctx, CreatePresetOptions{
		Name:      "Go SDK Probe",
		Key:       "go_sdk_probe",
		Transform: &Transform{Width: 100, Height: 100, Format: FormatWebP, Quality: 80},
	})
	if err != nil {
		t.Fatalf("CreatePreset: %v", err)
	}
	t.Logf("created %+v", p)
	if p.ID == "" || p.Key != "go_sdk_probe" {
		t.Fatalf("unexpected preset response: %+v", p)
	}
	defer func() {
		if err := c.DeletePreset(context.Background(), p.ID); err != nil {
			t.Errorf("DeletePreset(%s): %v", p.ID, err)
		}
	}()

	ps, err := c.ListPresets(ctx)
	if err != nil {
		t.Fatalf("ListPresets: %v", err)
	}
	var found bool
	for _, q := range ps {
		if q.ID == p.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("created preset %s missing from listing (%d presets)", p.ID, len(ps))
	}
}
