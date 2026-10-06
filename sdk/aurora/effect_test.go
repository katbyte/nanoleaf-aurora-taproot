package aurora

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// what an NL22 on 5.2.1 answers for an effect, spacing and all
const northernLights = `{"version":"2.0","animName":"kt Northern Lights","animType":"plugin","colorType":"HSB",` +
	`"palette":[{"hue":227,"saturation":100,"brightness":99,"probability":0.0},{"hue":182,"saturation":100,"brightness":100,"probability":0.0}],` +
	`"pluginType":"color","pluginUuid":"6970681a-20b5-4c5e-8813-bdaebc4ee4fa",` +
	`"pluginOptions":[{"name":"linDirection","value":"up"},{"name":"loop","value":true},{"name":"nColorsPerFrame","value":3},{"name":"transTime","value":107}],` +
	`"hasOverlay":false}`

func mustParse(t *testing.T, doc string) Effect {
	t.Helper()
	e, err := ParseEffect([]byte(doc))
	if err != nil {
		t.Fatalf("ParseEffect: %v", err)
	}
	return e
}

func TestEffectGoesBackAsItCame(t *testing.T) {
	t.Parallel()

	e := mustParse(t, northernLights)
	out, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	// byte for byte: a rewritten 0.0 or a reordered field is a different document to a controller
	if string(out) != northernLights {
		t.Errorf("an effect changed on its way through:\n got %s\nwant %s", out, northernLights)
	}

	// spacing in a file saved by hand is the one thing that does not survive
	spaced := mustParse(t, "{ \"animName\" : \"a\",\n  \"mainColorProb\" : 80.0 }")
	out, err = json.Marshal(spaced)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"animName":"a","mainColorProb":80.0}`; string(out) != want {
		t.Errorf("got %s, want %s", out, want)
	}
}

func TestEffectReads(t *testing.T) {
	t.Parallel()

	e := mustParse(t, northernLights)
	if e.Name() != "kt Northern Lights" || e.Type() != "plugin" || e.Version() != "2.0" || e.ColorType() != "HSB" {
		t.Errorf("name %q type %q version %q colours %q", e.Name(), e.Type(), e.Version(), e.ColorType())
	}
	if e.PluginType() != PluginTypeColor || e.PluginUUID() != "6970681a-20b5-4c5e-8813-bdaebc4ee4fa" {
		t.Errorf("plugin %q %q", e.PluginType(), e.PluginUUID())
	}
	if p := e.Palette(); len(p) != 2 || p[0] != (Color{Hue: 227, Saturation: 100, Brightness: 99}) {
		t.Errorf("palette %+v", p)
	}
	if o := e.PluginOptions(); len(o) != 4 || o[0].Name != "linDirection" || o[0].Value != "up" || o[1].Value != any(true) || o[3].Value != float64(107) {
		t.Errorf("options %+v", o)
	}
	if v, ok := e.Option("transTime"); !ok || v != float64(107) {
		t.Errorf("transTime %v %v", v, ok)
	}
	if _, ok := e.Option("delayTime"); ok {
		t.Error("an option the effect does not set")
	}
	want := []string{"version", "animName", "animType", "colorType", "palette", "pluginType", "pluginUuid", "pluginOptions", "hasOverlay"}
	if got := e.Fields(); !slices.Equal(got, want) {
		t.Errorf("fields %v", got)
	}
	if raw, ok := e.Field("hasOverlay"); !ok || string(raw) != "false" {
		t.Errorf("hasOverlay %s %v", raw, ok)
	}
	if e.IsZero() || !(Effect{}).IsZero() {
		t.Error("IsZero")
	}

	// fields of the wrong type, or missing, read as empty rather than failing
	odd := mustParse(t, `{"animName":7,"palette":"none","pluginOptions":{}}`)
	if odd.Name() != "" || odd.Palette() != nil || odd.PluginOptions() != nil || odd.PluginUUID() != "" {
		t.Errorf("odd: %q %v %v", odd.Name(), odd.Palette(), odd.PluginOptions())
	}
	// the documentation's own examples spell this one with a capital
	if p := mustParse(t, `{"Palette":[{"hue":1}]}`).Palette(); len(p) != 1 || p[0].Hue != 1 {
		t.Errorf("Palette %+v", p)
	}
}

func TestEffectChanges(t *testing.T) {
	t.Parallel()

	e := mustParse(t, northernLights)

	renamed := e.WithName("copy")
	if renamed.Name() != "copy" || e.Name() != "kt Northern Lights" {
		t.Errorf("WithName must leave the original alone: %q, %q", renamed.Name(), e.Name())
	}
	if !slices.Equal(renamed.Fields(), e.Fields()) {
		t.Errorf("a renamed effect keeps its fields in place: %v", renamed.Fields())
	}

	added, err := e.With("loop", true)
	if err != nil || added.Fields()[len(added.Fields())-1] != "loop" || len(e.Fields()) != 9 {
		t.Errorf("With adds at the end: %v, %v", added.Fields(), err)
	}
	if _, err := e.With("bad", make(chan int)); err == nil {
		t.Error("With a value JSON cannot hold should be an error")
	}

	gone := e.Without("hasOverlay").Without("not there")
	if _, ok := gone.Field("hasOverlay"); ok || len(gone.Fields()) != 8 || len(e.Fields()) != 9 {
		t.Errorf("Without: %v", gone.Fields())
	}
}

func TestEffectEqual(t *testing.T) {
	t.Parallel()

	a := mustParse(t, `{"animName":"a","palette":[{"hue":1,"probability":0.0}],"n":3}`)
	for doc, same := range map[string]bool{
		`{"animName":"a","palette":[{"hue":1,"probability":0.0}],"n":3}`: true,
		`{"n":3,"animName":"a","palette":[{"probability":0,"hue":1}]}`:   true,  // order and how a number is written
		`{"n":3.0,"animName":"a","palette":[{"hue":1,"probability":0}]}`: true,  //
		`{"animName":"a","palette":[{"hue":2,"probability":0.0}],"n":3}`: false, // a colour
		`{"animName":"a","palette":[{"hue":1,"probability":0.0}]}`:       false, // a field missing
		`{"animName":"a","palette":[],"n":3,"extra":null}`:               false,
	} {
		if got := a.Equal(mustParse(t, doc)); got != same {
			t.Errorf("Equal(%s) = %v, want %v", doc, got, same)
		}
	}

	b := mustParse(t, `{"animName":"b","palette":[{"hue":1,"probability":0}],"extra":1}`)
	if got, want := a.Differences(b), []string{"animName", "n", "extra"}; !slices.Equal(got, want) {
		t.Errorf("Differences = %v, want %v", got, want)
	}
}

func TestParseEffectRefuses(t *testing.T) {
	t.Parallel()

	for _, doc := range []string{``, `null`, `[]`, `"x"`, `{`, `{"a":}`, `{"a":1`, `{1:2}`} {
		if _, err := ParseEffect([]byte(doc)); err == nil || !strings.Contains(err.Error(), "reading an effect") {
			t.Errorf("ParseEffect(%q): %v", doc, err)
		}
	}

	// a name given twice keeps its first place and its last value
	if out, _ := json.Marshal(mustParse(t, `{"a":1,"b":2,"a":3}`)); string(out) != `{"a":3,"b":2}` {
		t.Errorf("got %s", out)
	}
}

func TestColorRGB(t *testing.T) {
	t.Parallel()

	for c, want := range map[Color][3]uint8{
		{Hue: 0, Saturation: 100, Brightness: 100}:    {255, 0, 0},
		{Hue: 120, Saturation: 100, Brightness: 100}:  {0, 255, 0},
		{Hue: 240, Saturation: 100, Brightness: 100}:  {0, 0, 255},
		{Hue: 60, Saturation: 100, Brightness: 100}:   {255, 255, 0},
		{Hue: 180, Saturation: 100, Brightness: 50}:   {0, 128, 128},
		{Hue: 300, Saturation: 50, Brightness: 100}:   {255, 128, 255},
		{Hue: 0, Saturation: 0, Brightness: 100}:      {255, 255, 255}, // no saturation is white, whatever the hue
		{Hue: 77, Saturation: 100, Brightness: 0}:     {0, 0, 0},
		{Hue: 360, Saturation: 100, Brightness: 100}:  {255, 0, 0}, // all the way round
		{Hue: -120, Saturation: 100, Brightness: 100}: {0, 0, 255},
		{Hue: 0, Saturation: 250, Brightness: 250}:    {255, 0, 0}, // out of range is the nearest in it
	} {
		if r, g, b := c.RGB(); [3]uint8{r, g, b} != want {
			t.Errorf("%+v is %d,%d,%d; want %v", c, r, g, b, want)
		}
	}
}

func TestShapeType(t *testing.T) {
	t.Parallel()

	if ShapeTriangle.String() != "triangle" || ShapeTriangle.SideLength() != 150 {
		t.Errorf("triangle: %s %v", ShapeTriangle, ShapeTriangle.SideLength())
	}
	if ShapeRhythm.SideLength() != 0 || ShapeType(99).String() != "unknown" || ShapeType(99).SideLength() != 0 {
		t.Error("a shape with no sides, and one nobody has listed")
	}
}
