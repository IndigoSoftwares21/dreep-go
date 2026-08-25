package dreep

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ListMediaOptions filters GET /media. Folder and FolderID are mutually
// exclusive; Recursive requires one of them.
type ListMediaOptions struct {
	Page      int
	Limit     int
	Folder    string
	FolderID  string
	Recursive bool
}

// ListMedia returns one page of ready assets, newest first. Pending uploads
// are excluded. An unknown folder path yields a 400 (*IsInvalidRequest*), so
// a typo does not look like an empty folder.
func (c *Client) ListMedia(ctx context.Context, o *ListMediaOptions) (*MediaPage, error) {
	opts := ListMediaOptions{}
	if o != nil {
		opts = *o
	}
	if opts.Folder != "" && opts.FolderID != "" {
		return nil, fmt.Errorf("dreep: folder and folderId are mutually exclusive")
	}
	q := url.Values{}
	if opts.Page > 0 {
		q.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	hasFolder := false
	if opts.Folder != "" {
		q.Set("folder", opts.Folder)
		hasFolder = true
	}
	if opts.FolderID != "" {
		q.Set("folderId", opts.FolderID)
		hasFolder = true
	}
	if hasFolder && opts.Recursive {
		q.Set("recursive", "true")
	}
	var out MediaPage
	if err := c.doJSON(ctx, http.MethodGet, "/media", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListAllMedia iterates every page of GET /media, calling fn for each asset
// until listing ends or fn returns an error. Page/Limit from o seed the walk
// (page defaults to 1); later pages follow the API's reported pagination.
func (c *Client) ListAllMedia(ctx context.Context, o *ListMediaOptions, fn func(*MediaAsset) error) error {
	if fn == nil {
		return fmt.Errorf("dreep: ListAllMedia requires a callback")
	}
	opts := ListMediaOptions{}
	if o != nil {
		opts = *o
	}
	page := opts.Page
	if page < 1 {
		page = 1
	}
	for {
		po := opts
		po.Page = page
		mp, err := c.ListMedia(ctx, &po)
		if err != nil {
			return err
		}
		for _, a := range mp.Assets {
			if err := fn(a); err != nil {
				return err
			}
		}
		limit := mp.Pagination.Limit
		if limit <= 0 {
			limit = len(mp.Assets)
		}
		if len(mp.Assets) == 0 || limit == 0 ||
			(mp.Pagination.Total > 0 && page*limit >= mp.Pagination.Total) {
			return nil
		}
		page++
	}
}

// DeleteMedia permanently removes the asset with the given ID.
func (c *Client) DeleteMedia(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("dreep: media id is required")
	}
	var out struct {
		Success bool `json:"success"`
	}
	return c.doJSON(ctx, http.MethodDelete, "/media/"+id, nil, nil, &out)
}
