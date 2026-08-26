package dreep

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Format is a target re-encode format. Image targets: jpeg, png, webp, avif,
// gif, tiff. Video targets: mp4, webm, mov, hls (.m3u8), gif. FormatTXT is
// used to request OCR text extraction at delivery time.
type Format string

const (
	FormatJPEG Format = "jpeg"
	FormatPNG  Format = "png"
	FormatWebP Format = "webp"
	FormatAVIF Format = "avif"
	FormatGIF  Format = "gif"
	FormatTIFF Format = "tiff"

	FormatMP4  Format = "mp4"
	FormatWebM Format = "webm"
	FormatMOV  Format = "mov"
	FormatHLS  Format = "hls"
	FormatM3U8 Format = "m3u8"

	FormatTXT Format = "txt"
)

// Fit controls how media is scaled when both width and height are given.
type Fit string

const (
	FitCover   Fit = "cover"
	FitContain Fit = "contain"
	FitFill    Fit = "fill"
	FitInside  Fit = "inside"
	FitOutside Fit = "outside"
)

// Gravity is the crop anchor point used when resizing images.
type Gravity string

const (
	GravityNorth     Gravity = "north"
	GravityNortheast Gravity = "northeast"
	GravityEast      Gravity = "east"
	GravitySoutheast Gravity = "southeast"
	GravitySouth     Gravity = "south"
	GravitySouthwest Gravity = "southwest"
	GravityWest      Gravity = "west"
	GravityNorthwest Gravity = "northwest"
	GravityCenter    Gravity = "center"
	GravityAttention Gravity = "attention"
	GravityEntropy   Gravity = "entropy"
)

// VideoCodec selects the video encoding codec.
type VideoCodec string

const (
	CodecLibX264   VideoCodec = "libx264"
	CodecLibVPXVP9 VideoCodec = "libvpx-vp9"
	CodecH264      VideoCodec = "h264"
	CodecHEVC      VideoCodec = "hevc"
)

// AccessControlType controls who may view assets inside a folder:
//
//   - public: fetchable by anyone via /api/v1/fetch/{mediaId}
//   - private: viewable only by authenticated project members in the dashboard
//   - signed: fetchable only through time-limited signed URLs
type AccessControlType string

const (
	AccessPublic  AccessControlType = "public"
	AccessPrivate AccessControlType = "private"
	AccessSigned  AccessControlType = "signed"
)

// Crop describes an explicit rectangular crop applied before resizing.
type Crop struct {
	Left   int `json:"left"`
	Top    int `json:"top"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Transform collects every upload-time and delivery-time transformation
// parameter. Fields left at their zero value are omitted. Applicability is
// enforced server-side per file type (image vs video vs raw).
type Transform struct {
	Format     Format     // re-encode target format
	Width      int        // resize width, pixels
	Height     int        // resize height, pixels
	Fit        Fit        // scaling mode for width/height
	Quality    int        // output quality, 1–100
	Crop       *Crop      // explicit image crop
	FPS        int        // video frame rate (e.g. 24, 30, 60)
	VideoCodec VideoCodec // video encoding codec
	TrimStart  *float64   // video trim start, seconds
	TrimEnd    *float64   // video trim end, seconds
	Gravity    Gravity    // crop anchor point for images
	DPR        float64    // device pixel ratio multiplier, max 3
	Rotate     int        // rotation degrees clockwise: 90, 180 or 270
	BG         string     // hex padding colour without '#', e.g. "ffffff"
	Radius     string     // corner radius in pixels ("max" for circle/pill)
	Preset     string     // saved transform preset key (sent as "p")
}

// transformMap renders t as the parameters the API expects on upload-time
// bodies (multipart form fields and confirm-upload JSON). Crop is rendered as
// the comma-separated "left,top,width,height" form, matching transformQuery
// so delivery URLs and uploads encode it identically.
func transformMap(t *Transform) map[string]any {
	if t == nil {
		return map[string]any{}
	}
	m := map[string]any{}
	setStr := func(k, v string) {
		if v != "" {
			m[k] = v
		}
	}
	if t.Format != "" {
		m["format"] = string(t.Format)
	}
	if t.Width > 0 {
		m["width"] = t.Width
	}
	if t.Height > 0 {
		m["height"] = t.Height
	}
	setStr("fit", string(t.Fit))
	if t.Quality > 0 {
		m["quality"] = t.Quality
	}
	if t.FPS > 0 {
		m["fps"] = t.FPS
	}
	setStr("videoCodec", string(t.VideoCodec))
	setStr("gravity", string(t.Gravity))
	if t.DPR > 0 {
		m["dpr"] = t.DPR
	}
	switch t.Rotate {
	case 90, 180, 270:
		m["rotate"] = t.Rotate
	}
	setStr("bg", t.BG)
	setStr("radius", t.Radius)
	setStr("p", t.Preset)
	if t.Crop != nil {
		m["crop"] = fmt.Sprintf("%d,%d,%d,%d", t.Crop.Left, t.Crop.Top, t.Crop.Width, t.Crop.Height)
	}
	if t.TrimStart != nil {
		m["trimStart"] = *t.TrimStart
	}
	if t.TrimEnd != nil {
		m["trimEnd"] = *t.TrimEnd
	}
	return m
}

// MediaAsset is a stored asset. It covers both the shape returned by Upload
// and the listing shape of GET /media; fields absent from either response are
// left zero-valued.
type MediaAsset struct {
	ID               string    `json:"id"`
	URL              string    `json:"url,omitempty"`
	Filename         string    `json:"filename,omitempty"`         // listing shape
	OriginalFilename string    `json:"originalFilename,omitempty"` // upload shape
	Mimetype         string    `json:"mimetype,omitempty"`         // upload shape
	Type             string    `json:"type,omitempty"`             // listing shape (mime type)
	Format           string    `json:"format,omitempty"`
	SizeBytes        flexInt64 `json:"sizeBytes,omitempty"`
	Width            int       `json:"width,omitempty"`
	Height           int       `json:"height,omitempty"`
	Status           string    `json:"status,omitempty"`
	AssetType        string    `json:"assetType,omitempty"` // "image" | "video" | "file"
	Folder           string    `json:"folder,omitempty"`    // resolved path (listings)
	FolderID         string    `json:"folderId,omitempty"`
	ProjectID        string    `json:"projectId,omitempty"`
	CreatedAt        time.Time `json:"createdAt,omitempty"`
}

// Pagination describes one page of a paginated listing.
type Pagination struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
}

// MediaPage is one page of ListMedia results.
type MediaPage struct {
	Assets     []*MediaAsset `json:"assets"`
	Pagination Pagination    `json:"pagination"`
}

// Folder is a Dreep storage folder. Path is always the full slug path from
// the project root.
type Folder struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Slug              string `json:"slug,omitempty"`
	Path              string `json:"path,omitempty"`
	ParentID          string `json:"parentId,omitempty"`
	AccessControlType string `json:"accessControlType,omitempty"`
}

// Preset is a named, saved transform.
type Preset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PresetOperation is one operation inside a preset, e.g.
// {Action: "resize", Params: {"width": 200, "height": 200}}.
type PresetOperation struct {
	Action string
	Params map[string]any
}

// MarshalJSON merges Action into the operation's parameter object under the
// required "action" key.
func (o PresetOperation) MarshalJSON() ([]byte, error) {
	m := make(map[string]any, len(o.Params)+1)
	for k, v := range o.Params {
		m[k] = v
	}
	m["action"] = o.Action
	return json.Marshal(m)
}

// UnmarshalJSON splits the "action" key out of the operation object so the
// remaining parameters land in Params.
func (o *PresetOperation) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	o.Action, _ = m["action"].(string)
	delete(m, "action")
	o.Params = m
	return nil
}

// Usage reports current billing-cycle counters from GET /usage.
type Usage struct {
	StorageBytes flexInt64 `json:"storageBytes"` // serialised as string by the API
	ImageCount   int       `json:"imageCount"`
	FileCount    int       `json:"fileCount"`
	FolderCount  int       `json:"folderCount"`
}

// flexInt64 decodes JSON numbers that the API sometimes sends as strings
// (documented BIGINT columns such as sizeBytes and storageBytes).
type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == `""` {
		*f = 0
		return nil
	}
	if len(s) > 1 && s[0] == '"' {
		s = strings.Trim(s, `"`)
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		*f = flexInt64(v)
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*f = flexInt64(v)
	return nil
}

// Int64 returns the value as a plain int64.
func (f flexInt64) Int64() int64 { return int64(f) }
