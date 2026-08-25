package dreep

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Error is the typed error returned for every non-2xx API response.
type Error struct {
	// StatusCode is the HTTP status returned by the API.
	StatusCode int
	// Code is the machine-readable error code (e.g. "invalid_request"),
	// when the API provides one.
	Code string
	// Message is the human-readable error message.
	Message string
}

func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("dreep: %d %s: %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("dreep: %d: %s", e.StatusCode, e.Message)
}

// IsNotFound reports whether err is a 404 from the API (e.g. unknown media id
// or a confirm call for an upload that never existed).
func IsNotFound(err error) bool { return hasStatus(err, http.StatusNotFound) }

// IsUnauthorized reports whether err is a 401 — a bad API key, or access to a
// private/signed asset without valid exp/sig parameters.
func IsUnauthorized(err error) bool { return hasStatus(err, http.StatusUnauthorized) }

// IsPaymentRequired reports whether err is a 402 — a billing limit was hit
// (storage full, transformation cap reached, or no background-removal credits).
func IsPaymentRequired(err error) bool { return hasStatus(err, http.StatusPaymentRequired) }

// IsConflict reports whether err is a 409 — for ConfirmUpload it means the
// file bytes have not finished landing in storage yet; retry shortly.
func IsConflict(err error) bool { return hasStatus(err, http.StatusConflict) }

// IsInvalidRequest reports whether the API rejected the request as malformed
// (the documented "invalid_request" code).
func IsInvalidRequest(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Code == "invalid_request"
}

func hasStatus(err error, codes ...int) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	for _, code := range codes {
		if e.StatusCode == code {
			return true
		}
	}
	return false
}

// apiErrorBody covers both documented error shapes:
//
//	{"error": true, "message": "...", "code": "invalid_request"}
//	{"error": "Billing limit reached", "details": "You have exceeded ..."}
type apiErrorBody struct {
	Error   json.RawMessage `json:"error"`
	Message string          `json:"message"`
	Code    string          `json:"code"`
	Details string          `json:"details"`
}

func (c *Client) errorFromResponse(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	drainAndClose(resp)

	e := &Error{
		StatusCode: resp.StatusCode,
		Message:    strings.TrimSpace(http.StatusText(resp.StatusCode)),
	}

	var ae apiErrorBody
	if len(body) > 0 && json.Unmarshal(body, &ae) == nil {
		var errString string
		raw := strings.TrimSpace(string(ae.Error))
		if raw != "" && raw != "true" && raw != "null" && raw[0] == '"' {
			_ = json.Unmarshal(ae.Error, &errString)
		}
		switch {
		case strings.TrimSpace(ae.Message) != "":
			e.Message = strings.TrimSpace(ae.Message)
		case errString != "":
			e.Message = errString
		case strings.TrimSpace(ae.Details) != "":
			e.Message = strings.TrimSpace(ae.Details)
		}
		e.Code = ae.Code
	}
	return e
}
