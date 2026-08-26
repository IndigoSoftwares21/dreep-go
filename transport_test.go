package dreep

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponseEnvelopeUnwrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		io.WriteString(w, `{"message":"Upload successful","code":201,"data":{"id":"med_env","url":"https://cdn/x.webp","status":"ready"}}`)
	}))
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	asset, err := c.Upload(context.Background(), UploadOptions{
		File:     strings.NewReader("x"),
		Filename: "a.png",
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if asset.ID != "med_env" || asset.Status != "ready" {
		t.Errorf("asset = %+v", asset)
	}
}

func TestPlainBodyStillDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"assets":[{"id":"med_1"}],"pagination":{"page":1,"limit":20,"total":1}}`)
	}))
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	page, err := c.ListMedia(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListMedia: %v", err)
	}
	if len(page.Assets) != 1 || page.Assets[0].ID != "med_1" || page.Pagination.Total != 1 {
		t.Errorf("page = %+v", page)
	}
}

func TestErrorMappingDocumentedShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": true, "message": "Invalid request parameters", "code": "invalid_request"}`))
	}))
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	_, err := c.ListMedia(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("want *Error, got %T: %v", err, err)
	}
	if e.StatusCode != 400 || e.Code != "invalid_request" || e.Message != "Invalid request parameters" {
		t.Errorf("unexpected error: %+v", e)
	}
	if !IsInvalidRequest(err) {
		t.Error("IsInvalidRequest = false")
	}
}

func TestErrorMappingBillingLimitFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		// used/limit as BIGINT-as-string, as the API serialises them.
		w.Write([]byte(`{"error": true, "message": "Background removal limit reached", "code": "limit_reached", "featureKey": "bg-removal", "used": "50", "limit": "50"}`))
	}))
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	_, err := c.RemoveBackground(context.Background(), strings.NewReader("x"), "med_1", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsPaymentRequired(err) {
		t.Errorf("IsPaymentRequired = false for %v", err)
	}
	key, ok := FeatureLimit(err)
	if !ok || key != "bg-removal" {
		t.Fatalf("FeatureLimit = %q, %v", key, ok)
	}
	var e *Error
	errors.As(err, &e)
	if e.Used.Int64() != 50 || e.Limit.Int64() != 50 {
		t.Errorf("used=%d limit=%d", e.Used.Int64(), e.Limit.Int64())
	}

	// Non-limit errors carry no feature metadata.
	_, plain := FeatureLimit(errors.New("x"))
	if plain {
		t.Error("FeatureLimit reported true for a non-dreep error")
	}
}

func TestErrorMappingBillingShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"error": "Billing limit reached", "details": "You have exceeded your plan's storage_bytes limit."}`))
	}))
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	err := c.DeleteMedia(context.Background(), "m1")
	if !IsPaymentRequired(err) {
		t.Fatalf("IsPaymentRequired(%v) = false", err)
	}
	var e *Error
	if jsonErr := asError(err, &e); jsonErr != nil {
		t.Fatal(jsonErr)
	}
	if e.Message != "Billing limit reached" {
		t.Errorf("Message = %q", e.Message)
	}
}

func TestErrorFallbackNonJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("bad gateway"))
	}))
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	_, err := c.ListMedia(context.Background(), nil)
	var e *Error
	if !asBool(err, &e) || e.StatusCode != 502 {
		t.Fatalf("want *Error 502, got %v", err)
	}
}

func TestAuthHeaderOnEveryRequest(t *testing.T) {
	var gotAuth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{})
		case http.MethodDelete:
			json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"error":true,"message":"nf","code":"not_found"}`))
		}
	}))
	defer srv.Close()

	c, _ := New("drp_live_secret", WithAPIBaseURL(srv.URL))
	_, _ = c.ListMedia(context.Background(), nil)
	_ = c.DeleteMedia(context.Background(), "m")
	_, _ = c.ListFolders(context.Background(), nil)

	for i, a := range gotAuth {
		if a != "Bearer drp_live_secret" {
			t.Errorf("request %d Authorization = %q", i, a)
		}
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Error("expected error for empty API key")
	}
	if _, err := New("k", func(c *Client) error { return errTest }); err == nil {
		t.Error("expected option error to propagate")
	}
}

type simpleErr struct{ msg string }

func (e *simpleErr) Error() string { return e.msg }

var errTest = &simpleErr{"boom"}

func asError(err error, target **Error) error {
	e, ok := err.(*Error)
	if !ok {
		return err
	}
	*target = e
	return nil
}

func asBool(err error, target **Error) bool {
	e, ok := err.(*Error)
	if ok {
		*target = e
	}
	return ok
}
