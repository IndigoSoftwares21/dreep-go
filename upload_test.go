package dreep

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func parseMultipart(r *http.Request) (map[string]string, []byte, error) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return nil, nil, err
	}
	fields := map[string]string{}
	for k := range r.MultipartForm.Value {
		fields[k] = r.FormValue(k)
	}
	f, ok := r.MultipartForm.File["file"]
	if !ok || len(f) == 0 {
		return fields, nil, io.EOF
	}
	file, err := f[0].Open()
	if err != nil {
		return fields, nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	return fields, data, err
}

func TestUploadSendsMultipartFields(t *testing.T) {
	var gotFields map[string]string
	var gotFile []byte
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		fields, data, err := parseMultipart(r)
		gotFields, gotFile = fields, data
		if err != nil {
			t.Errorf("multipart: %v", err)
			w.WriteHeader(400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": "med_123", "url": "https://cdn.dreep.cloud/med_123/hero.webp",
			"originalFilename": "hero.jpg", "mimetype": "image/webp", "format": "webp",
			"sizeBytes": 245000, "width": 1200, "status": "ready",
		})
	}))
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	asset, err := c.Upload(context.Background(), UploadOptions{
		File:        strings.NewReader("JPEGDATA"),
		Filename:    "hero.jpg",
		Destination: Destination{Folder: "avatars/2024/q1"},
		Transform: &Transform{
			Width: 800, Format: FormatWebP, Fit: FitCover,
			Quality: 80, Rotate: 90, BG: "ffffff",
			Crop: &Crop{Left: 1, Top: 2, Width: 3, Height: 4},
		},
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if asset.ID != "med_123" || asset.Width != 1200 || asset.SizeBytes.Int64() != 245000 {
		t.Errorf("asset = %+v", asset)
	}
	if gotAuth != "Bearer k" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if string(gotFile) != "JPEGDATA" {
		t.Errorf("file bytes = %q", gotFile)
	}
	want := map[string]string{
		"folder":  "avatars/2024/q1",
		"width":   "800",
		"format":  "webp",
		"fit":     "cover",
		"quality": "80",
		"rotate":  "90",
		"bg":      "ffffff",
		"crop":    `{"height":4,"left":1,"top":2,"width":3}`,
	}
	for k, v := range want {
		if gotFields[k] != v {
			t.Errorf("field %s = %q, want %q", k, gotFields[k], v)
		}
	}
}

func TestUploadKnownSizeSetsExactContentLength(t *testing.T) {
	var gotLen int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLen = r.ContentLength
		body, _ := io.ReadAll(r.Body)
		if int64(len(body)) != gotLen {
			t.Errorf("body length %d != ContentLength %d", len(body), gotLen)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "med_1"})
	}))
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	_, err := c.Upload(context.Background(), UploadOptions{
		File:        strings.NewReader("JPEGDATA"),
		Filename:    "hero.jpg",
		ContentType: "image/jpeg",
		Transform:   &Transform{Width: 800, Format: FormatWebP},
		KnownSize:   8,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	// The file alone is 8 bytes; multipart overhead must be included.
	if gotLen <= 8 {
		t.Errorf("ContentLength = %d, want file size plus multipart overhead", gotLen)
	}
}

func TestUploadValidation(t *testing.T) {
	c, _ := New("k")
	ctx := context.Background()
	if _, err := c.Upload(ctx, UploadOptions{Filename: "a"}); err == nil {
		t.Error("want error for missing File")
	}
	if _, err := c.Upload(ctx, UploadOptions{File: strings.NewReader("x")}); err == nil {
		t.Error("want error for missing Filename")
	}
	if _, err := c.Upload(ctx, UploadOptions{
		File: strings.NewReader("x"), Filename: "x",
		Destination: Destination{Folder: "a", Key: "b"},
	}); err == nil {
		t.Error("want error for conflicting destination")
	}
}

func TestUploadPresignedFullFlow(t *testing.T) {
	type step struct{ path string }
	var steps []string
	var putBody []byte

	mux := http.NewServeMux()
	mux.HandleFunc("/upload/presign", func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, "presign")
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if in["contentType"] != "image/webp" || in["sizeBytes"] != float64(6) {
			t.Errorf("presign body = %v", in)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": "upload-1", "uploadUrl": "http://" + r.Host + "/storage/put",
			"contentType": "image/webp", "alreadyExists": false,
		})
	})
	mux.HandleFunc("/storage/put", func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, "put")
		putBody, _ = io.ReadAll(r.Body)
		if ct := r.Header.Get("Content-Type"); ct != "image/webp" {
			t.Errorf("PUT content-type = %q", ct)
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("PUT must not carry the API key, got %q", auth)
		}
		w.WriteHeader(200)
	})
	mux.HandleFunc("/upload/upload-1/confirm", func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, "confirm")
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if fmt_Sprint(in["transform"]) == "" || in["presetKey"] != "thumb" {
			t.Errorf("confirm body = %v", in)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "med_9", "url": "u"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	asset, err := c.UploadPresigned(context.Background(), DirectUploadOptions{
		PresignOptions: PresignOptions{
			ContentType: "image/webp", SizeBytes: 6, Filename: "me.webp",
		},
		File:      strings.NewReader("WEBP!!"),
		Transform: &Transform{Width: 100},
		PresetKey: "thumb",
	})
	if err != nil {
		t.Fatalf("UploadPresigned: %v", err)
	}
	if asset == nil || asset.ID != "med_9" {
		t.Errorf("asset = %+v", asset)
	}
	if len(steps) != 3 || steps[0] != "presign" || steps[1] != "put" || steps[2] != "confirm" {
		t.Errorf("steps = %v", steps)
	}
	if string(putBody) != "WEBP!!" {
		t.Errorf("PUT body = %q", putBody)
	}
}

func TestUploadPresignedAlreadyExistsSkipsPut(t *testing.T) {
	puts := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/upload/presign", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id": "upload-2", "contentType": "application/pdf", "alreadyExists": true,
		})
	})
	mux.HandleFunc("/storage/", func(w http.ResponseWriter, r *http.Request) { puts++; w.WriteHeader(200) })
	mux.HandleFunc("/upload/upload-2/confirm", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"med_10"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	_, err := c.UploadPresigned(context.Background(), DirectUploadOptions{
		PresignOptions: PresignOptions{
			ContentType: "application/pdf", SizeBytes: 10, ContentHash: "deadbeef",
		},
		File: strings.NewReader("%PDF"),
	})
	if err != nil {
		t.Fatalf("UploadPresigned: %v", err)
	}
	if puts != 0 {
		t.Errorf("expected no PUT when alreadyExists, got %d", puts)
	}
}

func TestConfirmUploadConflictThenSuccess(t *testing.T) {
	attempts := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/upload/presign", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id": "u3", "uploadUrl": "http://" + r.Host + "/storage/u3",
		})
	})
	mux.HandleFunc("/storage/u3", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	mux.HandleFunc("/upload/u3/confirm", func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"error":true,"message":"not landed","code":"upload_in_progress"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "med_11"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, _ := New("k", WithAPIBaseURL(srv.URL))
	asset, err := c.UploadPresigned(context.Background(), DirectUploadOptions{
		PresignOptions: PresignOptions{ContentType: "text/plain", SizeBytes: 3},
		File:           strings.NewReader("abc"),
	})
	if err != nil {
		t.Fatalf("UploadPresigned: %v", err)
	}
	if asset.ID != "med_11" || attempts != 3 {
		t.Errorf("attempts=%d asset=%+v", attempts, asset)
	}
}

// tiny indirection so tests avoid importing fmt just for one call
func fmt_Sprint(v any) string { return sprintAny(v) }
