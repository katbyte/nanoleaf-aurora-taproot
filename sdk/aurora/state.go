package aurora

import (
	"context"
	"time"
)

// The colour modes a controller reports.
const (
	ColorModeEffect = "effect" // running an effect: the usual mode
	ColorModeHS     = "hs"     // one colour by hue and saturation
	ColorModeCT     = "ct"     // one white by colour temperature
)

// State is the power, brightness and colour of a controller.
type State struct {
	On         Switch `json:"on"`
	Brightness Range  `json:"brightness"`
	Hue        Range  `json:"hue"`
	Sat        Range  `json:"sat"`
	CT         Range  `json:"ct"`
	ColorMode  string `json:"colorMode"`
}

// Switch is an on or off value as the API wraps it.
type Switch struct {
	Value bool `json:"value"`
}

// Range is a value with the limits the controller allows for it.
type Range struct {
	Value int `json:"value"`
	Max   int `json:"max"`
	Min   int `json:"min"`
}

// State reads power, brightness and colour together. The documentation only
// lists the single values below; controllers answer for the whole as well.
func (c *Client) State(ctx context.Context) (State, error) {
	return read[State](ctx, c, "/state")
}

// On reports whether the panels are lit.
func (c *Client) On(ctx context.Context) (bool, error) {
	out, err := read[Switch](ctx, c, "/state/on")
	return out.Value, err
}

// Brightness reads the overall brightness, 0 to 100.
func (c *Client) Brightness(ctx context.Context) (Range, error) {
	return read[Range](ctx, c, "/state/brightness")
}

// Hue reads the hue, 0 to 360, used in the hs colour mode.
func (c *Client) Hue(ctx context.Context) (Range, error) {
	return read[Range](ctx, c, "/state/hue")
}

// Saturation reads the saturation, 0 to 100, used in the hs colour mode.
func (c *Client) Saturation(ctx context.Context) (Range, error) {
	return read[Range](ctx, c, "/state/sat")
}

// ColorTemperature reads the white temperature in kelvin, 1200 to 6500, used
// in the ct colour mode.
func (c *Client) ColorTemperature(ctx context.Context) (Range, error) {
	return read[Range](ctx, c, "/state/ct")
}

// ColorMode reads which of the ColorMode values the controller is in.
func (c *Client) ColorMode(ctx context.Context) (string, error) {
	return read[string](ctx, c, "/state/colorMode")
}

// SetOn turns the panels on or off.
func (c *Client) SetOn(ctx context.Context, on bool) error {
	return c.put(ctx, "/state", map[string]any{"on": Switch{Value: on}})
}

// SetBrightness sets the overall brightness, 0 to 100. A fade above zero has
// the controller move there over that long, to the whole second.
func (c *Client) SetBrightness(ctx context.Context, value int, fade time.Duration) error {
	change := map[string]any{"value": value}
	if secs := int(fade.Round(time.Second) / time.Second); secs > 0 {
		change["duration"] = secs
	}

	return c.put(ctx, "/state", map[string]any{"brightness": change})
}

// AdjustBrightness moves the brightness by delta, stopping at the limits.
func (c *Client) AdjustBrightness(ctx context.Context, delta int) error {
	return c.put(ctx, "/state", adjust("brightness", delta))
}

// SetHue sets the hue, 0 to 360, and puts the controller in the hs colour mode.
func (c *Client) SetHue(ctx context.Context, value int) error {
	return c.put(ctx, "/state", set("hue", value))
}

// AdjustHue moves the hue by delta, stopping at the limits.
func (c *Client) AdjustHue(ctx context.Context, delta int) error {
	return c.put(ctx, "/state", adjust("hue", delta))
}

// SetSaturation sets the saturation, 0 to 100, and puts the controller in the
// hs colour mode.
func (c *Client) SetSaturation(ctx context.Context, value int) error {
	return c.put(ctx, "/state", set("sat", value))
}

// AdjustSaturation moves the saturation by delta, stopping at the limits.
func (c *Client) AdjustSaturation(ctx context.Context, delta int) error {
	return c.put(ctx, "/state", adjust("sat", delta))
}

// SetColorTemperature sets the white temperature in kelvin, 1200 to 6500, and
// puts the controller in the ct colour mode.
func (c *Client) SetColorTemperature(ctx context.Context, kelvin int) error {
	return c.put(ctx, "/state", set("ct", kelvin))
}

// AdjustColorTemperature moves the white temperature by delta kelvin,
// stopping at the limits.
func (c *Client) AdjustColorTemperature(ctx context.Context, delta int) error {
	return c.put(ctx, "/state", adjust("ct", delta))
}

func set(key string, value int) map[string]any {
	return map[string]any{key: map[string]int{"value": value}}
}

func adjust(key string, delta int) map[string]any {
	return map[string]any{key: map[string]int{"increment": delta}}
}
