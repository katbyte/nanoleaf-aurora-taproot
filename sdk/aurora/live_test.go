//go:build live

// The client against a real controller. Nothing here is part of a normal test
// run, and nothing here changes the controller: every call is a read. It
// exists to prove the canned controller in auroratest still answers the way a
// real one does, and is run by hand after a firmware update or when the
// documentation changes:
//
//	AURORA_TEST_HOST=10.0.5.183 AURORA_TEST_TOKEN=... go test -tags live -run Live ./sdk/aurora/
//
// or `make test-live`, which takes the address and token from taproot's own
// configuration.
package aurora_test

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

func liveClient(t *testing.T) *aurora.Client {
	t.Helper()

	host, token := os.Getenv("AURORA_TEST_HOST"), os.Getenv("AURORA_TEST_TOKEN")
	if host == "" || token == "" {
		t.Skip("set AURORA_TEST_HOST and AURORA_TEST_TOKEN to run against a real controller")
	}
	c, err := aurora.New(host, token)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func TestLiveReads(t *testing.T) { //nolint:paralleltest // one at a time: a real controller is a small thing
	c := liveClient(t)
	ctx := t.Context()

	info, err := c.Info(ctx)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	t.Logf("%s: %s on firmware %s, %d panels, %d effects, running %q",
		info.Name, info.Model, info.FirmwareVersion, info.PanelLayout.Layout.NumPanels, len(info.Effects.List), info.Effects.Select)
	if info.Name == "" || info.Model == "" || info.FirmwareVersion == "" {
		t.Errorf("a controller with no name, model or firmware: %+v", info)
	}

	// every single value agrees with the whole
	state, err := c.State(ctx)
	if err != nil || state != info.State {
		t.Errorf("State: %+v, %v; the whole says %+v", state, err, info.State)
	}
	if on, err := c.On(ctx); err != nil || on != state.On.Value {
		t.Errorf("On: %v, %v", on, err)
	}
	for name, read := range map[string]struct {
		got  func(context.Context) (aurora.Range, error)
		want aurora.Range
	}{
		"brightness":  {c.Brightness, state.Brightness},
		"hue":         {c.Hue, state.Hue},
		"saturation":  {c.Saturation, state.Sat},
		"temperature": {c.ColorTemperature, state.CT},
		"orientation": {c.GlobalOrientation, info.PanelLayout.GlobalOrientation},
	} {
		if got, err := read.got(ctx); err != nil || got != read.want {
			t.Errorf("%s: %+v, %v; the whole says %+v", name, got, err, read.want)
		}
	}
	if mode, err := c.ColorMode(ctx); err != nil || mode != state.ColorMode {
		t.Errorf("ColorMode: %q, %v", mode, err)
	}

	layout, err := c.Layout(ctx)
	if err != nil || layout.NumPanels != len(layout.Panels) || layout.NumPanels == 0 {
		t.Errorf("Layout: %+v, %v", layout, err)
	}

	names, err := c.EffectNames(ctx)
	if err != nil || !slices.Equal(names, info.Effects.List) {
		t.Errorf("EffectNames: %v, %v", names, err)
	}
	if sel, err := c.SelectedEffect(ctx); err != nil || sel != info.Effects.Select {
		t.Errorf("SelectedEffect: %q, %v", sel, err)
	}

	all, err := c.Effects(ctx)
	if err != nil || len(all) != len(names) {
		t.Fatalf("Effects: %d of %d, %v", len(all), len(names), err)
	}
	for _, name := range names {
		e, rerr := c.Effect(ctx, name)
		if rerr != nil {
			t.Errorf("Effect(%q): %v", name, rerr)
			continue
		}
		i := slices.IndexFunc(all, func(x aurora.Effect) bool { return x.Name() == name })
		if i < 0 || !all[i].Equal(e) {
			t.Errorf("%q read alone differs from the one among all", name)
		}
	}
	if _, err := c.Effect(ctx, "taproot: no such scene"); !aurora.IsNotFound(err) {
		t.Errorf("an effect that is not there: %v", err)
	}

	plugins, err := c.Plugins(ctx)
	if err != nil || len(plugins) == 0 {
		t.Fatalf("Plugins: %d, %v", len(plugins), err)
	}
	for _, e := range all {
		if !slices.ContainsFunc(plugins, func(p aurora.Plugin) bool { return p.UUID == e.PluginUUID() }) {
			t.Errorf("%q runs plugin %s, which the controller does not list", e.Name(), e.PluginUUID())
		}
	}

	if info.Rhythm != nil {
		r, err := c.Rhythm(ctx)
		if err != nil || r != *info.Rhythm {
			t.Errorf("Rhythm: %+v, %v; the whole says %+v", r, err, *info.Rhythm)
		}
		if v, err := c.RhythmConnected(ctx); err != nil || v != r.Connected {
			t.Errorf("RhythmConnected: %v, %v", v, err)
		}
		if v, err := c.RhythmActive(ctx); err != nil || v != r.Active {
			t.Errorf("RhythmActive: %v, %v", v, err)
		}
		if v, err := c.RhythmID(ctx); err != nil || v != r.ID {
			t.Errorf("RhythmID: %v, %v", v, err)
		}
		if v, err := c.RhythmHardwareVersion(ctx); err != nil || v != r.HardwareVersion {
			t.Errorf("RhythmHardwareVersion: %v, %v", v, err)
		}
		if v, err := c.RhythmFirmwareVersion(ctx); err != nil || v != r.FirmwareVersion {
			t.Errorf("RhythmFirmwareVersion: %v, %v", v, err)
		}
		if v, err := c.RhythmAuxAvailable(ctx); err != nil || v != r.AuxAvailable {
			t.Errorf("RhythmAuxAvailable: %v, %v", v, err)
		}
		if v, err := c.RhythmMode(ctx); err != nil || v != r.Mode {
			t.Errorf("RhythmMode: %v, %v", v, err)
		}
		if v, err := c.RhythmPosition(ctx); err != nil || v != r.Position {
			t.Errorf("RhythmPosition: %+v, %v", v, err)
		}
	}
}

// The event stream opens and stays open; nothing is changed to make an event.
func TestLiveEventsOpen(t *testing.T) { //nolint:paralleltest // one at a time: a real controller is a small thing
	c := liveClient(t)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := c.Events(ctx, nil, func(e aurora.Event) { t.Logf("event: %+v", e) }); err != nil {
		t.Errorf("Events: %v", err)
	}
}

// A token the controller does not know is refused, and so is a token request
// when nobody has held the button.
func TestLiveRefusals(t *testing.T) { //nolint:paralleltest // one at a time: a real controller is a small thing
	c := liveClient(t)

	if _, err := c.WithToken("00000000000000000000000000000000").Info(t.Context()); !aurora.IsUnauthorized(err) {
		t.Errorf("an unknown token: %v", err)
	}
	if token, err := c.WithToken("").NewToken(t.Context()); err == nil {
		t.Errorf("the controller handed out a token (%d characters) with nobody at the button: delete it", len(token))
	}
}
