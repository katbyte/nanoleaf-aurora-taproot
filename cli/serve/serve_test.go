// The page's server against canned controllers (auroratest): what the page
// asks for, what it is told, and what reaches a controller because of it.
package serve

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/go-kt/cout"
	"github.com/katbyte/nanoleaf-aurora-taproot/cli"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/backup"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/push"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora/auroratest"
)

const northern = "kt Northern Lights"

// what a dry-run server prints for each change it holds back is for a person at a terminal, not a test log
func TestMain(m *testing.M) {
	cout.Out = io.Discard
	os.Exit(m.Run())
}

// site is a served page for one test: the server, where it is, the
// directory its controllers file is in, and what it logged.
type site struct {
	t   *testing.T
	s   *server
	url string
	dir string
	log *bytes.Buffer
}

func newSite(t *testing.T, change ...func(*cli.FlagData)) *site {
	t.Helper()

	f := &cli.FlagData{ConfigDir: t.TempDir(), Timeout: 3 * time.Second}
	for _, c := range change {
		c(f)
	}
	var log bytes.Buffer
	s := newServer(f, &log)
	s.hub.start(t.Context(), s)
	ts := httptest.NewServer(s.handler())
	t.Cleanup(ts.Close)

	return &site{t: t, s: s, url: ts.URL, dir: f.ConfigDir, log: &log}
}

// logged is the request log so far, without its colours.
func (w *site) logged() string {
	return ansi.ReplaceAllString(w.log.String(), "")
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// controller starts a canned controller and connects the site to it.
func (w *site) controller(name string) *auroratest.Controller {
	w.t.Helper()

	ctl := auroratest.New(w.t)
	ctl.Set("name", "Light Panels "+name)
	ctl.Set("serialNo", "S-"+name)
	st, err := store.Open(w.dir)
	if err != nil {
		w.t.Fatal(err)
	}
	st.Put(store.Controller{Name: name, Device: "Light Panels " + name, Host: ctl.Host(), Serial: "S-" + name, Token: ctl.Token()})
	if err := st.Save(); err != nil {
		w.t.Fatal(err)
	}
	w.s.stale()

	return ctl
}

// do makes a request the way the page does and returns the status and body.
func (w *site) do(method, path, body string, headers ...string) (code int, answer string) {
	w.t.Helper()

	var reader io.Reader = http.NoBody
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(w.t.Context(), method, w.url+path, reader)
	if err != nil {
		w.t.Fatal(err)
	}
	req.Header.Set(pageHeader, "page")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		if headers[i] == "Host" {
			req.Host = headers[i+1]
			continue
		}
		if headers[i+1] == "" {
			req.Header.Del(headers[i])
			continue
		}
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		w.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	data, _ := io.ReadAll(res.Body)

	return res.StatusCode, string(data)
}

// state reads what the page draws, fresh.
func (w *site) state() state {
	w.t.Helper()

	w.s.stale()
	code, body := w.do(http.MethodGet, "/api/state", "")
	if code != http.StatusOK {
		w.t.Fatalf("GET /api/state: %d %s", code, body)
	}
	var st state
	if err := json.Unmarshal([]byte(body), &st); err != nil {
		w.t.Fatal(err)
	}
	// no token may ever reach the browser
	saved, _ := store.Open(w.dir)
	for _, c := range saved.Controllers {
		if strings.Contains(body, c.Token) {
			w.t.Errorf("the state carries %s's token", c.Name)
		}
	}

	return st
}

func (w *site) wantError(code int, body string, status int, part string) {
	w.t.Helper()

	var e struct {
		Error string `json:"error"`
	}
	if code != status || json.Unmarshal([]byte(body), &e) != nil || !strings.Contains(e.Error, part) {
		w.t.Errorf("got %d %s; want %d with an error saying %q", code, body, status, part)
	}
}

func TestThePageAndItsFiles(t *testing.T) {
	t.Parallel()
	w := newSite(t)

	res, err := http.Get(w.url + "/") //nolint:noctx // a plain page load, of the test's own server
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "<title>🌱 taproot</title>") || strings.Contains(string(body), "{{version}}") {
		t.Errorf("the page: %d\n%.200s", res.StatusCode, body)
	}
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
		t.Errorf("content type %q", res.Header.Get("Content-Type"))
	}
	// nothing may run or load on the page that did not come from this server
	csp := res.Header.Get("Content-Security-Policy")
	for _, part := range []string{"default-src 'none'", "script-src 'self'", "style-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, part) {
			t.Errorf("the policy is missing %q: %s", part, csp)
		}
	}
	if strings.Contains(csp, "unsafe") {
		t.Errorf("the policy allows something unsafe: %s", csp)
	}

	for path, kind := range map[string]string{"/page.css": "text/css", "/page.js": "text/javascript"} {
		res, err := http.Get(w.url + path) //nolint:noctx // as above
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), kind) || len(data) < 1000 {
			t.Errorf("%s: %d %s, %d bytes", path, res.StatusCode, res.Header.Get("Content-Type"), len(data))
		}
	}
	// the page's files name each other: a script it loads must be one that is served
	for _, ref := range []string{`href="/page.css"`, `src="/page.js"`} {
		if !strings.Contains(string(body), ref) {
			t.Errorf("the page does not load %s", ref)
		}
	}

	if code, _ := w.do(http.MethodGet, "/nothing-here", ""); code != http.StatusNotFound {
		t.Errorf("a path that is not there: %d", code)
	}
	if !strings.Contains(w.logged(), "GET / 200") {
		t.Errorf("the page load was not logged:\n%s", w.logged())
	}
}

func TestState(t *testing.T) {
	t.Parallel()
	w := newSite(t)

	if st := w.state(); len(st.Controllers) != 0 || st.Controllers == nil || st.DryRun || st.Version == "" {
		t.Errorf("with no controllers: %+v", st)
	}

	w.controller("office")
	gone := w.controller("bedroom")
	gone.Unplug()

	st := w.state()
	if len(st.Controllers) != 2 || st.Controllers[0].Name != "bedroom" || st.Controllers[1].Name != "office" {
		t.Fatalf("controllers: %+v", st.Controllers)
	}
	if b := st.Controllers[0]; b.Reachable || b.Error == "" || b.Scenes == nil {
		t.Errorf("the one that is gone: %+v", b)
	}
	o := st.Controllers[1]
	if !o.Reachable || !o.On || o.Brightness != 33 || o.Running != northern || o.ColorMode != "effect" || o.Orientation != 88 || o.CT != 3000 {
		t.Errorf("office: %+v", o)
	}
	if o.Device != "Light Panels office" || o.Model != "NL22" || o.Firmware != "5.2.1" {
		t.Errorf("office's identity: %+v", o)
	}
	if o.Layout == nil || len(o.Layout.Panels) != 4 || o.Layout.SideLength != 150 {
		t.Errorf("office's panels: %+v", o.Layout)
	}
	if len(o.Scenes) != 17 {
		t.Fatalf("%d scenes", len(o.Scenes))
	}
	// what the page needs to draw the scene moving
	if s := o.Scenes[16]; s.Name != northern || s.Plugin != "Wheel" || s.Kind != "color" || len(s.Palette) != 7 ||
		s.Options["linDirection"] != "up" || s.Options["transTime"] != float64(107) || s.Options["nColorsPerFrame"] != float64(3) {
		t.Errorf("the running scene: %+v", s)
	}
	// the page's own polling is not logged
	if strings.Contains(w.logged(), "/api/state") {
		t.Errorf("polling was logged:\n%s", w.logged())
	}
}

func TestPowerBrightnessAndScene(t *testing.T) {
	t.Parallel()
	w := newSite(t)
	office := w.controller("office")

	if code, body := w.do(http.MethodPut, "/api/controllers/office/power", `{"on":false}`); code != http.StatusNoContent || office.State().On.Value {
		t.Errorf("power off: %d %s", code, body)
	}
	if code, body := w.do(http.MethodPut, "/api/controllers/office/brightness", `{"value":80}`); code != http.StatusNoContent || office.State().Brightness.Value != 80 {
		t.Errorf("brightness: %d %s", code, body)
	}
	if code, body := w.do(http.MethodPut, "/api/controllers/office/scene", `{"name":"Flames"}`); code != http.StatusNoContent || office.Selected() != "Flames" {
		t.Errorf("scene: %d %s", code, body)
	}
	if code, body := w.do(http.MethodPost, "/api/controllers/office/identify", ""); code != http.StatusNoContent {
		t.Errorf("identify: %d %s", code, body)
	}
	// the next reading shows it all
	if o := w.state().Controllers[0]; o.On || o.Brightness != 80 || o.Running != "Flames" {
		t.Errorf("after the changes: %+v", o)
	}
	if !strings.Contains(w.logged(), "PUT /api/controllers/office/power 204") {
		t.Errorf("a change was not logged:\n%s", w.logged())
	}

	for _, bad := range []struct{ path, body, part string }{
		{"/api/controllers/office/power", `{}`, "bad request"},
		{"/api/controllers/office/power", `nope`, "not the JSON this expects"},
		{"/api/controllers/office/brightness", `{"value":101}`, "brightness is 0 to 100"},
		{"/api/controllers/office/brightness", `{}`, "brightness is 0 to 100"},
		{"/api/controllers/office/scene", `{"name":""}`, "bad request"},
		{"/api/controllers/kitchen/power", `{"on":true}`, `no such controller called "kitchen"`},
	} {
		code, body := w.do(http.MethodPut, bad.path, bad.body)
		w.wantError(code, body, http.StatusBadRequest, bad.part)
	}
	code, body := w.do(http.MethodPut, "/api/controllers/office/scene", `{"name":"Nope"}`)
	w.wantError(code, body, http.StatusNotFound, "HTTP 404")

	office.Unplug()
	code, body = w.do(http.MethodPut, "/api/controllers/office/power", `{"on":true}`)
	w.wantError(code, body, http.StatusBadGateway, "PUT /state")
}

func TestBackupAndForget(t *testing.T) {
	t.Parallel()
	w := newSite(t)
	office := w.controller("office")
	away := w.controller("away")
	away.Unplug()

	code, body := w.do(http.MethodPost, "/api/controllers/office/backup", "")
	var done cli.BackedUp
	if code != http.StatusOK || json.Unmarshal([]byte(body), &done) != nil || done.Scenes != 17 || done.Running != northern {
		t.Fatalf("backup: %d %s", code, body)
	}
	if !strings.HasPrefix(done.Dir, filepath.Join(w.dir, "backups", "office")) {
		t.Errorf("backed up to %s", done.Dir)
	}
	if b, err := backup.Read(done.Dir); err != nil || len(b.Scenes) != 17 {
		t.Errorf("the backup: %v", err)
	}
	code, body = w.do(http.MethodPost, "/api/controllers/away/backup", "")
	w.wantError(code, body, http.StatusBadGateway, "reading the controller")
	code, body = w.do(http.MethodPost, "/api/controllers/kitchen/backup", "")
	w.wantError(code, body, http.StatusBadRequest, "no such controller")

	// one that cannot be reached is not forgotten unless asked to be, locally
	code, body = w.do(http.MethodDelete, "/api/controllers/away", "")
	w.wantError(code, body, http.StatusBadGateway, "could not have away delete the token")
	if code, body = w.do(http.MethodDelete, "/api/controllers/away?local=1", ""); code != http.StatusNoContent {
		t.Errorf("forget locally: %d %s", code, body)
	}

	if code, body = w.do(http.MethodDelete, "/api/controllers/office", ""); code != http.StatusNoContent {
		t.Errorf("forget: %d %s", code, body)
	}
	if wr := office.Writes(); len(wr) != 1 || wr[0].Method != http.MethodDelete || wr[0].Path != "" {
		t.Errorf("the controller was asked: %+v", wr)
	}
	if st, _ := store.Open(w.dir); len(st.Controllers) != 0 {
		t.Errorf("still saved: %+v", st.Names())
	}
	if len(w.state().Controllers) != 0 {
		t.Error("the page still shows a forgotten controller")
	}
	code, body = w.do(http.MethodDelete, "/api/controllers/office", "")
	w.wantError(code, body, http.StatusBadRequest, "no such controller")
}

func TestCopy(t *testing.T) {
	t.Parallel()
	w := newSite(t)
	office := w.controller("office")
	bedroom := w.controller("bedroom")
	hall := w.controller("hall")
	bedroom.RemoveEffect(northern)
	changed, _ := hall.Effect(northern)
	changed, _ = changed.With("palette", []aurora.Color{{Hue: 1}})
	hall.PutEffect(changed)

	copyTo := func(body string) []copied {
		t.Helper()
		code, out := w.do(http.MethodPost, "/api/copy", body)
		if code != http.StatusOK {
			t.Fatalf("copy: %d %s", code, out)
		}
		var res []copied
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}

	// to the one that lost it and the one that changed it: added on the first, refused on the second
	res := copyTo(`{"scene":"kt Northern Lights","from":"office","to":["bedroom","hall"],"select":true}`)
	if len(res) != 2 || res[0].Controller != "bedroom" || res[1].Controller != "hall" {
		t.Fatalf("results: %+v", res)
	}
	if r := res[0]; r.Error != "" || r.Results[0].Outcome != push.Added || r.Backup == "" {
		t.Errorf("bedroom: %+v", r)
	}
	if r := res[1]; r.Results[0].Outcome != push.Refused || !strings.Contains(r.Results[0].Reason, "--force") || r.Backup != "" {
		t.Errorf("hall: %+v", r)
	}
	src, _ := office.Effect(northern)
	if e, ok := bedroom.Effect(northern); !ok || !e.Equal(src) || bedroom.Selected() != northern {
		t.Error("the bedroom does not hold the scene, or is not running it")
	}
	if e, _ := hall.Effect(northern); !e.Equal(changed) || len(hall.Writes()) != 0 {
		t.Error("the hall's own scene was touched")
	}

	// told to replace it
	res = copyTo(`{"scene":"kt Northern Lights","from":"office","to":["hall"],"force":true}`)
	if r := res[0]; r.Results[0].Outcome != push.Replaced || r.Backup == "" {
		t.Errorf("hall, forced: %+v", r)
	}
	if e, _ := hall.Effect(northern); !e.Equal(src) {
		t.Error("the hall's scene was not replaced")
	}
	// what it replaced is in the backup taken first
	if b, err := backup.Read(res[0].Backup); err != nil || len(b.Scenes) != 17 {
		t.Errorf("the hall's backup: %v", err)
	}

	// the controller a scene comes from is only ever read
	if wr := office.Writes(); len(wr) != 0 {
		t.Errorf("the source was written to: %+v", wr)
	}

	// one that cannot be reached is reported, and does not stop the other
	hall.Unplug()
	bedroom.RemoveEffect("Flames")
	res = copyTo(`{"scene":"Flames","from":"office","to":["hall","bedroom","kitchen"]}`)
	if res[0].Error == "" || res[1].Results[0].Outcome != push.Added || !strings.Contains(res[2].Error, "no such controller") {
		t.Errorf("with one unreachable and one unknown: %+v", res)
	}

	for body, part := range map[string]string{
		`{"scene":"Flames","from":"office","to":[]}`:          "a copy needs a scene",
		`{"from":"office","to":["bedroom"]}`:                  "a copy needs a scene",
		`{"scene":"Flames","from":"office","to":["office"]}`:  "office is where the scene comes from",
		`{"scene":"Flames","from":"kitchen","to":["office"]}`: "no such controller",
		`not json`: "not the JSON this expects",
	} {
		code, out := w.do(http.MethodPost, "/api/copy", body)
		w.wantError(code, out, http.StatusBadRequest, part)
	}
	code, out := w.do(http.MethodPost, "/api/copy", `{"scene":"Nope","from":"office","to":["bedroom"]}`)
	w.wantError(code, out, http.StatusNotFound, `reading "Nope" from office`)
}

// A page with no login that controls real things must not take orders from
// another site by way of the browser of somebody reading it.
func TestOnlyThePageMayChangeThings(t *testing.T) {
	t.Parallel()
	w := newSite(t)
	office := w.controller("office")
	host := strings.TrimPrefix(w.url, "http://")
	const off = `{"on":false}`
	const path = "/api/controllers/office/power"

	// without the header the page sends, which another site's page cannot set
	code, body := w.do(http.MethodPut, path, off, pageHeader, "")
	w.wantError(code, body, http.StatusForbidden, "did not come from the taproot page")
	// from another site's address
	code, body = w.do(http.MethodPut, path, off, "Origin", "http://evil.example")
	w.wantError(code, body, http.StatusForbidden, "another site's page")
	code, body = w.do(http.MethodPut, path, off, "Origin", "null")
	w.wantError(code, body, http.StatusForbidden, "another site's page")
	// to a name pointed at this address by somebody else
	code, body = w.do(http.MethodPut, path, off, "Host", "evil.example:7668")
	w.wantError(code, body, http.StatusForbidden, `does not answer to the name "evil.example:7668"`)
	code, body = w.do(http.MethodGet, "/api/state", "", "Host", "evil.example")
	w.wantError(code, body, http.StatusForbidden, "--allow-host evil.example")

	if !office.State().On.Value || len(office.Writes()) != 0 {
		t.Fatal("a refused request reached the controller")
	}

	// from the page itself
	if code, body := w.do(http.MethodPut, path, off, "Origin", "http://"+host); code != http.StatusNoContent || office.State().On.Value {
		t.Errorf("the page's own request: %d %s", code, body)
	}
}

func TestHostAllowed(t *testing.T) {
	t.Parallel()

	own, _ := os.Hostname()
	plain := &server{}
	for host, want := range map[string]bool{
		"localhost":           true,
		"localhost:7668":      true,
		"LOCALHOST:7668":      true,
		"127.0.0.1:7668":      true,
		"10.0.0.12":           true,
		"[::1]:7668":          true,
		"[fe80::1]":           true,
		"mac.local:7668":      true,
		own:                   own != "",
		"taproot.example.com": false,
		"evil.example:7668":   false,
		"local":               false,
		"notlocalhost":        false,
		"":                    false,
	} {
		if got := plain.hostAllowed(host); got != want {
			t.Errorf("hostAllowed(%q) = %v, want %v", host, got, want)
		}
	}

	named := &server{allow: []string{" Taproot.Example.com ", "lights"}}
	for host, want := range map[string]bool{"taproot.example.com:443": true, "lights": true, "other.example.com": false} {
		if got := named.hostAllowed(host); got != want {
			t.Errorf("with names allowed, hostAllowed(%q) = %v, want %v", host, got, want)
		}
	}
	if !(&server{allow: []string{"*"}}).hostAllowed("anything.example") {
		t.Error("* allows any name")
	}
}

func TestConnectFromThePage(t *testing.T) { //nolint:paralleltest // shortens how often the page asks for a token
	oldEvery, oldFor := pairEvery, pairFor
	pairEvery = 10 * time.Millisecond
	defer func() { pairEvery, pairFor = oldEvery, oldFor }()

	w := newSite(t)
	ctl := auroratest.New(t)
	ctl.PairAfter(3)

	code, body := w.do(http.MethodPost, "/api/connect", `{"address":"`+ctl.Host()+`","name":"office"}`)
	var job pairing
	if code != http.StatusAccepted || json.Unmarshal([]byte(body), &job) != nil || job.State != pairWaiting || job.Address != ctl.Host() || job.Until.IsZero() {
		t.Fatalf("connect: %d %s", code, body)
	}
	// asking twice is one asking
	if again, _ := w.do(http.MethodPost, "/api/connect", `{"address":"`+ctl.Host()+`"}`); again != http.StatusAccepted {
		t.Errorf("a second connect: %d", again)
	}

	var st state
	waitFor(t, "the controller to be connected", func() bool {
		st = w.state()
		return len(st.Pairing) == 1 && st.Pairing[0].State == pairConnected
	})
	if p := st.Pairing[0]; p.Name != "office" || !strings.Contains(p.Message, "NL22, firmware 5.2.1, 4 panels, 17 scenes") {
		t.Errorf("how it ended: %+v", p)
	}
	if len(st.Controllers) != 1 || st.Controllers[0].Name != "office" || !st.Controllers[0].Reachable {
		t.Errorf("the page does not show the new controller: %+v", st.Controllers)
	}
	saved, _ := store.Open(w.dir)
	if len(saved.Controllers) != 1 || saved.Controllers[0].Token == "" {
		t.Fatalf("saved: %+v", saved.Names())
	}
	if strings.Contains(body, saved.Controllers[0].Token) {
		t.Error("the page was sent the token")
	}

	// a controller nobody goes to gives up when its time is up
	pairFor = 100 * time.Millisecond
	other := auroratest.New(t)
	w.do(http.MethodPost, "/api/connect", `{"address":"`+other.Host()+`"}`)
	waitFor(t, "the asking to give up", func() bool {
		for _, p := range w.state().Pairing {
			if p.Address == other.Host() && p.State == pairFailed && strings.Contains(p.Message, "no token after") {
				return true
			}
		}
		return false
	})

	// and one that is being asked can be stopped
	pairFor = time.Minute
	third := auroratest.New(t)
	w.do(http.MethodPost, "/api/connect", `{"address":"`+third.Host()+`"}`)
	if stopped, _ := w.do(http.MethodDelete, "/api/connect?address="+third.Host(), ""); stopped != http.StatusNoContent {
		t.Errorf("stop: %d", stopped)
	}
	for _, p := range w.state().Pairing {
		if p.Address == third.Host() {
			t.Errorf("a stopped connect is still listed: %+v", p)
		}
	}

	code, body = w.do(http.MethodPost, "/api/connect", `{"address":""}`)
	w.wantError(code, body, http.StatusBadRequest, "a controller address is required")
	code, body = w.do(http.MethodPost, "/api/connect", `nope`)
	w.wantError(code, body, http.StatusBadRequest, "not the JSON this expects")
}

// A change made anywhere, at the controller's own button say, reaches an
// open page as it happens.
func TestThePageHearsOfChanges(t *testing.T) {
	t.Parallel()
	w := newSite(t)
	office := w.controller("office")
	w.state() // the first reading starts the listening
	waitFor(t, "taproot to be listening to the controller", func() bool { return office.Listeners() == 1 })

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, w.url+"/api/events", http.NoBody)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("the stream: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	waitFor(t, "the page's stream to be open", func() bool {
		w.s.hub.mu.Lock()
		defer w.s.hub.mu.Unlock()
		return len(w.s.hub.pages) == 1
	})

	office.Emit(aurora.EventEffects, aurora.AttrSelectedEffect, "Flames")

	lines := bufio.NewScanner(res.Body)
	for lines.Scan() {
		if line := lines.Text(); strings.HasPrefix(line, "data: ") {
			if line != `data: {"controller":"office"}` {
				t.Errorf("the page was told %q", line)
			}
			return
		}
	}
	t.Fatalf("the stream ended without a word: %v", lines.Err())
}

// A controller that goes from the controllers file is no longer listened to.
func TestListeningFollowsTheControllersFile(t *testing.T) {
	t.Parallel()
	w := newSite(t)
	office := w.controller("office")
	w.state()
	waitFor(t, "taproot to be listening", func() bool { return office.Listeners() == 1 })

	st, _ := store.Open(w.dir)
	st.Remove("office")
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	w.state()
	waitFor(t, "taproot to stop listening", func() bool { return office.Listeners() == 0 })
}

func TestADryRunPageChangesNothing(t *testing.T) {
	t.Parallel()
	w := newSite(t, func(f *cli.FlagData) { f.DryRun = true })
	office := w.controller("office")
	bedroom := w.controller("bedroom")
	bedroom.RemoveEffect(northern)

	if !w.state().DryRun {
		t.Error("the page is not told it is a dry run")
	}
	for _, call := range []struct{ method, path, body string }{
		{http.MethodPut, "/api/controllers/office/power", `{"on":false}`},
		{http.MethodPut, "/api/controllers/office/scene", `{"name":"Flames"}`},
		{http.MethodPost, "/api/controllers/office/backup", ""},
		{http.MethodPost, "/api/copy", `{"scene":"kt Northern Lights","from":"office","to":["bedroom"]}`},
		{http.MethodDelete, "/api/controllers/office", ""},
	} {
		if code, body := w.do(call.method, call.path, call.body); code >= 300 {
			t.Errorf("%s %s: %d %s", call.method, call.path, code, body)
		}
	}
	if len(office.Writes())+len(bedroom.Writes()) != 0 {
		t.Errorf("a dry run wrote: %+v %+v", office.Writes(), bedroom.Writes())
	}
	if st, _ := store.Open(w.dir); len(st.Controllers) != 2 {
		t.Error("a dry run forgot a controller")
	}
	if entries, _ := os.ReadDir(filepath.Join(w.dir, "backups")); len(entries) != 0 {
		t.Errorf("a dry run took a backup: %v", entries)
	}
}

func TestFind(t *testing.T) { //nolint:paralleltest // shortens how long the page searches
	old := findFor
	findFor = 100 * time.Millisecond
	defer func() { findFor = old }()

	w := newSite(t)
	code, body := w.do(http.MethodGet, "/api/find", "")
	var found []cli.FoundController
	if code != http.StatusOK || json.Unmarshal([]byte(body), &found) != nil || found == nil {
		t.Errorf("find: %d %s", code, body) // what is on the network is not this test's to say: an empty list is still a list
	}
	code, body = w.do(http.MethodGet, "/api/find?scan=everywhere", "")
	w.wantError(code, body, http.StatusBadRequest, "is not a subnet like 10.0.5.0/24")
}

func TestRun(t *testing.T) { //nolint:paralleltest // prints through the process's console
	// a port that is free, as far as anybody can tell
	ln, err := net.Listen("tcp", "127.0.0.1:0") //nolint:noctx // found and let go at once
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, &cli.FlagData{ConfigDir: t.TempDir(), Timeout: time.Second, DryRun: true}, addr)
	}()

	waitFor(t, "the page to be served", func() bool {
		res, gerr := http.Get("http://" + addr + "/") //nolint:noctx // the test's own server
		if gerr != nil {
			return false
		}
		_ = res.Body.Close()
		return res.StatusCode == http.StatusOK
	})
	cancel()
	select {
	case stopped := <-done:
		if stopped != nil {
			t.Errorf("stopping is not an error: %v", stopped)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the server did not stop")
	}

	// an address that is taken says so
	busy, err := net.Listen("tcp", "127.0.0.1:0") //nolint:noctx // held for the length of the check
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = busy.Close() }()
	if err := run(t.Context(), &cli.FlagData{ConfigDir: t.TempDir()}, busy.Addr().String()); err == nil || !strings.Contains(err.Error(), "listening on") {
		t.Errorf("a port in use: %v", err)
	}
}

func TestReachableHosts(t *testing.T) {
	t.Parallel()

	if got := reachableHosts("127.0.0.1:7668"); len(got) != 1 || got[0] != "127.0.0.1" {
		t.Errorf("pinned to one address: %v", got)
	}
	if got := reachableHosts(":7668"); len(got) == 0 || got[0] != "localhost" {
		t.Errorf("on every interface: %v", got)
	}
}

// waitFor polls for something another goroutine is about to do.
func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
