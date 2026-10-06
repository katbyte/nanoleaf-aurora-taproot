package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/katbyte/go-kt/cout"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/discover"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// how often connect asks for a token: a press opens a window of about 30 seconds
var askEvery = 2 * time.Second

// how long list waits for one controller before calling it unreachable
const listPatience = 6 * time.Second

// FoundController is a controller on the network, and the name taproot holds
// its token under when it holds one.
type FoundController struct {
	discover.Found

	Address   string `json:"address"`
	Connected string `json:"connected,omitempty"`
}

// Search looks for controllers: by their announcements for as long as wait,
// and on every address of the subnet scan when one is given. Each is marked
// with the name it is connected under, when it is.
func Search(ctx context.Context, s *store.Store, wait time.Duration, scan string) ([]FoundController, error) {
	found, err := discover.Browse(ctx, wait)
	if err != nil {
		return nil, err
	}

	if scan != "" {
		prefix, err := netip.ParsePrefix(scan)
		if err != nil {
			return nil, fmt.Errorf("--scan %q is not a subnet like 10.0.5.0/24: %w", scan, err)
		}
		knocked, err := discover.Scan(ctx, prefix, discover.DefaultPort, 1500*time.Millisecond)
		if err != nil {
			return nil, err
		}
		for _, k := range knocked {
			// one that also announced itself is already listed, with its name
			if !slices.ContainsFunc(found, func(f discover.Found) bool { return slices.Contains(f.Addrs, k.Addrs[0]) }) {
				found = append(found, k)
			}
		}
	}

	out := make([]FoundController, len(found))
	for i, f := range found {
		out[i] = FoundController{Found: f, Address: f.Address()}
		for _, c := range s.Controllers {
			if (f.Name != "" && c.Device == f.Name) || hostIs(c.Host, f) {
				out[i].Connected = c.Name
			}
		}
	}

	return out, nil
}

// hostIs reports whether a saved address is one of a found controller's.
func hostIs(host string, f discover.Found) bool {
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		name = host
	}
	if strings.EqualFold(name, f.Host) {
		return true
	}
	addr, err := netip.ParseAddr(name)

	return err == nil && slices.Contains(f.Addrs, addr)
}

func (f *FlagData) Find(ctx context.Context) error {
	s, err := f.OpenStore()
	if err != nil {
		return err
	}

	cout.Printf("searching for controllers for %s...\n", f.Cmd.Wait)
	found, err := Search(ctx, s, f.Cmd.Wait, f.Cmd.Scan)
	if err != nil {
		return err
	}
	if done, err := f.Emit(found); done {
		return err
	}

	if len(found) == 0 {
		cout.Printf("<yellow>none found.</> controllers announce themselves over mDNS, which often does not cross from one network to another:\n")
		cout.Printf("  taproot find --scan 10.0.5.0/24   knocks on every address of a subnet\n")
		cout.Printf("  taproot connect 10.0.5.183        connects to one by its address\n")
		return nil
	}

	rows := make([][]string, 0, len(found))
	for _, c := range found {
		state := "not connected: taproot connect " + c.Address
		switch {
		case c.Connected != "":
			state = "connected as " + c.Connected
		case c.Address == "":
			state = "heard of, but its address did not come back: search again"
		}
		rows = append(rows, []string{orDash(c.Name), orDash(c.Address), orDash(c.Model), orDash(c.Firmware), state})
	}
	Table(rows)

	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Connected is what connect reports: the controller as it was saved, without
// its token.
type Connected struct {
	Name     string `json:"name"`
	Device   string `json:"device"`
	Host     string `json:"host"`
	Model    string `json:"model"`
	Firmware string `json:"firmwareVersion"`
	Panels   int    `json:"panels"`
	Scenes   int    `json:"scenes"`
	Already  bool   `json:"alreadyConnected,omitempty"`
}

func (f *FlagData) Connect(ctx context.Context, target string) error {
	s, err := f.OpenStore()
	if err != nil {
		return err
	}

	host, label, err := resolveTarget(ctx, s, target)
	if err != nil {
		return err
	}

	// a controller taproot already holds a working token for needs no button
	if existing, ok := savedAt(s, host); ok && f.Cmd.TokenFile == "" {
		if c, err := f.Client(existing); err == nil {
			if info, err := c.Info(ctx); err == nil {
				out := connected(existing, info)
				out.Already = true
				if done, err := f.Emit(out); done {
					return err
				}
				cout.Printf("already connected to <cyan>%s</> as <cyan>%s</>, and its token works\n", Escape(info.Name), existing.Name)
				return nil
			} else if !aurora.IsUnauthorized(err) {
				return fmt.Errorf("%s is saved as %s but cannot be reached: %w", host, existing.Name, err)
			}
			cout.Printf("<yellow>the saved token for %s no longer works</> (the controller was reset, or the token deleted): getting a new one\n", existing.Name)
		}
	}

	bare, err := f.Client(store.Controller{Name: label, Host: host})
	if err != nil {
		return err
	}

	var token string
	if f.Cmd.TokenFile != "" {
		if token, err = readTokenFile(f.Cmd.TokenFile); err != nil {
			return err
		}
	} else {
		cout.Printf("connecting to <cyan>%s</>\n", Escape(label))
		if token, err = f.askForToken(ctx, bare); err != nil {
			return err
		}
		if f.DryRun {
			_, err = f.Emit(Connected{Host: host})
			return err
		}
	}

	out, err := f.Adopt(ctx, host, token, f.Cmd.Name, f.Cmd.TokenFile == "")
	if err != nil {
		if f.Cmd.TokenFile != "" && aurora.IsUnauthorized(err) {
			return fmt.Errorf("%s does not know the token in %s", host, f.Cmd.TokenFile)
		}
		return err
	}
	if done, err := f.Emit(out); done {
		return err
	}
	if f.DryRun {
		return nil
	}
	cout.Printf("<green>connected:</> saved as <cyan>%s</> (%s, firmware %s, %d panels, %d scenes)\n",
		out.Name, out.Model, out.Firmware, out.Panels, out.Scenes)

	return nil
}

// Adopt reads a controller with a token for it and saves the two under name,
// or under the name the controller gives itself. fresh says the token was
// just handed out, so that if it cannot be saved the controller is asked to
// delete it again: a token nobody holds can never be removed short of a reset.
func (f *FlagData) Adopt(ctx context.Context, host, token, name string, fresh bool) (Connected, error) {
	c, err := f.Client(store.Controller{Name: host, Host: host, Token: token})
	if err != nil {
		return Connected{}, err
	}
	info, err := c.Info(ctx)
	if err != nil {
		if aurora.IsUnauthorized(err) {
			return Connected{}, err
		}
		return Connected{}, fmt.Errorf("got a token from %s but could not read the controller with it: %w", host, err)
	}

	ctl := store.Controller{
		Name: name, Device: info.Name, Host: host, Serial: info.SerialNo,
		Model: info.Model, Firmware: info.FirmwareVersion, Token: token, Added: time.Now().UTC().Truncate(time.Second),
	}
	// read the file again here: a page served for days must not save over what a command wrote meanwhile
	s, err := f.OpenStore()
	if err != nil {
		return Connected{}, err
	}
	if f.DryRun {
		cout.Printf("<yellow>dry run:</> would save <cyan>%s</> to %s\n", Escape(info.Name), s.Path())
		return connected(ctl, info), nil
	}
	ctl.Name, _ = s.Put(ctl)
	if err := s.Save(); err != nil {
		if fresh {
			_ = c.DeleteToken(ctx) // the save failing is the error to report
		}
		return Connected{}, err
	}

	return connected(ctl, info), nil
}

func connected(ctl store.Controller, info aurora.Info) Connected {
	return Connected{
		Name: ctl.Name, Device: info.Name, Host: ctl.Host, Model: info.Model, Firmware: info.FirmwareVersion,
		Panels: info.PanelLayout.Layout.NumPanels, Scenes: len(info.Effects.List),
	}
}

// savedAt is the controller the store holds for an address.
func savedAt(s *store.Store, host string) (store.Controller, bool) {
	i := slices.IndexFunc(s.Controllers, func(c store.Controller) bool { return strings.EqualFold(c.Host, host) })
	if i < 0 {
		return store.Controller{}, false
	}
	return s.Controllers[i], true
}

// resolveTarget turns what was given to connect into an address, and a label
// to call the controller by until it has said its name. An address is taken
// as it is; anything else is looked for among the controllers announcing
// themselves, as is nothing at all.
func resolveTarget(ctx context.Context, s *store.Store, target string) (host, label string, err error) {
	if looksLikeAddress(target) {
		return target, target, nil
	}

	cout.Printf("searching for controllers...\n")
	found, err := Search(ctx, s, 3*time.Second, "")
	if err != nil {
		return "", "", err
	}

	var hits []FoundController
	for _, c := range found {
		switch {
		case target == "" && c.Connected == "":
			hits = append(hits, c)
		case target != "" && strings.Contains(fold(c.Name+" "+c.Host), fold(target)):
			hits = append(hits, c)
		}
	}

	switch len(hits) {
	case 1:
		if hits[0].Address == "" {
			return "", "", fmt.Errorf("found %s, but its address did not come back: try again, or connect by address", hits[0].Name)
		}
		return hits[0].Address, hits[0].Name + " (" + hits[0].Address + ")", nil
	case 0:
		if target == "" {
			return "", "", errors.New("found no controller that is not connected already: name one by its address, taproot connect 10.0.5.183")
		}
		return "", "", fmt.Errorf("found no controller called %q: taproot find lists the ones that answer, or connect by address", target)
	default:
		names := make([]string, len(hits))
		for i, h := range hits {
			names[i] = h.Name + " (" + h.Address + ")"
		}
		return "", "", fmt.Errorf("found %d controllers, say which: %s", len(hits), strings.Join(names, ", "))
	}
}

// looksLikeAddress tells an address from a name to search for: an IP, a name
// with a dot in it, or anything with a port.
func looksLikeAddress(s string) bool {
	if s == "" {
		return false
	}
	if _, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil {
		return true
	}
	if _, _, err := net.SplitHostPort(s); err == nil {
		return true
	}

	return strings.Contains(s, ".") && !strings.ContainsAny(s, " ")
}

// fold strips a name down to its letters and digits, lower case.
func fold(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		}
		return -1
	}, s)
}

// askForToken asks the controller for a token until it hands one over, the
// wait runs out, or the person gives up, with a dot for every time it asks.
func (f *FlagData) askForToken(ctx context.Context, c *aurora.Client) (string, error) {
	dots := false
	token, err := AskForToken(ctx, c, f.Cmd.Wait, askEvery, f.DryRun, func(first bool) {
		if first {
			cout.Printf("hold the controller's power button for 5 to 7 seconds, until its light flashes: asking for up to %s (ctrl-c stops)\n", f.Cmd.Wait)
			return
		}
		dots = true
		cout.Printf(".")
	})
	if dots {
		cout.Printf("\n")
	}

	return token, err
}

// AskForToken asks a controller for a token, again after every interval,
// until it hands one over, wait runs out, or ctx ends. A press of the button
// opens a window of about 30 seconds, so an interval of a couple of seconds
// will not miss it. waiting is called each time the
// controller says nobody has held its button yet, with first set the first
// time. A controller that cannot be reached at all ends it at once, since no
// button will fix that; one that drops out part way gets three tries. dryRun
// says the client sends nothing, so the first answer, an empty one, is final.
func AskForToken(ctx context.Context, c *aurora.Client, wait, every time.Duration, dryRun bool, waiting func(first bool)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	tick := time.NewTicker(every)
	defer tick.Stop()

	unreachable := 0
	for first := true; ; first = false {
		token, err := c.NewToken(ctx)
		switch {
		case err == nil && (token != "" || dryRun):
			return token, nil
		case errors.Is(err, aurora.ErrNotPairing):
			unreachable = 0
			waiting(first)
		case ctx.Err() != nil:
			// the wait is over, or the person stopped it: the select below says which
		default:
			unreachable++
			if unreachable >= 3 || first {
				return "", fmt.Errorf("cannot reach the controller at %s: %w", c.Host(), err)
			}
		}

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return "", fmt.Errorf("no token after %s: the power button has to be held for 5 to 7 seconds, until the light flashes", wait)
			}
			return "", errors.New("stopped before the controller handed out a token")
		case <-tick.C:
		}
	}
}

// readTokenFile reads a token from a file holding it alone, or as a
// controller sends it.
func readTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the file the person named
	if err != nil {
		return "", fmt.Errorf("reading the token file: %w", err)
	}

	text := strings.TrimSpace(string(data))
	var wrapped struct {
		Token string `json:"auth_token"`
	}
	if json.Unmarshal(data, &wrapped) == nil && wrapped.Token != "" {
		text = wrapped.Token
	}
	if text == "" || strings.ContainsAny(text, " \t\n{}\"") {
		return "", fmt.Errorf("%s does not hold a token: a token alone, or {\"auth_token\": ...} as a controller sends it", path)
	}

	return text, nil
}

// Status is one controller as list shows it.
type Status struct {
	Name     string `json:"name"`
	Device   string `json:"device"`
	Host     string `json:"host"`
	Model    string `json:"model,omitempty"`
	Firmware string `json:"firmwareVersion,omitempty"`

	Reachable  bool   `json:"reachable"`
	Error      string `json:"error,omitempty"`
	On         bool   `json:"on"`
	Brightness int    `json:"brightness"`
	Running    string `json:"running,omitempty"`
	Panels     int    `json:"panels"`
	Scenes     int    `json:"scenes"`
}

// Statuses asks every controller what it is doing, all at once.
func (f *FlagData) Statuses(ctx context.Context, s *store.Store) []Status {
	out := make([]Status, len(s.Controllers))
	var wg sync.WaitGroup
	for i, ctl := range s.Controllers {
		out[i] = Status{Name: ctl.Name, Device: ctl.Device, Host: ctl.Host, Model: ctl.Model, Firmware: ctl.Firmware}
		wg.Go(func() {
			patient, cancel := context.WithTimeout(ctx, listPatience)
			defer cancel()

			c, err := f.Client(ctl)
			if err != nil {
				out[i].Error = err.Error()
				return
			}
			info, err := c.Info(patient)
			if err != nil {
				out[i].Error = err.Error()
				return
			}
			st := &out[i]
			st.Reachable, st.Device, st.Model, st.Firmware = true, info.Name, info.Model, info.FirmwareVersion
			st.On, st.Brightness, st.Running = info.State.On.Value, info.State.Brightness.Value, info.Effects.Select
			st.Panels, st.Scenes = info.PanelLayout.Layout.NumPanels, len(info.Effects.List)
		})
	}
	wg.Wait()
	slices.SortFunc(out, func(a, b Status) int { return strings.Compare(a.Name, b.Name) })

	return out
}

func (f *FlagData) List(ctx context.Context) error {
	s, err := f.OpenStore()
	if err != nil {
		return err
	}

	statuses := f.Statuses(ctx, s)
	if done, err := f.Emit(statuses); done {
		return err
	}
	if len(statuses) == 0 {
		cout.Printf("no controllers yet: <cyan>taproot find</> lists the ones on the network, <cyan>taproot connect</> gets a token for one\n")
		return nil
	}

	rows := [][]string{{"NAME", "ADDRESS", "MODEL", "FIRMWARE", "PANELS", "SCENES", "POWER", "RUNNING"}}
	for _, st := range statuses {
		if !st.Reachable {
			rows = append(rows, []string{st.Name, st.Host, orDash(st.Model), orDash(st.Firmware), "-", "-", "unreachable", "-"})
			continue
		}
		power := "off"
		if st.On {
			power = fmt.Sprintf("on %d%%", st.Brightness)
		}
		rows = append(rows, []string{
			st.Name, st.Host, st.Model, st.Firmware, strconv.Itoa(st.Panels), strconv.Itoa(st.Scenes), power, st.Running,
		})
	}
	Table(rows)
	for _, st := range statuses {
		if !st.Reachable {
			cout.Verbosef("%s: %s\n", st.Name, Escape(st.Error))
		}
	}

	return nil
}

// Described is one controller as info reports it with --json: what taproot
// calls it and where it is, and everything the controller says about itself,
// as it said it.
type Described struct {
	Name   string          `json:"name"`
	Host   string          `json:"host"`
	Device json.RawMessage `json:"device"`
}

func (f *FlagData) Info(ctx context.Context, ref string) error {
	ctl, c, err := f.Controller(ref)
	if err != nil {
		return err
	}
	info, err := c.Info(ctx)
	if err != nil {
		return err
	}
	if done, err := f.Emit(Described{Name: ctl.Name, Host: ctl.Host, Device: info.Raw}); done {
		return err
	}

	power := "off"
	if info.State.On.Value {
		power = fmt.Sprintf("on, brightness %d%%", info.State.Brightness.Value)
	}
	rows := [][]string{
		{"name", ctl.Name},
		{"calls itself", info.Name},
		{"address", ctl.Host},
		{"model", info.Model + " (" + info.Manufacturer + ")"},
		{"firmware", info.FirmwareVersion},
		{"hardware", info.HardwareVersion},
		{"serial", info.SerialNo},
		{"panels", fmt.Sprintf("%d, turned %d°", info.PanelLayout.Layout.NumPanels, info.PanelLayout.GlobalOrientation.Value)},
		{"power", power},
		{"running", info.Effects.Select + " (" + info.State.ColorMode + " mode)"},
	}
	if r := info.Rhythm; r != nil && r.Connected {
		source := "microphone"
		if r.Mode == aurora.RhythmModeAux {
			source = "aux cable"
		}
		rows = append(rows, []string{"rhythm", fmt.Sprintf("firmware %s, listening to the %s", r.FirmwareVersion, source)})
	}
	rows = append(rows, []string{"scenes", fmt.Sprintf("%d: %s", len(info.Effects.List), strings.Join(info.Effects.List, ", "))})
	Table(rows)

	return nil
}

func (f *FlagData) Forget(ctx context.Context, ref string) error {
	s, err := f.OpenStore()
	if err != nil {
		return err
	}
	ctl, err := s.Find(ref)
	if err != nil {
		return err
	}

	if !f.Cmd.Local {
		c, err := f.Client(ctl)
		if err != nil {
			return err
		}
		// a controller that no longer knows the token has nothing left to delete
		if err := c.DeleteToken(ctx); err != nil && !aurora.IsUnauthorized(err) {
			return fmt.Errorf("could not have %s delete the token: %w (--local removes it from the controllers file alone)", ctl.Name, err)
		}
	}

	if f.DryRun {
		cout.Printf("<yellow>dry run:</> would remove <cyan>%s</> from %s\n", ctl.Name, s.Path())
		_, err := f.Emit(map[string]string{"forgot": ctl.Name})
		return err
	}
	s.Remove(ctl.Name)
	if err := s.Save(); err != nil {
		return err
	}
	if done, err := f.Emit(map[string]string{"forgot": ctl.Name}); done {
		return err
	}
	if f.Cmd.Local {
		cout.Printf("forgot <cyan>%s</>: its token is gone from the controllers file, and still valid on the controller\n", ctl.Name)
		return nil
	}
	cout.Printf("forgot <cyan>%s</>: the controller has deleted the token\n", ctl.Name)

	return nil
}
