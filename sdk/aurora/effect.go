package aurora

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
)

// The fields every effect a controller holds carries.
const (
	FieldName          = "animName"
	FieldType          = "animType"
	FieldVersion       = "version"
	FieldColorType     = "colorType"
	FieldPalette       = "palette"
	FieldPluginType    = "pluginType"
	FieldPluginUUID    = "pluginUuid"
	FieldPluginOptions = "pluginOptions"

	fieldCommand = "command"
)

// The values of FieldPluginType.
const (
	PluginTypeColor  = "color"  // moves colours by itself
	PluginTypeRhythm = "rhythm" // moves colours to sound, through the Rhythm module
)

// Effect is a scene, kept exactly as a controller sent it: every field, in
// its order, with its value's bytes untouched.
//
// What an effect is made of differs between firmware versions, and a
// controller is the only thing that knows what it will accept back. So an
// Effect is not rebuilt from a struct: it is the controller's own document,
// with methods to read the fields that matter, and it goes back to a
// controller the way it came. Copying a scene is reading it from one
// controller and adding it to another, nothing in between.
//
// The zero Effect has no fields.
type Effect struct {
	fields []effectField
}

type effectField struct {
	key   string
	value json.RawMessage
}

// Color is one colour of an effect's palette, as hue (0 to 360), saturation
// and brightness (0 to 100 each).
type Color struct {
	Hue        float64 `json:"hue"`
	Saturation float64 `json:"saturation"`
	Brightness float64 `json:"brightness"`
	// Probability is how often the highlight plugin picks the colour.
	Probability float64 `json:"probability"`
}

// RGB is the colour as red, green and blue, each 0 to 255, for showing it on
// a screen. A value outside its range is taken as the nearest inside it.
func (c Color) RGB() (r, g, b uint8) {
	sat := min(max(c.Saturation, 0), 100) / 100
	val := min(max(c.Brightness, 0), 100) / 100
	hue := math.Mod(math.Mod(c.Hue, 360)+360, 360) / 60

	chroma := val * sat
	x := chroma * (1 - math.Abs(math.Mod(hue, 2)-1))
	var rf, gf, bf float64
	switch {
	case hue < 1:
		rf, gf, bf = chroma, x, 0
	case hue < 2:
		rf, gf, bf = x, chroma, 0
	case hue < 3:
		rf, gf, bf = 0, chroma, x
	case hue < 4:
		rf, gf, bf = 0, x, chroma
	case hue < 5:
		rf, gf, bf = x, 0, chroma
	default:
		rf, gf, bf = chroma, 0, x
	}
	m := val - chroma
	level := func(f float64) uint8 { return uint8(math.Round((f + m) * 255)) }

	return level(rf), level(gf), level(bf)
}

// PluginOption is one setting of the plugin an effect runs: transTime, loop,
// linDirection and so on. Value is a bool, a float64 or a string.
type PluginOption struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// ParseEffect reads an effect from JSON: what a controller sent, or a file
// that was saved from one.
func ParseEffect(data []byte) (Effect, error) {
	var e Effect
	err := e.UnmarshalJSON(data)

	return e, err
}

// UnmarshalJSON keeps every field of the object as it is written.
func (e *Effect) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("reading an effect: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return errors.New("reading an effect: not a JSON object")
	}

	var fields []effectField
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("reading an effect: %w", err)
		}
		key, ok := tok.(string)
		if !ok {
			return errors.New("reading an effect: a field without a name")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return fmt.Errorf("reading an effect: field %s: %w", key, err)
		}
		// a name given twice keeps its first place and its last value, as a JSON reader would have it
		if i := slices.IndexFunc(fields, func(f effectField) bool { return f.key == key }); i >= 0 {
			fields[i].value = value
			continue
		}
		fields = append(fields, effectField{key: key, value: value})
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("reading an effect: %w", err)
	}
	e.fields = fields

	return nil
}

// MarshalJSON writes the fields back in their order, values untouched.
func (e Effect) MarshalJSON() ([]byte, error) {
	return e.object(nil)
}

// object writes the effect as a JSON object, with lead in front of its own
// fields and in place of any of them that share a name.
func (e Effect) object(lead []effectField) ([]byte, error) {
	own := slices.DeleteFunc(slices.Clone(e.fields), func(f effectField) bool {
		return slices.ContainsFunc(lead, func(l effectField) bool { return l.key == f.key })
	})

	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range slices.Concat(lead, own) {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := json.Marshal(f.key)
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		if err := json.Compact(&b, f.value); err != nil {
			return nil, fmt.Errorf("effect field %s: %w", f.key, err)
		}
	}
	b.WriteByte('}')

	return b.Bytes(), nil
}

// IsZero reports whether the effect has no fields at all.
func (e Effect) IsZero() bool { return len(e.fields) == 0 }

// Fields lists the effect's field names in order.
func (e Effect) Fields() []string {
	names := make([]string, len(e.fields))
	for i, f := range e.fields {
		names[i] = f.key
	}

	return names
}

// Field is one field's value as the controller wrote it.
func (e Effect) Field(key string) (json.RawMessage, bool) {
	for _, f := range e.fields {
		if f.key == key {
			return f.value, true
		}
	}

	return nil, false
}

// text is a field that holds a string, empty when it is missing or holds
// something else.
func (e Effect) text(key string) string {
	var s string
	if raw, ok := e.Field(key); ok {
		_ = json.Unmarshal(raw, &s) // a field of another type reads as empty
	}

	return s
}

// Name is the effect's name, which is how a controller tells effects apart.
func (e Effect) Name() string { return e.text(FieldName) }

// Type is what kind of effect this is. Every effect on current firmware is
// "plugin"; older firmware named the motion here (wheel, flow, random) and
// "custom" and "static" carry their frames in animData.
func (e Effect) Type() string { return e.text(FieldType) }

// Version is the version of the effect format the controller wrote.
func (e Effect) Version() string { return e.text(FieldVersion) }

// ColorType says how the palette's colours are written: "HSB".
func (e Effect) ColorType() string { return e.text(FieldColorType) }

// PluginType is PluginTypeColor or PluginTypeRhythm.
func (e Effect) PluginType() string { return e.text(FieldPluginType) }

// PluginUUID names the plugin that draws the effect; Client.Plugins says
// which a controller has.
func (e Effect) PluginUUID() string { return e.text(FieldPluginUUID) }

// Palette is the effect's colours, nil when it has none.
func (e Effect) Palette() []Color {
	// the documentation's own examples write this one with a capital
	raw, ok := e.Field(FieldPalette)
	if !ok {
		raw, ok = e.Field("Palette")
	}
	if !ok {
		return nil
	}

	var out []Color
	_ = json.Unmarshal(raw, &out) // a palette of another shape reads as none

	return out
}

// PluginOptions is the settings of the plugin the effect runs, nil when it
// sets none.
func (e Effect) PluginOptions() []PluginOption {
	raw, ok := e.Field(FieldPluginOptions)
	if !ok {
		return nil
	}

	var out []PluginOption
	_ = json.Unmarshal(raw, &out) // options of another shape read as none

	return out
}

// Option is one plugin option by name.
func (e Effect) Option(name string) (any, bool) {
	for _, o := range e.PluginOptions() {
		if o.Name == name {
			return o.Value, true
		}
	}

	return nil, false
}

// With returns the effect with a field set to value, in the field's place if
// the effect has it and at the end if not.
func (e Effect) With(key string, value any) (Effect, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return Effect{}, fmt.Errorf("effect field %s: %w", key, err)
	}

	out := Effect{fields: slices.Clone(e.fields)}
	if i := slices.IndexFunc(out.fields, func(f effectField) bool { return f.key == key }); i >= 0 {
		out.fields[i].value = raw
		return out, nil
	}
	out.fields = append(out.fields, effectField{key: key, value: raw})

	return out, nil
}

// WithName returns the effect under another name. Added to a controller it is
// a separate effect from the one it was read as.
func (e Effect) WithName(name string) Effect {
	out, _ := e.With(FieldName, name) // a string always encodes
	return out
}

// Without returns the effect with a field removed.
func (e Effect) Without(key string) Effect {
	return Effect{fields: slices.DeleteFunc(slices.Clone(e.fields), func(f effectField) bool { return f.key == key })}
}

// Equal reports whether two effects say the same thing: the same fields with
// the same values, whatever their order and however the numbers are written.
func (e Effect) Equal(other Effect) bool {
	return len(e.Differences(other)) == 0
}

// Differences names the fields that two effects do not share or hold
// different values for, in the order they appear, e's first.
func (e Effect) Differences(other Effect) []string {
	var diff []string
	for _, f := range e.fields {
		theirs, ok := other.Field(f.key)
		if !ok || !sameJSON(f.value, theirs) {
			diff = append(diff, f.key)
		}
	}
	for _, f := range other.fields {
		if _, ok := e.Field(f.key); !ok {
			diff = append(diff, f.key)
		}
	}

	return diff
}

// sameJSON compares two values by what they mean: 0 and 0.0 are the same
// number, and an object's fields have no order.
func sameJSON(a, b json.RawMessage) bool {
	var av, bv any
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return bytes.Equal(a, b)
	}
	ab, aerr := json.Marshal(av)
	bb, berr := json.Marshal(bv)

	return aerr == nil && berr == nil && bytes.Equal(ab, bb)
}

// command is the body of a write that carries the effect: the command, then
// the effect's own fields, wrapped as the API expects.
func (e Effect) command(name string, extra ...effectField) ([]byte, error) {
	// the command goes first, in place of one a file saved from a write would carry
	inner, err := e.object(append([]effectField{{key: fieldCommand, value: quote(name)}}, extra...))
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]json.RawMessage{"write": inner})
}

// quote is s as a JSON string.
func quote(s string) json.RawMessage {
	raw, _ := json.Marshal(s) // a string always encodes
	return raw
}
