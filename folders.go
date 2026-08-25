package dreep

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// CreateFolderOptions describes POST /folders. Pass Name to create one folder
// (optionally under ParentID or ParentPath), or Path to create every missing
// segment of a chain in one call — the leaf folder is returned. Path is
// relative to the project root and cannot be combined with ParentID or
// ParentPath. The call is idempotent: existing segments are reused.
type CreateFolderOptions struct {
	Name       string
	Path       string
	ParentID   string
	ParentPath string

	// AccessControlType sets who can view assets inside the folder once
	// uploaded; subfolders inherit it.
	AccessControlType AccessControlType

	// DefaultExpirySeconds is a dashboard convenience for manually generated
	// signed links; SDK-generated SignedURLs always set exp explicitly.
	DefaultExpirySeconds int
}

// CreateFolder creates a folder (or a whole nested chain) and returns the leaf.
func (c *Client) CreateFolder(ctx context.Context, o CreateFolderOptions) (*Folder, error) {
	if o.Name == "" && o.Path == "" {
		return nil, fmt.Errorf("dreep: either Name or Path is required")
	}
	if o.Path != "" && (o.ParentID != "" || o.ParentPath != "") {
		return nil, fmt.Errorf("dreep: path cannot be combined with parentId or parentPath")
	}
	in := map[string]any{}
	setStr := func(k, v string) {
		if v != "" {
			in[k] = v
		}
	}
	setStr("name", o.Name)
	setStr("path", o.Path)
	setStr("parentId", o.ParentID)
	setStr("parentPath", o.ParentPath)
	setStr("accessControlType", string(o.AccessControlType))
	if o.DefaultExpirySeconds > 0 {
		in["defaultExpirySeconds"] = o.DefaultExpirySeconds
	}

	var out Folder
	if err := c.doJSON(ctx, http.MethodPost, "/folders", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListFoldersOptions scopes GET /folders. With everything unset the call
// returns every folder in the project at any depth. Set Path to list direct
// children of one path, or RootOnly to list root folders (parentId=null).
type ListFoldersOptions struct {
	Path     string
	ParentID string
	RootOnly bool
}

// FolderList is the response of ListFolders. CurrentFolder names the folder
// addressed by Path/ParentID when it exists, nil otherwise.
type FolderList struct {
	Folders       []Folder `json:"folders"`
	CurrentFolder *Folder  `json:"currentFolder"`
}

func (c *Client) ListFolders(ctx context.Context, o *ListFoldersOptions) (*FolderList, error) {
	q := url.Values{}
	if o != nil {
		switch {
		case o.RootOnly && (o.Path != "" || o.ParentID != ""):
			return nil, fmt.Errorf("dreep: RootOnly cannot be combined with Path or ParentID")
		case o.Path != "":
			q.Set("path", o.Path)
		case o.RootOnly:
			q.Set("parentId", "null")
		case o.ParentID != "":
			q.Set("parentId", o.ParentID)
		}
	}
	var out FolderList
	if err := c.doJSON(ctx, http.MethodGet, "/folders", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
