package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// What Nanoleaf's own app sends that the documentation does not list, read
// out of Nanoleaf Desktop 3.0.1 (sdk/aurora-api-specs/README.md says how).
// Each is sent the way the app sends it. What a Light Panels controller
// answers to each is recorded there as it is found out.

// The undocumented effect commands.
const (
	cmdEnableButtons   = "enableAllControllerButtons"
	cmdDisableButtons  = "disableAllControllerButtons"
	cmdEnableFade      = "enableSceneChangeAnimation"
	cmdDisableFade     = "disableSceneChangeAnimation"
	cmdPowerLoss       = "setPLRConfig"
	cmdShortIDMap      = "getShortIdMap"
	cmdAdjacency       = "getAdjacencyData"
	cmdTouchConfig     = "requestTouchConfig"
	cmdSetTouchConfig  = "configureTouch"
	cmdTouchKillSwitch = "getTouchKillSwitch"
	cmdSetTouchKill    = "setTouchKillSwitch"
	cmdSensorConfig    = "requestBrightnessSensorConfig"
	cmdSetSensorConfig = "setBrightnessSensorConfig"
)

// FirmwareUpgrade is what a controller says about a firmware update, from
// GET /firmwareUpgrade or the section of the same name in GET /. A
// controller that has not heard from Nanoleaf's cloud says nothing at all:
// an empty object, which reads as not Available.
type FirmwareUpgrade struct {
	// Available says an update is waiting to be installed.
	Available bool `json:"firmwareAvailability"`
	// NewVersion is the version waiting, empty when none is.
	NewVersion string `json:"newFirmwareVersion"`
	// Raw is the whole answer, for whatever else a controller puts in it.
	Raw json.RawMessage `json:"-"`
}

// FirmwareUpgrade reads what the controller says about a firmware update.
func (c *Client) FirmwareUpgrade(ctx context.Context) (FirmwareUpgrade, error) {
	var raw json.RawMessage
	if err := c.get(ctx, "/firmwareUpgrade", &raw); err != nil {
		return FirmwareUpgrade{}, err
	}
	var fw FirmwareUpgrade
	if err := json.Unmarshal(raw, &fw); err != nil {
		return FirmwareUpgrade{}, fmt.Errorf("GET /firmwareUpgrade: reading the answer: %w", err)
	}
	fw.Raw = raw

	return fw, nil
}

// TriggerFirmwareUpgrade tells the controller to fetch and install the
// firmware Nanoleaf's cloud has for it, as the app's update button does.
// The controller does the downloading; nothing travels through here. It is
// sent whether or not the controller says an update is waiting, and the
// body is not wrapped in "write" as effect commands are.
func (c *Client) TriggerFirmwareUpgrade(ctx context.Context) error {
	return c.put(ctx, "/firmwareUpgrade", map[string]string{"command": "triggerFirmwareUpgrade"})
}

// SetControllerButtons locks or unlocks the buttons on the controller.
func (c *Client) SetControllerButtons(ctx context.Context, enabled bool) error {
	return c.sendCommand(ctx, pick(enabled, cmdEnableButtons, cmdDisableButtons))
}

// SetSceneTransition turns the fade from one scene to the next on or off.
func (c *Client) SetSceneTransition(ctx context.Context, enabled bool) error {
	return c.sendCommand(ctx, pick(enabled, cmdEnableFade, cmdDisableFade))
}

// SetPowerLossRecovery says whether the panels come back on by themselves
// after the power has been cut.
func (c *Client) SetPowerLossRecovery(ctx context.Context, on bool) error {
	body, err := command(cmdPowerLoss, arg{"PLRConfig", on})
	if err != nil {
		return err
	}

	return c.put(ctx, "/effects", body)
}

// ShortIDMap and AdjacencyData are what the app's layout editor asks for:
// which panel touches which. Their shape is not documented, so each comes
// back as the controller sent it.
func (c *Client) ShortIDMap(ctx context.Context) (json.RawMessage, error) {
	return c.askCommand(ctx, cmdShortIDMap)
}

// AdjacencyData is described with ShortIDMap.
func (c *Client) AdjacencyData(ctx context.Context) (json.RawMessage, error) {
	return c.askCommand(ctx, cmdAdjacency)
}

// TouchConfig reads the touch settings, on the models with touch, which
// Light Panels are not; it and SetTouchConfig, TouchKillSwitch and
// SetTouchKillSwitch are here so that nothing stops another model. The
// settings go as they come.
func (c *Client) TouchConfig(ctx context.Context) (json.RawMessage, error) {
	return c.askCommand(ctx, cmdTouchConfig)
}

// SetTouchConfig is described with TouchConfig.
func (c *Client) SetTouchConfig(ctx context.Context, config json.RawMessage) error {
	return c.sendCommand(ctx, cmdSetTouchConfig, arg{"touchConfig", config})
}

// TouchKillSwitch is described with TouchConfig.
func (c *Client) TouchKillSwitch(ctx context.Context) (json.RawMessage, error) {
	return c.askCommand(ctx, cmdTouchKillSwitch)
}

// SetTouchKillSwitch is described with TouchConfig.
func (c *Client) SetTouchKillSwitch(ctx context.Context, on bool) error {
	return c.sendCommand(ctx, cmdSetTouchKill, arg{"touchKillSwitchOn", on})
}

// BrightnessSensorConfig and SetBrightnessSensorConfig are for the models
// with a light sensor, which Light Panels have not.
func (c *Client) BrightnessSensorConfig(ctx context.Context) (json.RawMessage, error) {
	return c.askCommand(ctx, cmdSensorConfig)
}

// SetBrightnessSensorConfig is described with BrightnessSensorConfig.
func (c *Client) SetBrightnessSensorConfig(ctx context.Context, config json.RawMessage) error {
	return c.sendCommand(ctx, cmdSetSensorConfig, arg{"brightnessSensorConfig", config})
}

// StaticColor is one panel's colour for a static effect.
type StaticColor struct {
	PanelID int
	R, G, B uint8
}

// StaticEffect is an effect that holds each panel at a colour: the
// documentation's static type, with its animData laid out as it says
// (panel count; then for each panel its id, one frame, R G B W and the
// transition in tenths of a second). The white element is ignored by the
// controller and sent as 0. A name makes it one that can be added; without
// one it is for Display.
func StaticEffect(name string, colours []StaticColor, transition time.Duration) (Effect, error) {
	if len(colours) == 0 {
		return Effect{}, errors.New("a static effect needs at least one panel")
	}
	tenths := max(0, int(transition.Round(100*time.Millisecond)/(100*time.Millisecond)))
	parts := []string{strconv.Itoa(len(colours))}
	for _, sc := range colours {
		parts = append(parts, fmt.Sprintf("%d 1 %d %d %d 0 %d", sc.PanelID, sc.R, sc.G, sc.B, tenths))
	}

	e := Effect{}
	var err error
	for _, f := range []struct {
		key   string
		value any
	}{
		{FieldName, name},
		{FieldType, "static"},
		{"animData", strings.Join(parts, " ")},
		{"loop", false},
		{FieldColorType, "HSB"},
		{"palette", []Color{}},
	} {
		if f.key == FieldName && name == "" {
			continue
		}
		if e, err = e.With(f.key, f.value); err != nil {
			return Effect{}, err
		}
	}

	return e, nil
}

// sendCommand sends an effect command with its parameters, if any.
func (c *Client) sendCommand(ctx context.Context, name string, args ...arg) error {
	body, err := command(name, args...)
	if err != nil {
		return err
	}

	return c.put(ctx, "/effects", body)
}

// askCommand sends an effect command that only asks, and returns the answer
// as it came.
func (c *Client) askCommand(ctx context.Context, name string) (json.RawMessage, error) {
	body, err := command(name)
	if err != nil {
		return nil, err
	}
	var out json.RawMessage
	if err := c.query(ctx, body, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func pick(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}
