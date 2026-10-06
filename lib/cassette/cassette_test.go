package cassette_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katbyte/nanoleaf-aurora-taproot/lib/cassette"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora/auroratest"
)

const scene = "kt Northern Lights"

// start starts a proxy and a client that talks to a controller through it,
// holding only the token every program under test is given.
func start(t *testing.T, opts cassette.Options) (*cassette.Proxy, *aurora.Client) {
	t.Helper()

	p, err := cassette.Start(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	c, err := aurora.New(p.Host(), cassette.Token)
	if err != nil {
		t.Fatal(err)
	}

	return p, c
}

// session is a run of requests a test makes twice: once to record, once to
// replay.
func session(t *testing.T, c *aurora.Client) (info aurora.Info, e aurora.Effect, names []string) {
	t.Helper()

	info, err := c.Info(t.Context())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if e, err = c.Effect(t.Context(), scene); err != nil {
		t.Fatalf("Effect: %v", err)
	}
	if err := c.AddEffect(t.Context(), e.WithName("a copy")); err != nil {
		t.Fatalf("AddEffect: %v", err)
	}
	if names, err = c.EffectNames(t.Context()); err != nil {
		t.Fatalf("EffectNames: %v", err)
	}
	if _, err := c.Effect(t.Context(), "nothing by this name"); !aurora.IsNotFound(err) {
		t.Fatalf("an effect that is not there: %v", err)
	}

	return info, e, names
}

func TestRecordThenReplay(t *testing.T) {
	t.Parallel()

	ctl := auroratest.New(t)
	ctl.Set("serialNo", "S18322A9999")
	token := ctl.Token()
	file := filepath.Join(t.TempDir(), "cassettes", "session.json")
	if cassette.Recorded(file) {
		t.Fatal("Recorded says a file is there before anything was recorded")
	}

	rec, c := start(t, cassette.Options{Mode: cassette.Record, File: file, Target: ctl.Host(), RealToken: token})
	info, e, names := session(t, c)
	if len(names) != 18 || info.Model != "NL22" || e.Name() != scene {
		t.Fatalf("through the proxy: %d names, %q, %q", len(names), info.Model, e.Name())
	}
	// the controller was really asked, under its real token
	if _, ok := ctl.Effect("a copy"); !ok || len(ctl.Requests()) != 5 {
		t.Fatalf("the controller got %d requests", len(ctl.Requests()))
	}
	if len(rec.Problems()) != 0 || len(rec.Exchanges()) != 5 {
		t.Fatalf("problems %v, %d exchanges", rec.Problems(), len(rec.Exchanges()))
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	// what was written down holds neither the token nor the unit's serial number
	data, err := os.ReadFile(file) //nolint:gosec // the recording this test made
	if err != nil || !cassette.Recorded(file) {
		t.Fatal(err)
	}
	if strings.Contains(string(data), token) || strings.Contains(string(data), "S18322A9999") {
		t.Error("the recording holds the token or the serial number")
	}
	var written cassette.Recording
	if err := json.Unmarshal(data, &written); err != nil || !written.Writes || written.Recorded == "" || len(written.Exchanges) != 5 {
		t.Fatalf("the file: %+v, %v", written, err)
	}
	if x := written.Exchanges[2]; x.Method != http.MethodPut || x.Path != "/effects" || x.Status != http.StatusNoContent || !strings.Contains(string(x.Request), `"command": "add"`) {
		t.Errorf("the add as written: %+v", x)
	}

	// with the controller gone, the same session gets the same answers
	ctl.Unplug()
	play, c := start(t, cassette.Options{Mode: cassette.Replay, File: file})
	info2, e2, names2 := session(t, c)
	if info2.Name != info.Name || info2.SerialNo != "S00000A0000" || !e2.Equal(e) || strings.Join(names2, "|") != strings.Join(names, "|") {
		t.Errorf("replayed differently: %q %q %v", info2.Name, info2.SerialNo, names2)
	}
	// byte for byte, the indenting of the file notwithstanding
	a, _ := json.Marshal(e)
	b, _ := json.Marshal(e2)
	if !bytes.Equal(a, b) {
		t.Errorf("the effect replayed as\n%s\nrecorded as\n%s", b, a)
	}
	if len(play.Problems()) != 0 {
		t.Errorf("problems: %v", play.Problems())
	}

	// asking again for something asked once gets the same answer again
	if again, err := c.EffectNames(t.Context()); err != nil || len(again) != 18 {
		t.Errorf("asked twice: %v, %v", again, err)
	}
	// something that was never recorded is refused, and noted
	if _, err := c.Brightness(t.Context()); err == nil {
		t.Error("a request with no recording was answered")
	}
	if p := play.Problems(); len(p) != 1 || !strings.Contains(p[0], "GET /state/brightness") || !strings.Contains(p[0], "make record") {
		t.Errorf("problems: %v", p)
	}
}

// The same request can get different answers as a session goes on: a list
// before and after something is added to it.
func TestReplayKeepsAnswersInOrder(t *testing.T) {
	t.Parallel()

	ctl := auroratest.New(t)
	file := filepath.Join(t.TempDir(), "order.json")
	rec, c := start(t, cassette.Options{Mode: cassette.Record, File: file, Target: ctl.Host(), RealToken: ctl.Token()})
	steps := func(c *aurora.Client) []int {
		counts := make([]int, 0, 3)
		for _, name := range []string{"", "one", "two"} {
			if name != "" {
				e, _ := c.Effect(t.Context(), "Flames")
				if err := c.AddEffect(t.Context(), e.WithName(name)); err != nil {
					t.Fatal(err)
				}
			}
			names, err := c.EffectNames(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			counts = append(counts, len(names))
		}
		return counts
	}
	if got := steps(c); got[0] != 17 || got[1] != 18 || got[2] != 19 {
		t.Fatalf("recorded %v", got)
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	_, c = start(t, cassette.Options{Mode: cassette.Replay, File: file})
	if got := steps(c); got[0] != 17 || got[1] != 18 || got[2] != 19 {
		t.Errorf("replayed %v", got)
	}
}

// A controller that must not be written to is behind a proxy that will not
// pass a write on, whatever the test does.
func TestReadOnlyRefusesWrites(t *testing.T) {
	t.Parallel()

	ctl := auroratest.New(t)
	p, c := start(t, cassette.Options{Mode: cassette.Live, Target: ctl.Host(), RealToken: ctl.Token(), ReadOnly: true})

	e, err := c.Effect(t.Context(), scene) // a read that travels as a write
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Effects(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Plugins(t.Context()); err != nil {
		t.Fatal(err)
	}
	for name, write := range map[string]error{
		"add":    c.AddEffect(t.Context(), e.WithName("x")),
		"delete": c.DeleteEffect(t.Context(), scene),
		"select": c.SelectEffect(t.Context(), "Flames"),
		"power":  c.SetOn(t.Context(), false),
		"token":  c.DeleteToken(t.Context()),
	} {
		if write == nil {
			t.Errorf("%s went through a read-only proxy", name)
		}
	}
	if w := ctl.Writes(); len(w) != 0 {
		t.Errorf("the controller was written to: %+v", w)
	}
	if got := p.Problems(); len(got) != 5 || !strings.Contains(got[0], "only to be read") {
		t.Errorf("problems: %v", got)
	}
}

func TestVerifyFindsWhatHasChanged(t *testing.T) {
	t.Parallel()

	ctl := auroratest.New(t)
	token := ctl.Token()
	file := filepath.Join(t.TempDir(), "reads.json")
	reads := func(c *aurora.Client) {
		t.Helper()
		if _, err := c.Info(t.Context()); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Effect(t.Context(), scene); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Brightness(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	rec, c := start(t, cassette.Options{Mode: cassette.Record, File: file, Target: ctl.Host(), RealToken: token, ReadOnly: true})
	reads(c)
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	// nothing has changed but the values: a scene started, the lights dimmed
	live, _ := aurora.New(ctl.Host(), token)
	_ = live.SelectEffect(t.Context(), "Flames")
	_ = live.SetBrightness(t.Context(), 90, 0)
	same, c := start(t, cassette.Options{Mode: cassette.Verify, File: file, Target: ctl.Host(), RealToken: token, ReadOnly: true})
	reads(c)
	if got := same.Problems(); len(got) != 0 {
		t.Errorf("values changing is not the answers changing: %v", got)
	}
	// and the test was still served what was recorded, not what the controller says today
	if b, _ := c.Brightness(t.Context()); b.Value != 33 {
		t.Errorf("served brightness %d, recorded 33", b.Value)
	}

	// new firmware: a field gone from one answer, another answer gone altogether
	ctl.StoreAs(func(e aurora.Effect) aurora.Effect { return e.Without("hasOverlay") })
	held, _ := ctl.Effect(scene)
	_ = live.AddEffect(t.Context(), held)
	ctl.Fail(http.MethodGet, "/state/brightness", http.StatusNotFound)
	changed, c := start(t, cassette.Options{Mode: cassette.Verify, File: file, Target: ctl.Host(), RealToken: token, ReadOnly: true})
	reads(c)
	got := changed.Problems()
	if len(got) != 2 || !strings.Contains(got[0], "PUT /effects: the answer has changed shape") || !strings.Contains(got[0], "hasOverlay:bool") ||
		!strings.Contains(got[1], "GET /state/brightness: the controller now answers 404, the recording has 200") {
		t.Errorf("problems: %v", got)
	}
}

func TestStartRefuses(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := cassette.Start(cassette.Options{Mode: cassette.Replay, File: filepath.Join(dir, "missing.json")}); err == nil {
		t.Error("replaying a recording that is not there")
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cassette.Start(cassette.Options{Mode: cassette.Replay, File: bad}); err == nil {
		t.Error("replaying a recording that is not one")
	}
	if _, err := cassette.Start(cassette.Options{Mode: cassette.Record, File: bad}); err == nil {
		t.Error("recording with no controller to ask")
	}
}

// A token is only ever handed out to a hand on a button: a proxy answers
// for no one.
func TestNoTokensThroughAProxy(t *testing.T) {
	t.Parallel()

	ctl := auroratest.New(t)
	ctl.Pair()
	p, c := start(t, cassette.Options{Mode: cassette.Live, Target: ctl.Host(), RealToken: ctl.Token()})
	if token, err := c.WithToken("").NewToken(t.Context()); err == nil {
		t.Errorf("a proxy handed out a token of %d characters", len(token))
	}
	// and a token other than the one a test is given gets nowhere
	if _, err := c.WithToken("some-other-token").Info(t.Context()); err == nil {
		t.Error("another token was passed on")
	}
	if len(ctl.Requests()) != 0 || len(p.Problems()) != 0 {
		t.Errorf("the controller got %+v", ctl.Requests())
	}
}

// An event stream is opened and left open; recorded, it replays as one that
// opens.
func TestAnEventStream(t *testing.T) {
	t.Parallel()

	ctl := auroratest.New(t)
	file := filepath.Join(t.TempDir(), "stream.json")
	rec, _ := start(t, cassette.Options{Mode: cassette.Record, File: file, Target: ctl.Host(), RealToken: ctl.Token(), ReadOnly: true})
	// open opens the stream and says how it was answered
	open := func(host string) (status int, kind string) {
		t.Helper()
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+host+"/api/v1/"+cassette.Token+"/events?id=1,3", http.NoBody)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		return res.StatusCode, res.Header.Get("Content-Type")
	}
	if status, kind := open(rec.Host()); status != http.StatusOK || kind != "text/event-stream" {
		t.Fatalf("the stream through the proxy: %d %s", status, kind)
	}
	if x := rec.Exchanges(); len(x) != 1 || !x[0].Stream || x[0].Path != "/events?id=1,3" {
		t.Fatalf("recorded: %+v", x)
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	play, _ := start(t, cassette.Options{Mode: cassette.Replay, File: file})
	if status, kind := open(play.Host()); status != http.StatusOK || kind != "text/event-stream" {
		t.Errorf("the stream replayed: %d %s", status, kind)
	}
}

func TestShape(t *testing.T) {
	t.Parallel()

	for doc, want := range map[string]string{
		`{"value":33,"max":100,"min":0}`:             "{max:number,min:number,value:number}",
		`{"min":1,"value":2,"max":3}`:                "{max:number,min:number,value:number}",
		`{"on":{"value":true},"colorMode":"effect"}`: "{colorMode:string,on:{value:bool}}",
		`["a","b"]`:                     "[string]",
		`[]`:                            "[]",
		`[{"hue":1,"probability":0.0}]`: "[{hue:number,probability:number}]",
		`"effect"`:                      "string",
		`null`:                          "null",
		``:                              "nothing",
		`{`:                             "not json",
		`{"animData":null,"palette":[],"plugins":[{}]}`: "{animData:null,palette:[],plugins:[{}]}",
	} {
		if got := cassette.Shape(json.RawMessage(doc)); got != want {
			t.Errorf("Shape(%s) = %s, want %s", doc, got, want)
		}
	}
}
