package aurora_test

import (
	"strings"
	"testing"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// What the app sends that the documentation does not list, against the canned
// controller's idea of the answers: a real one's are still to be recorded.

func TestFirmwareUpgrade(t *testing.T) {
	t.Parallel()

	ctl, c := paired(t)
	ctx := t.Context()

	// a controller that has not heard from the cloud answers {}
	fw, err := c.FirmwareUpgrade(ctx)
	if err != nil || fw.Available || fw.NewVersion != "" || string(fw.Raw) != "{}" {
		t.Fatalf("before the cloud: %+v, %v", fw, err)
	}
	wantRequest(t, ctl, "GET", "/firmwareUpgrade", "")

	ctl.SetFirmwareUpgrade(true, "5.3.3")
	if fw, err = c.FirmwareUpgrade(ctx); err != nil || !fw.Available || fw.NewVersion != "5.3.3" {
		t.Fatalf("with one waiting: %+v, %v", fw, err)
	}

	// the trigger goes as the app sends it: bare, not wrapped in write
	if err := c.TriggerFirmwareUpgrade(ctx); err != nil {
		t.Fatal(err)
	}
	wantRequest(t, ctl, "PUT", "/firmwareUpgrade", `{"command":"triggerFirmwareUpgrade"}`)
	if ctl.FirmwareTriggers() != 1 {
		t.Errorf("triggers: %d", ctl.FirmwareTriggers())
	}
}

func TestAppSettings(t *testing.T) {
	t.Parallel()

	ctl, c := paired(t)
	ctx := t.Context()

	for _, tc := range []struct {
		name string
		do   func() error
		body string
		set  func() bool
		want bool
	}{
		{"buttons off", func() error { return c.SetControllerButtons(ctx, false) }, `{"write":{"command":"disableAllControllerButtons"}}`, ctl.ButtonsEnabled, false},
		{"buttons on", func() error { return c.SetControllerButtons(ctx, true) }, `{"write":{"command":"enableAllControllerButtons"}}`, ctl.ButtonsEnabled, true},
		{"fade off", func() error { return c.SetSceneTransition(ctx, false) }, `{"write":{"command":"disableSceneChangeAnimation"}}`, ctl.SceneTransition, false},
		{"fade on", func() error { return c.SetSceneTransition(ctx, true) }, `{"write":{"command":"enableSceneChangeAnimation"}}`, ctl.SceneTransition, true},
		{"recovery off", func() error { return c.SetPowerLossRecovery(ctx, false) }, `{"write":{"command":"setPLRConfig","PLRConfig":false}}`, ctl.PowerLossRecovery, false},
		{"recovery on", func() error { return c.SetPowerLossRecovery(ctx, true) }, `{"write":{"command":"setPLRConfig","PLRConfig":true}}`, ctl.PowerLossRecovery, true},
	} {
		if err := tc.do(); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		wantRequest(t, ctl, "PUT", "/effects", tc.body)
		if tc.set() != tc.want {
			t.Errorf("%s: the controller has %v", tc.name, tc.set())
		}
	}

	// the layout editor's questions come back as they are, whatever their shape
	ids, err := c.ShortIDMap(ctx)
	if err != nil || !strings.Contains(string(ids), `"shortIdMap"`) {
		t.Errorf("short id map: %s, %v", ids, err)
	}
	adj, err := c.AdjacencyData(ctx)
	if err != nil || !strings.Contains(string(adj), `"adjacencyData"`) {
		t.Errorf("adjacency: %s, %v", adj, err)
	}
	if len(ctl.Writes()) != 6 {
		t.Errorf("asking counted as writing: %d writes", len(ctl.Writes()))
	}

	// touch and the light sensor are for other models: a Light Panels controller has neither
	if _, err := c.TouchConfig(ctx); !aurora.IsNotFound(err) {
		t.Errorf("touch on Light Panels: %v", err)
	}
	if err := c.SetBrightnessSensorConfig(ctx, []byte(`{"enabled":true}`)); !aurora.IsNotFound(err) {
		t.Errorf("a light sensor on Light Panels: %v", err)
	}
}

func TestStaticEffect(t *testing.T) {
	t.Parallel()

	colours := []aurora.StaticColor{{PanelID: 82, R: 255, G: 0, B: 255}, {PanelID: 60, R: 0, G: 255, B: 255}, {PanelID: 118}}
	// the documentation's own example, laid out as it says
	e, err := aurora.StaticEffect("", colours, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := e.Field("animData")
	if want := `"3 82 1 255 0 255 0 20 60 1 0 255 255 0 20 118 1 0 0 0 0 20"`; string(raw) != want {
		t.Errorf("animData %s, want %s", raw, want)
	}
	if e.Name() != "" || e.Type() != "static" {
		t.Errorf("unnamed: %q %q", e.Name(), e.Type())
	}
	named, _ := aurora.StaticEffect("Dots", colours, 0)
	if named.Name() != "Dots" {
		t.Errorf("named: %q", named.Name())
	}
	if _, err := aurora.StaticEffect("", nil, 0); err == nil {
		t.Error("no panels is no effect")
	}

	// shown, the controller reports the static placeholder as running
	ctl, c := paired(t)
	if err := c.DisplayEffect(t.Context(), e); err != nil {
		t.Fatal(err)
	}
	if ctl.Selected() != aurora.EffectStatic {
		t.Errorf("running %q", ctl.Selected())
	}
}
