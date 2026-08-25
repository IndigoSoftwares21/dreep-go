package dreep

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// RemoveBackgroundOptions configures a background-removal request.
type RemoveBackgroundOptions struct {
	Format Format // output format; PNG keeps transparency
	Destination
	AutoCreateFolders *bool
}

// CutoutAsset is the new transparent-background asset produced by
// RemoveBackground. Serve it over a colour with Transform.BG at delivery
// time — no extra removal charge.
type CutoutAsset struct {
	ID         string    `json:"id"`
	StorageKey string    `json:"storageKey,omitempty"`
	URL        string    `json:"url,omitempty"`
	Format     string    `json:"format,omitempty"`
	Width      int       `json:"width,omitempty"`
	Height     int       `json:"height,omitempty"`
	SizeBytes  flexInt64 `json:"sizeBytes,omitempty"`
}

// RemoveBackgroundResult wraps the cutout asset returned by POST /bg-remove.
type RemoveBackgroundResult struct {
	Asset CutoutAsset `json:"asset"`
}

// RemoveBackground uploads an image and stores a cutout of its subject with a
// transparent background as a new asset. Billed from a consumable add-on: a
// 402 (*IsPaymentRequired*) means no credits remain.
func (c *Client) RemoveBackground(ctx context.Context, file io.Reader, filename string, o *RemoveBackgroundOptions) (*RemoveBackgroundResult, error) {
	if file == nil {
		return nil, fmt.Errorf("dreep: file is required")
	}
	if filename == "" {
		return nil, fmt.Errorf("dreep: filename is required")
	}
	opts := RemoveBackgroundOptions{}
	if o != nil {
		opts = *o
	}
	if err := opts.Destination.validate(); err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		pw.CloseWithError(func() error {
			defer mw.Close()
			fw, err := mw.CreateFormFile("file", filename)
			if err != nil {
				return err
			}
			if _, err := io.Copy(fw, file); err != nil {
				return err
			}
			writeStr := func(k, v string) {
				if v != "" {
					mw.WriteField(k, v)
				}
			}
			writeStr("format", string(opts.Format))
			writeStr("folder", opts.Destination.Folder)
			writeStr("folderId", opts.Destination.FolderID)
			if opts.AutoCreateFolders != nil {
				mw.WriteField("autoCreateFolders", strconv.FormatBool(*opts.AutoCreateFolders))
			}
			return nil
		}())
	}()

	req, err := c.newRequest(ctx, http.MethodPost, c.APIBaseURL+"/bg-remove", pr, mw.FormDataContentType())
	if err != nil {
		pr.Close()
		return nil, err
	}
	var out RemoveBackgroundResult
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TextOptions optionally authenticates ExtractText against signed folders by
// attaching exp/sig computed with the configured signing secret.
type TextOptions struct {
	// SigningTTL is how long the generated exp stays valid. Zero disables
	// signing entirely (public folders need nothing).
	SigningTTL time.Duration
}

// ExtractText runs OCR on an already-stored image or PDF asset and returns
// the extracted plain text. It is an alias for fetching the asset with
// format=txt; results are cached in storage, so repeat reads are fast and free.
func (c *Client) ExtractText(ctx context.Context, mediaID string, o *TextOptions) (string, error) {
	if mediaID == "" {
		return "", fmt.Errorf("dreep: media id is required")
	}
	full := strings.TrimRight(c.CDNBaseURL, "/") + "/" + mediaID + ".txt"
	var q url.Values
	if o != nil && o.SigningTTL > 0 {
		exp, sig, err := c.signParams(mediaID, o.SigningTTL)
		if err != nil {
			return "", err
		}
		q = url.Values{}
		q.Set("exp", strconv.FormatInt(exp, 10))
		q.Set("sig", sig)
	}
	if len(q) > 0 {
		full += "?" + q.Encode()
	}
	req, err := c.newRequest(ctx, http.MethodGet, full, nil, "")
	if err != nil {
		return "", err
	}
	resp, derr := c.HTTP.Do(req)
	if derr != nil {
		return "", fmt.Errorf("dreep: %w", derr)
	}
	if resp.StatusCode >= 400 {
		return "", c.errorFromResponse(resp)
	}
	defer drainAndClose(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrBody))
	if err != nil {
		return "", fmt.Errorf("dreep: reading OCR response: %w", err)
	}
	return string(body), nil
}

// UploadAndExtractOptions configures UploadAndExtractText.
type UploadAndExtractOptions struct {
	Destination // optional: saving requires folder or folderId

	AutoCreateFolders *bool
}

// SavedTextAsset references the .txt asset stored alongside an upload when a
// destination folder was given.
type SavedTextAsset struct {
	ID         string `json:"id"`
	StorageKey string `json:"storageKey,omitempty"`
}

// OCRResult is the response of UploadAndExtractText.
type OCRResult struct {
	Text       string           `json:"text"`
	Blocks     []map[string]any `json:"blocks,omitempty"`
	SavedAsset *SavedTextAsset  `json:"savedAsset,omitempty"`
}

// UploadAndExtractText uploads an image or PDF and extracts its text via OCR.
// When Destination names a folder, the text is also stored there as a .txt
// asset and referenced in the result's SavedAsset field.
func (c *Client) UploadAndExtractText(ctx context.Context, file io.Reader, filename string, o *UploadAndExtractOptions) (*OCRResult, error) {
	if file == nil {
		return nil, fmt.Errorf("dreep: file is required")
	}
	if filename == "" {
		return nil, fmt.Errorf("dreep: filename is required")
	}
	opts := UploadAndExtractOptions{}
	if o != nil {
		opts = *o
	}
	if err := opts.Destination.validate(); err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		pw.CloseWithError(func() error {
			defer mw.Close()
			fw, err := mw.CreateFormFile("file", filename)
			if err != nil {
				return err
			}
			if _, err := io.Copy(fw, file); err != nil {
				return err
			}
			writeStr := func(k, v string) {
				if v != "" {
					mw.WriteField(k, v)
				}
			}
			writeStr("folder", opts.Destination.Folder)
			writeStr("folderId", opts.Destination.FolderID)
			if opts.AutoCreateFolders != nil {
				mw.WriteField("autoCreateFolders", strconv.FormatBool(*opts.AutoCreateFolders))
			}
			return nil
		}())
	}()

	req, err := c.newRequest(ctx, http.MethodPost, c.APIBaseURL+"/ocr", pr, mw.FormDataContentType())
	if err != nil {
		pr.Close()
		return nil, err
	}
	var out OCRResult
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
