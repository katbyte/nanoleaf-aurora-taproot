package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/katbyte/go-kt/cout"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// allControllers is what to name in place of a controller to mean every one.
const allControllers = "all"

// The values that are words.
const (
	wordOn         = "on"
	wordOff        = "off"
	wordMicrophone = "microphone"
	wordAux        = "aux"
	wordNone       = "none"
)

// setting is one thing about a controller that taproot get reads and taproot
// set changes: everything the API lets be set, by a name a person would use.
type setting struct {
	name    string
	aliases []string
	// takes says what it can be set to, for the help and for errors.
	takes string
	// read is its value now, out of everything a controller says in one answer.
	read func(info aurora.Info) any
	// write sets it to what was typed. nil for what is only ever read.
	write func(ctx context.Context, c *aurora.Client, info aurora.Info, value string, fade time.Duration) error
	// adjust moves it by an amount. nil for what is not a number.
	adjust func(ctx context.Context, c *aurora.Client, by int) error
}

// settings is every setting, in the order they are listed.
func settings() []setting {
	return []setting{
		{
			name: "power", takes: "on, off or toggle",
			read: func(info aurora.Info) any { return onOff(info.State.On.Value) },
			write: func(ctx context.Context, c *aurora.Client, info aurora.Info, value string, _ time.Duration) error {
				switch strings.ToLower(value) {
				case wordOn, "true", "1", "yes":
					return c.SetOn(ctx, true)
				case wordOff, "false", "0", "no":
					return c.SetOn(ctx, false)
				case "toggle":
					return c.SetOn(ctx, !info.State.On.Value)
				}
				return fmt.Errorf("power is on, off or toggle, not %q", value)
			},
		},
		{
			name: "brightness", takes: "0 to 100; --by dims or brightens, --fade takes time over it",
			read: func(info aurora.Info) any { return info.State.Brightness.Value },
			write: func(ctx context.Context, c *aurora.Client, info aurora.Info, value string, fade time.Duration) error {
				n, err := within("brightness", value, info.State.Brightness)
				if err != nil {
					return err
				}
				return c.SetBrightness(ctx, n, fade)
			},
			adjust: func(ctx context.Context, c *aurora.Client, by int) error { return c.AdjustBrightness(ctx, by) },
		},
		{
			name: "scene", takes: "the name of a scene the controller holds",
			read: func(info aurora.Info) any { return info.Effects.Select },
			write: func(ctx context.Context, c *aurora.Client, info aurora.Info, value string, _ time.Duration) error {
				name, err := MatchScene(info.Effects.List, value)
				if err != nil {
					return err
				}
				if err := c.SelectEffect(ctx, name); err != nil {
					return NoScene(ctx, c, name, err)
				}
				return nil
			},
		},
		{
			name: "hue", takes: "0 to 360: turns every panel one colour, in place of the scene",
			read: func(info aurora.Info) any { return info.State.Hue.Value },
			write: func(ctx context.Context, c *aurora.Client, info aurora.Info, value string, _ time.Duration) error {
				n, err := within("hue", value, info.State.Hue)
				if err != nil {
					return err
				}
				return c.SetHue(ctx, n)
			},
			adjust: func(ctx context.Context, c *aurora.Client, by int) error { return c.AdjustHue(ctx, by) },
		},
		{
			name: "saturation", aliases: []string{"sat"}, takes: "0 to 100: how strong that one colour is",
			read: func(info aurora.Info) any { return info.State.Sat.Value },
			write: func(ctx context.Context, c *aurora.Client, info aurora.Info, value string, _ time.Duration) error {
				n, err := within("saturation", value, info.State.Sat)
				if err != nil {
					return err
				}
				return c.SetSaturation(ctx, n)
			},
			adjust: func(ctx context.Context, c *aurora.Client, by int) error { return c.AdjustSaturation(ctx, by) },
		},
		{
			name: "temperature", aliases: []string{"ct", "kelvin"}, takes: "1200 to 6500 kelvin: turns every panel one white, in place of the scene",
			read: func(info aurora.Info) any { return info.State.CT.Value },
			write: func(ctx context.Context, c *aurora.Client, info aurora.Info, value string, _ time.Duration) error {
				n, err := within("temperature", value, info.State.CT)
				if err != nil {
					return err
				}
				return c.SetColorTemperature(ctx, n)
			},
			adjust: func(ctx context.Context, c *aurora.Client, by int) error { return c.AdjustColorTemperature(ctx, by) },
		},
		{
			name: "orientation", takes: "0 to 360 degrees: how the layout is turned to match the wall, which a scene's direction follows",
			read: func(info aurora.Info) any { return info.PanelLayout.GlobalOrientation.Value },
			write: func(ctx context.Context, c *aurora.Client, info aurora.Info, value string, _ time.Duration) error {
				n, err := within("orientation", value, info.PanelLayout.GlobalOrientation)
				if err != nil {
					return err
				}
				return c.SetGlobalOrientation(ctx, n)
			},
		},
		{
			name: "rhythm", aliases: []string{"source"}, takes: "microphone or aux: what the Rhythm module listens to",
			read: func(info aurora.Info) any {
				switch {
				case info.Rhythm == nil || !info.Rhythm.Connected:
					return wordNone
				case info.Rhythm.Mode == aurora.RhythmModeAux:
					return wordAux
				default:
					return wordMicrophone
				}
			},
			write: func(ctx context.Context, c *aurora.Client, info aurora.Info, value string, _ time.Duration) error {
				if info.Rhythm == nil || !info.Rhythm.Connected {
					return errors.New("no Rhythm module is plugged into this controller")
				}
				switch strings.ToLower(value) {
				case wordMicrophone, "mic":
					return c.SetRhythmMode(ctx, aurora.RhythmModeMicrophone)
				case wordAux:
					return c.SetRhythmMode(ctx, aurora.RhythmModeAux)
				}
				return fmt.Errorf("rhythm is microphone or aux, not %q", value)
			},
		},
		{
			// not set itself: it says which of the others the panels are showing
			name: "mode", takes: "read only: effect for a scene, hs for one colour, ct for one white",
			read: func(info aurora.Info) any { return info.State.ColorMode },
		},
	}
}

func onOff(on bool) string {
	if on {
		return wordOn
	}
	return wordOff
}

// within reads a whole number that has to be inside the limits the controller
// itself gives for it, and says what they are when it is not.
func within(name, value string, limits aurora.Range) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("%s is a whole number from %d to %d, not %q", name, limits.Min, limits.Max, value)
	}
	if n < limits.Min || n > limits.Max {
		return 0, fmt.Errorf("%s is %d to %d on this controller, not %d", name, limits.Min, limits.Max, n)
	}

	return n, nil
}

// findSetting is the setting a person means by a name.
func findSetting(name string) (setting, error) {
	all := settings()
	want := strings.ToLower(name)
	for _, s := range all {
		if s.name == want || slices.Contains(s.aliases, want) {
			return s, nil
		}
	}
	names := make([]string, len(all))
	for i, s := range all {
		names[i] = s.name
	}

	return setting{}, fmt.Errorf("no setting called %q: there is %s", name, strings.Join(names, ", "))
}

// SettingsHelp lists the settings and what each takes, for the help of get
// and set.
func SettingsHelp() string {
	var b strings.Builder
	for _, s := range settings() {
		fmt.Fprintf(&b, "  %-12s %s\n", s.name, s.takes)
	}

	return strings.TrimRight(b.String(), "\n")
}

// MatchScene is the scene a person means by what they typed, spelt as the
// controller spells it. A controller goes by the exact name, capitals and
// all, and a person should not have to: so a name that matches exactly is
// that scene, and failing that, the one scene it matches when capitals are
// ignored. A controller may hold two scenes whose names differ only in their
// capitals; typed in a way that is neither of them exactly, that is not
// guessed at, and both are named.
func MatchScene(held []string, typed string) (string, error) {
	typed = strings.TrimSpace(typed)
	if slices.Contains(held, typed) {
		return typed, nil
	}

	var near []string
	for _, name := range held {
		if strings.EqualFold(name, typed) {
			near = append(near, name)
		}
	}
	switch len(near) {
	case 1:
		return near[0], nil
	case 0:
		return "", fmt.Errorf("the controller has no scene called %q: it has %s", typed, strings.Join(held, ", "))
	default:
		return "", fmt.Errorf("%q could be %s: type the one you mean exactly", typed, strings.Join(near, " or "))
	}
}

// ResolveScene asks a controller what scenes it holds and returns the one a
// person means, spelt as the controller spells it.
func ResolveScene(ctx context.Context, c *aurora.Client, typed string) (string, error) {
	held, err := c.EffectNames(ctx)
	if err != nil {
		return "", err
	}

	return MatchScene(held, typed)
}

// NoScene turns a controller's bare "not found" for a scene into the name
// that was asked for and the names it could have been.
func NoScene(ctx context.Context, c *aurora.Client, name string, err error) error {
	if !aurora.IsNotFound(err) {
		return err
	}
	names, lerr := c.EffectNames(ctx)
	if lerr != nil {
		return fmt.Errorf("the controller has no scene called %q", name)
	}

	return fmt.Errorf("the controller has no scene called %q: it has %s", name, strings.Join(names, ", "))
}

// targets is the controllers a person means: the one named, or with "all"
// every one, unless one of them is itself called that.
func (f *FlagData) targets(ref string) ([]store.Controller, error) {
	s, err := f.OpenStore()
	if err != nil {
		return nil, err
	}
	if ref == allControllers && !slices.ContainsFunc(s.Controllers, func(c store.Controller) bool { return c.Name == allControllers }) {
		if len(s.Controllers) == 0 {
			return nil, errors.New("no controllers yet: taproot connect <address> gets a token for one")
		}
		return s.Controllers, nil
	}
	ctl, err := s.Find(ref)
	if err != nil {
		return nil, err
	}

	return []store.Controller{ctl}, nil
}

// Settings is one controller's settings as get reports them with --json.
type Settings struct {
	Controller string         `json:"controller"`
	Settings   map[string]any `json:"settings"`
	Error      string         `json:"error,omitempty"`
}

// show is a setting's value, coloured for what it is.
func show(name string, value any) string {
	switch v := value.(type) {
	case int:
		return Num(v)
	case string:
		switch {
		case name == "scene":
			return Scene(v)
		case v == wordOn:
			return "<green>on</>"
		case v == wordOff || v == wordNone:
			return Dim(v)
		}
		return Escape(v)
	}

	return Escape(fmt.Sprint(value))
}

// Get prints what a controller's settings are: all of them, or the one named.
func (f *FlagData) Get(ctx context.Context, ref, name string) error {
	controllers, err := f.targets(ref)
	if err != nil {
		return err
	}
	wanted := settings()
	if name != "" {
		one, err := findSetting(name)
		if err != nil {
			return err
		}
		wanted = []setting{one}
	}

	out := make([]Settings, len(controllers))
	var failed []error
	for i, ctl := range controllers {
		out[i] = Settings{Controller: ctl.Name, Settings: map[string]any{}}
		c, err := f.Client(ctl)
		if err == nil {
			var info aurora.Info
			if info, err = c.Info(ctx); err == nil {
				for _, s := range wanted {
					out[i].Settings[s.name] = s.read(info)
				}
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

	switch {
	// one value of one controller is the value and nothing else: what a script wants
	case len(out) == 1 && len(wanted) == 1 && out[0].Error == "":
		cout.Quietf("%s\n", Escape(fmt.Sprint(out[0].Settings[wanted[0].name])))
	// one controller: a setting a line, with what each takes
	case len(out) == 1 && out[0].Error == "":
		cout.Printf("%s\n", Name(out[0].Controller))
		rows := make([][]string, len(wanted))
		for i, s := range wanted {
			rows[i] = []string{" ", Dim(s.name), show(s.name, out[0].Settings[s.name]), Dim(s.takes)}
		}
		Table(rows)
	// several: a controller a line
	default:
		header := []string{Dim("NAME")}
		for _, s := range wanted {
			header = append(header, Dim(strings.ToUpper(s.name)))
		}
		rows := [][]string{header}
		for _, o := range out {
			row := []string{Name(o.Controller)}
			for _, s := range wanted {
				if o.Error != "" {
					row = append(row, "<red>?</>")
					continue
				}
				row = append(row, show(s.name, o.Settings[s.name]))
			}
			rows = append(rows, row)
		}
		Table(rows)
	}
	for _, err := range failed {
		cout.Errorf("<red>%s</>\n", Escape(err.Error()))
	}

	return errors.Join(failed...)
}

// Changed is one setting of one controller after set, as it reports with
// --json: what it was, and what the controller says it is now.
type Changed struct {
	Controller string `json:"controller"`
	Setting    string `json:"setting"`
	From       any    `json:"from"`
	To         any    `json:"to"`
	Error      string `json:"error,omitempty"`
}

// Set changes one setting of a controller, or of every controller: to a
// value, or when adjusting, by an amount. It reads the setting before and
// after, so what it reports is what the controller says, not what was asked.
func (f *FlagData) Set(ctx context.Context, ref, name, value string, adjusting bool) error {
	s, err := findSetting(name)
	if err != nil {
		return err
	}
	switch {
	case s.write == nil:
		return fmt.Errorf("%s is not set itself: it says what the panels are showing, and follows what is", s.name)
	case adjusting && value != "":
		return errors.New("give a value to set it to, or --by to move it: not both")
	case adjusting && s.adjust == nil:
		return fmt.Errorf("%s is not a number to move with --by: it is %s", s.name, s.takes)
	case !adjusting && value == "":
		return fmt.Errorf("set %s to what? it is %s", s.name, s.takes)
	case f.Cmd.Fade != 0 && s.name != "brightness":
		return errors.New("--fade is for brightness: nothing else changes over time")
	}
	controllers, err := f.targets(ref)
	if err != nil {
		return err
	}

	out := make([]Changed, len(controllers))
	var failed []error
	for i, ctl := range controllers {
		out[i] = Changed{Controller: ctl.Name, Setting: s.name}
		if err := f.change(ctx, ctl, s, value, adjusting, &out[i]); err != nil {
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

// change sets one setting of one controller and fills in what it was and is.
func (f *FlagData) change(ctx context.Context, ctl store.Controller, s setting, value string, adjusting bool, out *Changed) error {
	c, err := f.Client(ctl)
	if err != nil {
		return err
	}
	before, err := c.Info(ctx)
	if err != nil {
		return err
	}
	out.From, out.To = s.read(before), s.read(before)

	if adjusting {
		err = s.adjust(ctx, c, f.Cmd.By)
	} else {
		err = s.write(ctx, c, before, value, f.Cmd.Fade)
	}
	if err != nil || f.DryRun {
		return err
	}

	after, err := c.Info(ctx)
	if err != nil {
		return fmt.Errorf("it was sent, but the controller could not be read afterwards: %w", err)
	}
	out.To = s.read(after)
	if out.From == out.To {
		cout.Printf("%s: %s is %s\n", Name(ctl.Name), s.name, show(s.name, out.To))
		return nil
	}
	cout.Printf("%s: %s %s → %s\n", Name(ctl.Name), s.name, Dim(fmt.Sprint(out.From)), show(s.name, out.To))

	return nil
}
