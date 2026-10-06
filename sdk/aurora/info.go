package aurora

import (
	"context"
	"encoding/json"
)

// Info is everything a controller says about itself in one answer.
type Info struct {
	Name            string      `json:"name"`
	SerialNo        string      `json:"serialNo"`
	Manufacturer    string      `json:"manufacturer"`
	FirmwareVersion string      `json:"firmwareVersion"`
	HardwareVersion string      `json:"hardwareVersion"`
	Model           string      `json:"model"`
	State           State       `json:"state"`
	Effects         EffectsInfo `json:"effects"`
	PanelLayout     PanelLayout `json:"panelLayout"`
	// Rhythm is nil on a controller that says nothing about one.
	Rhythm *Rhythm `json:"rhythm"`

	// Raw is the whole answer. Newer firmware adds sections this struct does
	// not name (cloudHash, discovery, firmwareUpgrade, schedules).
	Raw json.RawMessage `json:"-"`
}

// EffectsInfo is the effects part of Info: what is running and what is held.
type EffectsInfo struct {
	Select string   `json:"select"`
	List   []string `json:"effectsList"`
}

// PanelLayout is the layout part of Info.
type PanelLayout struct {
	GlobalOrientation Range  `json:"globalOrientation"`
	Layout            Layout `json:"layout"`
}

// Info reads everything in one request.
func (c *Client) Info(ctx context.Context) (Info, error) {
	var raw json.RawMessage
	if err := c.get(ctx, "/", &raw); err != nil {
		return Info{}, err
	}

	var info Info
	if err := json.Unmarshal(raw, &info); err != nil {
		return Info{}, err
	}
	info.Raw = raw

	return info, nil
}

// Identify makes the panels flash in unison, to tell one controller from
// another.
func (c *Client) Identify(ctx context.Context) error {
	return c.put(ctx, "/identify", nil)
}
