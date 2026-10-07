// Package aurora is a client for the local HTTP API of Nanoleaf Light Panels,
// the original Aurora (model NL22). It talks to a controller on the local
// network: no cloud, no app, plain HTTP on port 16021.
//
// Nanoleaf publishes no machine-readable description of the API. This package
// is written against the documentation saved in ../aurora-api-specs and
// against what real controllers answer; where the two disagree the controller
// wins, and the README there records it.
//
// Standard library only, and nothing here knows about taproot.
package aurora

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultPort is the port the API listens on unless a controller's
	// discovery record says otherwise.
	DefaultPort = 16021

	// DefaultTimeout bounds one request. The controllers are small embedded
	// devices: they answer in tens of milliseconds when idle and in seconds
	// when busy, so this is generous rather than tight.
	DefaultTimeout = 30 * time.Second

	apiRoot          = "/api/v1"
	maxResponseBytes = 8 << 20
	errBodyPreview   = 300

	// shown wherever a path is printed, in place of the token it carries
	tokenPlaceholder = "<token>"
)

// ErrNoToken is returned by every call but NewToken on a client made without
// a token.
var ErrNoToken = errors.New("no token for this controller")

// ErrNotPairing is returned by NewToken while the controller is not handing
// out tokens.
var ErrNotPairing = errors.New("the controller is not handing out tokens: hold its power button for 5 to 7 seconds until the light flashes, then ask within 30 seconds")

// Client talks to one controller.
type Client struct {
	base   string // scheme, host and port
	host   string // host and port
	token  string
	http   *http.Client
	stream *http.Client // the event stream: http without the overall time limit
	dryRun func(Request)
}

// Request is a call the client would have made, handed to the function given
// to WithDryRun in place of sending it.
type Request struct {
	Method string
	// Path is the full path with the token replaced by a placeholder.
	Path string
	// Body is exactly what would have been sent, empty when there is none.
	Body []byte
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the HTTP client, for a caller that wants its own
// transport: logging, retries, a proxy. Redirects are refused whatever the
// client says, since the token travels in the path.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// WithTimeout bounds each request. Zero or less leaves DefaultTimeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.http.Timeout = d
		}
	}
}

// WithDryRun stops the client changing anything. Every call that would change
// the controller is handed to record instead of being sent, and returns as if
// it had worked; calls that only read are sent as usual.
func WithDryRun(record func(Request)) Option {
	return func(c *Client) { c.dryRun = record }
}

// New returns a client for the controller at host: an IP address or a host
// name, with a port when it is not DefaultPort. token may be empty, in which
// case only NewToken works.
func New(host, token string, opts ...Option) (*Client, error) {
	base, hostport, err := baseURL(host)
	if err != nil {
		return nil, err
	}

	c := &Client{base: base, host: hostport, token: token, http: &http.Client{Timeout: DefaultTimeout}}
	for _, opt := range opts {
		opt(c)
	}

	// a copy, so a caller's client is not changed under it
	h := *c.http
	h.CheckRedirect = refuseRedirect
	c.http = &h

	s := h
	s.Timeout = 0
	c.stream = &s

	return c, nil
}

// baseURL turns what a person types for a controller into the address the API
// is at.
func baseURL(host string) (base, hostport string, err error) {
	h := strings.TrimSpace(host)
	if h == "" {
		return "", "", errors.New("a controller address is required: an IP address or a host name, optionally with a port")
	}

	if strings.Contains(h, "://") {
		u, perr := url.Parse(h)
		if perr != nil || u.Host == "" {
			return "", "", fmt.Errorf("controller address %q is not a host or a url", host)
		}
		if u.Scheme != "http" {
			return "", "", fmt.Errorf("controller address %q: the API is plain http", host)
		}
		if u.User != nil {
			return "", "", fmt.Errorf("controller address %q must not carry credentials", host)
		}
		h = u.Host
	}
	h = strings.TrimRight(h, "/")

	if _, _, serr := net.SplitHostPort(h); serr != nil {
		// no port: a bare name, an IPv4 address, or an IPv6 one with or without brackets
		h = net.JoinHostPort(strings.Trim(h, "[]"), strconv.Itoa(DefaultPort))
	}
	if name, port, serr := net.SplitHostPort(h); serr != nil || name == "" || port == "" {
		return "", "", fmt.Errorf("controller address %q is not a host or host:port", host)
	}

	return (&url.URL{Scheme: "http", Host: h}).String(), h, nil
}

// refuseRedirect stops the client following a redirect. The token is part of
// the path, so a redirect anywhere else would hand it over, and Go turns a
// redirected PUT into a GET that reports success having changed nothing.
func refuseRedirect(_ *http.Request, via []*http.Request) error {
	return fmt.Errorf("%s was redirected: the address is not a controller's own", via[0].Method)
}

// Host is the controller's address as host:port.
func (c *Client) Host() string { return c.host }

// HasToken reports whether the client holds a token.
func (c *Client) HasToken() bool { return c.token != "" }

// WithToken returns a copy of the client that uses token.
func (c *Client) WithToken(token string) *Client {
	cp := *c
	cp.token = token

	return &cp
}

// StatusError is a controller answering with anything but success. Path never
// includes the token.
type StatusError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *StatusError) Error() string {
	msg := fmt.Sprintf("%s %s: HTTP %d %s", e.Method, e.Path, e.Status, http.StatusText(e.Status))
	if e.Body != "" {
		msg += ": " + e.Body
	}
	switch e.Status {
	case http.StatusUnauthorized:
		msg += " (the controller does not know this token: it was deleted, or the controller was reset)"
	case http.StatusUnprocessableEntity:
		msg += " (the controller understood the request and refused its contents)"
	}

	return msg
}

// IsNotFound reports whether err is a controller saying that what was asked
// for does not exist: an effect by a name it does not hold, a path it does
// not serve.
func IsNotFound(err error) bool {
	se, ok := errors.AsType[*StatusError](err)
	return ok && se.Status == http.StatusNotFound
}

// IsUnauthorized reports whether err is a controller refusing the token.
func IsUnauthorized(err error) bool {
	se, ok := errors.AsType[*StatusError](err)
	return ok && se.Status == http.StatusUnauthorized
}

// get reads path, decoding the answer into out.
func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.call(ctx, http.MethodGet, path, nil, out, false)
}

// read gets path and decodes the answer as a T.
func read[T any](ctx context.Context, c *Client, path string) (T, error) {
	var out T
	err := c.get(ctx, path, &out)

	return out, err
}

// put changes something: under WithDryRun it is recorded rather than sent.
// No change answers with anything worth reading.
func (c *Client) put(ctx context.Context, path string, body any) error {
	return c.call(ctx, http.MethodPut, path, body, nil, true)
}

// query is a PUT that only reads. The effect commands travel as writes
// whatever they do, and asking for an effect changes nothing.
func (c *Client) query(ctx context.Context, body, out any) error {
	return c.call(ctx, http.MethodPut, "/effects", body, out, false)
}

// del removes something.
func (c *Client) del(ctx context.Context, path string) error {
	return c.call(ctx, http.MethodDelete, path, nil, nil, true)
}

// call makes one request under the token and decodes a JSON answer into out.
// body is sent as JSON; a []byte or json.RawMessage is sent as it is.
func (c *Client) call(ctx context.Context, method, path string, body, out any, changes bool) error {
	raw, err := c.exchange(ctx, method, path, body, changes)
	if err != nil || out == nil || raw == nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("%s %s: the controller answered with nothing where it sends data", method, shown(path))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s %s: reading the answer: %w", method, shown(path), err)
	}

	return nil
}

// exchange makes one request under the token and returns the answer as it
// came: empty when the controller sent nothing, nil when a dry run kept the
// request from being sent.
func (c *Client) exchange(ctx context.Context, method, path string, body any, changes bool) ([]byte, error) {
	if c.token == "" {
		return nil, ErrNoToken
	}

	return c.send(ctx, method, apiRoot+"/"+c.token+path, apiRoot+"/"+tokenPlaceholder+path, path, body, changes)
}

// shown is a path as errors print it: relative to the token, never empty.
func shown(path string) string {
	if path == "" {
		return "/"
	}
	return path
}

// send does the work of call. full is the real path, redacted the same with
// the token hidden, and short the part after the token, for errors. It
// returns nil with no error when a dry run swallowed the request.
func (c *Client) send(ctx context.Context, method, full, redacted, short string, body any, changes bool) ([]byte, error) {
	payload, err := encode(body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, shown(short), err)
	}

	if changes && c.dryRun != nil {
		c.dryRun(Request{Method: method, Path: redacted, Body: payload})
		return nil, nil
	}

	var reader io.Reader = http.NoBody
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+full, reader)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, shown(short), unwrapURL(err))
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.http.Do(req)
	if err != nil {
		// the error carries the whole url, token included: keep what went wrong and drop where
		return nil, fmt.Errorf("%s %s on %s: %w", method, shown(short), c.host, unwrapURL(err))
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		preview, _ := io.ReadAll(io.LimitReader(res.Body, errBodyPreview))
		return nil, &StatusError{Method: method, Path: shown(short), Status: res.StatusCode, Body: strings.TrimSpace(string(preview))}
	}

	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s %s: reading the answer: %w", method, shown(short), err)
	}
	if len(raw) > maxResponseBytes {
		return nil, fmt.Errorf("%s %s: the answer is over %d bytes", method, shown(short), maxResponseBytes)
	}
	if raw == nil {
		raw = []byte{}
	}

	return raw, nil
}

// encode is a request body as bytes: nothing for nil, bytes as they are, and
// anything else as JSON.
func encode(body any) ([]byte, error) {
	switch b := body.(type) {
	case nil:
		return nil, nil
	case []byte:
		return b, nil
	case json.RawMessage:
		return b, nil
	default:
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding the request: %w", err)
		}
		return payload, nil
	}
}

// unwrapURL takes the address off an error from net/http, which quotes the
// url, and with it the token, in front of what went wrong.
func unwrapURL(err error) error {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		return urlErr.Err
	}
	return err
}
