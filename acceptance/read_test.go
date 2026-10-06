package acceptance

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/katbyte/nanoleaf-aurora-taproot/cli"
	"github.com/katbyte/nanoleaf-aurora-taproot/cli/scene"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/backup"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// Everything here only reads a controller. The proxy in front of it refuses
// anything else, and a refusal fails the test.

func TestList(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.reader("panels")

	var listed []cli.Status
	decode(t, r.ok("list", "--json"), &listed)
	if len(listed) != 1 {
		t.Fatalf("listed %d controllers", len(listed))
	}
	c := listed[0]
	if c.Name != "panels" || !c.Reachable || c.Model == "" || c.Firmware == "" || c.Panels == 0 || c.Scenes == 0 || c.Running == "" {
		t.Errorf("the controller: %+v", c)
	}

	want(t, r.ok("list"), "NAME", "panels", c.Model, c.Firmware, c.Running)
}

func TestInfo(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.reader("panels")

	var described struct {
		Name   string      `json:"name"`
		Device aurora.Info `json:"device"`
	}
	decode(t, r.ok("info", "panels", "--json"), &described)
	info := described.Device
	if described.Name != "panels" || info.Name == "" || info.Model == "" || info.FirmwareVersion == "" || info.PanelLayout.Layout.NumPanels != len(info.PanelLayout.Layout.Panels) {
		t.Errorf("info: %+v", described)
	}
	// a recording never holds the unit's own serial number
	if info.SerialNo != "S00000A0000" {
		t.Errorf("the serial number was not blanked: %q", info.SerialNo)
	}

	want(t, r.ok("info", "panels"), "calls itself  "+info.Name, "firmware", info.FirmwareVersion, "running", info.Effects.Select, "scenes")
	want(t, r.fails("info", "somewhere-else"), "no controller matches", "somewhere-else")
}

func TestScenes(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.reader("panels")

	var listed []scene.Listed
	decode(t, r.ok("scene", "list", "panels", "--json"), &listed)
	if len(listed) == 0 {
		t.Fatal("the controller holds no scenes")
	}
	running := 0
	for _, s := range listed {
		if s.Name == "" || s.PluginUUID == "" || (s.Kind != aurora.PluginTypeColor && s.Kind != aurora.PluginTypeRhythm) {
			t.Errorf("a scene: %+v", s)
		}
		if s.Plugin == "unknown plugin" {
			t.Errorf("%s runs a plugin the controller does not list: %s", s.Name, s.PluginUUID)
		}
		if s.Running {
			running++
		}
	}
	if running > 1 {
		t.Errorf("%d scenes are running at once", running)
	}
	want(t, r.ok("scene", "list", "panels"), "holds", listed[0].Name)

	// every scene at once, and each by itself, say the same
	var all struct {
		Animations []aurora.Effect `json:"animations"`
	}
	decode(t, r.ok("scene", "dump", "panels"), &all)
	if len(all.Animations) != len(listed) {
		t.Fatalf("dumped %d scenes, listed %d", len(all.Animations), len(listed))
	}
	for _, s := range listed {
		one, err := aurora.ParseEffect([]byte(r.ok("scene", "dump", "panels", s.Name)))
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		i := slices.IndexFunc(all.Animations, func(e aurora.Effect) bool { return e.Name() == s.Name })
		if i < 0 || !all.Animations[i].Equal(one) {
			t.Errorf("%s dumped by itself differs from the one among all", s.Name)
		}
		if len(one.Palette()) != s.Colours || one.PluginUUID() != s.PluginUUID {
			t.Errorf("%s: the dump and the list disagree", s.Name)
		}
	}

	out := r.fails("scene", "dump", "panels", "taproot test: no such scene")
	want(t, out, "no scene called", "taproot test: no such scene", "it has "+listed[0].Name)

	// with one controller, the side-by-side list is that controller's
	var compared []scene.Compared
	decode(t, r.ok("scene", "list", "--json"), &compared)
	if len(compared) != len(listed) {
		t.Errorf("compared %d scenes, listed %d", len(compared), len(listed))
	}
}

// A backup, read back and offered to the controller it came from, finds
// nothing to do: every scene is there already and the same.
func TestBackupThenRestoreIsANoOp(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.reader("panels")
	dir := filepath.Join(t.TempDir(), "backup")

	want(t, r.ok("backup", "panels", dir), "backed up panels", "scenes in "+dir)
	b, err := backup.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b.Manifest == nil || len(b.Scenes) == 0 || len(b.Scenes) != len(b.Manifest.Scenes) || b.Manifest.Controller.Model == "" {
		t.Fatalf("the backup: %+v", b.Manifest)
	}

	out := r.ok("restore", "panels", dir)
	want(t, out, "nothing to write")
	for _, e := range b.Scenes {
		want(t, out, "unchanged  "+e.Name())
	}
	if strings.Contains(out, "added") || strings.Contains(out, "replaced") || strings.Contains(out, "refused") {
		t.Errorf("a restore onto the controller a backup came from should change nothing:\n%s", out)
	}

	// into the dated directory when none is named
	want(t, r.ok("backup"), "backed up panels", filepath.Join(r.dir, "backups", "panels"))
}

// A dry run of a write says exactly what it would send and sends none of it:
// the proxy would refuse it, and the refusal would fail the test.
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.reader("panels")

	var listed []scene.Listed
	decode(t, r.ok("scene", "list", "panels", "--json"), &listed)
	name := listed[0].Name

	want(t, r.ok("scene", "copy", name, "--from", "panels", "--to", "panels", "--as", "taproot test copy", "--dry-run"), "would first back it up", "dry run:", "PUT /api/v1/<token>/effects", `{"write":{"command":"add",`, `"animName":"taproot test copy"`, "would be added")

	want(t, r.ok("scene", "select", "panels", name, "--dry-run"), "PUT /api/v1/<token>/effects")
	want(t, r.ok("forget", "panels", "--dry-run"), "DELETE /api/v1/<token>", "would remove")

	// and a write that is not a dry run is stopped by taproot before it is sent, when the scene is already there
	want(t, r.ok("scene", "copy", name, "--from", "panels", "--to", "panels", "--as", name), "nothing to write", "unchanged")
}

// The client itself, through the proxy: every read the API has.
func TestSDKReads(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.reader("panels")
	c := r.client("panels")
	ctx := t.Context()

	info, err := c.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	state, err := c.State(ctx)
	if err != nil || state != info.State {
		t.Errorf("State: %+v, %v", state, err)
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
	if m, err := c.ColorMode(ctx); err != nil || m != state.ColorMode {
		t.Errorf("ColorMode: %q, %v", m, err)
	}
	if l, err := c.Layout(ctx); err != nil || l.NumPanels != info.PanelLayout.Layout.NumPanels {
		t.Errorf("Layout: %+v, %v", l, err)
	}
	if names, err := c.EffectNames(ctx); err != nil || !slices.Equal(names, info.Effects.List) {
		t.Errorf("EffectNames: %v, %v", names, err)
	}
	if sel, err := c.SelectedEffect(ctx); err != nil || sel != info.Effects.Select {
		t.Errorf("SelectedEffect: %q, %v", sel, err)
	}
	plugins, err := c.Plugins(ctx)
	if err != nil || len(plugins) == 0 {
		t.Fatalf("Plugins: %d, %v", len(plugins), err)
	}
	all, err := c.Effects(ctx)
	if err != nil || len(all) != len(info.Effects.List) {
		t.Fatalf("Effects: %d, %v", len(all), err)
	}
	for _, e := range all {
		if !slices.ContainsFunc(plugins, func(p aurora.Plugin) bool { return p.UUID == e.PluginUUID() }) {
			t.Errorf("%q runs a plugin the controller does not list", e.Name())
		}
		// the document survives a trip through this program byte for byte
		raw, _ := json.Marshal(e)
		again, err := aurora.ParseEffect(raw)
		if err != nil || !again.Equal(e) {
			t.Errorf("%q does not survive being written and read: %v", e.Name(), err)
		}
	}
	if _, err := c.Effect(ctx, "taproot test: no such scene"); !aurora.IsNotFound(err) {
		t.Errorf("an effect that is not there: %v", err)
	}

	if info.Rhythm != nil {
		rh, err := c.Rhythm(ctx)
		if err != nil || rh != *info.Rhythm {
			t.Errorf("Rhythm: %+v, %v", rh, err)
		}
		if v, err := c.RhythmConnected(ctx); err != nil || v != rh.Connected {
			t.Errorf("RhythmConnected: %v, %v", v, err)
		}
		if v, err := c.RhythmActive(ctx); err != nil || v != rh.Active {
			t.Errorf("RhythmActive: %v, %v", v, err)
		}
		if v, err := c.RhythmID(ctx); err != nil || v != rh.ID {
			t.Errorf("RhythmID: %v, %v", v, err)
		}
		if v, err := c.RhythmHardwareVersion(ctx); err != nil || v != rh.HardwareVersion {
			t.Errorf("RhythmHardwareVersion: %v, %v", v, err)
		}
		if v, err := c.RhythmFirmwareVersion(ctx); err != nil || v != rh.FirmwareVersion {
			t.Errorf("RhythmFirmwareVersion: %v, %v", v, err)
		}
		if v, err := c.RhythmAuxAvailable(ctx); err != nil || v != rh.AuxAvailable {
			t.Errorf("RhythmAuxAvailable: %v, %v", v, err)
		}
		if v, err := c.RhythmMode(ctx); err != nil || v != rh.Mode {
			t.Errorf("RhythmMode: %v, %v", v, err)
		}
		if v, err := c.RhythmPosition(ctx); err != nil || v != rh.Position {
			t.Errorf("RhythmPosition: %+v, %v", v, err)
		}
	}
}
