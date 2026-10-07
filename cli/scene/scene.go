package scene

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/katbyte/go-kt/cout"
	"github.com/katbyte/nanoleaf-aurora-taproot/cli"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/push"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// Listed is one scene of a controller as the list command shows it.
type Listed struct {
	Name       string `json:"name"`
	Running    bool   `json:"running"`
	Kind       string `json:"kind"` // color or rhythm
	Plugin     string `json:"plugin"`
	PluginUUID string `json:"pluginUuid"`
	Colours    int    `json:"colours"`
}

func (f *Flags) List(ctx context.Context, ref string) error {
	ctl, c, err := f.Controller(ref)
	if err != nil {
		return err
	}
	effects, err := c.Effects(ctx)
	if err != nil {
		return err
	}
	running, err := c.SelectedEffect(ctx)
	if err != nil {
		return err
	}
	plugins, err := c.Plugins(ctx)
	if err != nil {
		return err
	}

	out := make([]Listed, len(effects))
	for i, e := range effects {
		out[i] = Listed{
			Name: e.Name(), Running: e.Name() == running, Kind: e.PluginType(), PluginUUID: e.PluginUUID(),
			Plugin: pluginName(plugins, e.PluginUUID()), Colours: len(e.Palette()),
		}
	}
	if done, err := f.Emit(out); done {
		return err
	}

	cout.Printf("%s holds %s scenes\n", cli.Name(ctl.Name), cli.Num(len(out)))
	rows := make([][]string, 0, len(out))
	for i, s := range out {
		mark, name := " ", cli.Scene(s.Name)
		if s.Running {
			mark, name = "<green>▶</>", "<magenta;op=bold>"+escape(s.Name)+"</>"
		}
		// the ones that move to sound stand out from the ones that move by themselves
		kind := cli.Dim(s.Kind)
		if s.Kind == aurora.PluginTypeRhythm {
			kind = "<fg=177>" + s.Kind + "</>"
		}
		// and each scene's own colours, where colours are shown at all
		rows = append(rows, []string{mark, name, kind, escape(s.Plugin), cli.Num(s.Colours) + " " + cli.Dim("colours"), cli.Swatch(effects[i].Palette())})
	}
	cli.Table(rows)

	return nil
}

func pluginName(plugins []aurora.Plugin, uuid string) string {
	if i := slices.IndexFunc(plugins, func(p aurora.Plugin) bool { return p.UUID == uuid }); i >= 0 {
		return plugins[i].Name
	}
	return "unknown plugin"
}

// Compared is one scene across the controllers: who holds it, and whether
// they all hold the same thing.
type Compared struct {
	Name string `json:"name"`
	// On maps each controller that holds the scene to true.
	On map[string]bool `json:"on"`
	// Differs is set when two controllers hold different scenes under the name.
	Differs bool `json:"differs,omitempty"`
}

// Compare lists every scene on every controller side by side.
func (f *Flags) Compare(ctx context.Context) error {
	s, err := f.OpenStore()
	if err != nil {
		return err
	}
	if len(s.Controllers) == 0 {
		return errors.New("no controllers yet: taproot connect <address> gets a token for one")
	}

	held := make([][]aurora.Effect, len(s.Controllers))
	errs := make([]error, len(s.Controllers))
	var wg sync.WaitGroup
	for i, ctl := range s.Controllers {
		wg.Go(func() {
			c, err := f.Client(ctl)
			if err == nil {
				held[i], err = c.Effects(ctx)
			}
			errs[i] = err
		})
	}
	wg.Wait()

	byName := map[string]*Compared{}
	first := map[string]aurora.Effect{}
	var names []string
	for i, ctl := range s.Controllers {
		for _, e := range held[i] {
			c, ok := byName[e.Name()]
			if !ok {
				c = &Compared{Name: e.Name(), On: map[string]bool{}}
				byName[e.Name()], first[e.Name()] = c, e
				names = append(names, e.Name())
			}
			c.On[ctl.Name] = true
			// the same scene on firmware that adds a field of its own is still the same scene
			c.Differs = c.Differs || !first[e.Name()].SameScene(e)
		}
	}
	slices.Sort(names)
	out := make([]Compared, len(names))
	for i, n := range names {
		out[i] = *byName[n]
	}

	if done, err := f.Emit(out); done {
		return err
	}

	header := []string{cli.Dim("SCENE")}
	for _, ctl := range s.Controllers {
		header = append(header, cli.Name(ctl.Name))
	}
	rows := [][]string{header}
	for _, c := range out {
		row := []string{cli.Scene(c.Name)}
		for i, ctl := range s.Controllers {
			switch {
			case errs[i] != nil:
				row = append(row, "<red>?</>")
			case c.On[ctl.Name]:
				row = append(row, "<green>yes</>")
			default:
				// the gap is what this list is for: the scene a controller has lost
				row = append(row, "<red>-</>")
			}
		}
		if c.Differs {
			row = append(row, cli.Note("differs between controllers"))
		}
		rows = append(rows, row)
	}
	cli.Table(rows)
	for i, ctl := range s.Controllers {
		if errs[i] != nil {
			cout.Errorf("<red>%s could not be read:</> %s\n", escape(ctl.Name), escape(errs[i].Error()))
		}
	}

	return nil
}

func (f *Flags) Dump(ctx context.Context, ref, name string) error {
	_, c, err := f.Controller(ref)
	if err != nil {
		return err
	}

	raw, err := document(ctx, c, name)
	if err != nil {
		return err
	}
	// indented for reading; a controller takes it back with any spacing
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return err
	}
	pretty.WriteByte('\n')

	if f.Cmd.Scene.Out == "" {
		_, err = cout.Out.Write(pretty.Bytes())
		return err
	}

	flag := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if f.Cmd.Force {
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	file, err := os.OpenFile(f.Cmd.Scene.Out, flag, 0o644) //nolint:gosec // a scene is no secret, and the path is the one the person gave
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s is already there: --force writes over it", f.Cmd.Scene.Out)
		}
		return err
	}
	if _, err := file.Write(pretty.Bytes()); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	cout.Printf("wrote %s\n", cli.Dim(f.Cmd.Scene.Out))

	return nil
}

// document is what dump writes: one scene as the controller sent it, or all
// of them in the controller's own shape for the lot.
func document(ctx context.Context, c *aurora.Client, name string) ([]byte, error) {
	if name != "" {
		// by the name as the controller spells it, whatever capitals were typed
		spelt, err := cli.ResolveScene(ctx, c, name)
		if err != nil {
			return nil, err
		}
		e, err := c.Effect(ctx, spelt)
		if err != nil {
			return nil, noScene(ctx, c, spelt, err)
		}
		return json.Marshal(e)
	}

	all, err := c.Effects(ctx)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{"animations": all})
}

// noScene turns a controller's bare "not found" into the name that was asked
// for and the names it could have been.
func noScene(ctx context.Context, c *aurora.Client, name string, err error) error {
	return cli.NoScene(ctx, c, name, err)
}

// read reads the scenes of a file taproot scene dump wrote: one scene, the
// controller's {"animations": [...]}, or a plain list of scenes.
func read(path string) ([]aurora.Effect, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path) //nolint:gosec // the file the person named
	}
	if err != nil {
		return nil, fmt.Errorf("reading the scene file: %w", err)
	}

	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '[' {
		var list []aurora.Effect
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return named(path, list)
	}

	one, err := aurora.ParseEffect(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if raw, ok := one.Field("animations"); ok {
		var list []aurora.Effect
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return named(path, list)
	}

	return named(path, []aurora.Effect{one})
}

func named(path string, list []aurora.Effect) ([]aurora.Effect, error) {
	if len(list) == 0 {
		return nil, fmt.Errorf("%s holds no scenes", path)
	}
	for i, e := range list {
		if e.Name() == "" {
			return nil, fmt.Errorf("%s: scene %d has no name (animName): is it a scene?", path, i+1)
		}
	}

	return list, nil
}

func (f *Flags) Push(ctx context.Context, ref, path string) error {
	ctl, c, err := f.Controller(ref)
	if err != nil {
		return err
	}
	scenes, err := read(path)
	if err != nil {
		return err
	}
	if f.Cmd.Scene.As != "" {
		if len(scenes) != 1 {
			return fmt.Errorf("--as names one scene, and %s holds %d", path, len(scenes))
		}
		scenes[0] = scenes[0].WithName(f.Cmd.Scene.As)
	}

	report, err := f.put(ctx, ctl, c, scenes)
	if err != nil {
		return err
	}

	return f.finish([]push.Report{report})
}

func (f *Flags) Copy(ctx context.Context, name string) error {
	s, err := f.OpenStore()
	if err != nil {
		return err
	}
	if f.Cmd.Scene.From == "" {
		return errors.New("--from says which controller has the scene")
	}
	if f.Cmd.Scene.ToAll == (len(f.Cmd.Scene.To) > 0) {
		return errors.New("say where to copy it: --to <controller>, or --to-all for every other one")
	}
	from, err := s.Find(f.Cmd.Scene.From)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}

	var targets []store.Controller
	if f.Cmd.Scene.ToAll {
		targets = slices.DeleteFunc(slices.Clone(s.Controllers), func(c store.Controller) bool { return c.Name == from.Name })
		if len(targets) == 0 {
			return fmt.Errorf("%s is the only controller taproot knows: there is nowhere to copy to", from.Name)
		}
	}
	for _, ref := range f.Cmd.Scene.To {
		to, ferr := s.Find(ref)
		if ferr != nil {
			return fmt.Errorf("--to: %w", ferr)
		}
		if to.Name == from.Name && f.Cmd.Scene.As == "" {
			return fmt.Errorf("%s is where the scene comes from: copying it onto itself needs --as to give the copy another name", from.Name)
		}
		if !slices.ContainsFunc(targets, func(c store.Controller) bool { return c.Name == to.Name }) {
			targets = append(targets, to)
		}
	}

	// the source is only read, so it gets a client that could not write if it tried
	source, err := aurora.New(from.Host, from.Token, aurora.WithHTTPClient(cli.NewHTTPClient(f.Timeout)), aurora.WithDryRun(func(aurora.Request) {}))
	if err != nil {
		return err
	}
	// the scene on the controller it comes from, by the name as that controller spells it
	name, err = cli.ResolveScene(ctx, source, name)
	if err != nil {
		return fmt.Errorf("%s: %w", from.Name, err)
	}
	e, err := source.Effect(ctx, name)
	if err != nil {
		return fmt.Errorf("%s: %w", from.Name, noScene(ctx, source, name, err))
	}
	if f.Cmd.Scene.As != "" {
		e = e.WithName(f.Cmd.Scene.As)
	}
	cout.Printf("copying %s from %s\n", cli.Scene(e.Name()), cli.Name(from.Name))

	reports := make([]push.Report, 0, len(targets))
	var unreachable []error
	for _, to := range targets {
		c, err := f.Client(to)
		if err == nil {
			var r push.Report
			if r, err = f.put(ctx, to, c, []aurora.Effect{e}); err == nil {
				reports = append(reports, r)
				continue
			}
		}
		// one controller that is off does not stop the scene reaching the others
		cout.Errorf("<red>%s:</> %s\n", escape(to.Name), escape(err.Error()))
		unreachable = append(unreachable, fmt.Errorf("%s: %w", to.Name, err))
	}

	if err := f.finish(reports); err != nil || len(unreachable) == 0 {
		return err
	}

	return errors.Join(unreachable...)
}

// put pushes scenes to one controller and, with --select, starts the first
// once it is there.
func (f *Flags) put(ctx context.Context, ctl store.Controller, c *aurora.Client, scenes []aurora.Effect) (push.Report, error) {
	report, err := push.Scenes(ctx, c, scenes, push.Options{
		Controller: ctl.Name, BackupRoot: f.BackupRoot(), Force: f.Cmd.Force, DryRun: f.DryRun,
	})
	if err != nil {
		return report, err
	}

	if f.Cmd.Scene.Select && len(report.Results) > 0 {
		switch first := report.Results[0]; first.Outcome {
		case push.Added, push.Replaced, push.Unchanged:
			if err := c.SelectEffect(ctx, first.Scene); err != nil {
				return report, fmt.Errorf("the scene is on %s but would not start: %w", ctl.Name, err)
			}
		case push.Refused, push.Failed, push.Deleted, push.Renamed:
			// it is not there to start; a push never deletes or renames
		}
	}

	return report, nil
}

// finish prints what was done, as text or as the JSON document, and turns a
// scene that did not make it into the command's error.
func (f *Flags) finish(reports []push.Report) error {
	done, err := f.Emit(reports)
	if err != nil {
		return err
	}

	var failed []error
	for _, r := range reports {
		if !done {
			cli.PrintReport(r)
		}
		if err := r.Err(); err != nil {
			failed = append(failed, err)
		}
	}

	return errors.Join(failed...)
}

func (f *Flags) Delete(ctx context.Context, ref string, names []string) error {
	ctl, c, err := f.Controller(ref)
	if err != nil {
		return err
	}

	keep := f.Cmd.Scene.Except
	switch {
	case len(names) > 0 && len(keep) > 0:
		return errors.New("name the scenes to delete, or with --except the ones to keep: not both")
	case len(names) == 0 && len(keep) == 0:
		return errors.New("say which scenes to delete, or with --except which to keep")
	}
	held, err := c.EffectNames(ctx)
	if err != nil {
		return err
	}

	// each name as the controller spells it. One that cannot be matched stops everything: a slip of the
	// hand in the name of a scene to keep would otherwise delete it with the rest
	for i, typed := range names {
		if names[i], err = cli.MatchScene(held, typed); err != nil {
			return fmt.Errorf("%s: %w (nothing was deleted)", ctl.Name, err)
		}
	}
	for i, typed := range keep {
		if keep[i], err = cli.MatchScene(held, typed); err != nil {
			return fmt.Errorf("--except: %s: %w (nothing was deleted)", ctl.Name, err)
		}
	}
	if len(keep) > 0 {
		for _, name := range held {
			if !slices.Contains(keep, name) {
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			cout.Printf("%s holds nothing but the scenes to keep\n", cli.Name(ctl.Name))
			_, err = f.Emit([]push.Report{})
			return err
		}
	}

	report, err := push.Delete(ctx, c, names, push.Options{
		Controller: ctl.Name, BackupRoot: f.BackupRoot(), Force: f.Cmd.Force, DryRun: f.DryRun,
	})
	if err != nil {
		return err
	}

	return f.finish([]push.Report{report})
}

// Paint holds each named panel at a colour, shown without being saved, as
// the app's painting does; with --save it is stored as a static scene as
// well, through the same careful path as a push.
func (f *Flags) Paint(ctx context.Context, ref string, specs []string) error {
	ctl, c, err := f.Controller(ref)
	if err != nil {
		return err
	}
	info, err := c.Info(ctx)
	if err != nil {
		return err
	}
	colours, err := paintColors(info.PanelLayout.Layout.Panels, specs)
	if err != nil {
		return err
	}

	e, err := aurora.StaticEffect("", colours, f.Cmd.Scene.Over)
	if err != nil {
		return err
	}
	if err := c.DisplayEffect(ctx, e); err != nil {
		return fmt.Errorf("%s would not show it: %w", ctl.Name, err)
	}
	painted := make([]string, 0, len(colours))
	for _, sc := range colours {
		painted = append(painted, fmt.Sprintf("%s %s", cli.Num(sc.PanelID), cli.RGBSwatch(sc.R, sc.G, sc.B)))
	}
	if done, err := f.Emit(map[string]any{"controller": ctl.Name, "painted": colours, "saved": f.Cmd.Scene.Save}); done {
		return err
	}
	if !f.DryRun {
		cout.Printf("%s: painted %s\n", cli.Name(ctl.Name), strings.Join(painted, ", "))
	}
	if f.Cmd.Scene.Save == "" {
		return nil
	}

	saved, err := aurora.StaticEffect(f.Cmd.Scene.Save, colours, f.Cmd.Scene.Over)
	if err != nil {
		return err
	}
	report, err := f.put(ctx, ctl, c, []aurora.Effect{saved})
	if err != nil {
		return err
	}

	return f.finish([]push.Report{report})
}

// paintColors reads panel=colour pairs against the panels a controller has.
// A panel is its id as taproot info shows it, or all; a colour is six hex
// digits or a name. Later pairs win, so "all=000000 7=ff0000" is one red
// panel and the rest off.
func paintColors(panels []aurora.Panel, specs []string) ([]aurora.StaticColor, error) {
	if len(specs) == 0 {
		return nil, errors.New("say which panels to paint and what colour: panel=colour, or all=colour")
	}
	ids := make([]int, 0, len(panels))
	for _, p := range panels {
		if p.ShapeType != aurora.ShapeRhythm { // the Rhythm module has no light of its own
			ids = append(ids, p.ID)
		}
	}
	slices.Sort(ids)
	chosen := map[int][3]uint8{}
	for _, spec := range specs {
		which, colour, ok := strings.Cut(spec, "=")
		if !ok {
			return nil, fmt.Errorf("%q is not panel=colour", spec)
		}
		r, g, b, err := cli.ParseColor(colour)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(which, "all") {
			for _, id := range ids {
				chosen[id] = [3]uint8{r, g, b}
			}
			continue
		}
		id, err := strconv.Atoi(which)
		if err != nil || !slices.Contains(ids, id) {
			return nil, fmt.Errorf("no panel %q: the controller has %s", which, cli.Escape(fmt.Sprint(ids)))
		}
		chosen[id] = [3]uint8{r, g, b}
	}

	out := make([]aurora.StaticColor, 0, len(chosen))
	for _, id := range ids {
		if c, ok := chosen[id]; ok {
			out = append(out, aurora.StaticColor{PanelID: id, R: c[0], G: c[1], B: c[2]})
		}
	}

	return out, nil
}

// Rename gives a scene on a controller another name. The old name is matched
// as the controller spells it; the new one is taken as typed.
func (f *Flags) Rename(ctx context.Context, ref, name, newName string) error {
	ctl, c, err := f.Controller(ref)
	if err != nil {
		return err
	}
	name, err = cli.ResolveScene(ctx, c, name)
	if err != nil {
		return err
	}

	report, err := push.Rename(ctx, c, name, newName, push.Options{
		Controller: ctl.Name, BackupRoot: f.BackupRoot(), DryRun: f.DryRun,
	})
	if err != nil {
		return err
	}

	return f.finish([]push.Report{report})
}

func (f *Flags) Select(ctx context.Context, ref, name string) error {
	ctl, c, err := f.Controller(ref)
	if err != nil {
		return err
	}
	name, err = cli.ResolveScene(ctx, c, name)
	if err != nil {
		return err
	}
	if err := c.SelectEffect(ctx, name); err != nil {
		return noScene(ctx, c, name, err)
	}
	if done, err := f.Emit(map[string]string{"controller": ctl.Name, "running": name}); done {
		return err
	}
	if !f.DryRun {
		cout.Printf("%s is now running %s\n", cli.Name(ctl.Name), cli.Scene(name))
	}

	return nil
}

// escape keeps a scene's name from being read as colour tags.
func escape(s string) string { return cli.Escape(s) }
