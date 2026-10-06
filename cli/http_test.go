package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/katbyte/go-kt/clog"
)

func TestRedact(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{ //nolint:gosec // the tokens here are made up, to be redacted
		"GET /api/v1/aB3dE6gH9jK2mN5pQ8sT1vW4yZ7cF0xR/state HTTP/1.1": "GET /api/v1/<token>/state HTTP/1.1",
		"DELETE /api/v1/aB3dE6gH9jK2mN5pQ8sT1vW4yZ7cF0xR HTTP/1.1":    "DELETE /api/v1/<token> HTTP/1.1",
		`{"auth_token":"aB3dE6gH9jK2mN5pQ8sT1vW4yZ7cF0xR"}`:           `{"auth_token":"<token>"}`,
		`{"auth_token" : "short"}`:                                    `{"auth_token" : "<token>"}`,
		// the paths with no token in them are left alone
		"POST /api/v1/new HTTP/1.1":   "POST /api/v1/new HTTP/1.1",
		"GET /api/v1/ HTTP/1.1":       "GET /api/v1/ HTTP/1.1",
		`{"animName":"Flames"}`:       `{"animName":"Flames"}`,
		"GET /api/state HTTP/1.1":     "GET /api/state HTTP/1.1",
		"Host: 10.0.5.183:16021\r\n":  "Host: 10.0.5.183:16021\r\n",
		"Location: /api/v1/events?id": "Location: /api/v1/events?id",
	} {
		if got := string(Redact([]byte(in))); got != want {
			t.Errorf("Redact(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

// TAPROOT_LOG=trace dumps every exchange, and a token is in the path of
// every one of them: what is logged must have it taken out.
func TestTraceLoggingLeavesTheTokenOut(t *testing.T) { //nolint:paralleltest // changes the process's logger
	const token = "aB3dE6gH9jK2mN5pQ8sT1vW4yZ7cF0xR" //nolint:gosec // made up, to be redacted

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/events"):
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/new":
			_, _ = io.WriteString(w, `{"auth_token":"`+token+`"}`)
		default:
			_, _ = io.WriteString(w, `{"value":true}`)
		}
	}))
	t.Cleanup(srv.Close)

	var logged bytes.Buffer
	oldOut, oldLevel := clog.Log.Out, clog.Log.GetLevel()
	clog.Log.SetOutput(&logged)
	clog.Log.SetLevel(logrus.TraceLevel)
	defer func() {
		clog.Log.SetOutput(oldOut)
		clog.Log.SetLevel(oldLevel)
	}()

	client := NewHTTPClient(5 * time.Second)
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/" + token + "/state/on"},
		{http.MethodPost, "/api/v1/new"},
		{http.MethodGet, "/api/v1/" + token + "/events"},
	} {
		req, err := http.NewRequestWithContext(t.Context(), call.method, srv.URL+call.path, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		// what the caller reads is untouched: only the log is redacted
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if call.path == "/api/v1/new" && !strings.Contains(string(body), token) {
			t.Errorf("the answer itself lost its token: %s", body)
		}
	}

	out := logged.String()
	if strings.Contains(out, token) {
		t.Errorf("the token is in the log:\n%s", out)
	}
	for _, part := range []string{"aurora request", "aurora response", "/api/v1/<token>/state/on", `auth_token\":\"<token>`, "text/event-stream"} {
		if !strings.Contains(out, part) {
			t.Errorf("the log is missing %q:\n%s", part, out)
		}
	}

	// at the usual level nothing is dumped at all
	logged.Reset()
	clog.Log.SetLevel(logrus.WarnLevel)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/"+token+"/state/on", http.NoBody)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if logged.Len() != 0 {
		t.Errorf("logged without being asked to:\n%s", logged.String())
	}
}
