// Package auroratest is a canned Light Panels controller for tests: an HTTP
// server that answers the way a real NL22 does, starting from what one on
// firmware 5.2.1 actually sent (fixtures/), and that remembers what is done to
// it. Tests of the client, the commands and the page all run against it, so
// none needs a device.
//
// What it does for a read is what the capture says. What it does for a write
// is what the documentation says and the live notes in
// ../../aurora-api-specs/README.md confirm; where neither does, it is marked.
package auroratest

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

//go:embed fixtures
var fixtures embed.FS

// DefaultFixture is the capture a Controller starts from: an NL22 with four
// panels and a Rhythm module, on firmware 5.2.1, holding seventeen effects.
const DefaultFixture = "nl22-5.2.1"

// Request is one request a Controller received.
type Request struct {
	Method string
	// Path is the path after the token, "/new" for a token request.
	Path string
	Body string
}

// Controller is a canned controller. Its methods are safe to call while it
// is being talked to.
type Controller struct {
	Server *httptest.Server

	mu       sync.Mutex
	info     map[string]json.RawMessage // the sections of the whole answer this type keeps no state for
	state    aurora.State
	orient   aurora.Range
	layout   json.RawMessage
	rhythm   map[string]json.RawMessage
	effects  []aurora.Effect
	selected string
	plugins  json.RawMessage

	tokens   map[string]bool
	issued   int
	pairing  bool
	pairWait int // token requests still to refuse before pairing opens by itself; -1 for never
	requests []Request
	storeAs  func(aurora.Effect) aurora.Effect
	failures map[string]int // "METHOD /path" to the status to answer with
	watchers map[chan string]bool
}

// New starts a controller from DefaultFixture and stops it when the test ends.
func New(tb testing.TB) *Controller {
	tb.Helper()

	c := &Controller{tokens: map[string]bool{}, pairWait: -1, failures: map[string]int{}, watchers: map[chan string]bool{}}
	if err := c.load(DefaultFixture); err != nil {
		tb.Fatalf("auroratest: %v", err)
	}
	c.Server = httptest.NewServer(http.HandlerFunc(c.serve))
	tb.Cleanup(c.Server.Close)

	return c
}

func (c *Controller) load(name string) error {
	read := func(file string, into any) error {
		data, err := fixtures.ReadFile("fixtures/" + name + "/" + file)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, into)
	}

	if err := read("info.json", &c.info); err != nil {
		return fmt.Errorf("fixture %s: info: %w", name, err)
	}
	var effects struct {
		Animations []aurora.Effect `json:"animations"`
	}
	if err := read("effects.json", &effects); err != nil {
		return fmt.Errorf("fixture %s: effects: %w", name, err)
	}
	if err := read("plugins.json", &c.plugins); err != nil {
		return fmt.Errorf("fixture %s: plugins: %w", name, err)
	}
	c.effects = effects.Animations

	var selected struct {
		Select string `json:"select"`
	}
	var panels struct {
		GlobalOrientation aurora.Range    `json:"globalOrientation"`
		Layout            json.RawMessage `json:"layout"`
	}
	for section, into := range map[string]any{"state": &c.state, "effects": &selected, "panelLayout": &panels, "rhythm": &c.rhythm} {
		if err := json.Unmarshal(c.info[section], into); err != nil {
			return fmt.Errorf("fixture %s: info.%s: %w", name, section, err)
		}
	}
	c.selected, c.orient, c.layout = selected.Select, panels.GlobalOrientation, panels.Layout

	return nil
}

// Host is the controller's address as host:port.
func (c *Controller) Host() string {
	return strings.TrimPrefix(c.Server.URL, "http://")
}

// Token hands out a token the controller accepts, as if it had been paired
// with some time ago.
func (c *Controller) Token() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.issue()
}

func (c *Controller) issue() string {
	c.issued++
	token := fmt.Sprintf("%032d", c.issued)
	c.tokens[token] = true

	return token
}

// Pair opens the pairing window, as holding the power button does. It closes
// again when a token has been handed out.
func (c *Controller) Pair() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pairing = true
}

// PairAfter opens the pairing window once n token requests have been refused:
// somebody reaching the button while a client waits.
func (c *Controller) PairAfter(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pairWait = n
}

// Unplug takes the controller off the network: nothing answers at its
// address any more, and whatever was connected to it is cut off, an open
// event stream included.
func (c *Controller) Unplug() {
	_ = c.Server.Listener.Close()
	c.Server.CloseClientConnections()
}

// Fail makes the controller answer status to every request of that method to
// that path (the part after the token), until told otherwise with status 0.
func (c *Controller) Fail(method, path string, status int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if status == 0 {
		delete(c.failures, method+" "+path)
		return
	}
	c.failures[method+" "+path] = status
}

// FailCommand makes the controller answer status to one effect command, add
// or delete say, and leave the others alone, until told otherwise with
// status 0. The commands all share a path, so Fail cannot tell them apart.
func (c *Controller) FailCommand(command string, status int) {
	c.Fail("command", command, status)
}

// StoreAs has the controller change each effect it is given before keeping
// it, the way firmware that writes a scene its own way would: dropping a
// field it has no use for, say. What comes back when the effect is read is
// what was kept, not what was sent.
func (c *Controller) StoreAs(change func(aurora.Effect) aurora.Effect) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.storeAs = change
}

// Requests is every request received so far, in order.
func (c *Controller) Requests() []Request {
	c.mu.Lock()
	defer c.mu.Unlock()

	return slices.Clone(c.requests)
}

// Writes is the requests that were not reads: everything but GETs and the
// effect commands that only ask.
func (c *Controller) Writes() []Request {
	var out []Request
	for _, r := range c.Requests() {
		if r.Method == http.MethodGet {
			continue
		}
		if cmd := commandOf(r.Body); cmd == "request" || cmd == "requestAll" || cmd == "requestPlugins" {
			continue
		}
		out = append(out, r)
	}

	return out
}

func commandOf(body string) string {
	var b struct {
		Write struct {
			Command string `json:"command"`
		} `json:"write"`
	}
	_ = json.Unmarshal([]byte(body), &b) // a body that is not a command has none

	return b.Write.Command
}

// EffectNames lists the effects the controller holds, as it would.
func (c *Controller) EffectNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.names()
}

func (c *Controller) names() []string {
	names := make([]string, len(c.effects))
	for i, e := range c.effects {
		names[i] = e.Name()
	}

	return names
}

// Effect is one effect the controller holds.
func (c *Controller) Effect(name string) (aurora.Effect, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i := c.index(name); i >= 0 {
		return c.effects[i], true
	}

	return aurora.Effect{}, false
}

// RemoveEffect takes an effect off the controller, as if it had been lost.
func (c *Controller) RemoveEffect(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i := c.index(name); i >= 0 {
		c.effects = slices.Delete(c.effects, i, i+1)
	}
}

// PutEffect stores an effect on the controller directly.
func (c *Controller) PutEffect(e aurora.Effect) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store(e)
}

// Selected is the name of the effect that is running.
func (c *Controller) Selected() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.selected
}

// State is the controller's power, brightness and colour.
func (c *Controller) State() aurora.State {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.state
}

// Set changes something the controller says about itself: "name",
// "serialNo", "firmwareVersion". Every Controller starts out as the same
// device, so two that are to be told apart need this.
func (c *Controller) Set(field string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.info[field] = mustJSON(value)
}

func (c *Controller) index(name string) int {
	return slices.IndexFunc(c.effects, func(e aurora.Effect) bool { return e.Name() == name })
}

// store keeps an effect under its name, in the order a controller lists them:
// by byte, so capitals come before lower case.
func (c *Controller) store(e aurora.Effect) {
	if i := c.index(e.Name()); i >= 0 {
		c.effects[i] = e
		return
	}
	c.effects = append(c.effects, e)
	slices.SortFunc(c.effects, func(a, b aurora.Effect) int { return strings.Compare(a.Name(), b.Name()) })
}

func (c *Controller) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body) // a body cut short reads as what arrived

	rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1")
	if !ok {
		http.NotFound(w, r)
		return
	}

	if rest == "/new" {
		c.record(r.Method, rest, body)
		c.newToken(w, r)
		return
	}

	token, path, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
	path = "/" + path
	if rest == "/"+token { // no slash after the token: only a DELETE comes this way
		path = ""
	}
	c.record(r.Method, path, body)

	c.mu.Lock()
	known := c.tokens[token]
	status := c.failures[r.Method+" "+path]
	if status == 0 {
		status = c.failures["command "+commandOf(string(body))]
	}
	c.mu.Unlock()
	switch {
	case !known:
		w.WriteHeader(http.StatusUnauthorized)
	case status != 0:
		w.WriteHeader(status)
	case path == "/events" && r.Method == http.MethodGet:
		c.events(w, r)
	default:
		c.mu.Lock()
		code, answer, notes := c.handle(r.Method, path, token, body)
		c.mu.Unlock()
		reply(w, code, answer)
		for _, n := range notes {
			c.broadcast(n)
		}
	}
}

func (c *Controller) record(method, path string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, Request{Method: method, Path: path, Body: string(body)})
}

func (c *Controller) newToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.pairing && c.pairWait == 0 {
		c.pairing, c.pairWait = true, -1
	}
	if !c.pairing {
		if c.pairWait > 0 {
			c.pairWait--
		}
		w.WriteHeader(http.StatusForbidden) // as a real one: 403 with nothing in it
		return
	}
	c.pairing = false
	reply(w, http.StatusOK, map[string]string{"auth_token": c.issue()})
}

func reply(w http.ResponseWriter, code int, answer any) {
	if answer == nil {
		w.WriteHeader(code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(answer) // the test's client went away
}

// handle answers one request under a known token. It runs with the lock held
// and returns the events the change causes, to send once it is released.
func (c *Controller) handle(method, path, token string, body []byte) (code int, answer any, notes []string) {
	get, put := method == http.MethodGet, method == http.MethodPut

	switch {
	case path == "" && method == http.MethodDelete:
		delete(c.tokens, token)
		return http.StatusNoContent, nil, nil
	case path == "/" && get:
		return http.StatusOK, c.whole(), nil
	case path == "/identify" && put:
		return http.StatusNoContent, nil, nil

	case path == "/state" && get:
		return http.StatusOK, c.state, nil
	case path == "/state" && put:
		return c.putState(body)
	case strings.HasPrefix(path, "/state/") && get:
		return one(c.state, strings.TrimPrefix(path, "/state/"))

	case path == "/effects/effectsList" && get:
		return http.StatusOK, c.names(), nil
	case path == "/effects/select" && get:
		return http.StatusOK, c.selected, nil
	case path == "/effects" && put:
		return c.putEffects(body)

	case path == "/panelLayout/layout" && get:
		return http.StatusOK, c.layout, nil
	case path == "/panelLayout/globalOrientation" && get:
		return http.StatusOK, c.orient, nil
	case path == "/panelLayout" && put:
		return c.putOrientation(body)

	case path == "/rhythm" && get:
		return http.StatusOK, c.rhythm, nil
	case path == "/rhythm/rhythmMode" && put:
		var in struct {
			Mode *int `json:"rhythmMode"`
		}
		if json.Unmarshal(body, &in) != nil || in.Mode == nil {
			return http.StatusBadRequest, nil, nil
		}
		c.rhythm["rhythmMode"] = mustJSON(*in.Mode)
		return http.StatusNoContent, nil, nil
	case strings.HasPrefix(path, "/rhythm/") && get:
		if v, ok := c.rhythm[strings.TrimPrefix(path, "/rhythm/")]; ok {
			return http.StatusOK, v, nil
		}
	}

	return http.StatusNotFound, nil, nil
}

// whole is the answer to a read of everything: the capture's sections, with
// the ones that can change filled in from what the controller holds now.
func (c *Controller) whole() map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(c.info))
	maps.Copy(out, c.info)
	out["state"] = mustJSON(c.state)
	out["effects"] = mustJSON(map[string]any{"effectsList": c.names(), "select": c.selected})
	out["panelLayout"] = mustJSON(map[string]any{"globalOrientation": c.orient, "layout": c.layout})
	out["rhythm"] = mustJSON(c.rhythm)

	return out
}

// one is a single value of a section, by its name in the API.
func one(section any, name string) (code int, answer any, notes []string) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(mustJSON(section), &fields) != nil {
		return http.StatusInternalServerError, nil, nil
	}
	if v, ok := fields[name]; ok {
		return http.StatusOK, v, nil
	}

	return http.StatusNotFound, nil, nil
}

type change struct {
	Value     *json.RawMessage `json:"value"`
	Increment *int             `json:"increment"`
}

// apply moves a ranged value the way the documentation describes: a value
// outside the range is refused, an increment stops at the limits.
func (ch change) apply(r *aurora.Range) bool {
	switch {
	case ch.Value != nil:
		var v int
		if json.Unmarshal(*ch.Value, &v) != nil || v < r.Min || v > r.Max {
			return false
		}
		r.Value = v
	case ch.Increment != nil:
		r.Value = min(r.Max, max(r.Min, r.Value+*ch.Increment))
	default:
		return false
	}

	return true
}

func (c *Controller) putState(body []byte) (code int, answer any, notes []string) {
	var in map[string]change
	if json.Unmarshal(body, &in) != nil || len(in) == 0 {
		return http.StatusBadRequest, nil, nil
	}

	for key, ch := range in {
		var ok bool
		switch key {
		case "on":
			ok = ch.Value != nil && json.Unmarshal(*ch.Value, &c.state.On.Value) == nil
			notes = append(notes, event(aurora.EventState, aurora.AttrOn, c.state.On.Value))
		case "brightness":
			ok = ch.apply(&c.state.Brightness)
			notes = append(notes, event(aurora.EventState, aurora.AttrBrightness, c.state.Brightness.Value))
		case "hue":
			ok = ch.apply(&c.state.Hue)
			c.state.ColorMode, c.selected = aurora.ColorModeHS, aurora.EffectSolid
		case "sat":
			ok = ch.apply(&c.state.Sat)
			c.state.ColorMode, c.selected = aurora.ColorModeHS, aurora.EffectSolid
		case "ct":
			ok = ch.apply(&c.state.CT)
			c.state.ColorMode, c.selected = aurora.ColorModeCT, aurora.EffectSolid
		default:
			return http.StatusBadRequest, nil, nil
		}
		if !ok {
			return http.StatusUnprocessableEntity, nil, nil
		}
	}

	return http.StatusNoContent, nil, notes
}

func (c *Controller) putOrientation(body []byte) (code int, answer any, notes []string) {
	var in struct {
		GlobalOrientation change `json:"globalOrientation"`
	}
	if json.Unmarshal(body, &in) != nil {
		return http.StatusBadRequest, nil, nil
	}
	if !in.GlobalOrientation.apply(&c.orient) {
		return http.StatusUnprocessableEntity, nil, nil
	}

	return http.StatusNoContent, nil, []string{event(aurora.EventLayout, aurora.AttrGlobalOrientation, c.orient.Value)}
}

func (c *Controller) putEffects(body []byte) (code int, answer any, notes []string) {
	var in struct {
		Select *string        `json:"select"`
		Write  *aurora.Effect `json:"write"`
	}
	if json.Unmarshal(body, &in) != nil {
		return http.StatusBadRequest, nil, nil
	}

	if in.Select != nil {
		if c.index(*in.Select) < 0 {
			return http.StatusNotFound, nil, nil
		}
		return http.StatusNoContent, nil, c.run(*in.Select)
	}
	if in.Write == nil {
		return http.StatusBadRequest, nil, nil
	}

	// a command is an effect's worth of fields with the command among them
	cmd := *in.Write
	name := cmd.Name()
	var command string
	if raw, ok := cmd.Field("command"); ok {
		_ = json.Unmarshal(raw, &command) // a command that is not a string is no command
	}

	switch command {
	case "request":
		if i := c.index(name); i >= 0 {
			return http.StatusOK, c.effects[i], nil
		}
		return http.StatusNotFound, nil, nil // as a real one: 404 with nothing in it
	case "requestAll":
		return http.StatusOK, map[string]any{"animations": c.effects}, nil
	case "requestPlugins":
		return http.StatusOK, c.plugins, nil
	case "add":
		if name == "" {
			return http.StatusBadRequest, nil, nil
		}
		kept := cmd.Without("command")
		if c.storeAs != nil {
			kept = c.storeAs(kept)
		}
		c.store(kept)
		return http.StatusNoContent, nil, nil
	case "delete":
		i := c.index(name)
		if i < 0 {
			return http.StatusNotFound, nil, nil
		}
		c.effects = slices.Delete(c.effects, i, i+1)
		return http.StatusNoContent, nil, nil
	case "rename":
		var to string
		if raw, ok := cmd.Field("newName"); ok {
			_ = json.Unmarshal(raw, &to) // a name that is not a string is no name
		}
		i := c.index(name)
		if i < 0 || to == "" {
			return http.StatusNotFound, nil, nil
		}
		renamed := c.effects[i].WithName(to)
		c.effects = slices.Delete(c.effects, i, i+1)
		c.store(renamed)
		return http.StatusNoContent, nil, nil
	case "display":
		return http.StatusNoContent, nil, c.run(aurora.EffectDynamic)
	case "displayTemp":
		return http.StatusNoContent, nil, nil
	}

	return http.StatusBadRequest, nil, nil
}

// run makes an effect the one that is running.
func (c *Controller) run(name string) []string {
	c.selected, c.state.ColorMode = name, aurora.ColorModeEffect

	return []string{event(aurora.EventEffects, aurora.AttrSelectedEffect, name)}
}

// event is one event as it goes down the stream.
func event(kind aurora.EventType, attr int, value any) string {
	data := mustJSON(map[string]any{"events": []map[string]any{{"attr": attr, "value": value}}})
	return "id: " + strconv.Itoa(int(kind)) + "\ndata: " + string(data) + "\n\n"
}

// Emit sends an event to everything listening, as a change made at the
// controller itself would.
func (c *Controller) Emit(kind aurora.EventType, attr int, value any) {
	c.broadcast(event(kind, attr, value))
}

// Listeners is how many event streams are open.
func (c *Controller) Listeners() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.watchers)
}

func (c *Controller) broadcast(note string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for ch := range c.watchers {
		select {
		case ch <- note:
		default: // a listener that has stopped reading misses it
		}
	}
}

func (c *Controller) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := make(chan string, 32)
	c.mu.Lock()
	c.watchers[ch] = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.watchers, ch)
		c.mu.Unlock()
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case note := <-ch:
			if _, err := io.WriteString(w, note); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// mustJSON encodes a value this package made itself.
func mustJSON(v any) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic("auroratest: " + err.Error())
	}

	return bytes.TrimSpace(b.Bytes())
}
