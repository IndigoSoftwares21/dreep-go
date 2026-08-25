package dreep

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrNoSigningSecret is returned by SignedURL when the client was constructed
// without WithSigningSecret.
var ErrNoSigningSecret = errors.New("dreep: signing secret not configured (pass dreep.WithSigningSecret)")

// URL builds a delivery-time transformation URL for an asset without any
// network call. Transformation parameters are applied on the fly at request
// time:
//
//	client.URL("med_123", &dreep.Transform{Width: 400})
//	// https://cdn.dreep.cloud/api/v1/fetch/med_123?width=400
//
// When t.Format is set it is appended to the path as an extension, matching
// the documented behaviour (e.g. ".../med_123.webp").
func (c *Client) URL(assetID string, t *Transform) string {
	u := strings.TrimRight(c.CDNBaseURL, "/") + "/" + assetID
	if t != nil && t.Format != "" && t.Format != FormatTXT {
		ext := string(t.Format)
		ext = strings.TrimPrefix(ext, ".")
		if !strings.HasSuffix(u, "."+ext) {
			u += "." + ext
		}
	}
	q := transformQuery(t)
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}
	return u
}

// SignedURL returns a time-limited signed URL for an asset in a signed
// folder. The signature is an HMAC-SHA256 hex digest of "<assetID>:<expires>"
// keyed with the project's URL Signing Secret; it locks only the asset ID and
// expiry, so transform parameters remain free to vary at delivery time.
func (c *Client) SignedURL(assetID string, ttl time.Duration, t *Transform) (string, error) {
	exp, sig, err := c.signParams(assetID, ttl)
	if err != nil {
		return "", err
	}
	base := c.URL(assetID, t)
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%sexp=%d&sig=%s", base, sep, exp, sig), nil
}

// signParams computes the exp/sig pair shared by SignedURL and ExtractText.
func (c *Client) signParams(assetID string, ttl time.Duration) (int64, string, error) {
	if c.signingSecret == "" {
		return 0, "", ErrNoSigningSecret
	}
	expires := c.now().Add(ttl).Unix()
	mac := hmac.New(sha256.New, []byte(c.signingSecret))
	fmt.Fprintf(mac, "%s:%d", assetID, expires)
	return expires, hex.EncodeToString(mac.Sum(nil)), nil
}

// transformQuery converts t into delivery-time query parameters. Crop is sent
// comma-separated ("left,top,width,height") rather than as a JSON object.
func transformQuery(t *Transform) url.Values {
	q := url.Values{}
	if t == nil {
		return q
	}
	setStr := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	if t.Format != "" {
		q.Set("format", string(t.Format))
	}
	if t.Width > 0 {
		q.Set("width", strconv.Itoa(t.Width))
	}
	if t.Height > 0 {
		q.Set("height", strconv.Itoa(t.Height))
	}
	setStr("fit", string(t.Fit))
	if t.Quality > 0 {
		q.Set("quality", strconv.Itoa(t.Quality))
	}
	if t.FPS > 0 {
		q.Set("fps", strconv.Itoa(t.FPS))
	}
	setStr("videoCodec", string(t.VideoCodec))
	setStr("gravity", string(t.Gravity))
	if t.DPR > 0 {
		q.Set("dpr", formatFloat(t.DPR))
	}
	switch t.Rotate {
	case 90, 180, 270:
		q.Set("rotate", strconv.Itoa(t.Rotate))
	}
	setStr("bg", t.BG)
	setStr("radius", t.Radius)
	setStr("p", t.Preset)
	if t.Crop != nil {
		q.Set("crop", fmt.Sprintf("%d,%d,%d,%d", t.Crop.Left, t.Crop.Top, t.Crop.Width, t.Crop.Height))
	}
	if t.TrimStart != nil {
		q.Set("trimStart", formatFloat(*t.TrimStart))
	}
	if t.TrimEnd != nil {
		q.Set("trimEnd", formatFloat(*t.TrimEnd))
	}
	return q
}
