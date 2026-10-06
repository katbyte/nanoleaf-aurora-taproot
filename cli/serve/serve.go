package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/go-kt/cout"
	"github.com/katbyte/go-kt/version"
	"github.com/katbyte/nanoleaf-aurora-taproot/assets"
	"github.com/katbyte/nanoleaf-aurora-taproot/cli"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// pageHeader is the header the page sends with every request that changes
// something. A browser will not let another site's page set it without
// asking first, and taproot never says yes.
const pageHeader = "X-Taproot"

// server is the page and what is behind it.
type server struct {
	f     *cli.FlagData
	allow []string  // the names the page may be reached by, besides the ones always allowed
	out   io.Writer // the request log

	// file is held across every read-change-save of the controllers file
	file sync.Mutex

	snap    snapshot
	plugins sync.Map // controller name to the plugins it has, which do not change
	hub     *hub
	pairs   *pairings
}

// run serves the page on addr until ctx ends.
func run(ctx context.Context, f *cli.FlagData, addr string) error {
	if !strings.Contains(addr, ":") {
		addr = ":" + addr // a bare port
	}

	s := newServer(f, cout.Out)
	srv := &http.Server{Addr: addr, Handler: s.handler(), ReadHeaderTimeout: 10 * time.Second}

	// the event streams the page holds open never end by themselves: this ends them when the server stops
	live, stop := context.WithCancel(ctx)
	defer stop()
	srv.BaseContext = func(net.Listener) context.Context { return live }
	s.hub.start(live, s)

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	port := ""
	if _, p, perr := net.SplitHostPort(ln.Addr().String()); perr == nil {
		port = p // the real port, when 0 asked for a free one
	}

	cout.Printf("\n🌱 taproot %s serving — ctrl-c stops\n", version.Version)
	if f.DryRun {
		cout.Printf("  <yellow>dry run:</> every change made on the page is printed here instead of sent\n")
	}
	for _, h := range reachableHosts(addr) {
		cout.Printf("  <cyan>http://%s/</>\n", net.JoinHostPort(h, port))
	}
	cout.Printf("  controllers and backups in %s\n\n", f.ConfigDir)

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serving: %w", err)
	case <-ctx.Done():
		cout.Printf("\nstopping\n")
		stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx) //nolint:contextcheck // the context that ended is the reason this runs
	}
}

func newServer(f *cli.FlagData, out io.Writer) *server {
	return &server{f: f, allow: f.Cmd.Serve.AllowHosts, out: out, hub: newHub(), pairs: newPairings()}
}

// handler is the page, its two files, and the API behind it.
func (s *server) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", page)
	mux.HandleFunc("GET /page.css", static(assets.PageCSS, "text/css; charset=utf-8"))
	mux.HandleFunc("GET /page.js", static(assets.PageJS, "text/javascript; charset=utf-8"))

	mux.HandleFunc("GET /api/state", s.getState)
	mux.HandleFunc("GET /api/events", s.hub.serve)
	mux.HandleFunc("GET /api/find", s.getFind)
	mux.HandleFunc("POST /api/connect", s.postConnect)
	mux.HandleFunc("DELETE /api/connect", s.deleteConnect)
	mux.HandleFunc("POST /api/copy", s.postCopy)
	mux.HandleFunc("PUT /api/controllers/{name}/power", s.putPower)
	mux.HandleFunc("PUT /api/controllers/{name}/brightness", s.putBrightness)
	mux.HandleFunc("PUT /api/controllers/{name}/scene", s.putScene)
	mux.HandleFunc("POST /api/controllers/{name}/identify", s.postIdentify)
	mux.HandleFunc("POST /api/controllers/{name}/backup", s.postBackup)
	mux.HandleFunc("DELETE /api/controllers/{name}", s.deleteController)

	return s.logged(s.guarded(mux))
}

func page(w http.ResponseWriter, _ *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	// the page shows names that come from controllers; this is the second lock on that door, after
	// the script only ever setting them as text: nothing runs or loads that did not come from here
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	_, _ = io.WriteString(w, strings.ReplaceAll(assets.PageHTML, "{{version}}", version.Version)) // the browser went away
}

func static(body, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache") // a new taproot's page must win over a cached one
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.WriteString(w, body) // the browser went away
	}
}

// guarded refuses what did not come from the page itself.
//
// The page has no login and controls real things, and a browser will carry a
// request to it from any site the person happens to be reading. Two checks
// stop that. The name the request was sent to must be one this server
// expects, which stops a site pointing a name of its own at this address.
// And a request that changes anything must carry the page's header and come
// from the page's own address, which a browser will not let another site
// arrange.
func (s *server) guarded(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAllowed(r.Host) {
			fail(w, http.StatusForbidden, fmt.Errorf("taproot does not answer to the name %q: start it with --allow-host %s (or TAPROOT_ALLOW_HOSTS) if this is how you reach it", r.Host, hostOnly(r.Host)))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get(pageHeader) == "" {
				fail(w, http.StatusForbidden, errors.New("this request did not come from the taproot page"))
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				if u, err := url.Parse(origin); err != nil || !strings.EqualFold(u.Host, r.Host) {
					fail(w, http.StatusForbidden, errors.New("this request came from another site's page"))
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

// hostAllowed reports whether the page may be reached by this name. An IP
// address always may: a name is what can be made to point somewhere it
// should not.
func (s *server) hostAllowed(hostport string) bool {
	host := strings.ToLower(strings.Trim(hostOnly(hostport), "[]"))
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".local") || net.ParseIP(host) != nil {
		return true
	}
	if own, err := os.Hostname(); err == nil && strings.EqualFold(own, host) {
		return true
	}

	return slices.ContainsFunc(s.allow, func(a string) bool {
		a = strings.ToLower(strings.TrimSpace(a))
		return a == "*" || a == host
	})
}

// logged prints who did what. The page's own polling is left out: it is
// nothing anyone did.
func (s *server) logged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if r.Method == http.MethodGet && r.URL.Path != "/" && r.URL.Path != "/api/find" {
			return
		}
		colour := "green"
		if sw.status >= 400 {
			colour = "red"
		}
		_, _ = fmt.Fprint(s.out, cout.Sprintf("<gray>%s</> <cyan>%s</> %s %s <%s>%d</> <gray>%s</>\n", // a log line that cannot be written has nowhere to say so
			start.Format("2006-01-02 15:04:05"), from(r), r.Method, r.URL.Path, colour, sw.status, time.Since(start).Round(time.Millisecond)))
	})
}

// from is the address a request came from, without its port.
func from(r *http.Request) string {
	return hostOnly(r.RemoteAddr)
}

// statusWriter remembers the status a handler answered with, for the log.
type statusWriter struct {
	http.ResponseWriter

	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush lets an event stream through the log's wrapper.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// reply answers with v as JSON.
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		clog.Log.Debugf("writing an answer: %v", err)
	}
}

// fail answers with an error the page can show as it is.
func fail(w http.ResponseWriter, status int, err error) {
	reply(w, status, map[string]string{"error": err.Error()})
}

// failFor answers with an error from a controller or the controllers file,
// under the status that says whose fault it was.
func failFor(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway // the controller, or the way to it
	switch {
	case aurora.IsNotFound(err):
		status = http.StatusNotFound
	case errors.Is(err, errNoSuchController), errors.Is(err, errBadRequest):
		status = http.StatusBadRequest
	}
	fail(w, status, err)
}

var (
	errNoSuchController = errors.New("no such controller")
	errBadRequest       = errors.New("bad request")
)

// read decodes a request's JSON body into v.
func read(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: the request is not the JSON this expects: %w", errBadRequest, err)
	}
	return nil
}

// reachableHosts names the addresses the page can be opened on: localhost,
// the machine's hostname, and each non-loopback IPv4 address — or just the
// bound address when the listener is pinned to one.
func reachableHosts(addr string) []string {
	host, _, _ := net.SplitHostPort(addr) // an address that does not split has no host to pin to
	if host != "" && host != "0.0.0.0" && host != "::" {
		return []string{host}
	}
	hosts := []string{"localhost"}
	if hn, err := os.Hostname(); err == nil && hn != "" {
		hn = strings.ToLower(hn)
		if !strings.Contains(hn, ".") {
			hn += ".local" // the name other machines resolve over mdns
		}
		hosts = append(hosts, hn)
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return hosts
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && ipn.IP.To4() != nil {
			hosts = append(hosts, ipn.IP.String())
		}
	}
	return hosts
}
