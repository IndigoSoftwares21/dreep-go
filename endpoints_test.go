package dreep

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func sprintAny(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestListMediaPagination(t *testing.T) {
	const total = 45
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		if page < 1 {
			page = 1
		}
		if limit <= 0 {
			limit = 20
		}
		start := (page - 1) * limit
		end := start + limit
		if end > total {
			end = total
		}
		assets := []map[string]any{}
		for i := start; i < end; i++ {
			assets = append(assets, map[string]any{
				"id":        fmt.Sprintf("med_%03d", i),
				"filename":  fmt.Sprintf("f%d.webp", i),
				"url":       "u",
				"type":      "image/webp",
				"sizeBytes": "56696", // documented BIGINT-as-string shape
				"folder":    "avatars",
			})
		}
		json.NewEncoder(w).Encode(map[string]any{
			"assets": assets,
			"pagination": map[string]int{
				"page": page, "limit": limit, "total": total,
			},
		})
	}))
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))

	mp, err := c.ListMedia(context.Background(), &ListMediaOptions{Page: 2, Limit: 20})
	if err != nil {
		t.Fatalf("ListMedia: %v", err)
	}
	if len(mp.Assets) != 20 || mp.Pagination.Total != total {
		t.Errorf("page = %+v / %d assets", mp.Pagination, len(mp.Assets))
	}
	if mp.Assets[0].SizeBytes.Int64() != 56696 {
		t.Errorf("flexInt64 string decode failed: %v", mp.Assets[0].SizeBytes)
	}

	count := 0
	err = c.ListAllMedia(context.Background(), &ListMediaOptions{Limit: 20}, func(a *MediaAsset) error {
		count++
		if a.Filename == "" {
			t.Errorf("asset %d missing filename", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ListAllMedia: %v", err)
	}
	if count != total {
		t.Errorf("iterated %d assets, want %d", count, total)
	}
}

func TestListAllMediaStopsOnCallbackError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"assets":     []map[string]any{{"id": "a"}, {"id": "b"}},
			"pagination": map[string]int{"page": 1, "limit": 2, "total": 40},
		})
	}))
	defer srv.Close()
	c, _ := New("k", WithAPIBaseURL(srv.URL))
	sentinel := &simpleErr{"stop"}
	err := c.ListAllMedia(context.Background(), nil, func(a *MediaAsset) error {
		if a.ID == "b" {
			return sentinel
		}
		return nil
	})
	if err != sentinel {
		t.Errorf("err = %v, want sentinel", err)
	}
}

func TestFoldersCreateAndList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/folders", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			if in["path"] != "invoices/2024/q1" || in["accessControlType"] != "signed" || in["defaultExpirySeconds"] != float64(86400) {
				t.Errorf("create body = %v", in)
			}
			json.NewEncoder(w).Encode(map[string]any{"id": "f1", "name": "q1", "path": "invoices/2024/q1"})
		case http.MethodGet:
			if q := r.URL.Query().Get("parentId"); q != "null" {
				t.Errorf("parentId query = %q, want literal null", q)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"folders": []map[string]any{
					{"id": "f1", "name": "q1", "path": "invoices/2024/q1", "accessControlType": "public"},
				},
				"currentFolder": nil,
			})
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))

	f, err := c.CreateFolder(context.Background(), CreateFolderOptions{
		Path:                 "invoices/2024/q1",
		AccessControlType:    AccessSigned,
		DefaultExpirySeconds: 86400,
	})
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if f.Path != "invoices/2024/q1" {
		t.Errorf("folder = %+v", f)
	}
	if _, err := c.CreateFolder(context.Background(), CreateFolderOptions{
		Path: "a", ParentID: "x",
	}); err == nil {
		t.Error("want error combining path+parentId")
	}

	fl, err := c.ListFolders(context.Background(), &ListFoldersOptions{RootOnly: true})
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(fl.Folders) != 1 || fl.CurrentFolder != nil {
		t.Errorf("folders = %+v", fl)
	}
}

func TestPresetsLifecycle(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/presets", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var in struct {
				Name       string            `json:"name"`
				Operations []PresetOperation `json:"operations"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			if in.Name != "Thumbnail Generation" {
				t.Errorf("name = %q", in.Name)
			}
			b, _ := json.Marshal(in.Operations)
			if !strings.Contains(string(b), `"action":"resize"`) ||
				!strings.Contains(string(b), `"width":200`) {
				t.Errorf("operations = %s", b)
			}
			json.NewEncoder(w).Encode(map[string]any{"id": "p1", "name": in.Name})
		case http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"presets": []map[string]any{{"id": "p1", "name": "Thumbnail Generation"}},
			})
		}
	})
	mux.HandleFunc("/presets/p1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	p, err := c.CreatePreset(context.Background(), "Thumbnail Generation", []PresetOperation{
		{Action: "resize", Params: map[string]any{"width": 200, "height": 200}},
	})
	if err != nil {
		t.Fatalf("CreatePreset: %v", err)
	}
	if p.ID != "p1" {
		t.Errorf("preset = %+v", p)
	}
	ps, err := c.ListPresets(context.Background())
	if err != nil || len(ps) != 1 || ps[0].Name != "Thumbnail Generation" {
		t.Errorf("presets = %+v err=%v", ps, err)
	}
	if err := c.DeletePreset(context.Background(), "p1"); err != nil {
		t.Fatalf("DeletePreset: %v", err)
	}
}

func TestRemoveBackground(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields, data, err := parseMultipart(r)
		if err != nil {
			t.Errorf("multipart: %v", err)
		}
		if fields["format"] != "png" || fields["folder"] != "products/cutouts" || string(data) != "IMG" {
			t.Errorf("fields=%v data=%q", fields, data)
		}
		w.Write([]byte(`{"asset":{"id":"cut_1","url":"u","format":"png","width":100,"height":100,"sizeBytes":2048}}`))
	}))
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	res, err := c.RemoveBackground(context.Background(), strings.NewReader("IMG"), "shoe.png",
		&RemoveBackgroundOptions{Format: FormatPNG, Destination: Destination{Folder: "products/cutouts"}})
	if err != nil {
		t.Fatalf("RemoveBackground: %v", err)
	}
	if res.Asset.ID != "cut_1" || res.Asset.SizeBytes.Int64() != 2048 {
		t.Errorf("result = %+v", res)
	}
}

func TestExtractTextSigned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/med_5.txt") {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("exp") == "" || r.URL.Query().Get("sig") == "" {
			t.Error("missing exp/sig for signed folder OCR")
		}
		w.Write([]byte("Invoice #1234\nTotal: $50.00"))
	}))
	defer srv.Close()

	c := testClient(t)
	c.CDNBaseURL = srv.URL + "/fetch"

	text, err := c.ExtractText(context.Background(), "med_5", &TextOptions{SigningTTL: time.Hour})
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if text != "Invoice #1234\nTotal: $50.00" {
		t.Errorf("text = %q", text)
	}
}

func TestUploadAndExtractText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields, data, err := parseMultipart(r)
		if err != nil || fields["folder"] != "receipts/2024" || string(data) != "PDF" {
			t.Errorf("fields=%v data=%q err=%v", fields, data, err)
		}
		w.Write([]byte(`{"text":"hello world","savedAsset":{"id":"txt_1","storageKey":"receipts/2024/doc.txt"}}`))
	}))
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	res, err := c.UploadAndExtractText(context.Background(), strings.NewReader("PDF"), "doc.pdf",
		&UploadAndExtractOptions{Destination: Destination{Folder: "receipts/2024"}})
	if err != nil {
		t.Fatalf("UploadAndExtractText: %v", err)
	}
	if res.Text != "hello world" || res.SavedAsset == nil || res.SavedAsset.ID != "txt_1" {
		t.Errorf("result = %+v", res)
	}
}

func TestGetUsageDecodesStringStorageBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"storageBytes": "170461024", "imageCount": 109, "fileCount": 19, "folderCount": 11}`))
	}))
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	u, err := c.GetUsage(context.Background())
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if u.StorageBytes.Int64() != 170461024 || u.ImageCount != 109 {
		t.Errorf("usage = %+v", u)
	}
}
