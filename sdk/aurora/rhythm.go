package aurora

import "context"

// The sound sources of a Rhythm module.
const (
	RhythmModeMicrophone = 0
	RhythmModeAux        = 1
)

// Rhythm is the sound module plugged into a set of Light Panels.
type Rhythm struct {
	Connected       bool     `json:"rhythmConnected"`
	Active          bool     `json:"rhythmActive"`
	ID              int      `json:"rhythmId"`
	HardwareVersion string   `json:"hardwareVersion"`
	FirmwareVersion string   `json:"firmwareVersion"`
	AuxAvailable    bool     `json:"auxAvailable"`
	Mode            int      `json:"rhythmMode"`
	Position        Position `json:"rhythmPos"`
}

// Position is a place in the layout, in the same terms as a Panel.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	O float64 `json:"o"`
}

// Rhythm reads everything about the module together. The documentation only
// lists the single values below; controllers answer for the whole as well.
func (c *Client) Rhythm(ctx context.Context) (Rhythm, error) {
	return read[Rhythm](ctx, c, "/rhythm")
}

// RhythmConnected reports whether a module is plugged in.
func (c *Client) RhythmConnected(ctx context.Context) (bool, error) {
	return read[bool](ctx, c, "/rhythm/rhythmConnected")
}

// RhythmActive reports whether the module's microphone is listening.
func (c *Client) RhythmActive(ctx context.Context) (bool, error) {
	return read[bool](ctx, c, "/rhythm/rhythmActive")
}

// RhythmID reads the module's id among the panels.
func (c *Client) RhythmID(ctx context.Context) (int, error) {
	return read[int](ctx, c, "/rhythm/rhythmId")
}

// RhythmHardwareVersion reads the module's hardware version.
func (c *Client) RhythmHardwareVersion(ctx context.Context) (string, error) {
	return read[string](ctx, c, "/rhythm/hardwareVersion")
}

// RhythmFirmwareVersion reads the module's firmware version.
func (c *Client) RhythmFirmwareVersion(ctx context.Context) (string, error) {
	return read[string](ctx, c, "/rhythm/firmwareVersion")
}

// RhythmAuxAvailable reports whether a cable is plugged into the module's
// 3.5mm socket.
func (c *Client) RhythmAuxAvailable(ctx context.Context) (bool, error) {
	return read[bool](ctx, c, "/rhythm/auxAvailable")
}

// RhythmMode reads the module's sound source: RhythmModeMicrophone or
// RhythmModeAux.
func (c *Client) RhythmMode(ctx context.Context) (int, error) {
	return read[int](ctx, c, "/rhythm/rhythmMode")
}

// SetRhythmMode picks the module's sound source: RhythmModeMicrophone or
// RhythmModeAux.
func (c *Client) SetRhythmMode(ctx context.Context, mode int) error {
	return c.put(ctx, "/rhythm/rhythmMode", map[string]int{"rhythmMode": mode})
}

// RhythmPosition reads where the module sits in the layout.
func (c *Client) RhythmPosition(ctx context.Context) (Position, error) {
	return read[Position](ctx, c, "/rhythm/rhythmPos")
}
