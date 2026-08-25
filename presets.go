package dreep

import (
	"context"
	"fmt"
	"net/http"
)

// CreatePreset saves a named transform that can later be referenced by key on
// uploads (Transform.Preset), presigned uploads (PresetKey), and delivery URLs.
func (c *Client) CreatePreset(ctx context.Context, name string, operations []PresetOperation) (*Preset, error) {
	if name == "" {
		return nil, fmt.Errorf("dreep: preset name is required")
	}
	if len(operations) == 0 {
		return nil, fmt.Errorf("dreep: at least one preset operation is required")
	}
	in := map[string]any{
		"name":       name,
		"operations": operations,
	}
	var out Preset
	if err := c.doJSON(ctx, http.MethodPost, "/presets", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPresets returns every saved transform preset in the project.
func (c *Client) ListPresets(ctx context.Context) ([]Preset, error) {
	var out struct {
		Presets []Preset `json:"presets"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/presets", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Presets, nil
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
