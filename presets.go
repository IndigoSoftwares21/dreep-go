package dreep

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// presetKeyRE matches the live API's key constraint: lowercase letters,
// numbers, and underscores only.
var presetKeyRE = regexp.MustCompile(`^[a-z0-9_]+$`)

// CreatePresetOptions describes one CreatePreset call. The transform
// parameters are stored flat on the request body (verified against the live
// API: the body is {"name", "key", "format", "width", …} — not an operations
// array).
type CreatePresetOptions struct {
	// Name is the human-readable display name. Required.
	Name string

	// Key is the identifier used to reference the preset at delivery time
	// (Transform.Preset, PresignOptions.PresetKey, and the "p" query
	// parameter). Required; lowercase letters, numbers, and underscores.
	Key string

	// Transform holds the saved parameters. Required; at minimum set a
	// Format or a dimension so the API has something to apply.
	Transform *Transform
}

// CreatePreset saves a named transform that can later be referenced by its
// Key on uploads (Transform.Preset), presigned uploads (PresetKey), and
// delivery URLs ("p=").
func (c *Client) CreatePreset(ctx context.Context, o CreatePresetOptions) (*Preset, error) {
	if o.Name == "" {
		return nil, fmt.Errorf("dreep: preset name is required")
	}
	if !presetKeyRE.MatchString(o.Key) {
		return nil, fmt.Errorf("dreep: preset key %q must be lowercase letters, numbers, and underscores only", o.Key)
	}
	if o.Transform == nil {
		return nil, fmt.Errorf("dreep: CreatePresetOptions.Transform is required")
	}
	in := map[string]any{
		"name": o.Name,
		"key":  o.Key,
	}
	for k, v := range transformMap(o.Transform) {
		in[k] = v
	}
	var out Preset
	if err := c.doJSON(ctx, http.MethodPost, "/presets", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPresets returns every saved transform preset in the project.
// GET /presets responds with a bare top-level array (inside the response
// envelope).
func (c *Client) ListPresets(ctx context.Context) ([]Preset, error) {
	var out []Preset
	if err := c.doJSON(ctx, http.MethodGet, "/presets", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeletePreset removes the preset with the given ID.
func (c *Client) DeletePreset(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("dreep: preset id is required")
	}
	var out struct {
		Success bool `json:"success"`
	}
	return c.doJSON(ctx, http.MethodDelete, "/presets/"+id, nil, nil, &out)
}
