// The client against a canned controller that answers as a real NL22 on
// firmware 5.2.1 did (auroratest). These prove the client asks for each thing
// the way a controller expects and reads what comes back; live_test.go, which
// needs a device and is not part of a normal run, proves the canned answers
// are still what a real one sends.
package aurora_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora/auroratest"
)

const scene = "kt Northern Lights"

// paired is a canned controller and a client that already holds a token for it.
func paired(t *testing.T, opts ...aurora.Option) (*auroratest.Controller, *aurora.Client) {
	t.Helper()

	ctl := auroratest.New(t)
	c, err := aurora.New(ctl.Host(), ctl.Token(), opts...)
	if err != nil {
		t.Fatal(err)
	}

	return ctl, c
}

// last is the most recent request the controller received.
func last(t *testing.T, ctl *auroratest.Controller) auroratest.Request {
	t.Helper()

	reqs := ctl.Requests()
	if len(reqs) == 0 {
		t.Fatal("the controller was never asked anything")
	}

	return reqs[len(reqs)-1]
}

func wantRequest(t *testing.T, ctl *auroratest.Controller, method, path, body string) {
	t.Helper()

	if got := last(t, ctl); got.Method != method || got.Path != path || got.Body != body {
		t.Errorf("sent  %s %s %s\nwant  %s %s %s", got.Method, got.Path, got.Body, method, path, body)
	}
}

func TestPairing(t *testing.T) {
	t.Parallel()

	ctl := auroratest.New(t)
	c, err := aurora.New(ctl.Host(), "")
	if err != nil {
		t.Fatal(err)
	}

	// nobody has held the button
	if _, err := c.NewToken(t.Context()); !errors.Is(err, aurora.ErrNotPairing) {
		t.Fatalf("want ErrNotPairing, got %v", err)
	}
	wantRequest(t, ctl, http.MethodPost, "/new", "")

	ctl.Pair()
	token, err := c.NewToken(t.Context())
	if err != nil || token == "" {
		t.Fatalf("NewToken: %q, %v", token, err)
	}
	// the window closes behind the token
	if _, err := c.NewToken(t.Context()); !errors.Is(err, aurora.ErrNotPairing) {
		t.Errorf("a second token from one press: %v", err)
	}

	c = c.WithToken(token)
	if _, err := c.Info(t.Context()); err != nil {
		t.Fatalf("the new token does not work: %v", err)
	}

	if err := c.DeleteToken(t.Context()); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	wantRequest(t, ctl, http.MethodDelete, "", "")
	if _, err := c.Info(t.Context()); !aurora.IsUnauthorized(err) {
		t.Errorf("a deleted token still works: %v", err)
	}
}

func TestInfo(t *testing.T) {
	t.Parallel()

	ctl, c := paired(t)
	info, err := c.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	wantRequest(t, ctl, http.MethodGet, "/", "")

	if info.Name != "Light Panels 53:A6:3C" || info.Model != "NL22" || info.FirmwareVersion != "5.2.1" || info.HardwareVersion != "1.6-2" || info.Manufacturer != "Nanoleaf" {
		t.Errorf("identity: %+v", info)
	}
	if info.Effects.Select != scene || len(info.Effects.List) != 17 {
		t.Errorf("effects: %q, %d listed", info.Effects.Select, len(info.Effects.List))
	}
	if !info.State.On.Value || info.State.Brightness != (aurora.Range{Value: 33, Max: 100, Min: 0}) || info.State.ColorMode != aurora.ColorModeEffect {
		t.Errorf("state: %+v", info.State)
	}
	if info.State.CT != (aurora.Range{Value: 3000, Max: 6500, Min: 1200}) {
		t.Errorf("ct: %+v", info.State.CT)
	}
	if l := info.PanelLayout.Layout; l.NumPanels != 4 || len(l.Panels) != 4 || l.SideLength != 150 {
		t.Errorf("layout: %+v", l)
	}
	if info.PanelLayout.GlobalOrientation.Value != 88 {
		t.Errorf("orientation: %+v", info.PanelLayout.GlobalOrientation)
	}
	if r := info.Rhythm; r == nil || !r.Connected || r.ID != 186 || r.FirmwareVersion != "2.4.3" || r.Position.O != 180 {
		t.Errorf("rhythm: %+v", r)
	}
	// the sections newer firmware adds are kept, though nothing here names them
	for _, section := range []string{"schedules", "cloudHash", "firmwareUpgrade", "discovery"} {
		if !strings.Contains(string(info.Raw), `"`+section+`"`) {
			t.Errorf("Raw has lost %s", section)
		}
	}
}

func TestState(t *testing.T) {
	t.Parallel()

	ctl, c := paired(t)
	ctx := t.Context()

	if s, err := c.State(ctx); err != nil || s.Brightness.Value != 33 || !s.On.Value {
		t.Errorf("State: %+v, %v", s, err)
	}
	if on, err := c.On(ctx); err != nil || !on {
		t.Errorf("On: %v, %v", on, err)
	}
	wantRequest(t, ctl, http.MethodGet, "/state/on", "")
	if b, err := c.Brightness(ctx); err != nil || b != (aurora.Range{Value: 33, Max: 100}) {
		t.Errorf("Brightness: %+v, %v", b, err)
	}
	if h, err := c.Hue(ctx); err != nil || h.Max != 360 {
		t.Errorf("Hue: %+v, %v", h, err)
	}
	if s, err := c.Saturation(ctx); err != nil || s.Max != 100 {
		t.Errorf("Saturation: %+v, %v", s, err)
	}
	if ct, err := c.ColorTemperature(ctx); err != nil || ct.Value != 3000 {
		t.Errorf("ColorTemperature: %+v, %v", ct, err)
	}
	if m, err := c.ColorMode(ctx); err != nil || m != aurora.ColorModeEffect {
		t.Errorf("ColorMode: %q, %v", m, err)
	}
	wantRequest(t, ctl, http.MethodGet, "/state/colorMode", "")

	if err := c.SetOn(ctx, false); err != nil || ctl.State().On.Value {
		t.Errorf("SetOn: %v", err)
	}
	wantRequest(t, ctl, http.MethodPut, "/state", `{"on":{"value":false}}`)

	if err := c.SetBrightness(ctx, 60, 0); err != nil || ctl.State().Brightness.Value != 60 {
		t.Errorf("SetBrightness: %v", err)
	}
	wantRequest(t, ctl, http.MethodPut, "/state", `{"brightness":{"value":60}}`)
	if err := c.SetBrightness(ctx, 10, 2400*time.Millisecond); err != nil {
		t.Errorf("SetBrightness with a fade: %v", err)
	}
	wantRequest(t, ctl, http.MethodPut, "/state", `{"brightness":{"duration":2,"value":10}}`)

	// an increment stops at the limit; a value past it is refused
	if err := c.AdjustBrightness(ctx, 500); err != nil || ctl.State().Brightness.Value != 100 {
		t.Errorf("AdjustBrightness: %v, now %d", err, ctl.State().Brightness.Value)
	}
	wantRequest(t, ctl, http.MethodPut, "/state", `{"brightness":{"increment":500}}`)
	if err := c.SetBrightness(ctx, 101, 0); err == nil || !strings.Contains(err.Error(), "refused its contents") {
		t.Errorf("a brightness of 101: %v", err)
	}

	if err := c.SetHue(ctx, 120); err != nil || ctl.State().Hue.Value != 120 || ctl.State().ColorMode != aurora.ColorModeHS {
		t.Errorf("SetHue: %v, %+v", err, ctl.State())
	}
	wantRequest(t, ctl, http.MethodPut, "/state", `{"hue":{"value":120}}`)
	if err := c.AdjustHue(ctx, -20); err != nil || ctl.State().Hue.Value != 100 {
		t.Errorf("AdjustHue: %v", err)
	}
	if err := c.SetSaturation(ctx, 40); err != nil || ctl.State().Sat.Value != 40 {
		t.Errorf("SetSaturation: %v", err)
	}
	wantRequest(t, ctl, http.MethodPut, "/state", `{"sat":{"value":40}}`)
	if err := c.AdjustSaturation(ctx, 5); err != nil || ctl.State().Sat.Value != 45 {
		t.Errorf("AdjustSaturation: %v", err)
	}
	if err := c.SetColorTemperature(ctx, 4000); err != nil || ctl.State().CT.Value != 4000 || ctl.State().ColorMode != aurora.ColorModeCT {
		t.Errorf("SetColorTemperature: %v, %+v", err, ctl.State())
	}
	wantRequest(t, ctl, http.MethodPut, "/state", `{"ct":{"value":4000}}`)
	if err := c.AdjustColorTemperature(ctx, -100); err != nil || ctl.State().CT.Value != 3900 {
		t.Errorf("AdjustColorTemperature: %v", err)
	}
	wantRequest(t, ctl, http.MethodPut, "/state", `{"ct":{"increment":-100}}`)
	// one colour is not an effect, and the controller says so
	if ctl.Selected() != aurora.EffectSolid {
		t.Errorf("running %q", ctl.Selected())
	}
}

func TestReadingEffects(t *testing.T) {
	t.Parallel()

	ctl, c := paired(t)
	ctx := t.Context()

	names, err := c.EffectNames(ctx)
	if err != nil || len(names) != 17 || names[16] != scene || names[0] != "Color Burst" {
		t.Fatalf("EffectNames: %v, %v", names, err)
	}
	wantRequest(t, ctl, http.MethodGet, "/effects/effectsList", "")

	if s, err := c.SelectedEffect(ctx); err != nil || s != scene {
		t.Errorf("SelectedEffect: %q, %v", s, err)
	}

	e, err := c.Effect(ctx, scene)
	if err != nil {
		t.Fatal(err)
	}
	wantRequest(t, ctl, http.MethodPut, "/effects", `{"write":{"command":"request","animName":"kt Northern Lights"}}`)
	if e.Name() != scene || e.PluginUUID() != "6970681a-20b5-4c5e-8813-bdaebc4ee4fa" || len(e.Palette()) != 7 {
		t.Errorf("the effect: %q %q, %d colours", e.Name(), e.PluginUUID(), len(e.Palette()))
	}
	if v, _ := e.Option("transTime"); v != float64(107) {
		t.Errorf("transTime: %v", v)
	}
	// what the controller holds and what the client read are the same document
	held, _ := ctl.Effect(scene)
	a, _ := json.Marshal(held)
	b, _ := json.Marshal(e)
	if !bytes.Equal(a, b) {
		t.Errorf("read differs from held:\n held %s\n read %s", a, b)
	}

	if _, err := c.Effect(ctx, "no such scene"); !aurora.IsNotFound(err) {
		t.Errorf("an effect that is not there: %v", err)
	}

	all, err := c.Effects(ctx)
	if err != nil || len(all) != 17 {
		t.Fatalf("Effects: %d, %v", len(all), err)
	}
	wantRequest(t, ctl, http.MethodPut, "/effects", `{"write":{"command":"requestAll"}}`)
	// one at a time and all at once give the same effect
	if i := slices.IndexFunc(all, func(x aurora.Effect) bool { return x.Name() == scene }); i < 0 || !all[i].Equal(e) {
		t.Error("the effect read alone differs from the one among all")
	}
	rhythm := 0
	for _, x := range all {
		if x.Type() != "plugin" {
			t.Errorf("%s is a %q", x.Name(), x.Type())
		}
		if x.PluginType() == aurora.PluginTypeRhythm {
			rhythm++
		}
	}
	if rhythm != 8 {
		t.Errorf("%d effects move to sound, want 8", rhythm)
	}

	plugins, err := c.Plugins(ctx)
	if err != nil || len(plugins) != 14 {
		t.Fatalf("Plugins: %d, %v", len(plugins), err)
	}
	wantRequest(t, ctl, http.MethodPut, "/effects", `{"write":{"command":"requestPlugins","version":"2.0"}}`)
	i := slices.IndexFunc(plugins, func(p aurora.Plugin) bool { return p.UUID == e.PluginUUID() })
	if i < 0 || plugins[i].Name != "Wheel" || plugins[i].Type != aurora.PluginTypeColor || len(plugins[i].Config) != 4 {
		t.Fatalf("the plugin the effect runs: %+v", plugins)
	}
	if tt := plugins[i].Config[1]; tt.Name != "transTime" || tt.Type != "int" || tt.MinValue == nil || *tt.MinValue != 1 || *tt.MaxValue != 600 {
		t.Errorf("transTime: %+v", tt)
	}
	if dir := plugins[i].Config[3]; dir.Name != "linDirection" || !slices.Equal(dir.Strings, []string{"left", "right", "up", "down"}) {
		t.Errorf("linDirection: %+v", dir)
	}

	// none of that changed anything
	if w := ctl.Writes(); len(w) != 0 {
		t.Errorf("reading wrote: %+v", w)
	}
}

func TestWritingEffects(t *testing.T) {
	t.Parallel()

	ctl, c := paired(t)
	ctx := t.Context()

	e, err := c.Effect(ctx, scene)
	if err != nil {
		t.Fatal(err)
	}

	// a copy under another name, sent exactly as it was read with the command in front
	if err := c.AddEffect(ctx, e.WithName("a copy")); err != nil {
		t.Fatal(err)
	}
	sent := last(t, ctl)
	if !strings.HasPrefix(sent.Body, `{"write":{"command":"add","version":"2.0","animName":"a copy","animType":"plugin"`) {
		t.Errorf("add sent %s", sent.Body)
	}
	for _, kept := range []string{`"probability":0.0`, `"hasOverlay":false`, `{"name":"transTime","value":107}`} {
		if !strings.Contains(sent.Body, kept) {
			t.Errorf("add lost %s", kept)
		}
	}
	back, err := c.Effect(ctx, "a copy")
	if err != nil || !back.Equal(e.WithName("a copy")) {
		t.Errorf("what was added is not what comes back: %v, differs in %v", err, back.Differences(e.WithName("a copy")))
	}
	if names := ctl.EffectNames(); len(names) != 18 || names[16] != "a copy" {
		t.Errorf("after the add: %v", names)
	}

	if err := c.AddEffect(ctx, aurora.Effect{}); err == nil || !strings.Contains(err.Error(), "needs a name") {
		t.Errorf("an effect with no name: %v", err)
	}

	if err := c.RenameEffect(ctx, "a copy", "renamed"); err != nil {
		t.Fatal(err)
	}
	wantRequest(t, ctl, http.MethodPut, "/effects", `{"write":{"command":"rename","animName":"a copy","newName":"renamed"}}`)
	if _, ok := ctl.Effect("renamed"); !ok {
		t.Error("the rename did not take")
	}

	if err := c.SelectEffect(ctx, "renamed"); err != nil || ctl.Selected() != "renamed" {
		t.Errorf("SelectEffect: %v, running %q", err, ctl.Selected())
	}
	wantRequest(t, ctl, http.MethodPut, "/effects", `{"select":"renamed"}`)
	if err := c.SelectEffect(ctx, "no such scene"); !aurora.IsNotFound(err) {
		t.Errorf("selecting an effect that is not there: %v", err)
	}

	if err := c.DeleteEffect(ctx, "renamed"); err != nil {
		t.Fatal(err)
	}
	wantRequest(t, ctl, http.MethodPut, "/effects", `{"write":{"command":"delete","animName":"renamed"}}`)
	if names := ctl.EffectNames(); len(names) != 17 {
		t.Errorf("after the delete: %v", names)
	}
	if err := c.DeleteEffect(ctx, "renamed"); !aurora.IsNotFound(err) {
		t.Errorf("deleting twice: %v", err)
	}

	if err := c.DisplayEffect(ctx, e); err != nil || ctl.Selected() != aurora.EffectDynamic {
		t.Errorf("DisplayEffect: %v, running %q", err, ctl.Selected())
	}
	if got := last(t, ctl).Body; !strings.HasPrefix(got, `{"write":{"command":"display","version":"2.0","animName":"kt Northern Lights"`) {
		t.Errorf("display sent %s", got)
	}
	if err := c.DisplayEffectFor(ctx, e, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := last(t, ctl).Body; !strings.HasPrefix(got, `{"write":{"command":"displayTemp","duration":5,"version":"2.0"`) {
		t.Errorf("displayTemp sent %s", got)
	}
	if err := c.DisplaySavedEffectFor(ctx, "Flames", 90*time.Second); err != nil {
		t.Fatal(err)
	}
	wantRequest(t, ctl, http.MethodPut, "/effects", `{"write":{"command":"displayTemp","duration":90,"animName":"Flames"}}`)

	if _, err := c.StartExternalControl(ctx, "v1"); err != nil {
		t.Fatal(err)
	}
	wantRequest(t, ctl, http.MethodPut, "/effects", `{"write":{"command":"display","animType":"extControl","extControlVersion":"v1"}}`)
	if _, err := c.StartExternalControl(ctx, ""); err != nil {
		t.Fatal(err)
	}
	wantRequest(t, ctl, http.MethodPut, "/effects", `{"write":{"command":"display","animType":"extControl"}}`)

	// a command this package has no method for
	raw, err := c.Write(ctx, struct {
		Command string `json:"command"`
	}{"requestAll"})
	if err != nil || !strings.HasPrefix(string(raw), `{"animations":[`) {
		t.Errorf("Write: %.40s, %v", raw, err)
	}
}

func TestLayout(t *testing.T) {
	t.Parallel()

	ctl, c := paired(t)
	ctx := t.Context()

	l, err := c.Layout(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantRequest(t, ctl, http.MethodGet, "/panelLayout/layout", "")
	if l.NumPanels != 4 || l.SideLength != 150 || len(l.Panels) != 4 {
		t.Fatalf("layout: %+v", l)
	}
	if p := l.Panels[0]; p != (aurora.Panel{ID: 234, X: 299, Y: 43, O: 180, ShapeType: aurora.ShapeTriangle}) {
		t.Errorf("first panel: %+v", p)
	}

	if o, err := c.GlobalOrientation(ctx); err != nil || o != (aurora.Range{Value: 88, Max: 360}) {
		t.Errorf("GlobalOrientation: %+v, %v", o, err)
	}
	if err := c.SetGlobalOrientation(ctx, 120); err != nil {
		t.Fatal(err)
	}
	wantRequest(t, ctl, http.MethodPut, "/panelLayout", `{"globalOrientation":{"value":120}}`)
	if o, _ := c.GlobalOrientation(ctx); o.Value != 120 { // the value says whether it worked
		t.Errorf("the orientation did not take: %+v", o)
	}

	if err := c.Identify(ctx); err != nil {
		t.Fatal(err)
	}
	wantRequest(t, ctl, http.MethodPut, "/identify", "")
}

func TestRhythm(t *testing.T) {
	t.Parallel()

	ctl, c := paired(t)
	ctx := t.Context()

	r, err := c.Rhythm(ctx)
	if err != nil || !r.Connected || r.Active || r.ID != 186 || r.HardwareVersion != "2.0" || r.Mode != aurora.RhythmModeMicrophone {
		t.Errorf("Rhythm: %+v, %v", r, err)
	}
	if v, err := c.RhythmConnected(ctx); err != nil || !v {
		t.Errorf("RhythmConnected: %v, %v", v, err)
	}
	wantRequest(t, ctl, http.MethodGet, "/rhythm/rhythmConnected", "")
	if v, err := c.RhythmActive(ctx); err != nil || v {
		t.Errorf("RhythmActive: %v, %v", v, err)
	}
	if v, err := c.RhythmID(ctx); err != nil || v != 186 {
		t.Errorf("RhythmID: %v, %v", v, err)
	}
	if v, err := c.RhythmHardwareVersion(ctx); err != nil || v != "2.0" {
		t.Errorf("RhythmHardwareVersion: %v, %v", v, err)
	}
	if v, err := c.RhythmFirmwareVersion(ctx); err != nil || v != "2.4.3" {
		t.Errorf("RhythmFirmwareVersion: %v, %v", v, err)
	}
	if v, err := c.RhythmAuxAvailable(ctx); err != nil || v {
		t.Errorf("RhythmAuxAvailable: %v, %v", v, err)
	}
	if v, err := c.RhythmPosition(ctx); err != nil || v.O != 180 || v.Y < 43 || v.Y > 44 {
		t.Errorf("RhythmPosition: %+v, %v", v, err)
	}

	if err := c.SetRhythmMode(ctx, aurora.RhythmModeAux); err != nil {
		t.Fatal(err)
	}
	wantRequest(t, ctl, http.MethodPut, "/rhythm/rhythmMode", `{"rhythmMode":1}`)
	if v, err := c.RhythmMode(ctx); err != nil || v != aurora.RhythmModeAux {
		t.Errorf("RhythmMode: %v, %v", v, err)
	}
}

// A dry run sends every read and none of the writes, and hands over exactly
// what each write would have been.
func TestDryRun(t *testing.T) {
	t.Parallel()

	var would []aurora.Request
	ctl, c := paired(t, aurora.WithDryRun(func(r aurora.Request) { would = append(would, r) }))
	ctx := t.Context()

	e, err := c.Effect(ctx, scene)
	if err != nil {
		t.Fatalf("a dry run still reads: %v", err)
	}

	for name, write := range map[string]func() error{
		"add":         func() error { return c.AddEffect(ctx, e.WithName("a copy")) },
		"delete":      func() error { return c.DeleteEffect(ctx, scene) },
		"rename":      func() error { return c.RenameEffect(ctx, scene, "x") },
		"select":      func() error { return c.SelectEffect(ctx, "Flames") },
		"display":     func() error { return c.DisplayEffect(ctx, e) },
		"state":       func() error { return c.SetOn(ctx, false) },
		"orientation": func() error { return c.SetGlobalOrientation(ctx, 0) },
		"identify":    func() error { return c.Identify(ctx) },
		"token":       func() error { return c.DeleteToken(ctx) },
		"new token": func() error {
			token, terr := c.NewToken(ctx)
			if token != "" {
				t.Error("a dry run handed out a token")
			}
			return terr
		},
		"external control": func() error {
			_, xerr := c.StartExternalControl(ctx, "")
			return xerr
		},
		"write": func() error {
			_, werr := c.Write(ctx, map[string]string{"command": "delete", "animName": scene})
			return werr
		},
	} {
		if err := write(); err != nil {
			t.Errorf("%s: a dry run reports what it would do, not an error: %v", name, err)
		}
	}

	if w := ctl.Writes(); len(w) != 0 {
		t.Fatalf("a dry run wrote to the controller: %+v", w)
	}
	if len(would) != 12 {
		t.Fatalf("%d writes recorded, want 12: %+v", len(would), would)
	}
	if len(ctl.EffectNames()) != 17 || ctl.Selected() != scene || !ctl.State().On.Value {
		t.Error("the controller changed")
	}

	for _, r := range would {
		if strings.Contains(r.Path, "0000") {
			t.Errorf("a recorded path carries the token: %s", r.Path)
		}
	}
	i := slices.IndexFunc(would, func(r aurora.Request) bool { return strings.Contains(string(r.Body), `"command":"add"`) })
	if i < 0 || would[i].Method != http.MethodPut || would[i].Path != "/api/v1/<token>/effects" {
		t.Fatalf("the add: %+v", would)
	}
	// the body recorded is the body that would have gone: sending it does the add
	live, err := aurora.New(ctl.Host(), ctl.Token())
	if err != nil {
		t.Fatal(err)
	}
	var cmd struct {
		Write aurora.Effect `json:"write"`
	}
	if err := json.Unmarshal(would[i].Body, &cmd); err != nil {
		t.Fatal(err)
	}
	if err := live.AddEffect(ctx, cmd.Write); err != nil {
		t.Fatal(err)
	}
	if got := last(t, ctl).Body; got != string(would[i].Body) {
		t.Errorf("recorded %s\n    sent %s", would[i].Body, got)
	}
}

func TestEvents(t *testing.T) {
	t.Parallel()

	ctl, c := paired(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var mu sync.Mutex
	var got []aurora.Event
	done := make(chan error, 1)
	go func() {
		done <- c.Events(ctx, []aurora.EventType{aurora.EventState, aurora.EventEffects}, func(e aurora.Event) {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, e)
		})
	}()
	waitFor(t, "the stream to open", func() bool { return ctl.Listeners() == 1 })
	if q := last(t, ctl); q.Method != http.MethodGet || q.Path != "/events" {
		t.Errorf("asked %+v", q)
	}

	// changes made through the API come back as events, as do ones made at the controller
	other, err := aurora.New(ctl.Host(), ctl.Token())
	if err != nil {
		t.Fatal(err)
	}
	if err := other.SelectEffect(ctx, "Flames"); err != nil {
		t.Fatal(err)
	}
	if err := other.SetBrightness(ctx, 80, 0); err != nil {
		t.Fatal(err)
	}
	ctl.Emit(aurora.EventState, aurora.AttrOn, false)

	waitFor(t, "three events", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 3
	})
	mu.Lock()
	if e := got[0]; e.Type != aurora.EventEffects || e.Attr != aurora.AttrSelectedEffect || string(e.Value) != `"Flames"` {
		t.Errorf("the effect change: %+v", e)
	}
	if e := got[1]; e.Type != aurora.EventState || e.Attr != aurora.AttrBrightness || string(e.Value) != "80" {
		t.Errorf("the brightness change: %+v", e)
	}
	if e := got[2]; e.Type != aurora.EventState || e.Attr != aurora.AttrOn || string(e.Value) != "false" {
		t.Errorf("the power change: %+v", e)
	}
	mu.Unlock()

	cancel()
	if err := <-done; err != nil {
		t.Errorf("stopping is not an error: %v", err)
	}
}

func TestEventsEndWhenTheControllerDoes(t *testing.T) {
	t.Parallel()

	ctl, c := paired(t)
	done := make(chan error, 1)
	go func() { done <- c.Events(t.Context(), nil, func(aurora.Event) {}) }()
	waitFor(t, "the stream to open", func() bool { return ctl.Listeners() == 1 })

	ctl.Server.CloseClientConnections()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a stream the controller dropped must be an error, so a caller knows to listen again")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Events did not return")
	}

	ctl.Fail(http.MethodGet, "/events", http.StatusNotFound)
	if err := c.Events(t.Context(), nil, func(aurora.Event) {}); !aurora.IsNotFound(err) {
		t.Errorf("a controller too old to have events: %v", err)
	}
}

func TestRefusals(t *testing.T) {
	t.Parallel()

	ctl := auroratest.New(t)
	stranger, err := aurora.New(ctl.Host(), "not-a-token-the-controller-knows")
	if err != nil {
		t.Fatal(err)
	}
	_, err = stranger.Info(t.Context())
	if !aurora.IsUnauthorized(err) || aurora.IsNotFound(err) {
		t.Fatalf("an unknown token: %v", err)
	}
	if strings.Contains(err.Error(), "not-a-token") {
		t.Errorf("the error quotes the token: %v", err)
	}
	if !strings.Contains(err.Error(), "does not know this token") {
		t.Errorf("the error should say what a 401 means: %v", err)
	}

	c, err := aurora.New(ctl.Host(), ctl.Token())
	if err != nil {
		t.Fatal(err)
	}
	ctl.Fail(http.MethodPut, "/effects", http.StatusInternalServerError)
	err = c.SelectEffect(t.Context(), "Flames")
	if se, ok := errors.AsType[*aurora.StatusError](err); !ok || se.Status != http.StatusInternalServerError || se.Path != "/effects" {
		t.Errorf("a failing controller: %v", err)
	}
	ctl.Fail(http.MethodPut, "/effects", 0)
	if err := c.SelectEffect(t.Context(), "Flames"); err != nil {
		t.Errorf("once it recovers: %v", err)
	}

	if c.Host() != ctl.Host() {
		t.Errorf("Host: %q", c.Host())
	}
}

// waitFor polls for something another goroutine is about to do.
func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
