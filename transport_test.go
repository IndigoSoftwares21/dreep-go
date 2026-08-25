package dreep

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
