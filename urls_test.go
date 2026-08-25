package dreep

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	c, err := New("drp_live_test", WithSigningSecret("topsecret"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.now = func() time.Time { return time.Unix(1700000000, 0) }
	return c
}

func TestSignedURLMatchesReferenceAlgorithm(t *testing.T) {
	c := testClient(t)
	got, err := c.SignedURL("med_123", time.Hour, nil)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}

	const expires = int64(1700000000 + 3600)
	mac := hmac.New(sha256.New, []byte("topsecret"))
	fmt.Fprintf(mac, "med_123:%d", expires)
	wantSig := hex.EncodeToString(mac.Sum(nil))
	want := fmt.Sprintf("https://cdn.dreep.cloud/api/v1/fetch/med_123?exp=%d&sig=%s", expires, wantSig)

	if got != want {
		t.Errorf("SignedURL mismatch:\n got %s\nwant %s", got, want)
	}
}

func TestSignedURLAppendsTransformsAfterSignatureParams(t *testing.T) {
	c := testClient(t)
	got, err := c.SignedURL("abc", time.Minute, &Transform{Format: FormatWebP, Width: 500})
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse %q: %v", got, err)
	}
	if !strings.HasSuffix(u.Path, "/abc.webp") {
		t.Errorf("path = %q, want suffix /abc.webp", u.Path)
	}
	q := u.Query()
	if q.Get("width") != "500" || q.Get("format") != "webp" {
		t.Errorf("transform params missing: %v", q)
	}
	if q.Get("exp") == "" || len(q.Get("sig")) != 64 {
		t.Errorf("exp/sig missing or malformed: %v", q)
	}
}

func TestSignedURLRequiresSecret(t *testing.T) {
	c, _ := New("k")
	if _, err := c.SignedURL("x", time.Minute, nil); err != ErrNoSigningSecret {
		t.Errorf("err = %v, want ErrNoSigningSecret", err)
	}
}

func TestURLBuilding(t *testing.T) {
	c := testClient(t)

	cases := []struct {
		name string
		id   string
		tr   *Transform
		want string
	}{
		{"no transform", "med_1", nil, "https://cdn.dreep.cloud/api/v1/fetch/med_1"},
		{
			"width only",
			"med_1", &Transform{Width: 400},
			"https://cdn.dreep.cloud/api/v1/fetch/med_1?width=400",
		},
		{
			"format becomes extension",
			"med_1", &Transform{Width: 400, Format: FormatWebP},
			"https://cdn.dreep.cloud/api/v1/fetch/med_1.webp?format=webp&width=400",
		},
		{
			"crop comma form",
			"med_2", &Transform{Crop: &Crop{Left: 10, Top: 20, Width: 100, Height: 100}},
			"https://cdn.dreep.cloud/api/v1/fetch/med_2?crop=10%2C20%2C100%2C100",
		},
		{
			"floats and enums",
			"vid", &Transform{DPR: 2.5, TrimStart: Ptr(1.5), TrimEnd: Ptr(9.25), Fit: FitCover, Gravity: GravityAttention},
			"https://cdn.dreep.cloud/api/v1/fetch/vid?dpr=2.5&fit=cover&gravity=attention&trimEnd=9.25&trimStart=1.5",
		},
		{
			"rotate allowed values only",
			"r", &Transform{Rotate: 45},
			"https://cdn.dreep.cloud/api/v1/fetch/r",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.URL(tc.id, tc.tr); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}
