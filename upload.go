package dreep

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Destination names where an upload lands. At most one field may be set;
// omitting all three uploads into the project's default folder. Missing
// folders in Folder are created automatically (unless AutoCreateFolders is
// false). Key is a full S3-style object key whose last segment becomes the
// stored filename.
type Destination struct {
	Folder   string
	Key      string
	FolderID string
}

func (d Destination) validate() error {
	n := 0
	for _, v := range []string{d.Folder, d.Key, d.FolderID} {
		if v != "" {
			n++
		}
	}
	if n > 1 {
		return fmt.Errorf("dreep: folder, key and folderId are mutually exclusive")
	}
	return nil
}

// UploadOptions describes one multipart Upload call. File and Filename are
// required; KnownSize optionally supplies the content length up front for a
// streaming reader (otherwise chunked transfer encoding is used).
type UploadOptions struct {
	File        io.Reader
	Filename    string
	ContentType string // optional explicit MIME type for the file part

	Destination

	// AutoCreateFolders defaults to true; set to false to require the folder
	// path to already exist (the API then answers 404 for unknown paths).
	AutoCreateFolders *bool

	Transform *Transform

	// KnownSize, when positive, is passed as the request ContentLength.
	KnownSize int64
}

func (c *Client) Upload(ctx context.Context, o UploadOptions) (*MediaAsset, error) {
	if o.File == nil {
		return nil, fmt.Errorf("dreep: UploadOptions.File is required")
	}
	if o.Filename == "" {
		return nil, fmt.Errorf("dreep: UploadOptions.Filename is required")
	}
	if err := o.Destination.validate(); err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		pw.CloseWithError(writeUploadForm(mw, o))
	}()

	req, err := c.newRequest(ctx, http.MethodPost, c.APIBaseURL+"/upload", pr, mw.FormDataContentType())
	if err != nil {
		pr.Close()
		return nil, err
	}
	if o.KnownSize > 0 {
		req.ContentLength = o.KnownSize
	}

	var asset MediaAsset
	if err := c.do(req, &asset); err != nil {
		return nil, err
	}
	return &asset, nil
}

// writeUploadForm streams the multipart body: file first, then metadata and
// transform fields.
func writeUploadForm(mw *multipart.Writer, o UploadOptions) error {
	defer mw.Close()

	fw, err := mw.CreateFormFile("file", o.Filename)
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, o.File); err != nil {
		return err
	}

	writeStr := func(k, v string) {
		if v != "" {
			mw.WriteField(k, v)
		}
	}
	writeStr("folder", o.Destination.Folder)
	writeStr("key", o.Destination.Key)
	writeStr("folderId", o.Destination.FolderID)
	if o.AutoCreateFolders != nil {
		mw.WriteField("autoCreateFolders", strconv.FormatBool(*o.AutoCreateFolders))
	}

	if o.Transform == nil {
		return nil
	}
	for k, v := range transformMap(o.Transform) {
		var s string
		switch val := v.(type) {
		case string:
			s = val
		case int:
			s = strconv.Itoa(val)
		case float64:
			s = formatFloat(val)
		default:
			b, err := json.Marshal(val)
			if err != nil {
				return err
			}
			s = string(b)
		}
		mw.WriteField(k, s)
	}
	return nil
}

// PresignOptions describes POST /upload/presign — the first step of the
// direct-to-storage upload flow used for large files and browser uploads.
type PresignOptions struct {
	ContentType string // required, e.g. "image/webp"
	SizeBytes   int64  // required
	Filename    string // optional when Key is given

	Destination

	AutoCreateFolders *bool
	Format            Format // target re-encode format applied on confirm
	PresetKey         string
	ContentHash       string // SHA-256 of the file; dedupes against existing content
}

// PresignedUpload is the response of PresignUpload. When AlreadyExists is
// true the content was deduplicated and there is nothing to PUT — go straight
// to ConfirmUpload with ID.
type PresignedUpload struct {
	ID            string `json:"id"`
	UploadURL     string `json:"uploadUrl,omitempty"`
	ContentType   string `json:"contentType,omitempty"`
	AlreadyExists bool   `json:"alreadyExists"`
}

func (c *Client) PresignUpload(ctx context.Context, o PresignOptions) (*PresignedUpload, error) {
	if o.ContentType == "" {
		return nil, fmt.Errorf("dreep: PresignOptions.ContentType is required")
	}
	if o.SizeBytes <= 0 {
		return nil, fmt.Errorf("dreep: PresignOptions.SizeBytes must be positive")
	}
	if err := o.Destination.validate(); err != nil {
		return nil, err
	}
	in := map[string]any{
		"contentType": o.ContentType,
		"sizeBytes":   o.SizeBytes,
	}
	setStr := func(k, v string) {
		if v != "" {
			in[k] = v
		}
	}
	setStr("filename", o.Filename)
	setStr("folder", o.Destination.Folder)
	setStr("key", o.Destination.Key)
	setStr("folderId", o.Destination.FolderID)
	setStr("format", string(o.Format))
	setStr("presetKey", o.PresetKey)
	setStr("contentHash", o.ContentHash)
	if o.AutoCreateFolders != nil {
		in["autoCreateFolders"] = strconv.FormatBool(*o.AutoCreateFolders)
	}

	var out PresignedUpload
	if err := c.doJSON(ctx, http.MethodPost, "/upload/presign", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ConfirmOptions carries the optional transform applied when finalising a
// presigned upload.
type ConfirmOptions struct {
	Transform *Transform
	PresetKey string
}

// ConfirmUpload finalises a presigned upload after its bytes reached storage.
// The call is idempotent: an already-ready asset is returned as-is.
//
// On success the API's asset payload is decoded when present; some responses
// carry an empty body, in which case a nil asset with a nil error is
// returned.
//
// A 409 (*see IsConflict*) means the bytes have not landed yet — retry
// shortly. UploadPresigned does this automatically.
func (c *Client) ConfirmUpload(ctx context.Context, id string, o *ConfirmOptions) (*MediaAsset, error) {
	if id == "" {
		return nil, fmt.Errorf("dreep: upload id is required")
	}
	in := map[string]any{}
	if o != nil {
		if t := transformMap(o.Transform); len(t) > 0 {
			in["transform"] = t
		}
		if o.PresetKey != "" {
			in["presetKey"] = o.PresetKey
		}
	}
	req, err := c.newRequest(ctx, http.MethodPost,
		fmt.Sprintf("%s/upload/%s/confirm", c.APIBaseURL, id),
		strings.NewReader(mustJSON(in)), "application/json")
	if err != nil {
		return nil, err
	}
	resp, derr := c.HTTP.Do(req)
	if derr != nil {
		return nil, fmt.Errorf("dreep: %w", derr)
	}
	if resp.StatusCode >= 400 {
		return nil, c.errorFromResponse(resp)
	}
	var asset MediaAsset
	if err := decodeJSONBody(resp, &asset); err != nil {
		return nil, err
	}
	if asset.ID == "" {
		return nil, nil
	}
	return &asset, nil
}

// DirectUploadOptions drives UploadPresigned, the three-step convenience:
// presign → PUT raw bytes to storage → confirm.
type DirectUploadOptions struct {
	PresignOptions

	// File is the raw content to PUT to storage.
	File io.Reader

	// Transform, applied at confirm time.
	Transform *Transform

	// PresetKey, applied at confirm time.
	PresetKey string
}

// UploadPresigned performs the full presigned-upload flow in one call:
//
//  1. POST /upload/presign to obtain a short-lived storage URL,
//  2. PUT the file bytes straight to storage (skipped when the API reports
//     AlreadyExists via ContentHash deduplication),
//  3. POST /upload/{id}/confirm, retrying on 409 while the bytes settle.
//
// Use it for large files or whenever bytes should bypass the Dreep API.
func (c *Client) UploadPresigned(ctx context.Context, o DirectUploadOptions) (*MediaAsset, error) {
	if o.File == nil {
		return nil, fmt.Errorf("dreep: DirectUploadOptions.File is required")
	}
	pre, err := c.PresignUpload(ctx, o.PresignOptions)
	if err != nil {
		return nil, err
	}
	if !pre.AlreadyExists {
		if err := putBytes(ctx, c.HTTP, pre.UploadURL, o.ContentType, o.File); err != nil {
			return nil, err
		}
	}
	for attempt := 0; ; attempt++ {
		asset, err := c.ConfirmUpload(ctx, pre.ID, &ConfirmOptions{Transform: o.Transform, PresetKey: o.PresetKey})
		if err == nil {
			return asset, nil
		}
		if !IsConflict(err) || attempt >= 4 {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// putBytes PUTs raw content to a presigned storage URL. The URL is already
// signed by the API — no Authorization header is attached.
func putBytes(ctx context.Context, hc *http.Client, url, contentType string, r io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, r)
	if err != nil {
		return fmt.Errorf("dreep: building storage PUT: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("dreep: storage PUT: %w", err)
	}
	if resp.StatusCode >= 400 {
		return cErrorFromStorageResponse(resp)
	}
	drainAndClose(resp)
	return nil
}

func cErrorFromStorageResponse(resp *http.Response) error {
	e := &Error{StatusCode: resp.StatusCode, Message: resp.Status}
	drainAndClose(resp)
	return e
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
