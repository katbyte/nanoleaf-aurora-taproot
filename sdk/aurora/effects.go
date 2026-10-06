package aurora

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// The names a controller reports as running when no saved effect is.
const (
	EffectStatic  = "*Static*"  // panels holding colours somebody set
	EffectDynamic = "*Dynamic*" // an effect being shown without being saved
	EffectSolid   = "*Solid*"   // one colour, from the hs or ct colour mode
)

// The effect commands. Each travels as {"write": {"command": ...}} in a PUT
// to /effects, whether it changes anything or only asks.
const (
	cmdAdd            = "add"
	cmdDelete         = "delete"
	cmdRequest        = "request"
	cmdRequestAll     = "requestAll"
	cmdRequestPlugins = "requestPlugins"
	cmdRename         = "rename"
	cmdDisplay        = "display"
	cmdDisplayTemp    = "displayTemp"
)

// Plugin is a motion a controller can run: the code that turns an effect's
// palette into moving light.
type Plugin struct {
	UUID        string         `json:"uuid"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Author      string         `json:"author"`
	Type        string         `json:"type"` // PluginTypeColor or PluginTypeRhythm
	Tags        []string       `json:"tags"`
	Features    []string       `json:"features"`
	Config      []PluginConfig `json:"pluginConfig"`
}

// PluginConfig is one option a plugin takes and the values it allows.
type PluginConfig struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"` // bool, int, double or string
	DefaultValue any      `json:"defaultValue"`
	MinValue     *float64 `json:"minValue"`
	MaxValue     *float64 `json:"maxValue"`
	Strings      []string `json:"strings"` // the values a string option allows
}

// ExternalControl is where a controller listens for streamed panel colours
// once it has been put into that mode.
type ExternalControl struct {
	Address  string `json:"streamControlIpAddr"`
	Port     int    `json:"streamControlPort"`
	Protocol string `json:"streamControlProtocol"`
}

// EffectNames lists the names of the effects a controller holds.
func (c *Client) EffectNames(ctx context.Context) ([]string, error) {
	return read[[]string](ctx, c, "/effects/effectsList")
}

// SelectedEffect is the name of the effect that is running, or one of
// EffectStatic, EffectDynamic and EffectSolid when none is.
func (c *Client) SelectedEffect(ctx context.Context) (string, error) {
	return read[string](ctx, c, "/effects/select")
}

// SelectEffect starts an effect the controller holds, by name.
func (c *Client) SelectEffect(ctx context.Context, name string) error {
	return c.put(ctx, "/effects", map[string]string{"select": name})
}

// Effect reads one effect by name, whole. A name the controller does not hold
// is an error IsNotFound recognises.
func (c *Client) Effect(ctx context.Context, name string) (Effect, error) {
	var out Effect
	body, err := command(cmdRequest, arg{FieldName, name})
	if err != nil {
		return out, err
	}
	err = c.query(ctx, "/effects", body, &out)

	return out, err
}

// Effects reads every effect a controller holds, whole.
func (c *Client) Effects(ctx context.Context) ([]Effect, error) {
	var out struct {
		Animations []Effect `json:"animations"`
	}
	body, err := command(cmdRequestAll)
	if err != nil {
		return nil, err
	}

	err = c.query(ctx, "/effects", body, &out)

	return out.Animations, err
}

// Plugins lists the plugins a controller has. An effect can only run on a
// controller that has the plugin it names.
func (c *Client) Plugins(ctx context.Context) ([]Plugin, error) {
	var out struct {
		Plugins []Plugin `json:"plugins"`
	}
	body, err := command(cmdRequestPlugins, arg{FieldVersion, "2.0"})
	if err != nil {
		return nil, err
	}

	err = c.query(ctx, "/effects", body, &out)

	return out.Plugins, err
}

// AddEffect stores an effect on the controller under its name. An effect
// already there under that name is replaced without a word: the controller
// does not tell the two cases apart, so a caller that minds must look first.
func (c *Client) AddEffect(ctx context.Context, e Effect) error {
	if e.Name() == "" {
		return errors.New("an effect needs a name (animName) to be added")
	}
	body, err := e.command(cmdAdd)
	if err != nil {
		return err
	}

	return c.put(ctx, "/effects", body)
}

// DeleteEffect removes an effect from the controller, for good.
func (c *Client) DeleteEffect(ctx context.Context, name string) error {
	body, err := command(cmdDelete, arg{FieldName, name})
	if err != nil {
		return err
	}

	return c.put(ctx, "/effects", body)
}

// RenameEffect gives an effect on the controller another name.
func (c *Client) RenameEffect(ctx context.Context, name, newName string) error {
	body, err := command(cmdRename, arg{FieldName, name}, arg{"newName", newName})
	if err != nil {
		return err
	}

	return c.put(ctx, "/effects", body)
}

// DisplayEffect shows an effect on the panels without storing it. The
// controller reports EffectDynamic or EffectStatic as running while it is up.
func (c *Client) DisplayEffect(ctx context.Context, e Effect) error {
	body, err := e.command(cmdDisplay)
	if err != nil {
		return err
	}

	return c.put(ctx, "/effects", body)
}

// DisplayEffectFor shows an effect without storing it, then goes back to what
// was running. The time is rounded to the second, and is at least one.
func (c *Client) DisplayEffectFor(ctx context.Context, e Effect, d time.Duration) error {
	secs, err := json.Marshal(seconds(d))
	if err != nil {
		return err
	}
	body, err := e.command(cmdDisplayTemp, effectField{key: "duration", value: secs})
	if err != nil {
		return err
	}

	return c.put(ctx, "/effects", body)
}

// DisplaySavedEffectFor shows an effect the controller holds, then goes back
// to what was running. The time is rounded to the second, and is at least one.
func (c *Client) DisplaySavedEffectFor(ctx context.Context, name string, d time.Duration) error {
	body, err := command(cmdDisplayTemp, arg{"duration", seconds(d)}, arg{FieldName, name})
	if err != nil {
		return err
	}

	return c.put(ctx, "/effects", body)
}

// StartExternalControl puts the controller into the mode where panel colours
// are streamed to it over UDP, and says where it is listening. Whatever was
// running stops. version is "v1" (Light Panels) or "v2"; empty leaves it to
// the controller, which takes it as v1.
func (c *Client) StartExternalControl(ctx context.Context, version string) (ExternalControl, error) {
	args := []arg{{FieldType, "extControl"}}
	if version != "" {
		args = append(args, arg{"extControlVersion", version})
	}

	var out ExternalControl
	body, err := command(cmdDisplay, args...)
	if err != nil {
		return out, err
	}
	raw, err := c.exchange(ctx, http.MethodPut, "/effects", body, true)
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return out, err // newer controllers answer with nothing and listen on their own address
	}
	err = json.Unmarshal(raw, &out)

	return out, err
}

// Write sends an effect command this package has no method for and returns
// what the controller answered, which is nothing for most commands. command
// is the object that goes inside "write"; send a struct or an Effect rather
// than a map where the order of its fields matters, since the documentation
// asks for the command to come first. It is treated as changing the
// controller.
func (c *Client) Write(ctx context.Context, command any) (json.RawMessage, error) {
	return c.exchange(ctx, http.MethodPut, "/effects", map[string]any{"write": command}, true)
}

// arg is one parameter of an effect command.
type arg struct {
	key   string
	value any
}

// command is the body of an effect command that carries no effect: the
// command first, as the documentation asks, then its parameters in order.
func command(name string, args ...arg) ([]byte, error) {
	fields := make([]effectField, 0, len(args))
	for _, a := range args {
		raw, err := json.Marshal(a.value)
		if err != nil {
			return nil, fmt.Errorf("effect command %s: %s: %w", name, a.key, err)
		}
		fields = append(fields, effectField{key: a.key, value: raw})
	}

	return Effect{}.command(name, fields...)
}

// seconds is a duration as the whole seconds a command takes, at least one.
func seconds(d time.Duration) int {
	return max(1, int(d.Round(time.Second)/time.Second))
}
