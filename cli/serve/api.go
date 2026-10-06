package serve

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/katbyte/go-kt/version"
	"github.com/katbyte/nanoleaf-aurora-taproot/cli"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/backup"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/push"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// how long a controller gets to answer before the page shows it as unreachable
const patience = 6 * time.Second

// how long one reading of every controller is good for: several tabs asking at once share it
const freshFor = time.Second

// how long the page's search listens for controllers
var findFor = 3 * time.Second

// state is everything the page draws.
type state struct {
	Version     string           `json:"version"`
	DryRun      bool             `json:"dryRun"`
	Controllers []controllerView `json:"controllers"`
	Pairing     []pairing        `json:"pairing"`
}

// controllerView is one controller as the page draws it. It carries no token.
type controllerView struct {
	Name     string `json:"name"`
	Device   string `json:"device"`
	Host     string `json:"host"`
	Model    string `json:"model"`
	Firmware string `json:"firmware"`

	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`

	On          bool   `json:"on"`
	Brightness  int    `json:"brightness"`
	ColorMode   string `json:"colorMode"`
	Hue         int    `json:"hue"`
	Sat         int    `json:"sat"`
	CT          int    `json:"ct"`
	Running     string `json:"running"`
	Orientation int    `json:"orientation"`

	Layout *aurora.Layout `json:"layout,omitempty"`
	Scenes []sceneView    `json:"scenes"`
}

// sceneView is the part of a scene the page needs to list it and to draw it
// moving: its colours, and how its plugin moves them.
type sceneView struct {
	Name       string         `json:"name"`
	Kind       string         `json:"kind"` // color or rhythm
	Plugin     string         `json:"plugin"`
	PluginUUID string         `json:"pluginUuid"`
	Palette    []aurora.Color `json:"palette"`
	Options    map[string]any `json:"options"`
}

// snapshot is the last reading of every controller.
type snapshot struct {
	mu    sync.Mutex
	at    time.Time
	views []controllerView
}

// stale throws the reading away: something has just changed.
func (s *server) stale() {
	s.snap.mu.Lock()
	defer s.snap.mu.Unlock()
	s.snap.at = time.Time{}
}

func (s *server) getState(w http.ResponseWriter, r *http.Request) {
	views, err := s.views(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	reply(w, http.StatusOK, state{Version: version.Version, DryRun: s.f.DryRun, Controllers: views, Pairing: s.pairs.list()})
}

// views reads every controller, all at once, unless that was done a moment ago.
func (s *server) views(ctx context.Context) ([]controllerView, error) {
	s.snap.mu.Lock()
	defer s.snap.mu.Unlock()
	if time.Since(s.snap.at) < freshFor {
		return s.snap.views, nil
	}

	st, err := s.f.OpenStore()
	if err != nil {
		return nil, err
	}
	s.hub.sync(s, st.Controllers) //nolint:contextcheck // its listeners live as long as the server, not this request

	views := make([]controllerView, len(st.Controllers))
	var wg sync.WaitGroup
	for i, ctl := range st.Controllers {
		wg.Go(func() { views[i] = s.view(ctx, ctl) })
	}
	wg.Wait()
	slices.SortFunc(views, func(a, b controllerView) int { return strings.Compare(a.Name, b.Name) })
	s.snap.views, s.snap.at = views, time.Now()

	return views, nil
}

// view reads one controller. One that does not answer is still listed, with
// what the controllers file remembers of it and why it could not be read.
func (s *server) view(ctx context.Context, ctl store.Controller) controllerView {
	v := controllerView{Name: ctl.Name, Device: ctl.Device, Host: ctl.Host, Model: ctl.Model, Firmware: ctl.Firmware, Scenes: []sceneView{}}

	ctx, cancel := context.WithTimeout(ctx, patience)
	defer cancel()

	c, err := s.f.Client(ctl)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	info, err := c.Info(ctx)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	effects, err := c.Effects(ctx)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	plugins := s.pluginsOf(ctx, ctl.Name, c)

	v.Reachable, v.Device, v.Model, v.Firmware = true, info.Name, info.Model, info.FirmwareVersion
	v.On, v.Brightness, v.ColorMode = info.State.On.Value, info.State.Brightness.Value, info.State.ColorMode
	v.Hue, v.Sat, v.CT = info.State.Hue.Value, info.State.Sat.Value, info.State.CT.Value
	v.Running, v.Orientation = info.Effects.Select, info.PanelLayout.GlobalOrientation.Value
	v.Layout = &info.PanelLayout.Layout
	for _, e := range effects {
		sv := sceneView{Name: e.Name(), Kind: e.PluginType(), PluginUUID: e.PluginUUID(), Palette: e.Palette(), Options: map[string]any{}}
		if i := slices.IndexFunc(plugins, func(p aurora.Plugin) bool { return p.UUID == sv.PluginUUID }); i >= 0 {
			sv.Plugin = plugins[i].Name
		}
		for _, o := range e.PluginOptions() {
			sv.Options[o.Name] = o.Value
		}
		v.Scenes = append(v.Scenes, sv)
	}

	return v
}

// pluginsOf is the plugins a controller has, asked for once: they only change
// with its firmware.
func (s *server) pluginsOf(ctx context.Context, name string, c *aurora.Client) []aurora.Plugin {
	if cached, ok := s.plugins.Load(name); ok {
		if plugins, ok := cached.([]aurora.Plugin); ok {
			return plugins
		}
	}
	plugins, err := c.Plugins(ctx)
	if err != nil {
		return nil // the scenes are listed without their plugins' names, and it is asked again next time
	}
	s.plugins.Store(name, plugins)

	return plugins
}

// controller is the controller a request names, and a client for it.
func (s *server) controller(r *http.Request) (store.Controller, *aurora.Client, error) {
	return s.byName(r.PathValue("name"))
}

func (s *server) byName(name string) (store.Controller, *aurora.Client, error) {
	st, err := s.f.OpenStore()
	if err != nil {
		return store.Controller{}, nil, err
	}
	i := slices.IndexFunc(st.Controllers, func(c store.Controller) bool { return c.Name == name })
	if i < 0 {
		return store.Controller{}, nil, fmt.Errorf("%w called %q", errNoSuchController, name)
	}
	c, err := s.f.Client(st.Controllers[i])

	return st.Controllers[i], c, err
}

// change runs one change to a controller and answers with nothing when it
// worked.
func (s *server) change(w http.ResponseWriter, r *http.Request, do func(context.Context, *aurora.Client) error) {
	_, c, err := s.controller(r)
	if err == nil {
		err = do(r.Context(), c)
	}
	s.stale()
	if err != nil {
		failFor(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) putPower(w http.ResponseWriter, r *http.Request) {
	var in struct {
		On *bool `json:"on"`
	}
	if err := read(r, &in); err != nil || in.On == nil {
		failFor(w, errors.Join(errBadRequest, err))
		return
	}
	s.change(w, r, func(ctx context.Context, c *aurora.Client) error { return c.SetOn(ctx, *in.On) })
}

func (s *server) putBrightness(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Value *int `json:"value"`
	}
	if err := read(r, &in); err != nil || in.Value == nil || *in.Value < 0 || *in.Value > 100 {
		failFor(w, errors.Join(fmt.Errorf("%w: brightness is 0 to 100", errBadRequest), err))
		return
	}
	s.change(w, r, func(ctx context.Context, c *aurora.Client) error { return c.SetBrightness(ctx, *in.Value, 0) })
}

func (s *server) putScene(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := read(r, &in); err != nil || in.Name == "" {
		failFor(w, errors.Join(errBadRequest, err))
		return
	}
	s.change(w, r, func(ctx context.Context, c *aurora.Client) error { return c.SelectEffect(ctx, in.Name) })
}

func (s *server) postIdentify(w http.ResponseWriter, r *http.Request) {
	s.change(w, r, func(ctx context.Context, c *aurora.Client) error { return c.Identify(ctx) })
}

func (s *server) postBackup(w http.ResponseWriter, r *http.Request) {
	ctl, c, err := s.controller(r)
	if err != nil {
		failFor(w, err)
		return
	}
	dir := backup.Dir(s.f.BackupRoot(), ctl.Name, time.Now())
	if s.f.DryRun {
		reply(w, http.StatusOK, cli.BackedUp{Controller: ctl.Name, Dir: dir})
		return
	}
	m, err := backup.Take(r.Context(), c, dir)
	if err != nil {
		failFor(w, err)
		return
	}
	reply(w, http.StatusOK, cli.BackedUp{Controller: ctl.Name, Dir: dir, Scenes: len(m.Scenes), Running: m.Running})
}

// deleteController forgets a controller: its token is deleted from the
// controller, unless that is asked not to be, and from the controllers file.
func (s *server) deleteController(w http.ResponseWriter, r *http.Request) {
	ctl, c, err := s.controller(r)
	if err != nil {
		failFor(w, err)
		return
	}
	if r.URL.Query().Get("local") == "" {
		if err := c.DeleteToken(r.Context()); err != nil && !aurora.IsUnauthorized(err) {
			failFor(w, fmt.Errorf("could not have %s delete the token: %w", ctl.Name, err))
			return
		}
	}
	if !s.f.DryRun {
		s.file.Lock()
		st, err := s.f.OpenStore()
		if err == nil {
			st.Remove(ctl.Name)
			err = st.Save()
		}
		s.file.Unlock()
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
	}
	s.plugins.Delete(ctl.Name)
	s.stale()
	w.WriteHeader(http.StatusNoContent)
}

// copyRequest is the page's copy button.
type copyRequest struct {
	Scene  string   `json:"scene"`
	From   string   `json:"from"`
	To     []string `json:"to"`
	Force  bool     `json:"force"`
	Select bool     `json:"select"`
}

// copied is what happened on one controller a scene was copied to.
type copied struct {
	push.Report

	Error string `json:"error,omitempty"`
}

func (s *server) postCopy(w http.ResponseWriter, r *http.Request) {
	var in copyRequest
	if err := read(r, &in); err != nil || in.Scene == "" || in.From == "" || len(in.To) == 0 {
		failFor(w, errors.Join(fmt.Errorf("%w: a copy needs a scene, where it is from, and where it is to go", errBadRequest), err))
		return
	}
	if slices.Contains(in.To, in.From) {
		failFor(w, fmt.Errorf("%w: %s is where the scene comes from", errBadRequest, in.From))
		return
	}

	// the controller a scene comes from is only read: it gets a client that could not write if it tried
	from, _, err := s.byName(in.From)
	if err != nil {
		failFor(w, err)
		return
	}
	source, err := aurora.New(from.Host, from.Token, aurora.WithHTTPClient(cli.NewHTTPClient(s.f.Timeout)), aurora.WithDryRun(func(aurora.Request) {}))
	if err != nil {
		failFor(w, err)
		return
	}
	e, err := source.Effect(r.Context(), in.Scene)
	if err != nil {
		failFor(w, fmt.Errorf("reading %q from %s: %w", in.Scene, in.From, err))
		return
	}

	out := make([]copied, 0, len(in.To))
	for _, name := range in.To {
		var res copied
		res.Controller = name
		ctl, c, err := s.byName(name)
		if err == nil {
			res.Report, err = push.Scenes(r.Context(), c, []aurora.Effect{e}, push.Options{
				Controller: ctl.Name, BackupRoot: s.f.BackupRoot(), Force: in.Force, DryRun: s.f.DryRun,
			})
		}
		if err == nil && in.Select && len(res.Results) == 1 {
			if o := res.Results[0].Outcome; o == push.Added || o == push.Replaced || o == push.Unchanged {
				err = c.SelectEffect(r.Context(), e.Name())
			}
		}
		if err != nil {
			res.Error = err.Error()
		}
		out = append(out, res)
	}
	s.stale()
	reply(w, http.StatusOK, out)
}

func (s *server) getFind(w http.ResponseWriter, r *http.Request) {
	st, err := s.f.OpenStore()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	found, err := cli.Search(r.Context(), st, findFor, strings.TrimSpace(r.URL.Query().Get("scan")))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if found == nil {
		found = []cli.FoundController{}
	}
	reply(w, http.StatusOK, found)
}
