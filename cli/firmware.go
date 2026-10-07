package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/katbyte/go-kt/cout"

	"github.com/katbyte/nanoleaf-aurora-taproot/lib/backup"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// FirmwareStatus is one controller's answer about a firmware update, as the
// --json document has it.
type FirmwareStatus struct {
	Controller string          `json:"controller"`
	Firmware   string          `json:"firmwareVersion,omitempty"`
	Available  bool            `json:"available"`
	NewVersion string          `json:"newVersion,omitempty"`
	Raw        json.RawMessage `json:"raw,omitempty"` // the answer as the controller sent it
	Error      string          `json:"error,omitempty"`
}

// Firmware asks each controller what it knows of a firmware update. The
// endpoint is not documented, and a controller that has not heard from
// Nanoleaf's cloud answers with nothing, so the answer is shown as it came.
func (f *FlagData) Firmware(ctx context.Context, ref string) error {
	controllers, err := f.targets(ref)
	if err != nil {
		return err
	}

	out := make([]FirmwareStatus, len(controllers))
	var failed []error
	for i, ctl := range controllers {
		out[i] = FirmwareStatus{Controller: ctl.Name, Firmware: ctl.Firmware}
		c, err := f.Client(ctl)
		if err == nil {
			var fw aurora.FirmwareUpgrade
			if fw, err = c.FirmwareUpgrade(ctx); err == nil {
				out[i].Available, out[i].NewVersion, out[i].Raw = fw.Available, fw.NewVersion, fw.Raw
			}
		}
		if err != nil {
			out[i].Error = err.Error()
			failed = append(failed, fmt.Errorf("%s: %w", ctl.Name, err))
		}
	}
	if done, err := f.Emit(out); done {
		return errors.Join(append(failed, err)...)
	}

	for _, st := range out {
		switch {
		case st.Error != "":
			cout.Printf("%s: <red>%s</>\n", Name(st.Controller), Escape(st.Error))
		case st.Available:
			// said so that it cannot be read as having happened: this command only asks
			cout.Printf("%s: on %s, <green>update to %s available</>, not installed. To have the controller fetch and install it: <cyan>taproot firmware trigger %s</>\n",
				Name(st.Controller), Dim(orDash(st.Firmware)), Escape(st.NewVersion), Escape(st.Controller))
		case len(st.Raw) <= 2: // {} or nothing: it has not been told of one
			cout.Printf("%s: on %s, the controller has not heard of an update %s\n",
				Name(st.Controller), Dim(orDash(st.Firmware)), Dim("(it answers {}: it has not heard from Nanoleaf's cloud)"))
		default:
			cout.Printf("%s: on %s, no update waiting %s\n", Name(st.Controller), Dim(orDash(st.Firmware)), Dim(Escape(string(st.Raw))))
		}
	}

	return errors.Join(failed...)
}

// firmwarePoll is how often a controller is asked whether it is back, while
// an upgrade is watched.
var firmwarePoll = 4 * time.Second

// the phases of a watched upgrade
const (
	phaseAsked = "asked"
	phaseQuiet = "quiet"
)

// Triggered is what --json says of a trigger, and of the watch afterwards.
type Triggered struct {
	Controller string `json:"controller"`
	Sent       bool   `json:"sent"`
	Backup     string `json:"backup,omitempty"`
	Error      string `json:"error,omitempty"`

	// what the watch saw, when there was one
	Was        string   `json:"was,omitempty"`        // the firmware before
	Now        string   `json:"now,omitempty"`        // the firmware when it came back
	Quiet      string   `json:"quiet,omitempty"`      // how long it did not answer
	Took       string   `json:"took,omitempty"`       // from the trigger to it answering again
	Scenes     int      `json:"scenes,omitempty"`     // how many it holds afterwards
	Lost       []string `json:"lost,omitempty"`       // scenes it held before and not after
	Running    string   `json:"running,omitempty"`    // what it is running afterwards
	NotBack    bool     `json:"notBack,omitempty"`    // the watch gave up before it answered again
	Unchanged  bool     `json:"unchanged,omitempty"`  // it came back on the same firmware
	WatchError string   `json:"watchError,omitempty"` // the watch itself failed
}

// TriggerFirmware tells each controller to fetch and install its firmware
// from Nanoleaf's cloud, as the app's update button does. It is the biggest
// write there is, so the controller is backed up first; and it is sent
// whether or not the controller says an update is waiting. Then, for as long
// as --wait allows, the controller is watched: it goes quiet while it
// installs, and when it answers again what it holds is compared with what
// it held, since an upgrade has been seen to drop a scene.
func (f *FlagData) TriggerFirmware(ctx context.Context, ref string) error {
	controllers, err := f.targets(ref)
	if err != nil {
		return err
	}

	out := make([]Triggered, len(controllers))
	var failed []error
	for i, ctl := range controllers {
		out[i] = Triggered{Controller: ctl.Name}
		if err := f.triggerOne(ctx, ctl, &out[i]); err != nil {
			out[i].Error = err.Error()
			failed = append(failed, fmt.Errorf("%s: %w", ctl.Name, err))
			cout.Errorf("<red>%s:</> %s\n", Escape(ctl.Name), Escape(err.Error()))
		}
	}
	if _, err := f.Emit(out); err != nil {
		return err
	}

	return errors.Join(failed...)
}

func (f *FlagData) triggerOne(ctx context.Context, ctl store.Controller, t *Triggered) error {
	c, err := f.Client(ctl)
	if err != nil {
		return err
	}
	before, err := c.Info(ctx)
	if err != nil {
		return fmt.Errorf("reading it before anything is done: %w", err)
	}
	t.Was = before.FirmwareVersion

	t.Backup = backup.Dir(f.BackupRoot(), ctl.Name, time.Now())
	if !f.DryRun {
		if _, err := backup.Take(ctx, c, t.Backup); err != nil {
			t.Backup = ""
			return fmt.Errorf("backing it up before its firmware is replaced: %w (nothing was triggered)", err)
		}
		cout.Printf("%s: <green>backed up</> to %s\n", Name(ctl.Name), Dim(t.Backup))
	}
	if err := c.TriggerFirmwareUpgrade(ctx); err != nil {
		return fmt.Errorf("it did not take the trigger: %w", err)
	}
	t.Sent = true
	if f.DryRun {
		return nil
	}
	cout.Printf("%s: <green>told to fetch and install its firmware</> %s\n", Name(ctl.Name), Dim("(it fetches from Nanoleaf's cloud itself; nothing passes through here)"))
	if f.Cmd.Wait <= 0 {
		cout.Printf("  not watching: <cyan>taproot firmware %s</> and <cyan>taproot list</> say how it is getting on\n", Escape(ctl.Name))
		return nil
	}

	return f.watchUpgrade(ctx, ctl, c, before, t)
}

// watchUpgrade follows a controller through an upgrade: asked, gone quiet, back.
func (f *FlagData) watchUpgrade(ctx context.Context, ctl store.Controller, c *aurora.Client, before aurora.Info, t *Triggered) error {
	started := time.Now()
	deadline := started.Add(f.Cmd.Wait)
	cout.Printf("  watching for up to %s %s\n", f.Cmd.Wait, Dim("(ctrl-c stops watching, not the upgrade)"))

	var quietSince time.Time
	var after aurora.Info
	phase := phaseAsked
	say := func(line string) { cout.Printf("\r  %-70s\n", line) }
	for {
		if time.Now().After(deadline) {
			t.NotBack = true
			say(fmt.Sprintf("<yellow>not back after %s</>: leave it be a while, then <cyan>taproot list</>", f.Cmd.Wait.Round(time.Second)))
			return nil
		}
		select {
		case <-ctx.Done():
			t.WatchError = ctx.Err().Error()
			return nil
		case <-time.After(firmwarePoll):
		}

		read, cancel := context.WithTimeout(ctx, firmwarePoll)
		info, err := c.Info(read)
		cancel()
		switch {
		case err != nil && phase != phaseQuiet:
			phase = phaseQuiet
			quietSince = time.Now()
			say(fmt.Sprintf("gone quiet after %s: <yellow>installing</>", time.Since(started).Round(time.Second)))
		case err != nil:
			cout.Printf("\r  installing… quiet for %s", Dim(time.Since(quietSince).Round(time.Second).String()))
		case phase == phaseAsked && info.FirmwareVersion == before.FirmwareVersion && time.Since(started) < f.Cmd.Wait:
			// still answering on the old firmware: it has not started yet, or it had nothing to fetch
			cout.Printf("\r  still answering on %s, %s", Dim(info.FirmwareVersion), Dim(time.Since(started).Round(time.Second).String()))
			if time.Since(started) > 90*time.Second {
				t.Unchanged, t.Now = true, info.FirmwareVersion
				say(fmt.Sprintf("<yellow>still on %s after %s</>: it did not start an upgrade, which is what one with nothing to fetch does", info.FirmwareVersion, time.Since(started).Round(time.Second)))
				return nil
			}
		default:
			after = info
			t.Now, t.Took = info.FirmwareVersion, time.Since(started).Round(time.Second).String()
			if !quietSince.IsZero() {
				t.Quiet = time.Since(quietSince).Round(time.Second).String()
			}
			return cameBack(ctl, before, after, t)
		}
	}
}

// cameBack says what the controller is like now, against what it was.
func cameBack(ctl store.Controller, before, after aurora.Info, t *Triggered) error {
	t.Scenes, t.Running = len(after.Effects.List), after.Effects.Select
	for _, name := range before.Effects.List {
		if !slices.Contains(after.Effects.List, name) {
			t.Lost = append(t.Lost, name)
		}
	}
	t.Unchanged = after.FirmwareVersion == before.FirmwareVersion

	cout.Printf("\r%-72s\n", "")
	switch {
	case t.Unchanged:
		cout.Printf("%s: back after %s, <yellow>still on %s</>\n", Name(ctl.Name), t.Took, Escape(after.FirmwareVersion))
	default:
		cout.Printf("%s: <green>back on %s</> after %s %s\n", Name(ctl.Name), Escape(after.FirmwareVersion), t.Took, Dim("(was "+before.FirmwareVersion+", quiet for "+t.Quiet+")"))
	}
	cout.Printf("  holds %s scenes %s, running %s\n", Num(t.Scenes), Dim(fmt.Sprintf("(had %d)", len(before.Effects.List))), Scene(orDash(t.Running)))
	if len(t.Lost) > 0 {
		names := make([]string, len(t.Lost))
		for i, n := range t.Lost {
			names[i] = Scene(n)
		}
		cout.Printf("  <yellow>gone since the upgrade:</> %s — in the backup taken first: <cyan>taproot restore %s %s</> puts them back\n",
			strings.Join(names, ", "), Escape(ctl.Name), Dim(t.Backup))
	}
	if after.Effects.Select != before.Effects.Select {
		cout.Printf("  it was running %s before\n", Scene(orDash(before.Effects.Select)))
	}

	return nil
}
