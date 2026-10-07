package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
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

// search is how connect looks for controllers; a test puts canned ones here
var search = Search

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

	// say how it is looking: three seconds of nothing leaves a person wondering what is being waited for
	cout.Printf("searching for controllers for <yellow>%s</>\n", f.Cmd.Wait)
	for _, way := range discover.Ways() {
		cout.Printf("  %s\n", Dim(way))
	}
	if f.Cmd.Scan != "" {
		cout.Printf("  %s\n", Dim(fmt.Sprintf("knocking on port %d at every address of %s", discover.DefaultPort, f.Cmd.Scan)))
	}
	found, err := Search(ctx, s, f.Cmd.Wait, f.Cmd.Scan)
	if err != nil {
		return err
	}
	if done, err := f.Emit(found); done {
		return err
	}

	if len(found) == 0 {
		cout.Printf("%s controllers announce themselves over mDNS, which often does not cross from one network to another:\n", Note("none found."))
		cout.Printf("  <cyan>taproot find --scan 10.0.5.0/24</>   knocks on every address of a subnet\n")
		cout.Printf("  <cyan>taproot connect 10.0.5.183</>        connects to one by its address\n")
		return nil
	}

	cout.Printf("found %s\n", Num(len(found)))
	rows := [][]string{{Dim("CALLS ITSELF"), Dim("ADDRESS"), Dim("MODEL"), Dim("FIRMWARE"), Dim("FOUND BY"), ""}}
	for _, c := range found {
		state := Dim("not connected:") + " taproot connect " + c.Address
		switch {
		case c.Connected != "":
			state = "<green>connected</> as " + Name(c.Connected)
		case c.Address == "":
			state = Note("its address did not come back: search again")
		}
		name := Dim("-")
		if c.Name != "" {
			name = Device(c.Name)
		}
		rows = append(rows, []string{name, orDash(c.Address), Dim(orDash(c.Model)), Dim(orDash(c.Firmware)), Dim(foundBy(c.Via)), state})
	}
	Table(rows)

	return nil
}

// foundBy says in words how a controller was found.
func foundBy(via []string) string {
	words := make([]string, 0, len(via))
	for _, v := range via {
		switch v {
		case discover.ViaAnnouncement:
			words = append(words, "its announcement")
		case discover.ViaSystem:
			words = append(words, "the system")
		case discover.ViaScan:
			words = append(words, "a knock")
		}
	}

	return strings.Join(words, ", ")
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
				cout.Printf("already connected to %s as %s, and its token <green>works</>\n", Device(info.Name), Name(existing.Name))
				return nil
			} else if !aurora.IsUnauthorized(err) {
				return fmt.Errorf("%s is saved as %s but cannot be reached: %w", host, existing.Name, err)
			}
			cout.Printf("%s %s getting a new one\n", Note("the saved token for "+existing.Name+" no longer works"), Dim("(the controller was reset, or the token deleted):"))
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
		cout.Printf("connecting to %s\n", Device(label))
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
	cout.Printf("<green>connected:</> saved as %s %s %s %s %s %s\n",
		Name(out.Name), Dim("("+out.Model+", firmware "+out.Firmware+","), Num(out.Panels), Dim("panels,"), Num(out.Scenes), Dim("scenes)"))

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
		cout.Printf("<yellow>dry run:</> would save %s to %s\n", Device(info.Name), Dim(s.Path()))
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
	found, err := search(ctx, s, 3*time.Second, "")
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

// asked is how one controller's asking ended, in connect all.
type asked struct {
	c     FoundController
	token string
	err   error
}

// ConnectAll asks every controller that is not connected yet for a token, all
// at once, so the one whose button is held is the one that connects. Each
// that does is saved at once under the name it gives itself, and then, when
// there is somebody at a terminal to ask, they are asked what to call it.
// It goes on until every one is connected, until q is typed, until ctx ends,
// or, when limited is set, until --wait runs out.
func (f *FlagData) ConnectAll(ctx context.Context, in io.Reader, limited bool) error {
	if f.Cmd.Name != "" || f.Cmd.TokenFile != "" {
		return errors.New("connect all asks for each controller's name as it connects: --name and --token-file are for connecting one")
	}
	s, err := f.OpenStore()
	if err != nil {
		return err
	}

	cout.Printf("searching for controllers...\n")
	found, err := search(ctx, s, 3*time.Second, f.Cmd.Scan)
	if err != nil {
		return err
	}
	var waiting []FoundController
	for _, c := range found {
		if c.Connected == "" && c.Address != "" {
			waiting = append(waiting, c)
		}
	}
	switch {
	case len(found) == 0:
		return errors.New("found no controllers: taproot find says how it looks, and taproot connect <address> connects to one directly")
	case len(waiting) == 0:
		cout.Printf("every controller found is connected already (%s): <cyan>taproot list</> shows them\n", Num(len(found)))
		_, err := f.Emit([]Connected{})
		return err
	}

	cout.Printf("%s not connected yet:\n", Num(len(waiting)))
	rows := make([][]string, len(waiting))
	for i, c := range waiting {
		rows[i] = []string{" ", Device(label(c)), c.Address, Dim(orDash(c.Model)), Dim(orDash(c.Firmware))}
	}
	Table(rows)

	// a dry run asks nobody: it says what it would ask
	if f.DryRun {
		for _, c := range waiting {
			if bare, cerr := f.Client(store.Controller{Name: label(c), Host: c.Address}); cerr == nil {
				_, _ = bare.NewToken(ctx)
			}
		}
		_, err := f.Emit([]Connected{})
		return err
	}

	wait := 24 * time.Hour // until stopped
	stops := "type q and enter, or press ctrl-c, to stop"
	if limited {
		wait = f.Cmd.Wait
		stops = "stops after " + wait.String() + ", or type q and enter, or press ctrl-c"
	}
	cout.Printf("hold the power button of the one you want for <yellow>5 to 7 seconds</>, until its light flashes\n")
	cout.Printf("asking %s every %s %s\n", all(len(waiting)), askEvery, Dim("("+stops+")"))

	// each controller is asked by itself, so one that is slow or gone does not hold up the others
	asking, stop := context.WithCancel(ctx)
	defer stop()
	results := make(chan asked, len(waiting))
	var askers sync.WaitGroup
	for _, c := range waiting {
		askers.Go(func() {
			bare, cerr := f.Client(store.Controller{Name: label(c), Host: c.Address})
			if cerr != nil {
				results <- asked{c: c, err: cerr}
				return
			}
			token, aerr := AskForToken(asking, bare, wait, askEvery, false, func(bool) {})
			results <- asked{c: c, token: token, err: aerr}
		})
	}

	// what is typed: a name for the controller that just connected, or q. With nobody to type, as in a
	// script or with --json, every controller keeps the name it gives itself
	var lines chan string
	if atTerminal(in) && !f.Out.JSON {
		lines = make(chan string)
		go func() {
			defer close(lines)
			typed := bufio.NewScanner(in)
			for typed.Scan() {
				select {
				case lines <- typed.Text():
				case <-asking.Done():
					return
				}
			}
		}()
	}

	var (
		connectedNow []Connected
		unnamed      []Connected // connected and saved, and still to be asked what to call
		left         = len(waiting)
		dotted       bool
		timedOut     bool
	)
	newline := func() {
		if dotted {
			cout.Printf("\n")
			dotted = false
		}
	}
	prompt := func() {
		if len(unnamed) > 0 {
			cout.Printf("  what should %s be called? %s ", Device(unnamed[0].Device), Dim("(enter keeps "+unnamed[0].Name+")"))
		}
	}
	// say prints a line of news on a line of its own, whatever was on screen: a row of dots, or a
	// question still waiting for its answer, which is asked again underneath
	say := func(news func()) {
		if len(unnamed) > 0 {
			cout.Printf("\n")
		}
		newline()
		news()
		prompt()
	}
	// adopt saves a controller that has just handed over a token, before anything else can go wrong
	adopt := func(a asked) {
		saved, aerr := f.Adopt(context.WithoutCancel(ctx), a.c.Address, a.token, "", true)
		if aerr != nil {
			cout.Errorf("<red>%s connected but could not be saved:</> %s\n", Escape(label(a.c)), Escape(aerr.Error()))
			return
		}
		cout.Printf("<green>connected</> %s %s as %s\n", Device(saved.Device), Dim("("+a.c.Address+", "+saved.Model+", firmware "+saved.Firmware+")"), Name(saved.Name))
		connectedNow = append(connectedNow, saved)
		if lines != nil {
			unnamed = append(unnamed, saved)
		}
	}

	dots := time.NewTicker(askEvery)
	defer dots.Stop()
loop:
	for left > 0 || len(unnamed) > 0 {
		select {
		case <-ctx.Done():
			break loop
		case a := <-results:
			left--
			_, waitOver := errors.AsType[waitOverError](a.err)
			switch {
			case a.err == nil:
				say(func() { adopt(a) })
			case asking.Err() != nil:
				// stopped: not this controller's doing
			case waitOver:
				timedOut = true
			default:
				say(func() { cout.Errorf("<red>%s:</> %s\n", Escape(label(a.c)), Escape(a.err.Error())) })
			}
		case line, open := <-lines:
			if !open {
				lines, unnamed = nil, nil // nobody is typing: the names they have are the names they keep
				continue
			}
			line = strings.TrimSpace(line)
			if len(unnamed) == 0 {
				if strings.EqualFold(line, "q") {
					break loop
				}
				continue
			}
			if !f.name(&unnamed[0], line, connectedNow) {
				prompt() // that name would not do: ask again
				continue
			}
			unnamed = unnamed[1:]
			prompt()
		case <-dots.C:
			if len(unnamed) == 0 {
				cout.Printf("<darkGray>.</>")
				dotted = true
			}
		}
	}
	newline()

	// stop asking, and keep any token that was handed over in the moment it took to stop
	stop()
	askers.Wait()
	close(results)
	for a := range results {
		if a.err == nil && a.token != "" {
			left--
			adopt(a)
		}
	}

	if done, err := f.Emit(connectedNow); done {
		return err
	}
	names := make([]string, len(connectedNow))
	for i, c := range connectedNow {
		names[i] = Name(c.Name)
	}
	if len(names) > 0 {
		cout.Printf("connected %s: %s\n", Num(len(names)), strings.Join(names, ", "))
	}
	var rest []string
	for _, c := range waiting {
		if !slices.ContainsFunc(connectedNow, func(done Connected) bool { return done.Host == c.Address }) {
			rest = append(rest, label(c)+" ("+c.Address+")")
		}
	}
	if len(rest) > 0 {
		cout.Printf("%s %s\n", Note("still not connected:"), Escape(strings.Join(rest, ", ")))
	}
	if timedOut && len(connectedNow) == 0 {
		return fmt.Errorf("no controller handed out a token in %s: the power button has to be held for 5 to 7 seconds, until the light flashes", wait)
	}

	return nil
}

// name gives a controller that has just connected the name that was typed
// for it, and reports whether that is settled: an empty line keeps the name
// it has, and a name that will not do is said so and asked for again.
func (f *FlagData) name(c *Connected, typed string, all []Connected) bool {
	if typed == "" {
		return true
	}
	s, err := f.OpenStore()
	if err != nil {
		cout.Errorf("  <red>%s</>\n", Escape(err.Error()))
		return false
	}
	name, err := s.Rename(c.Name, typed)
	if err == nil {
		err = s.Save()
	}
	if err != nil {
		cout.Errorf("  <red>%s</>\n", Escape(err.Error()))
		return false
	}
	for i := range all {
		if all[i].Host == c.Host {
			all[i].Name = name
		}
	}
	c.Name = name
	cout.Printf("  saved as %s\n", Name(name))

	return true
}

// atTerminal reports whether there is somebody to ask on the other end of a
// reader: a terminal, or a reader a test is driving. Input that is a file or
// a pipe is nobody: asking it a question would wait for an answer for ever.
func atTerminal(in io.Reader) bool {
	if in == nil {
		return false
	}
	file, ok := in.(*os.File)
	if !ok {
		return true
	}
	info, err := file.Stat()

	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// label is what to call a found controller before it has a name of taproot's:
// the name it announced, or failing that its address.
func label(c FoundController) string {
	if c.Name != "" {
		return c.Name
	}
	return c.Address
}

// all is "both" or "all three" or "all 7": how many are being asked, in words.
func all(n int) string {
	switch n {
	case 1:
		return "it"
	case 2:
		return "both"
	default:
		return "all " + Num(n)
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
			cout.Printf("hold the controller's power button for <yellow>5 to 7 seconds</>, until its light flashes: asking for up to <yellow>%s</> %s\n", f.Cmd.Wait, Dim("(ctrl-c stops)"))
			return
		}
		dots = true
		cout.Printf("<darkGray>.</>")
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
				return "", waitOverError{wait}
			}
			return "", errors.New("stopped before the controller handed out a token")
		case <-tick.C:
		}
	}
}

// waitOverError is nobody having held the button in the time allowed.
type waitOverError struct{ wait time.Duration }

func (e waitOverError) Error() string {
	return fmt.Sprintf("no token after %s: the power button has to be held for 5 to 7 seconds, until the light flashes", e.wait)
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

	// taproot's name for each, and beside it the name the controller gives itself, which is the one
	// other apps and the network know it by
	header := []string{"NAME", "CALLS ITSELF", "ADDRESS", "MODEL", "FIRMWARE", "PANELS", "SCENES", "POWER", "RUNNING"}
	for i, h := range header {
		header[i] = Dim(h)
	}
	rows := [][]string{header}
	for _, st := range statuses {
		row := []string{Name(st.Name), Device(orDash(st.Device)), st.Host, Dim(orDash(st.Model)), Dim(orDash(st.Firmware))}
		if !st.Reachable {
			rows = append(rows, append(row, Dim("-"), Dim("-"), "<red>unreachable</>", Dim("-")))
			continue
		}
		power := Dim("off")
		if st.On {
			power = fmt.Sprintf("<green>on</> <yellow>%d%%</>", st.Brightness)
		}
		rows = append(rows, append(row, Num(st.Panels), Num(st.Scenes), power, Scene(st.Running)))
	}
	Table(rows)
	for _, st := range statuses {
		if !st.Reachable {
			cout.Verbosef("%s: %s\n", Name(st.Name), Escape(st.Error))
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

	power := Dim("off")
	if info.State.On.Value {
		power = fmt.Sprintf("<green>on</>, brightness <yellow>%d%%</>", info.State.Brightness.Value)
	}
	rows := [][]string{
		{Dim("name"), Name(ctl.Name)},
		{Dim("calls itself"), Device(info.Name)},
		{Dim("address"), Escape(ctl.Host)},
		{Dim("model"), Escape(info.Model + " (" + info.Manufacturer + ")")},
		{Dim("firmware"), Escape(info.FirmwareVersion)},
		{Dim("hardware"), Escape(info.HardwareVersion)},
		{Dim("serial"), Escape(info.SerialNo)},
		{Dim("panels"), Num(info.PanelLayout.Layout.NumPanels) + fmt.Sprintf(", turned %d°", info.PanelLayout.GlobalOrientation.Value)},
		{Dim("power"), power},
		{Dim("running"), Scene(info.Effects.Select) + " " + Dim("("+info.State.ColorMode+" mode)")},
	}
	if r := info.Rhythm; r != nil && r.Connected {
		source := wordMicrophone
		if r.Mode == aurora.RhythmModeAux {
			source = "aux cable"
		}
		rows = append(rows, []string{Dim("rhythm"), Escape(fmt.Sprintf("firmware %s, listening to the %s", r.FirmwareVersion, source))})
	}
	scenes := make([]string, len(info.Effects.List))
	for i, name := range info.Effects.List {
		scenes[i] = Scene(name)
	}
	rows = append(rows, []string{Dim("scenes"), Num(len(scenes)) + ": " + strings.Join(scenes, ", ")})
	Table(rows)

	return nil
}

// Renamed is what rename reports.
type Renamed struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Rename changes what taproot calls a controller. Only the controllers file
// changes: the name a controller gives itself is not something the API can
// set.
func (f *FlagData) Rename(ref, to string) error {
	s, err := f.OpenStore()
	if err != nil {
		return err
	}
	ctl, err := s.Find(ref)
	if err != nil {
		return err
	}
	name, err := s.Rename(ctl.Name, to)
	if err != nil {
		return err
	}
	out := Renamed{From: ctl.Name, To: name}

	switch {
	case name == ctl.Name:
		cout.Printf("%s is already called that\n", Name(name))
	case f.DryRun:
		cout.Printf("<yellow>dry run:</> would rename %s to %s in %s\n", Name(ctl.Name), Name(name), Dim(s.Path()))
	default:
		if err := s.Save(); err != nil {
			return err
		}
		cout.Printf("%s (%s) is now %s\n", Name(ctl.Name), Device(ctl.Device), Name(name))
		// backups are never moved: the ones already taken stay where they were put
		if old := filepath.Join(f.BackupRoot(), ctl.Name); isDir(old) {
			cout.Printf("its earlier backups stay in %s; new ones go in %s\n", Dim(old), Dim(filepath.Join(f.BackupRoot(), name)))
		}
	}
	_, err = f.Emit(out)

	return err
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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
		cout.Printf("<yellow>dry run:</> would remove %s from %s\n", Name(ctl.Name), Dim(s.Path()))
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
		cout.Printf("forgot %s: its token is gone from the controllers file, and %s\n", Name(ctl.Name), Note("still valid on the controller"))
		return nil
	}
	cout.Printf("forgot %s: the controller has deleted the token\n", Name(ctl.Name))

	return nil
}
