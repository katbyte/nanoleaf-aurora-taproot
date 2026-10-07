package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/katbyte/go-kt/cout"

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

// Triggered is what --json says of a trigger.
type Triggered struct {
	Controller string `json:"controller"`
	Sent       bool   `json:"sent"`
	Error      string `json:"error,omitempty"`
}

// TriggerFirmware tells each controller to fetch and install its firmware
// from Nanoleaf's cloud, as the app's update button does. It is sent
// whether or not the controller says an update is waiting: what it does
// then is one of the things not yet known.
func (f *FlagData) TriggerFirmware(ctx context.Context, ref string) error {
	controllers, err := f.targets(ref)
	if err != nil {
		return err
	}

	out := make([]Triggered, len(controllers))
	var failed []error
	for i, ctl := range controllers {
		out[i] = Triggered{Controller: ctl.Name}
		c, err := f.Client(ctl)
		if err == nil {
			err = c.TriggerFirmwareUpgrade(ctx)
		}
		if err != nil {
			out[i].Error = err.Error()
			failed = append(failed, fmt.Errorf("%s: %w", ctl.Name, err))
			cout.Errorf("<red>%s:</> %s\n", Escape(ctl.Name), Escape(err.Error()))
			continue
		}
		out[i].Sent = true
		if !f.DryRun {
			cout.Printf("%s: <green>told it to fetch and install its firmware</>; it fetches from Nanoleaf's cloud itself, so watch <cyan>taproot firmware %s</> and <cyan>taproot list</>\n",
				Name(ctl.Name), Escape(ctl.Name))
		}
	}
	if _, err := f.Emit(out); err != nil {
		return err
	}

	return errors.Join(failed...)
}
