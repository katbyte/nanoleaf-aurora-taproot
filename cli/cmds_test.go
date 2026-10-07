// The commands, run as taproot runs them: the whole command tree built the
// way cmd/taproot builds it, given arguments, against canned controllers
// that answer as a real one did (auroratest). None of this needs a device.
//
// The commands share their flags through viper, which is one per process, so
// these tests run one at a time.
package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/katbyte/go-kt/cout"
	"github.com/katbyte/nanoleaf-aurora-taproot/cli"
	"github.com/katbyte/nanoleaf-aurora-taproot/cli/scene"
	"github.com/katbyte/nanoleaf-aurora-taproot/cli/serve"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/backup"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora/auroratest"
)

const northern = "kt Northern Lights"

// home is a configuration directory for one test, and the canned controllers
// taproot is connected to in it.
type home struct {
	t   *testing.T
	dir string
	// stdin is what is typed at the command, for the one that asks questions
	stdin io.Reader
}

func newHome(t *testing.T) *home {
	t.Helper()
	// nothing of the person's own setup may reach a test
	for _, name := range []string{"TAPROOT_CONFIG_DIR", "TAPROOT_BACKUP_DIR", "TAPROOT_TIMEOUT", "TAPROOT_OUTPUT_QUIET", "TAPROOT_OUTPUT_SILENT", "TAPROOT_ALLOW_HOSTS"} {
		t.Setenv(name, "")
	}

	return &home{t: t, dir: t.TempDir()}
}

// controller starts a canned controller and connects taproot to it under a
// name, as taproot connect would have.
func (h *home) controller(name string) *auroratest.Controller {
	h.t.Helper()

	ctl := auroratest.New(h.t)
	ctl.Set("name", "Light Panels "+name)
	ctl.Set("serialNo", "S-"+name)

	s, err := store.Open(h.dir)
	if err != nil {
		h.t.Fatal(err)
	}
	s.Put(store.Controller{Name: name, Device: "Light Panels " + name, Host: ctl.Host(), Serial: "S-" + name, Model: "NL22", Firmware: "5.2.1", Token: ctl.Token()})
	if err := s.Save(); err != nil {
		h.t.Fatal(err)
	}

	return ctl
}

// run runs taproot with arguments and returns everything it printed. No
// token may ever be in that.
func (h *home) run(args ...string) (string, error) {
	h.t.Helper()

	viper.Reset()
	root, err := cli.Make()
	if err != nil {
		h.t.Fatal(err)
	}
	root.AddCommand(scene.Command(), serve.Command())

	var out bytes.Buffer
	// what a command prints to the terminal, errors included, is what is being tested
	oldOut, oldErr, oldLevel := cout.Out, cout.Err, cout.Level
	cout.Out, cout.Err = &out, &out                                              //nolint:reassign // it is how output is captured
	defer func() { cout.Out, cout.Err, cout.Level = oldOut, oldErr, oldLevel }() //nolint:reassign // and put back
	root.SetOut(&out)
	root.SetErr(&out)
	if h.stdin != nil {
		root.SetIn(h.stdin)
	}
	root.SetArgs(append([]string{"--config-dir", h.dir}, args...))
	err = root.ExecuteContext(h.t.Context())

	// colours are for a terminal; what is checked here is the words
	text := ansi.ReplaceAllString(out.String(), "")
	printed := text
	if err != nil {
		printed += err.Error()
	}
	if s, serr := store.Open(h.dir); serr == nil {
		for _, c := range s.Controllers {
			if c.Token != "" && strings.Contains(printed, c.Token) {
				h.t.Errorf("taproot %s printed %s's token", strings.Join(args, " "), c.Name)
			}
		}
	}

	return text, err
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// ok runs taproot and fails the test if the command does.
func (h *home) ok(args ...string) string {
	h.t.Helper()

	out, err := h.run(args...)
	if err != nil {
		h.t.Fatalf("taproot %s: %v\n%s", strings.Join(args, " "), err, out)
	}

	return out
}

// fails runs taproot and returns the error the command must end in.
func (h *home) fails(args ...string) string {
	h.t.Helper()

	out, err := h.run(args...)
	if err == nil {
		h.t.Fatalf("taproot %s: want an error, got\n%s", strings.Join(args, " "), out)
	}

	return err.Error()
}

func (h *home) saved() []store.Controller {
	h.t.Helper()

	s, err := store.Open(h.dir)
	if err != nil {
		h.t.Fatal(err)
	}

	return s.Controllers
}

func want(t *testing.T, got string, parts ...string) {
	t.Helper()

	for _, part := range parts {
		if !strings.Contains(got, part) {
			t.Errorf("missing %q in:\n%s", part, got)
		}
	}
}

func TestVersionAndHelp(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	want(t, h.ok("version"), "taproot")
	// with no command it lists them, rather than saying how to ask for the list
	want(t, h.ok(), "Usage:", "Available Commands:", "connect", "scene", "serve", "--dry-run")
	want(t, h.ok("scene"), "list", "dump", "push", "copy", "select")
}

func TestConnectWaitsForTheButton(t *testing.T) { //nolint:paralleltest // the commands share viper
	restore := cli.SetAskEvery(10 * time.Millisecond)
	defer restore()
	h := newHome(t)
	ctl := auroratest.New(t)
	ctl.PairAfter(3) // somebody gets to the button on the fourth ask

	want(t, h.ok("connect", ctl.Host(), "--name", "office", "--wait", "10s"), "hold the controller's power button for 5 to 7 seconds", "connected:", "saved as", "office", "NL22, firmware 5.2.1, 4 panels, 17 scenes")

	saved := h.saved()
	if len(saved) != 1 || saved[0].Name != "office" || saved[0].Host != ctl.Host() || saved[0].Token == "" || saved[0].Device != "Light Panels 53:A6:3C" || saved[0].Added.IsZero() {
		t.Fatalf("saved: %+v", saved)
	}
	asked := 0
	for _, r := range ctl.Requests() {
		if r.Path == "/new" {
			asked++
		}
	}
	if asked != 4 {
		t.Errorf("asked for a token %d times, want 4", asked)
	}

	// the token it saved is one the controller accepts
	want(t, h.ok("list"), "office", "on 33%", northern)

	// connecting again needs no button
	want(t, h.ok("connect", ctl.Host()), "already connected", "its token works")

	// a controller that has since been reset no longer knows the token: it is asked for a new one
	c, _ := aurora.New(ctl.Host(), saved[0].Token)
	if err := c.DeleteToken(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctl.Pair()
	want(t, h.ok("connect", ctl.Host(), "--wait", "5s"), "no longer works", "connected:")
	if again := h.saved(); len(again) != 1 || again[0].Token == saved[0].Token || again[0].Name != "office" {
		t.Errorf("after connecting again: %+v", again)
	}
}

// unconnected is a canned controller on the network that taproot has no
// token for yet, and how a search would report it.
func unconnected(t *testing.T, name string) (*auroratest.Controller, cli.FoundController) {
	t.Helper()

	ctl := auroratest.New(t)
	ctl.Set("name", "Light Panels "+name)
	ctl.Set("serialNo", "S-"+name)
	return ctl, cli.FoundController{Address: ctl.Host(), Name: "Light Panels " + name, Model: "NL22", Firmware: "5.3.2"}
}

// asks is how many times a controller has been asked for a token.
func asks(ctl *auroratest.Controller) int {
	n := 0
	for _, r := range ctl.Requests() {
		if r.Path == "/new" {
			n++
		}
	}

	return n
}

// settled waits until no controller is being asked any more: the askers of a
// run that has just stopped may still have a request in the air, and the
// token the next run is waiting for must not go to one of those.
func settled(t *testing.T, ctls ...*auroratest.Controller) {
	t.Helper()
	count := func() int {
		n := 0
		for _, c := range ctls {
			n += asks(c)
		}
		return n
	}
	for last := count(); ; last = count() {
		time.Sleep(40 * time.Millisecond)
		if count() == last {
			return
		}
	}
}

func (h *home) named(name string) bool {
	return slices.ContainsFunc(h.saved(), func(c store.Controller) bool { return c.Name == name })
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Somebody with two controllers that look the same on the network, standing
// at one of them: connect all asks both, the one whose button is held is the
// one that answers, and it is named there and then.
func TestConnectAll(t *testing.T) { //nolint:paralleltest // the commands share viper
	restoreAsk := cli.SetAskEvery(10 * time.Millisecond)
	defer restoreAsk()
	h := newHome(t)
	office := h.controller("office") // connected already: not one to ask
	a, foundA := unconnected(t, "AA")
	b, foundB := unconnected(t, "BB")
	restoreSearch := cli.SetSearch(foundA, foundB, cli.FoundController{Address: office.Host(), Name: "Light Panels office"})
	defer restoreSearch()

	typed, typing := io.Pipe()
	defer func() { _ = typing.Close() }()
	h.stdin = typed
	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := h.run("connect", "all")
		done <- result{out, err}
	}()

	// nobody has touched anything yet: both are being asked, and asked again
	waitFor(t, "both to be asked", func() bool { return asks(a) >= 2 && asks(b) >= 2 })
	if len(h.saved()) != 1 {
		t.Fatal("a controller was saved before any button was held")
	}

	// the button on the second one: it is saved at once, under the name it gives itself, and then named
	b.Pair()
	waitFor(t, "the one whose button was held to be saved", func() bool { return h.named("light-panels-bb") })
	_, _ = io.WriteString(typing, "Bedroom\n")
	waitFor(t, "it to take the name typed for it", func() bool { return h.named("bedroom") })
	if h.named("light-panels-bb") || h.named("light-panels-aa") {
		t.Errorf("after naming one: %+v", h.saved())
	}

	// the other goes on being asked meanwhile; a name that is taken is asked for again
	a.Pair()
	waitFor(t, "the other to be saved", func() bool { return h.named("light-panels-aa") })
	_, _ = io.WriteString(typing, "office\n")
	_, _ = io.WriteString(typing, "\n") // enter alone keeps the name it has
	// with every one connected and named there is nothing left to wait for
	var res result
	select {
	case res = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("connect all did not finish once every controller was connected")
	}
	if res.err != nil {
		t.Fatalf("connect all: %v\n%s", res.err, res.out)
	}
	want(t, res.out, "2 not connected yet", "Light Panels AA", "Light Panels BB", "hold the power button", "asking both every",
		"type q and enter, or press ctrl-c, to stop", "connected Light Panels BB", "what should Light Panels BB be called?", "(enter keeps light-panels-bb)",
		"saved as bedroom", "connected Light Panels AA", "office is already what Light Panels office is called", "connected 2: bedroom, light-panels-aa")
	if strings.Contains(res.out, "still not connected") {
		t.Errorf("with both connected:\n%s", res.out)
	}
	saved := h.saved()
	if len(saved) != 3 || !h.named("bedroom") || !h.named("light-panels-aa") || !h.named("office") {
		t.Fatalf("saved: %+v", saved)
	}
	for _, c := range saved {
		if c.Token == "" {
			t.Errorf("%s was saved with no token", c.Name)
		}
	}
	// the one that was connected already was never asked for another token
	if asks(office) != 0 {
		t.Errorf("a controller that was connected already was asked %d times", asks(office))
	}

	// with nothing left to connect it says so, and asks nobody
	want(t, h.ok("connect", "all"), "every controller found is connected already (3)")
}

func TestConnectAllStops(t *testing.T) { //nolint:paralleltest // the commands share viper
	restoreAsk := cli.SetAskEvery(10 * time.Millisecond)
	defer restoreAsk()
	h := newHome(t)
	a, foundA := unconnected(t, "AA")
	b, foundB := unconnected(t, "BB")
	restoreSearch := cli.SetSearch(foundA, foundB)
	defer restoreSearch()

	// q, then enter, when there is no question on screen
	typed, typing := io.Pipe()
	defer func() { _ = typing.Close() }()
	h.stdin = typed
	done := make(chan string, 1)
	go func() { done <- h.ok("connect", "all") }()
	waitFor(t, "both to be asked", func() bool { return asks(a) >= 1 && asks(b) >= 1 })
	_, _ = io.WriteString(typing, "not a command\nQ\n")
	select {
	case out := <-done:
		want(t, out, "still not connected: Light Panels AA", "Light Panels BB")
		if strings.Contains(out, "connected 1") || len(h.saved()) != 0 {
			t.Errorf("stopped before any button was held:\n%s", out)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("q did not stop connect all")
	}

	// with nobody at a terminal, and a time to stop at: the one that is ready connects under its own name.
	// Nobody is a file, as a script's input is: a reader that is not a file counts as a person, and its
	// end would race the first token for whether a name gets asked for
	nobody, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = nobody.Close() }()
	h.stdin = nobody
	settled(t, a, b) // a request from the run just stopped must not be the one that takes the token
	a.Pair()
	out := h.ok("connect", "all", "--wait", "400ms")
	want(t, out, "stops after 400ms", "connected Light Panels AA", "connected 1: light-panels-aa", "still not connected: Light Panels BB")
	if strings.Contains(out, "what should") {
		t.Errorf("a question was asked of nobody:\n%s", out)
	}
	if !h.named("light-panels-aa") || len(h.saved()) != 1 {
		t.Fatalf("saved: %+v", h.saved())
	}

	// and when the time runs out with nothing connected, that is an error
	want(t, h.fails("connect", "all", "--wait", "100ms"), "no controller handed out a token in 100ms")

	// for a script: what connected, as a list, with no questions
	settled(t, a, b)
	b.Pair()
	var connected []cli.Connected
	// the wait is only how long it would go on asking: it connects the moment BB answers, which under a loaded
	// test run can be later than a second or two
	if err := json.Unmarshal([]byte(h.ok("connect", "all", "--wait", "15s", "--json")), &connected); err != nil || len(connected) != 1 || connected[0].Name != "light-panels-bb" {
		t.Errorf("connect all --json: %+v, %v", connected, err)
	}
}

func TestConnectAllRefuses(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	a, foundA := unconnected(t, "AA")
	a.Pair()

	restoreNone := cli.SetSearch()
	want(t, h.fails("connect", "all"), "found no controllers")
	restoreNone()

	restoreSearch := cli.SetSearch(foundA)
	defer restoreSearch()
	want(t, h.fails("connect", "all", "--name", "office"), "--name and --token-file are for connecting one")

	// a dry run says what it would ask and asks nobody
	want(t, h.ok("connect", "all", "--dry-run"), "1 not connected yet", "Light Panels AA", "dry run:", "POST /api/v1/new")
	if asks(a) != 0 || len(h.saved()) != 0 {
		t.Errorf("a dry run asked %d times and saved %d", asks(a), len(h.saved()))
	}

	// a controller that has gone since it was found is said so, and the rest go on
	a.Unplug()
	want(t, h.ok("connect", "all", "--wait", "300ms", "--timeout", "1s"), "cannot reach the controller at "+a.Host(), "still not connected: Light Panels AA")
}

func TestConnectGivesUp(t *testing.T) { //nolint:paralleltest // the commands share viper
	restore := cli.SetAskEvery(10 * time.Millisecond)
	defer restore()
	h := newHome(t)
	ctl := auroratest.New(t)

	want(t, h.fails("connect", ctl.Host(), "--wait", "150ms"), "no token after 150ms", "until the light flashes")
	if len(h.saved()) != 0 {
		t.Error("something was saved")
	}

	// nothing at the address at all: no button will fix that, so it says so at once
	srv := httptest.NewServer(http.NotFoundHandler())
	gone := strings.TrimPrefix(srv.URL, "http://")
	srv.Close()
	start := time.Now()
	want(t, h.fails("connect", gone, "--wait", "1m", "--timeout", "2s"), "cannot reach the controller at "+gone)
	if time.Since(start) > 20*time.Second {
		t.Errorf("an unreachable controller took %s to say so", time.Since(start))
	}
}

func TestConnectWithATokenInHand(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	ctl := auroratest.New(t)
	token := ctl.Token()

	// as a controller sends it, and alone
	for name, body := range map[string]string{"wrapped.json": `{"auth_token":"` + token + `"}`, "bare.txt": token + "\n"} {
		file := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		want(t, h.ok("connect", ctl.Host(), "--token-file", file), "connected:", "light-panels-53-a6-3c")
	}
	if saved := h.saved(); len(saved) != 1 || saved[0].Token != token {
		t.Fatalf("saved %d controllers", len(saved))
	}
	// no token was asked for
	for _, r := range ctl.Requests() {
		if r.Path == "/new" {
			t.Error("a token was asked for with one in hand")
		}
	}

	bad := filepath.Join(t.TempDir(), "bad")
	for body, msg := range map[string]string{
		"not-a-token-the-controller-knows": "does not know the token in",
		"":                                 "does not hold a token",
		`{"something":"else"}`:             "does not hold a token",
	} {
		if err := os.WriteFile(bad, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		want(t, h.fails("connect", ctl.Host(), "--token-file", bad), msg)
	}
	want(t, h.fails("connect", ctl.Host(), "--token-file", filepath.Join(t.TempDir(), "missing")), "reading the token file")
}

func TestConnectDryRun(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	ctl := auroratest.New(t)
	ctl.Pair()

	want(t, h.ok("connect", ctl.Host(), "--dry-run"), "dry run:", "POST /api/v1/new")
	if len(h.saved()) != 0 || len(ctl.Requests()) != 0 {
		t.Errorf("a dry run saved %d controllers and sent %d requests", len(h.saved()), len(ctl.Requests()))
	}
}

// find says how it is looking while it looks, since a few seconds of nothing
// leaves a person wondering what is being waited for.
func TestFindSaysWhatItIsDoing(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)

	// what is on the network is not this test's to say; what find says it is doing is
	want(t, h.ok("find", "--wait", "200ms"), "searching for controllers for 200ms", "(mDNS, _nanoleafapi._tcp)")
	want(t, h.ok("search", "--wait", "200ms", "--scan", "127.0.0.1/32"), "knocking on port 16021 at every address of 127.0.0.1/32")
	want(t, h.fails("find", "--wait", "200ms", "--scan", "everywhere"), "is not a subnet like 10.0.5.0/24")

	// for a script: a list, and nothing else
	var found []cli.FoundController
	if err := json.Unmarshal([]byte(h.ok("find", "--wait", "200ms", "--json")), &found); err != nil {
		t.Errorf("find --json: %v", err)
	}
}

func TestListAndInfo(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	want(t, h.ok("list"), "no controllers yet", "taproot connect")

	h.controller("office")
	gone := h.controller("bedroom")
	gone.Unplug()

	// each under taproot's name for it, with the name it gives itself beside it; one that is gone still has the name it gave
	out := h.ok("list", "--timeout", "2s")
	want(t, out, "NAME", "CALLS ITSELF", "on 33%", northern, "unreachable")
	for line := range strings.SplitSeq(out, "\n") {
		for _, name := range []string{"office", "bedroom"} {
			if strings.HasPrefix(line, name+" ") && !strings.Contains(line, "Light Panels "+name) {
				t.Errorf("%s is listed without its own name: %q", name, line)
			}
		}
	}

	var listed []cli.Status
	if err := json.Unmarshal([]byte(h.ok("list", "--json", "--timeout", "2s")), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].Name != "bedroom" || listed[0].Reachable || listed[0].Error == "" {
		t.Errorf("the one that is gone: %+v", listed)
	}
	if o := listed[1]; o.Name != "office" || !o.Reachable || !o.On || o.Brightness != 33 || o.Running != northern || o.Panels != 4 || o.Scenes != 17 {
		t.Errorf("office: %+v", o)
	}

	want(t, h.ok("info", "office"), "calls itself  Light Panels office", "model", "NL22 (Nanoleaf)", "firmware", "5.2.1",
		"panels", "4, turned 88°", "on, brightness 33%", "kt Northern Lights (effect mode)", "listening to the microphone", "scenes", "17: Color Burst")
	var info struct {
		Name   string      `json:"name"`
		Device aurora.Info `json:"device"`
	}
	if err := json.Unmarshal([]byte(h.ok("info", "office", "--json")), &info); err != nil || info.Name != "office" ||
		info.Device.Name != "Light Panels office" || info.Device.FirmwareVersion != "5.2.1" || info.Device.State.Brightness.Value != 33 {
		t.Errorf("info --json: %+v, %v", info, err)
	}

	want(t, h.fails("info", "kitchen"), `no controller matches "kitchen"`)
	want(t, h.fails("info", "bedroom", "--timeout", "2s"), "GET /")
}

func TestRename(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("light-panels-53-a6-3c")
	h.controller("bedroom")
	h.ok("backup", "53a6")

	want(t, h.ok("rename", "53a6", "Office", "--dry-run"), "dry run:", "would rename", "light-panels-53-a6-3c", "office")
	if h.saved()[1].Name != "light-panels-53-a6-3c" {
		t.Fatal("a dry run renamed the controller")
	}

	// by any part of what it is known by, to a name tidied for the command line
	out := h.ok("rename", "53a6", "Office")
	want(t, out, "light-panels-53-a6-3c", "is now", "office",
		"its earlier backups stay in "+filepath.Join(h.dir, "backups", "light-panels-53-a6-3c"), "new ones go in "+filepath.Join(h.dir, "backups", "office"))
	saved := h.saved()
	if len(saved) != 2 || saved[1].Name != "office" || saved[1].Token == "" || saved[1].Host != office.Host() {
		t.Fatalf("saved: %+v", saved)
	}
	// it answers to the new name, and nothing was said to the controller about it
	want(t, h.ok("info", "office"), "name          office", "calls itself  Light Panels light-panels-53-a6-3c")
	if len(office.Writes()) != 0 {
		t.Errorf("renaming wrote to the controller: %+v", office.Writes())
	}
	// the backup taken before is where it was
	if list, err := backup.List(filepath.Join(h.dir, "backups")); err != nil || len(list) != 1 || list[0].Controller != "light-panels-53-a6-3c" {
		t.Errorf("the earlier backup: %+v, %v", list, err)
	}

	want(t, h.ok("rename", "office", "office"), "is already called that")
	var renamed cli.Renamed
	if err := json.Unmarshal([]byte(h.ok("name", "office", "study", "--json")), &renamed); err != nil || renamed.From != "office" || renamed.To != "study" {
		t.Errorf("rename --json: %+v, %v", renamed, err)
	}

	want(t, h.fails("rename", "study", "bedroom"), "bedroom is already what Light Panels bedroom is called")
	want(t, h.fails("rename", "study", "!!"), "at least one letter or digit")
	want(t, h.fails("rename", "kitchen", "pantry"), `no controller matches "kitchen"`)
}

func TestGet(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	want(t, h.fails("get", "all"), "no controllers yet")

	office := h.controller("office")
	out := h.ok("get", "office")
	want(t, out, "office", "power        on", "brightness   33", "scene        kt Northern Lights", "hue          0", "saturation   0",
		"temperature  3000", "orientation  88", "rhythm       microphone", "mode         effect", "0 to 100", "read only")

	// one setting is its value and nothing else, for a script, whatever the output is turned down to
	if got := h.ok("get", "office", "brightness"); got != "33\n" {
		t.Errorf("get brightness printed %q", got)
	}
	if got := h.ok("get", "office", "ct", "--quiet"); got != "3000\n" {
		t.Errorf("get ct --quiet printed %q", got)
	}
	if got := h.ok("get", "office", "SCENE"); got != northern+"\n" {
		t.Errorf("get scene printed %q", got)
	}

	var all []cli.Settings
	if err := json.Unmarshal([]byte(h.ok("get", "office", "--json")), &all); err != nil || len(all) != 1 || len(all[0].Settings) != 9 {
		t.Fatalf("get --json: %+v, %v", all, err)
	}
	if s := all[0].Settings; s["power"] != "on" || s["brightness"] != float64(33) || s["scene"] != northern || s["rhythm"] != "microphone" {
		t.Errorf("settings: %+v", s)
	}
	var one []cli.Settings
	if err := json.Unmarshal([]byte(h.ok("get", "office", "power", "--json")), &one); err != nil || len(one[0].Settings) != 1 || one[0].Settings["power"] != "on" {
		t.Errorf("get power --json: %+v, %v", one, err)
	}

	// every controller, one to a line; one that is gone says so and the rest are still read
	gone := h.controller("bedroom")
	gone.Unplug()
	out, err := h.run("get", "all", "--timeout", "2s")
	if err == nil {
		t.Error("a controller that could not be read should be an error")
	}
	want(t, out, "NAME", "BRIGHTNESS", "SCENE", "office", "33", northern, "bedroom", "?")

	want(t, h.fails("get", "office", "volume"), `no setting called "volume"`, "power, brightness, scene")
	want(t, h.fails("get", "kitchen"), `no controller matches "kitchen"`)
	if len(office.Writes()) != 0 {
		t.Error("get wrote to the controller")
	}
}

func TestSet(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office")
	state := office.State

	// to a value, and what is reported is what the controller says afterwards
	want(t, h.ok("set", "office", "brightness", "50"), "office: brightness 33 → 50")
	if state().Brightness.Value != 50 {
		t.Fatalf("brightness is %d", state().Brightness.Value)
	}
	want(t, h.ok("set", "office", "brightness", "50"), "office: brightness is 50")

	// by an amount, down as well as up, stopping at the limit
	want(t, h.ok("set", "office", "brightness", "--by", "-10"), "50 → 40")
	want(t, h.ok("set", "office", "brightness", "--by=500"), "40 → 100")
	// over a time
	h.ok("set", "office", "brightness", "20", "--fade", "3s")
	if w := office.Writes(); w[len(w)-1].Body != `{"brightness":{"duration":3,"value":20}}` {
		t.Errorf("a fade sent %s", w[len(w)-1].Body)
	}

	// a value the controller would refuse is not sent, and the limits are said
	before := len(office.Writes())
	want(t, h.fails("set", "office", "brightness", "101"), "brightness is 0 to 100 on this controller, not 101")
	want(t, h.fails("set", "office", "brightness", "bright"), `brightness is a whole number from 0 to 100, not "bright"`)
	want(t, h.fails("set", "office", "temperature", "100"), "temperature is 1200 to 6500")
	if len(office.Writes()) != before {
		t.Error("a value out of range was sent")
	}

	want(t, h.ok("set", "office", "power", "off"), "office: power on → off")
	want(t, h.ok("set", "office", "power", "toggle"), "power off → on")
	want(t, h.ok("set", "office", "power", "toggle"), "power on → off")
	want(t, h.fails("set", "office", "power", "maybe"), `power is on, off or toggle, not "maybe"`)

	want(t, h.ok("set", "office", "scene", "Flames"), "office: scene kt Northern Lights → Flames")
	if office.Selected() != "Flames" {
		t.Errorf("running %q", office.Selected())
	}
	want(t, h.fails("set", "office", "scene", "Nope"), `no scene called "Nope"`, "it has Color Burst")

	// one colour, in place of the scene, and the mode follows
	want(t, h.ok("set", "office", "hue", "120"), "hue 0 → 120")
	want(t, h.ok("set", "office", "sat", "80"), "saturation 0 → 80")
	if got := h.ok("get", "office", "mode"); got != "hs\n" {
		t.Errorf("after a colour the mode is %q", got)
	}
	want(t, h.ok("set", "office", "hue", "--by", "30"), "120 → 150")
	want(t, h.ok("set", "office", "kelvin", "4000"), "temperature 3000 → 4000")
	if got := h.ok("get", "office", "mode"); got != "ct\n" {
		t.Errorf("after a white the mode is %q", got)
	}
	want(t, h.ok("set", "office", "orientation", "90"), "orientation 88 → 90")
	want(t, h.ok("set", "office", "rhythm", "aux"), "rhythm microphone → aux")
	want(t, h.ok("set", "office", "source", "mic"), "rhythm aux → microphone")
	want(t, h.fails("set", "office", "rhythm", "radio"), "rhythm is microphone or aux")

	for _, bad := range []struct {
		args []string
		msg  string
	}{
		{[]string{"set", "office", "mode", "hs"}, "mode is not set itself"},
		{[]string{"set", "office", "volume", "11"}, `no setting called "volume"`},
		{[]string{"set", "office", "brightness"}, "set brightness to what? it is 0 to 100"},
		{[]string{"set", "office", "brightness", "10", "--by", "5"}, "not both"},
		{[]string{"set", "office", "power", "--by", "1"}, "power is not a number to move with --by"},
		{[]string{"set", "office", "hue", "10", "--fade", "2s"}, "--fade is for brightness"},
		{[]string{"set", "kitchen", "power", "on"}, `no controller matches "kitchen"`},
	} {
		want(t, h.fails(bad.args...), bad.msg)
	}

	// a dry run says what it would send and sends none of it
	before = len(office.Writes())
	want(t, h.ok("set", "office", "brightness", "5", "--dry-run"), "dry run:", "PUT /api/v1/<token>/state", `{"brightness":{"value":5}}`)
	if len(office.Writes()) != before || state().Brightness.Value != 20 {
		t.Error("a dry run changed the controller")
	}

	// every controller at once; one that is gone is said, and the rest are still set
	bedroom := h.controller("bedroom")
	gone := h.controller("hall")
	gone.Unplug()
	out, err := h.run("set", "all", "brightness", "15", "--timeout", "2s")
	if err == nil {
		t.Error("a controller that could not be set should be an error")
	}
	want(t, out, "office: brightness 20 → 15", "bedroom: brightness 33 → 15", "hall:")
	if state().Brightness.Value != 15 || bedroom.State().Brightness.Value != 15 {
		t.Error("set all did not reach every controller")
	}

	var changed []cli.Changed
	if err := json.Unmarshal([]byte(h.ok("set", "bedroom", "power", "off", "--json")), &changed); err != nil || len(changed) != 1 ||
		changed[0].Controller != "bedroom" || changed[0].Setting != "power" || changed[0].From != "on" || changed[0].To != "off" {
		t.Errorf("set --json: %+v, %v", changed, err)
	}
}

func TestForget(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office")
	bedroom := h.controller("bedroom")
	away := h.controller("away")
	away.Unplug()
	token := h.saved()[0].Token

	want(t, h.ok("forget", "office", "--dry-run"), "dry run:", "DELETE /api/v1/<token>", "would remove")
	if len(h.saved()) != 3 {
		t.Fatal("a dry run forgot a controller")
	}

	want(t, h.ok("forget", "office"), "forgot", "the controller has deleted the token")
	if len(h.saved()) != 2 {
		t.Fatalf("still saved: %+v", h.saved())
	}
	if w := office.Writes(); len(w) != 1 || w[0].Method != http.MethodDelete {
		t.Errorf("what the controller was asked: %+v", w)
	}

	// one that cannot be reached is not forgotten by accident, and can be on purpose
	want(t, h.fails("forget", "away", "--timeout", "2s"), "could not have away delete the token", "--local")
	if len(h.saved()) != 2 {
		t.Fatal("a controller that could not be reached was forgotten")
	}
	want(t, h.ok("forget", "away", "--local"), "still valid on the controller")

	// --local leaves the controller alone
	want(t, h.ok("forget", "bedroom", "--local"), "forgot")
	if len(h.saved()) != 0 || len(bedroom.Writes()) != 0 {
		t.Errorf("saved %d, bedroom was asked %+v", len(h.saved()), bedroom.Writes())
	}
	_ = token
}

func TestBackupAndRestore(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	want(t, h.fails("backup"), "no controllers yet")

	office := h.controller("office")
	bedroom := h.controller("bedroom")
	bedroom.RemoveEffect(northern)
	root := filepath.Join(h.dir, "backups")

	// one, into the dated directory
	want(t, h.ok("backup", "office"), "backed up", "17 scenes in "+filepath.Join(root, "office"))
	// all, and one into a directory of its own choosing
	want(t, h.ok("backup", "--backup-dir", filepath.Join(h.dir, "elsewhere")), "office", "bedroom", "16 scenes")
	mine := filepath.Join(t.TempDir(), "mine")
	want(t, h.ok("backup", "office", mine), "17 scenes in "+mine)
	want(t, h.fails("backup", "office", mine), "1 of 1 controllers were not backed up")
	want(t, h.ok("backup", "office", "--dry-run"), "dry run:", "would back up")
	if len(office.Writes())+len(bedroom.Writes()) != 0 {
		t.Error("a backup wrote to a controller")
	}

	list, err := backup.List(root)
	if err != nil || len(list) != 1 || list[0].Controller != "office" {
		t.Fatalf("backups under the default root: %+v, %v", list, err)
	}

	// the office's backup onto the bedroom: the one scene it lacks is added, the rest are left alone
	out := h.ok("restore", "bedroom", mine, "--dry-run")
	want(t, out, "restoring 17 scenes taken from Light Panels office", "would first back it up", "would be added", northern, "unchanged", "Flames",
		"PUT /api/v1/<token>/effects", `{"write":{"command":"add","version":"2.0","animName":"kt Northern Lights"`)
	if _, ok := bedroom.Effect(northern); ok {
		t.Fatal("a dry run restored the scene")
	}

	out = h.ok("restore", "bedroom", mine)
	want(t, out, "bedroom", "backed up to "+filepath.Join(root, "bedroom"), "added", northern)
	if e, ok := bedroom.Effect(northern); !ok {
		t.Fatal("the scene was not restored")
	} else if src, _ := office.Effect(northern); !e.Equal(src) {
		t.Error("the scene was restored changed")
	}
	if w := bedroom.Writes(); len(w) != 1 {
		t.Errorf("the restore wrote %d times, want once: %+v", len(w), w)
	}

	// a second time there is nothing to do
	want(t, h.ok("restore", "bedroom", mine), "nothing to write")

	// a scene changed on the controller since is not replaced without being told to
	changed, _ := bedroom.Effect("Flames")
	changed, _ = changed.With("palette", []aurora.Color{{Hue: 1}})
	bedroom.PutEffect(changed)
	want(t, h.fails("restore", "bedroom", mine), `"Flames" refused`, "--force")
	var report struct {
		Results []struct {
			Scene   string `json:"scene"`
			Outcome string `json:"outcome"`
		} `json:"results"`
	}
	out, err = h.run("restore", "bedroom", mine, "--force", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil || len(report.Results) != 17 {
		t.Fatalf("restore --json: %v\n%s", err, out)
	}
	replaced := 0
	for _, r := range report.Results {
		if r.Outcome == "replaced" && r.Scene == "Flames" {
			replaced++
		}
	}
	if replaced != 1 {
		t.Errorf("Flames was not replaced: %+v", report.Results)
	}

	want(t, h.fails("restore", "bedroom", t.TempDir()), "is not a backup")
}

func TestSceneListAndCompare(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	want(t, h.fails("scene", "list"), "no controllers yet")

	h.controller("office")
	bedroom := h.controller("bedroom")
	bedroom.RemoveEffect(northern)
	changed, _ := bedroom.Effect("Flames")
	changed, _ = changed.With("palette", []aurora.Color{{Hue: 1}})
	bedroom.PutEffect(changed)

	out := h.ok("scene", "list", "office")
	want(t, out, "holds 17 scenes", "▶  kt Northern Lights", "color", "Wheel", "7 colours", "Fireworks", "rhythm")

	var listed []scene.Listed
	if err := json.Unmarshal([]byte(h.ok("scene", "list", "office", "--json")), &listed); err != nil || len(listed) != 17 {
		t.Fatalf("scene list --json: %v", err)
	}
	if last := listed[16]; last.Name != northern || !last.Running || last.Plugin != "Wheel" || last.Kind != "color" || last.Colours != 7 {
		t.Errorf("the running scene: %+v", last)
	}

	// the bedroom is on newer firmware, which adds a field of its own to every scene: Forest is still the same scene
	forest, _ := bedroom.Effect("Forest")
	forest, _ = forest.With("rhythmFeatureSource", 1)
	bedroom.PutEffect(forest)

	// side by side: the one the bedroom lost, and the one it holds differently
	out = h.ok("scene", "list")
	want(t, out, "SCENE", "bedroom", "office")
	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, northern):
			if f := strings.Fields(strings.TrimPrefix(line, northern)); len(f) != 2 || f[0] != "-" || f[1] != "yes" {
				t.Errorf("the lost scene: %q", line)
			}
		case strings.HasPrefix(line, "Flames"):
			want(t, line, "differs between controllers")
		case strings.HasPrefix(line, "Forest"):
			if strings.Contains(line, "differs") {
				t.Errorf("Forest is the same on both: %q", line)
			}
		}
	}

	var compared []scene.Compared
	if err := json.Unmarshal([]byte(h.ok("scene", "ls", "--json")), &compared); err != nil || len(compared) != 17 {
		t.Fatalf("scene list --json: %v", err)
	}
	for _, c := range compared {
		switch c.Name {
		case northern:
			if c.On["bedroom"] || !c.On["office"] || c.Differs {
				t.Errorf("%s: %+v", c.Name, c)
			}
		case "Flames":
			if !c.Differs || !c.On["bedroom"] || !c.On["office"] {
				t.Errorf("%s: %+v", c.Name, c)
			}
		}
	}

	bedroom.Unplug()
	want(t, h.ok("scene", "list", "--timeout", "2s"), "bedroom could not be read", "?")
}

func TestSceneDump(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office")

	out := h.ok("scene", "dump", "office", northern)
	e, err := aurora.ParseEffect([]byte(out))
	if held, _ := office.Effect(northern); err != nil || !e.Equal(held) {
		t.Fatalf("the dump is not the scene: %v\n%s", err, out)
	}
	want(t, out, "{\n  \"version\": \"2.0\",\n  \"animName\": \"kt Northern Lights\"", `"probability": 0.0`)

	var all struct {
		Animations []aurora.Effect `json:"animations"`
	}
	if err := json.Unmarshal([]byte(h.ok("scene", "dump", "office")), &all); err != nil || len(all.Animations) != 17 {
		t.Fatalf("a dump of everything: %d scenes, %v", len(all.Animations), err)
	}

	file := filepath.Join(t.TempDir(), "scene.json")
	want(t, h.ok("scene", "dump", "office", northern, "--out", file), "wrote "+file)
	read := func() string {
		t.Helper()
		data, err := os.ReadFile(file) //nolint:gosec // the file this test had written
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if read() != out {
		t.Error("the file differs from what is printed")
	}
	// a file that is there is not written over
	want(t, h.fails("scene", "dump", "office", "Flames", "-o", file), "is already there", "--force")
	if read() != out {
		t.Error("the file was written over")
	}
	h.ok("scene", "dump", "office", "Flames", "-o", file, "--force")
	if !strings.Contains(read(), `"animName": "Flames"`) {
		t.Error("--force did not write over the file")
	}

	want(t, h.fails("scene", "dump", "office", "Nope"), `no scene called "Nope"`, "it has Color Burst, Fireworks")
	if len(office.Writes()) != 0 {
		t.Error("a dump wrote to the controller")
	}
}

func TestScenePush(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office")
	bedroom := h.controller("bedroom")
	bedroom.RemoveEffect(northern)
	bedroom.RemoveEffect("Flames")
	dir := t.TempDir()

	one := filepath.Join(dir, "one.json")
	h.ok("scene", "dump", "office", northern, "-o", one)
	everything := filepath.Join(dir, "all.json")
	h.ok("scene", "dump", "office", "-o", everything)

	// one scene, under another name, and started
	out := h.ok("scene", "push", "bedroom", one, "--as", "mine", "--select")
	want(t, out, "backed up to", "added", "mine")
	if _, ok := bedroom.Effect(northern); ok {
		t.Error("--as also stored the scene under its own name")
	}
	if e, ok := bedroom.Effect("mine"); !ok || e.PluginUUID() == "" || bedroom.Selected() != "mine" {
		t.Errorf("mine: there %v, running %q", ok, bedroom.Selected())
	}

	// every scene of a controller: the two it lacks are added and the rest left alone
	out = h.ok("scene", "push", "bedroom", everything)
	want(t, out, "added      Flames", "added      kt Northern Lights", "unchanged  Forest")
	if len(bedroom.EffectNames()) != 18 {
		t.Errorf("the bedroom now holds %d scenes, want 18", len(bedroom.EffectNames()))
	}

	// a plain list of scenes is taken too
	held, _ := office.Effect("Forest")
	list, _ := json.Marshal([]aurora.Effect{held.WithName("listed")})
	asList := filepath.Join(dir, "list.json")
	if err := os.WriteFile(asList, list, 0o600); err != nil {
		t.Fatal(err)
	}
	want(t, h.ok("scene", "push", "bedroom", asList), "added", "listed")

	for name, c := range map[string]struct{ body, msg string }{
		"empty.json":    {`{"animations":[]}`, "holds no scenes"},
		"nameless.json": {`{"palette":[]}`, "has no name"},
		"broken.json":   {`{`, "reading an effect"},
		"brokenlist":    {`[{]`, "brokenlist"},
		"notalist.json": {`{"animations":7}`, "notalist.json"},
	} {
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, []byte(c.body), 0o600); err != nil {
			t.Fatal(err)
		}
		want(t, h.fails("scene", "push", "bedroom", file), c.msg)
	}
	want(t, h.fails("scene", "push", "bedroom", filepath.Join(dir, "missing.json")), "reading the scene file")
	want(t, h.fails("scene", "push", "bedroom", everything, "--as", "x"), "--as names one scene", "holds 17")
}

func TestSceneCopy(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office")
	bedroom := h.controller("bedroom")
	hall := h.controller("hall")
	bedroom.RemoveEffect(northern)
	hall.RemoveEffect(northern)

	want(t, h.fails("scene", "copy", northern, "--to", "bedroom"), "--from says which controller has the scene")
	want(t, h.fails("scene", "copy", northern, "--from", "office"), "say where to copy it")
	want(t, h.fails("scene", "copy", northern, "--from", "office", "--to", "bedroom", "--to-all"), "say where to copy it")
	want(t, h.fails("scene", "copy", northern, "--from", "kitchen", "--to", "bedroom"), `--from: no controller matches "kitchen"`)
	want(t, h.fails("scene", "copy", northern, "--from", "office", "--to", "kitchen"), `--to: no controller matches "kitchen"`)
	want(t, h.fails("scene", "copy", northern, "--from", "office", "--to", "office"), "copying it onto itself needs --as")
	want(t, h.fails("scene", "copy", "Nope", "--from", "office", "--to", "bedroom"), `office: the controller has no scene called "Nope"`)

	// a dry run says exactly what it would send, and sends none of it
	out := h.ok("scene", "copy", northern, "--from", "office", "--to", "bedroom", "--dry-run")
	want(t, out, "copying", "would first back it up", "dry run:", "would send to", "PUT /api/v1/<token>/effects", `{"write":{"command":"add","version":"2.0","animName":"kt Northern Lights","animType":"plugin"`, "would be added")
	if _, ok := bedroom.Effect(northern); ok || len(bedroom.Writes()) != 0 {
		t.Fatal("a dry run copied the scene")
	}
	var dry struct {
		Result []struct {
			DryRun bool `json:"dryRun"`
		} `json:"result"`
		WouldSend []cli.WouldSend `json:"wouldSend"`
	}
	if err := json.Unmarshal([]byte(h.ok("scene", "copy", northern, "--from", "office", "--to", "bedroom", "--dry-run", "--json")), &dry); err != nil {
		t.Fatal(err)
	}
	if len(dry.Result) != 1 || !dry.Result[0].DryRun || len(dry.WouldSend) != 1 || dry.WouldSend[0].Controller != "bedroom" || dry.WouldSend[0].Method != http.MethodPut {
		t.Errorf("the dry run's document: %+v", dry)
	}

	// to every other controller, and started there
	out = h.ok("scene", "copy", northern, "--from", "office", "--to-all", "--select")
	want(t, out, "bedroom", "hall", "added")
	src, _ := office.Effect(northern)
	for name, ctl := range map[string]*auroratest.Controller{"bedroom": bedroom, "hall": hall} {
		if e, ok := ctl.Effect(northern); !ok || !e.Equal(src) {
			t.Errorf("%s does not hold the scene as the office does", name)
		}
		if ctl.Selected() != northern {
			t.Errorf("%s is running %q", name, ctl.Selected())
		}
	}
	// the controller it came from was only ever read
	if w := office.Writes(); len(w) != 0 {
		t.Errorf("the source was written to: %+v", w)
	}

	// again: nothing to do
	want(t, h.ok("scene", "copy", northern, "--from", "office", "--to", "bedroom,hall"), "nothing to write", "unchanged")

	// a copy of the scene beside itself, under another name
	want(t, h.ok("scene", "cp", northern, "--from", "office", "--to", "office", "--as", "a copy"), "added", "a copy")
	if _, ok := office.Effect("a copy"); !ok {
		t.Error("the copy is not on the office")
	}

	// a controller holding something else under the name is refused, and the others still get theirs
	other, _ := hall.Effect("Flames")
	hall.PutEffect(other.WithName("Forest"))
	bedroom.RemoveEffect("Forest")
	msg := h.fails("scene", "copy", "Forest", "--from", "office", "--to-all")
	want(t, msg, `hall: "Forest" refused`, "--force")
	if _, ok := bedroom.Effect("Forest"); !ok {
		t.Error("the bedroom did not get its copy because the hall refused")
	}
	want(t, h.ok("scene", "copy", "Forest", "--from", "office", "--to", "hall", "--force"), "replaced", "Forest")

	// a controller that is off does not stop the others
	hall.Unplug()
	bedroom.RemoveEffect(northern)
	msg = h.fails("scene", "copy", northern, "--from", "office", "--to-all", "--timeout", "2s")
	want(t, msg, "hall:")
	if _, ok := bedroom.Effect(northern); !ok {
		t.Error("the bedroom did not get its copy because the hall was off")
	}
}

func TestSceneDelete(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office") // running kt Northern Lights, holding 17

	want(t, h.fails("scene", "delete", "office"), "say which scenes to delete")
	want(t, h.fails("scene", "delete", "office", "Flames", "--except", "Forest"), "not both")

	// without being told to, nothing goes
	want(t, h.fails("scene", "delete", "office", "Flames"), "Flames", "not deleted without --force")
	if len(office.EffectNames()) != 17 || len(office.Writes()) != 0 {
		t.Fatal("a scene was deleted without --force")
	}

	// a dry run says what it would send
	out := h.ok("scene", "delete", "office", "Flames", "--force", "--dry-run")
	want(t, out, "would first back it up", "would be deleted", `{"write":{"command":"delete","animName":"Flames"}}`)
	if len(office.EffectNames()) != 17 {
		t.Fatal("a dry run deleted a scene")
	}

	out = h.ok("scene", "rm", "office", "Flames", "Forest", "--force")
	want(t, out, "backed up to "+filepath.Join(h.dir, "backups", "office"), "deleted    Flames", "deleted    Forest")
	if names := office.EffectNames(); len(names) != 15 || slices.Contains(names, "Flames") {
		t.Fatalf("left: %v", names)
	}

	// down to the one that is wanted: everything but it, and it is the one running, so it stays
	want(t, h.fails("scene", "delete", "office", "--except", "Northern Light", "--force"), `--except: office: the controller has no scene called "Northern Light"`, "nothing was deleted", "it has Color Burst")
	want(t, h.fails("scene", "delete", "office", "Nemo", "Nemoo", "--force"), `no scene called "Nemoo"`, "nothing was deleted")
	if len(office.EffectNames()) != 15 {
		t.Fatal("a slip in the name to keep deleted scenes")
	}
	out = h.ok("scene", "delete", "office", "--except", northern, "--force")
	want(t, out, "deleted    Color Burst", "deleted    Streaking Notes")
	if names := office.EffectNames(); len(names) != 1 || names[0] != northern {
		t.Fatalf("left: %v", names)
	}
	want(t, h.ok("scene", "delete", "office", "--except", northern, "--force"), "holds nothing but the scenes to keep")

	// the one that is running is never deleted from under the controller
	want(t, h.fails("scene", "delete", "office", northern, "--force"), "it is the scene that is running")

	// and what went can be put back from the backup taken before it did
	list, err := backup.List(filepath.Join(h.dir, "backups"))
	if err != nil || len(list) != 2 || list[0].Scenes != 15 || list[1].Scenes != 17 {
		t.Fatalf("backups, newest first: %+v, %v", list, err)
	}
	want(t, h.ok("restore", "office", list[1].Dir), "added      Flames", "added      Forest")
	if len(office.EffectNames()) != 17 {
		t.Errorf("after restoring: %v", office.EffectNames())
	}
}

func TestSceneRename(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office") // running kt Northern Lights, holding 17

	// the old name is matched as the controller spells it; a name it has not got stops here
	want(t, h.fails("scene", "rename", "office", "Flamez", "Fire"), `no scene called "Flamez"`)
	// a name another scene has, and the running scene, are refused
	want(t, h.fails("scene", "rename", "office", "Flames", "Forest"), "refused", `already holds a scene called "Forest"`)
	want(t, h.fails("scene", "rename", "office", northern, "Fire"), "refused", "it is the scene that is running")
	if len(office.Writes()) != 0 {
		t.Fatal("a scene was renamed that should not have been")
	}

	// a dry run says what it would send
	out := h.ok("scene", "rename", "office", "flames", "Fire", "--dry-run")
	want(t, out, "would first back it up", "would be renamed Flames → Fire", `{"write":{"command":"rename","animName":"Flames","newName":"Fire"}}`)
	if held := office.EffectNames(); !slices.Contains(held, "Flames") || len(office.Writes()) != 0 {
		t.Fatal("a dry run renamed a scene")
	}

	out = h.ok("scene", "mv", "office", "flames", "Fire")
	want(t, out, "backed up to "+filepath.Join(h.dir, "backups", "office"), "renamed    Flames → Fire")
	if held := office.EffectNames(); len(held) != 17 || slices.Contains(held, "Flames") || !slices.Contains(held, "Fire") {
		t.Fatalf("held: %v", held)
	}
}

// What the app does that the documentation does not list: firmware, the
// settings a controller does not say back, and painting panels.

func TestFirmware(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office")

	// a controller that has not heard from the cloud
	want(t, h.ok("firmware", "office"), "office", "5.2.1", "has not heard of an update", "{}")
	office.SetFirmwareUpgrade(false, "")
	want(t, h.ok("firmware", "office"), "no update waiting")
	office.SetFirmwareUpgrade(true, "5.3.3")
	want(t, h.ok("firmware", "office"), "update to 5.3.3 available, not installed", "taproot firmware trigger office")
	if office.FirmwareTriggers() != 0 || len(office.Writes()) != 0 {
		t.Fatal("asking about firmware sent something")
	}

	var status []cli.FirmwareStatus
	if err := json.Unmarshal([]byte(h.ok("firmware", "office", "--json")), &status); err != nil || len(status) != 1 || !status[0].Available || status[0].NewVersion != "5.3.3" {
		t.Fatalf("--json: %+v, %v", status, err)
	}

	// the trigger: a dry run shows exactly what would go, bare as the app sends it, and the backup it would take first
	want(t, h.ok("firmware", "trigger", "office", "--dry-run"), "dry run", "PUT /api/v1/<token>/firmwareUpgrade", `{"command":"triggerFirmwareUpgrade"}`)
	if office.FirmwareTriggers() != 0 {
		t.Fatal("a dry run triggered an upgrade")
	}
	// sent and not watched: backed up first
	out := h.ok("firmware", "trigger", "office", "--wait", "0")
	want(t, out, "backed up to "+filepath.Join(h.dir, "backups", "office"), "told to fetch and install", "not watching", "taproot firmware office")
	if office.FirmwareTriggers() != 1 {
		t.Fatalf("triggers: %d", office.FirmwareTriggers())
	}

	// watched through: it goes quiet, comes back on the new firmware, and a scene it had is gone
	restore := cli.SetFirmwarePoll(20 * time.Millisecond)
	defer restore()
	go func() {
		time.Sleep(60 * time.Millisecond)
		office.Fail(http.MethodGet, "/", http.StatusServiceUnavailable)
		time.Sleep(120 * time.Millisecond)
		office.Set("firmwareVersion", "5.3.3")
		office.RemoveEffect("Flames")
		office.Fail(http.MethodGet, "/", 0)
	}()
	out = h.ok("firmware", "trigger", "office", "--wait", "10s")
	want(t, out, "gone quiet", "installing", "back on 5.3.3", "holds 16 scenes", "(had 17)", "gone since the upgrade:", "Flames", "taproot restore office")

	// a controller that will not take it
	office.Fail(http.MethodPut, "/firmwareUpgrade", http.StatusNotFound)
	want(t, h.fails("firmware", "trigger", "office", "--wait", "0"), "office", "did not take the trigger", "404")
}

func TestSetWhatTheControllerDoesNotSayBack(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office")

	want(t, h.ok("set", "office", "buttons", "off"), "office: buttons set to off", "does not say")
	if office.ButtonsEnabled() {
		t.Error("the buttons were not disabled")
	}
	want(t, h.ok("set", "office", "fade", "off"), "fade set to off")
	if office.SceneTransition() {
		t.Error("the fade was not turned off")
	}
	want(t, h.ok("set", "office", "plr", "on"), "recovery set to on")
	want(t, h.ok("set", "office", "recovery", "off"), "recovery set to off")
	if office.PowerLossRecovery() {
		t.Error("recovery was not turned off")
	}
	want(t, h.fails("set", "office", "buttons", "maybe"), `buttons is on or off, not "maybe"`)

	// they cannot be read, and get says so rather than guessing
	want(t, h.fails("get", "office", "buttons"), "buttons is set, not got")
	if out := h.ok("get", "office"); strings.Contains(out, "buttons") || strings.Contains(out, "recovery") {
		t.Errorf("get lists what it cannot read:\n%s", out)
	}
}

func TestScenePaint(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office") // panels 10, 136, 147, 234

	want(t, h.fails("scene", "paint", "office"), "panel=colour")
	want(t, h.fails("scene", "paint", "office", "10"), `"10" is not panel=colour`)
	want(t, h.fails("scene", "paint", "office", "10=mauve"), `"mauve" is not a colour`, "ff8800", "red")
	want(t, h.fails("scene", "paint", "office", "99=red"), `no panel "99"`, "[10 136 147 234]")
	if len(office.Writes()) != 0 {
		t.Fatal("a bad request was sent")
	}

	// shown, not saved: later pairs win
	out := h.ok("scene", "paint", "office", "all=off", "147=ff0000", "--over", "1.5s")
	want(t, out, "office: painted", "10 000000", "147 ff0000", "234 000000")
	w := office.Writes()
	if last := w[len(w)-1].Body; !strings.Contains(last, `"animType":"static"`) || !strings.Contains(last, `"animData":"4 10 1 0 0 0 0 15 136 1 0 0 0 0 15 147 1 255 0 0 0 15 234 1 0 0 0 0 15"`) {
		t.Errorf("sent %s", last)
	}
	if office.Selected() != "*Static*" {
		t.Errorf("running %q", office.Selected())
	}
	if len(office.EffectNames()) != 17 {
		t.Error("painting saved a scene")
	}

	// saved as well, as carefully as a push
	out = h.ok("scene", "paint", "office", "all=blue", "--save", "Blue")
	want(t, out, "painted", "backed up to", "added      Blue")
	if !slices.Contains(office.EffectNames(), "Blue") {
		t.Error("--save did not save")
	}
	// a dry run sends nothing
	before := len(office.Writes())
	want(t, h.ok("scene", "paint", "office", "all=red", "--dry-run"), "dry run", `"animType":"static"`)
	if len(office.Writes()) != before {
		t.Error("a dry run painted")
	}
}

// A controller goes by a scene's exact name, capitals and all. A person
// pointing at a scene that is there should not have to.
func TestSceneNamesWhateverTheCapitals(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office")
	bedroom := h.controller("bedroom")
	bedroom.RemoveEffect(northern)

	// read
	e, err := aurora.ParseEffect([]byte(h.ok("scene", "dump", "office", "KT NORTHERN LIGHTS")))
	if err != nil || e.Name() != northern {
		t.Errorf("dump: %q, %v", e.Name(), err)
	}
	// started, by a command and by a setting
	want(t, h.ok("scene", "select", "office", "flames"), "is now running Flames")
	want(t, h.ok("set", "office", "scene", " kt northern lights "), "scene Flames → kt Northern Lights")
	if office.Selected() != northern {
		t.Errorf("running %q", office.Selected())
	}
	// copied: it arrives under the name the controller it came from spells it
	want(t, h.ok("scene", "copy", "kt northern lights", "--from", "office", "--to", "bedroom"), "copying kt Northern Lights", "added      kt Northern Lights")
	if _, ok := bedroom.Effect(northern); !ok || len(bedroom.EffectNames()) != 17 {
		t.Errorf("the bedroom holds %v", bedroom.EffectNames())
	}
	// deleted, by name and by what to keep
	want(t, h.ok("scene", "delete", "office", "FOREST", "--force"), "deleted    Forest")
	want(t, h.ok("scene", "delete", "office", "--except", "kt northern lights", "--except", "NEMO", "--force"), "deleted    Flames")
	if names := office.EffectNames(); len(names) != 2 || names[0] != "Nemo" || names[1] != northern {
		t.Fatalf("left: %v", names)
	}

	// a controller holding two scenes that differ only in their capitals: typed exactly, each is itself
	nemo, _ := office.Effect("Nemo")
	office.PutEffect(nemo.WithName("nemo"))
	want(t, h.ok("scene", "select", "office", "nemo"), "is now running nemo")
	want(t, h.ok("scene", "select", "office", "Nemo"), "is now running Nemo")
	// typed as neither, it is not guessed at
	for _, args := range [][]string{
		{"scene", "select", "office", "NEMO"},
		{"scene", "dump", "office", "NEMO"},
		{"set", "office", "scene", "NEMO"},
		{"scene", "delete", "office", "NEMO", "--force"},
		{"scene", "delete", "office", "--except", "NEMO", "--force"},
		{"scene", "copy", "NEMO", "--from", "office", "--to", "bedroom"},
	} {
		want(t, h.fails(args...), `"NEMO" could be Nemo or nemo: type the one you mean exactly`)
	}
	if office.Selected() != "Nemo" || len(office.EffectNames()) != 3 {
		t.Errorf("a name that could be two scenes changed something: running %q, holding %v", office.Selected(), office.EffectNames())
	}

	// a name to store a scene under is taken as it is typed
	want(t, h.ok("scene", "copy", "nemo", "--from", "office", "--to", "bedroom", "--as", "Little Fish"), "added      Little Fish")
	if _, ok := bedroom.Effect("Little Fish"); !ok {
		t.Error("--as did not keep its capitals")
	}
}

func TestSceneSelect(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office")

	want(t, h.ok("scene", "select", "office", "Flames"), "office", "is now running", "Flames")
	if office.Selected() != "Flames" {
		t.Errorf("running %q", office.Selected())
	}
	want(t, h.fails("scene", "select", "office", "Nope"), `no scene called "Nope"`, "it has Color Burst")

	want(t, h.ok("scene", "select", "office", "Forest", "--dry-run"), `{"select":"Forest"}`)
	if office.Selected() != "Flames" {
		t.Error("a dry run started a scene")
	}
	var out struct {
		Running string `json:"running"`
	}
	if err := json.Unmarshal([]byte(h.ok("scene", "run", "office", "Forest", "--json")), &out); err != nil || out.Running != "Forest" {
		t.Errorf("select --json: %+v, %v", out, err)
	}
}

// A scene can be named anything, colour tags and format verbs included, and
// is printed as it is named.
func TestOddSceneNames(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office")
	bedroom := h.controller("bedroom")
	e, _ := office.Effect("Flames")
	office.PutEffect(e.WithName("<red>100%s</>"))

	out := h.ok("scene", "copy", "<red>100%s</>", "--from", "office", "--to", "bedroom")
	want(t, out, "‹red›100%s‹/›")
	if strings.Contains(out, "MISSING") || strings.Contains(out, "%%") {
		t.Errorf("the name was read as a format or a colour:\n%s", out)
	}
	if _, ok := bedroom.Effect("<red>100%s</>"); !ok {
		t.Error("the scene did not arrive under its own name")
	}
}

func TestSettingsFromTheEnvironment(t *testing.T) {
	h := newHome(t)
	h.controller("office")

	// run without --config-dir: the environment says where
	t.Setenv("TAPROOT_CONFIG_DIR", h.dir)
	viper.Reset()
	root, err := cli.Make()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	oldOut, oldLevel := cout.Out, cout.Level
	cout.Out = &out
	defer func() { cout.Out, cout.Level = oldOut, oldLevel }()
	root.SetArgs([]string{"list", "--quiet"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	// quiet prints nothing for a table, and the command still ran against the right directory
	if out.Len() != 0 {
		t.Errorf("--quiet printed:\n%s", out.String())
	}

	f := cli.GetFlags()
	if f.ConfigDir != h.dir || f.BackupRoot() != filepath.Join(h.dir, "backups") || f.Timeout != aurora.DefaultTimeout {
		t.Errorf("flags: %+v", f)
	}
	t.Setenv("TAPROOT_BACKUP_DIR", "/elsewhere")
	t.Setenv("TAPROOT_TIMEOUT", "5s")
	if f := cli.GetFlags(); f.BackupRoot() != "/elsewhere" || f.Timeout != 5*time.Second {
		t.Errorf("flags from the environment: %+v", f)
	}
}

// The scene that started all this ships with taproot, and push takes it by name.
func TestPushFromTheLibrary(t *testing.T) { //nolint:paralleltest // the commands share viper
	h := newHome(t)
	office := h.controller("office")
	office.RemoveEffect(northern)

	want(t, h.fails("scene", "push", "office", "No Such Thing"), `no file "No Such Thing"`, "the library has no scene of that name")
	out := h.ok("scene", "push", "office", northern)
	want(t, out, "added      "+northern)
	if !slices.Contains(office.EffectNames(), northern) {
		t.Error("the built-in scene was not pushed")
	}
}
