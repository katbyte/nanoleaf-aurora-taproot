// Package cassette is a recording proxy for a controller, for the acceptance
// tests: it sits between the taproot binary and a real controller, writes
// down every exchange, and plays them back when there is no controller to
// ask, which is how those tests run without one.
//
// A controller cannot be started in a container the way a server can, so a
// recording made from a real one is the nearest thing a test run can have.
// The proxy rather than a recorder inside the client, so that what is tested
// is the binary as it ships.
//
// The program under test is given Token, never a real one: the proxy swaps
// the real token in on the way to the controller, so a recording cannot
// contain it. Serial numbers are blanked on the way to disk.
package cassette

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Token is the token to give the program under test. The proxy knows the
// real one; nothing else needs to.
const Token = "cassette"

// Mode is what the proxy does with a request.
type Mode int

const (
	// Replay answers from the recording and never touches the network. A
	// request that was not recorded is refused loudly rather than answered
	// with nothing.
	Replay Mode = iota
	// Record asks the real controller and writes down what it answers.
	Record
	// Verify answers from the recording, and also asks the real controller
	// and compares the shape of what it says, without rewriting anything:
	// firmware that has changed an answer shows up here.
	Verify
	// Live asks the real controller and records nothing.
	Live
)

// Options configure a Proxy.
type Options struct {
	Mode Mode
	// File is the recording.
	File string
	// Target is the real controller as host:port, and RealToken its token.
	// Neither is needed to Replay.
	Target    string
	RealToken string
	// ReadOnly refuses anything that would change the controller, without
	// passing it on. It is how a controller that must not be written to is
	// kept safe from a test that goes wrong.
	ReadOnly bool
}

// Exchange is one request and its answer.
type Exchange struct {
	Method string `json:"method"`
	// Path is the part after the token.
	Path     string          `json:"path"`
	Request  json.RawMessage `json:"request,omitempty"`
	Status   int             `json:"status"`
	Response json.RawMessage `json:"response,omitempty"`
	// Stream marks an event stream, which is opened and never read to an end.
	Stream bool `json:"stream,omitempty"`
}

// Recording is a cassette file.
type Recording struct {
	// Recorded is the day it was made.
	Recorded string `json:"recorded"`
	// Writes says the recording holds requests that change a controller, so
	// it cannot be checked against one without changing it again.
	Writes    bool       `json:"writes"`
	Exchanges []Exchange `json:"exchanges"`
}

// Proxy is a running proxy.
type Proxy struct {
	opts   Options
	server *httptest.Server
	client *http.Client

	mu       sync.Mutex
	rec      Recording
	next     map[string]int // how far through each request's recorded answers a replay is
	problems []string
}

// Start starts a proxy. Close stops it, and when it was recording writes the
// file.
func Start(opts Options) (*Proxy, error) {
	p := &Proxy{opts: opts, next: map[string]int{}, client: &http.Client{Timeout: 30 * time.Second}}

	if opts.Mode == Replay || opts.Mode == Verify {
		data, err := os.ReadFile(opts.File) // a recording named by the test
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &p.rec); err != nil {
			return nil, fmt.Errorf("%s: %w", opts.File, err)
		}
	}
	if opts.Mode != Replay && (opts.Target == "" || opts.RealToken == "") {
		return nil, errors.New("a proxy that reaches a controller needs its address and its token")
	}

	p.server = httptest.NewServer(http.HandlerFunc(p.serve))

	return p, nil
}

// Recorded reports whether a recording is there to replay.
func Recorded(file string) bool {
	_, err := os.Stat(file)
	return !errors.Is(err, fs.ErrNotExist)
}

// Host is where the proxy listens, as host:port: the address to give the
// program under test in place of the controller's.
func (p *Proxy) Host() string {
	return strings.TrimPrefix(p.server.URL, "http://")
}

// Problems is what went wrong so far: a request with no recording, a write a
// read-only proxy refused, an answer whose shape has changed. A test that
// finds any here has failed.
func (p *Proxy) Problems() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return slices.Clone(p.problems)
}

// Exchanges is every exchange so far, in order.
func (p *Proxy) Exchanges() []Exchange {
	p.mu.Lock()
	defer p.mu.Unlock()

	return slices.Clone(p.rec.Exchanges)
}

// Close stops the proxy and, when it was recording, writes the file.
func (p *Proxy) Close() error {
	p.server.CloseClientConnections()
	p.server.Close()
	if p.opts.Mode != Record {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.rec.Recorded = time.Now().Format(time.DateOnly)
	data, err := json.MarshalIndent(p.rec, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.opts.File), 0o750); err != nil {
		return err
	}

	return os.WriteFile(p.opts.File, append(data, '\n'), 0o644) //nolint:gosec // a recording is test data, for anyone to read
}

func (p *Proxy) problem(format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.problems = append(p.problems, fmt.Sprintf(format, args...))
}

func (p *Proxy) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	// a token is never handed out through a proxy: that needs a hand on a button
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/"+Token)
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if r.URL.RawQuery != "" {
		rest += "?" + r.URL.RawQuery
	}
	ex := Exchange{Method: r.Method, Path: rest, Request: asJSON(body)}

	if changes(r.Method, body) {
		if p.opts.ReadOnly {
			p.problem("%s %s %s: refused, this controller is only to be read", r.Method, rest, body)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		p.mu.Lock()
		p.rec.Writes = true
		p.mu.Unlock()
	}

	switch p.opts.Mode {
	case Replay:
		p.replay(w, ex)
	case Verify:
		recorded, found := p.replay(w, ex)
		p.mu.Lock()
		writes := p.rec.Writes
		p.mu.Unlock()
		// a recording with writes in it cannot be checked without making them again, which is recording
		if found && !recorded.Stream && !writes {
			p.verify(r, ex, recorded)
		}
	case Record, Live:
		p.forward(w, r, ex, body)
	}
}

// changes reports whether a request would change a controller: anything but
// a GET and the effect commands that only ask.
func changes(method string, body []byte) bool {
	if method == http.MethodGet {
		return false
	}
	var cmd struct {
		Write struct {
			Command string `json:"command"`
		} `json:"write"`
	}
	_ = json.Unmarshal(body, &cmd)

	return !slices.Contains([]string{"request", "requestAll", "requestPlugins"}, cmd.Write.Command)
}

// key is what a request is recognised by when it is replayed.
func (e Exchange) key() string {
	// compacted, since the file is written indented and the wire is not
	return e.Method + " " + e.Path + " " + string(fromJSON(e.Request))
}

// replay answers from the recording: the next answer recorded for this
// request, and the last one again once they run out.
func (p *Proxy) replay(w http.ResponseWriter, ex Exchange) (Exchange, bool) {
	p.mu.Lock()
	var matches []Exchange
	for _, rec := range p.rec.Exchanges {
		if rec.key() == ex.key() {
			matches = append(matches, rec)
		}
	}
	i := min(p.next[ex.key()], len(matches)-1)
	p.next[ex.key()]++
	p.mu.Unlock()

	if len(matches) == 0 {
		p.problem("%s %s %s: not in the recording %s (make record)", ex.Method, ex.Path, ex.Request, p.opts.File)
		w.WriteHeader(http.StatusNotImplemented)
		return Exchange{}, false
	}

	answer := matches[i]
	if answer.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(answer.Status)
		return answer, true
	}
	if len(answer.Response) > 0 {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(answer.Status)
	_, _ = w.Write(fromJSON(answer.Response))

	return answer, true
}

// forward asks the real controller, passes its answer back, and when
// recording writes the exchange down.
func (p *Proxy) forward(w http.ResponseWriter, r *http.Request, ex Exchange, body []byte) {
	res, err := p.ask(r, ex.Path, body)
	if err != nil {
		p.problem("%s %s: %v", ex.Method, ex.Path, err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() { _ = res.Body.Close() }()

	// an event stream is opened and left: there is no end to read to
	if res.Header.Get("Content-Type") == "text/event-stream" {
		ex.Status, ex.Stream = res.StatusCode, true
		p.keep(ex)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(res.StatusCode)
		return
	}

	answer, _ := io.ReadAll(res.Body)
	answer = scrub(answer, p.opts.RealToken)
	ex.Status, ex.Response = res.StatusCode, asJSON(answer)
	p.keep(ex)

	if ct := res.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(answer)
}

func (p *Proxy) keep(ex Exchange) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rec.Exchanges = append(p.rec.Exchanges, ex)
}

// ask sends a request on to the real controller under its real token.
func (p *Proxy) ask(r *http.Request, rest string, body []byte) (*http.Response, error) {
	var reader io.Reader = http.NoBody
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, "http://"+p.opts.Target+"/api/v1/"+p.opts.RealToken+rest, reader)
	if err != nil {
		return nil, errors.New("building the request for the controller")
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	res, err := p.client.Do(req) //nolint:gosec // the controller the test was pointed at
	if err != nil {
		// the error quotes the url, and the url holds the token
		return nil, fmt.Errorf("the controller at %s did not answer", p.opts.Target)
	}

	return res, nil
}

// verify asks the real controller what it would answer now and notes where
// the shape of that differs from what was recorded.
func (p *Proxy) verify(r *http.Request, ex, recorded Exchange) {
	res, err := p.ask(r, ex.Path, fromJSON(ex.Request))
	if err != nil {
		p.problem("%s %s: %v", ex.Method, ex.Path, err)
		return
	}
	defer func() { _ = res.Body.Close() }()
	now, _ := io.ReadAll(res.Body)

	if res.StatusCode != recorded.Status {
		p.problem("%s %s: the controller now answers %d, the recording has %d", ex.Method, ex.Path, res.StatusCode, recorded.Status)
		return
	}
	if was, is := Shape(recorded.Response), Shape(asJSON(now)); was != is {
		p.problem("%s %s: the answer has changed shape\n  recorded: %s\n  now:      %s", ex.Method, ex.Path, was, is)
	}
}

var serial = regexp.MustCompile(`("serialNo"\s*:\s*")[^"]*`)

// scrub takes out of an answer what must not be written down: the serial
// number that identifies the unit, and the token should a controller ever
// echo it.
func scrub(answer []byte, token string) []byte {
	answer = serial.ReplaceAll(answer, []byte("${1}S00000A0000"))
	if token != "" {
		answer = bytes.ReplaceAll(answer, []byte(token), []byte(Token))
	}

	return answer
}

// asJSON is a body as it goes in the file: itself when it is JSON, a JSON
// string of it when it is not, nothing when it is empty.
func asJSON(body []byte) json.RawMessage {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil
	}
	if json.Valid(body) {
		return json.RawMessage(body)
	}
	quoted, _ := json.Marshal(string(body))

	return quoted
}

// fromJSON is a body as it goes on the wire. Compacting undoes the indenting
// the file was written with, so what is replayed is byte for byte what was
// recorded.
func fromJSON(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return nil
	}
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return raw
	}

	return b.Bytes()
}

// Shape is the structure of a JSON value without its values: the names of an
// object's fields and the type of each, to any depth. Two answers with the
// same shape carry the same kind of thing whatever they say. A list is the
// shape of its first item, since a controller's lists hold one kind of thing.
func Shape(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "nothing"
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return "not json"
	}

	return shapeOf(v)
}

func shapeOf(v any) string {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k + ":" + shapeOf(t[k])
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		if len(t) == 0 {
			return "[]"
		}
		return "[" + shapeOf(t[0]) + "]"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	default:
		return "null"
	}
}
