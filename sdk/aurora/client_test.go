package aurora

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBaseURL(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ in, base, host string }{
		{"10.0.5.183", "http://10.0.5.183:16021", "10.0.5.183:16021"},
		{" 10.0.5.183 ", "http://10.0.5.183:16021", "10.0.5.183:16021"},
		{"10.0.5.183:8080", "http://10.0.5.183:8080", "10.0.5.183:8080"},
		{"Light-Panels-53-A6-3C.local", "http://Light-Panels-53-A6-3C.local:16021", "Light-Panels-53-A6-3C.local:16021"},
		{"http://panels.lan", "http://panels.lan:16021", "panels.lan:16021"},
		{"http://panels.lan:16021/", "http://panels.lan:16021", "panels.lan:16021"},
		{"fe80::1", "http://[fe80::1]:16021", "[fe80::1]:16021"},
		{"[fe80::1]", "http://[fe80::1]:16021", "[fe80::1]:16021"},
		{"[fe80::1]:9", "http://[fe80::1]:9", "[fe80::1]:9"},
	} {
		base, host, err := baseURL(tc.in)
		if err != nil || base != tc.base || host != tc.host {
			t.Errorf("baseURL(%q) = %q, %q, %v; want %q, %q", tc.in, base, host, err, tc.base, tc.host)
		}
	}

	for _, bad := range []string{"", "  ", "https://panels.lan", "http://", "http://kt:secret@panels.lan", ":16021"} {
		if base, _, err := baseURL(bad); err == nil {
			t.Errorf("baseURL(%q) = %q; want an error", bad, base)
		}
	}
}

// A token is a password that travels in the path, so nothing the client
// reports may quote the path it asked for.
func TestErrorsNeverCarryTheToken(t *testing.T) {
	t.Parallel()
	const token = "s3cretTokenThatMustNotLeak0000000"

	t.Run("a controller that is not there", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.NotFoundHandler())
		addr := strings.TrimPrefix(srv.URL, "http://")
		srv.Close()

		c, err := New(addr, token, WithTimeout(2*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Info(t.Context())
		if err == nil {
			t.Fatal("want an error from a closed port")
		}
		if strings.Contains(err.Error(), token) {
			t.Errorf("the error quotes the token: %v", err)
		}
		if !strings.Contains(err.Error(), addr) {
			t.Errorf("the error should say which controller: %v", err)
		}
	})

	t.Run("a controller that refuses", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "no: "+r.URL.Path, http.StatusTeapot) // even a body that repeats the path
		}))
		t.Cleanup(srv.Close)

		c, err := New(srv.URL, token)
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Brightness(t.Context())
		se, ok := err.(*StatusError) //nolint:errorlint // the client returns it bare
		if !ok || se.Status != http.StatusTeapot || se.Path != "/state/brightness" || se.Method != http.MethodGet {
			t.Fatalf("got %#v", err)
		}
		if strings.Contains(se.Path, token) {
			t.Errorf("the error's path quotes the token: %q", se.Path)
		}
	})

	t.Run("a redirect", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://elsewhere.invalid"+r.URL.Path, http.StatusFound) //nolint:gosec // the redirect is what is being tested
		}))
		t.Cleanup(srv.Close)

		c, err := New(srv.URL, token)
		if err != nil {
			t.Fatal(err)
		}
		err = c.SetOn(t.Context(), true)
		if err == nil || !strings.Contains(err.Error(), "redirected") {
			t.Fatalf("a redirect must be refused, got %v", err)
		}
		if strings.Contains(err.Error(), token) {
			t.Errorf("the error quotes the token: %v", err)
		}
	})
}

func TestAnswersThatAreNotWhatWasAsked(t *testing.T) {
	t.Parallel()

	serve := func(t *testing.T, body string) *Client {
		t.Helper()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body)) // the test's client went away
		}))
		t.Cleanup(srv.Close)
		c, err := New(srv.URL, "token")
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	t.Run("nothing where data is sent", func(t *testing.T) {
		t.Parallel()
		if _, err := serve(t, "").EffectNames(t.Context()); err == nil || !strings.Contains(err.Error(), "answered with nothing") {
			t.Errorf("got %v", err)
		}
	})
	t.Run("not json", func(t *testing.T) {
		t.Parallel()
		if _, err := serve(t, "<html>").EffectNames(t.Context()); err == nil || !strings.Contains(err.Error(), "reading the answer") {
			t.Errorf("got %v", err)
		}
	})
	t.Run("too long", func(t *testing.T) {
		t.Parallel()
		if _, err := serve(t, strings.Repeat(" ", maxResponseBytes+1)).EffectNames(t.Context()); err == nil || !strings.Contains(err.Error(), "bytes") {
			t.Errorf("got %v", err)
		}
	})
}

func TestNoToken(t *testing.T) {
	t.Parallel()

	c, err := New("10.0.5.183", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.HasToken() {
		t.Error("HasToken with none")
	}
	if _, err := c.Info(t.Context()); err != ErrNoToken { //nolint:errorlint // returned bare
		t.Errorf("Info: %v", err)
	}
	if err := c.Events(t.Context(), nil, func(Event) {}); err != ErrNoToken { //nolint:errorlint // returned bare
		t.Errorf("Events: %v", err)
	}
	if !c.WithToken("x").HasToken() || c.HasToken() {
		t.Error("WithToken must return a copy")
	}
}

func TestEncode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   any
		want string
	}{
		{nil, ""},
		{[]byte(`{"a":1}`), `{"a":1}`},
		{json.RawMessage(`[1]`), `[1]`},
		{map[string]int{"a": 1}, `{"a":1}`},
	} {
		got, err := encode(tc.in)
		if err != nil || string(got) != tc.want {
			t.Errorf("encode(%v) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	if _, err := encode(make(chan int)); err == nil {
		t.Error("a value JSON cannot hold should be an error")
	}
}

func TestCommandComesFirst(t *testing.T) {
	t.Parallel()

	// the documentation asks for the command to be the first field, so no body may be built from a map
	body, err := command(cmdRename, arg{FieldName, "a"}, arg{"newName", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"write":{"command":"rename","animName":"a","newName":"b"}}`; string(body) != want {
		t.Errorf("got  %s\nwant %s", body, want)
	}

	e, err := ParseEffect([]byte(`{"command":"request","version":"2.0","animName":"x","duration":9}`))
	if err != nil {
		t.Fatal(err)
	}
	body, err = e.command(cmdDisplayTemp, effectField{key: "duration", value: json.RawMessage("3")})
	if err != nil {
		t.Fatal(err)
	}
	// the effect's own command and duration give way to the ones being sent
	if want := `{"write":{"command":"displayTemp","duration":3,"version":"2.0","animName":"x"}}`; string(body) != want {
		t.Errorf("got  %s\nwant %s", body, want)
	}
}

func TestSeconds(t *testing.T) {
	t.Parallel()

	for d, want := range map[time.Duration]int{0: 1, 400 * time.Millisecond: 1, 1500 * time.Millisecond: 2, time.Minute: 60} {
		if got := seconds(d); got != want {
			t.Errorf("seconds(%s) = %d, want %d", d, got, want)
		}
	}
}

func TestEventsStopsWhenTheCallerDoes(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush() //nolint:errcheck,forcetypeassert // httptest's writer flushes
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	// a time limit on the client must not cut the stream off
	c, err := New(srv.URL, "token", WithTimeout(50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	deadline, _ := ctx.Deadline()
	if err := c.Events(ctx, nil, func(Event) {}); err != nil {
		t.Errorf("a caller that stops listening is not an error: %v", err)
	}
	if took := 300*time.Millisecond - time.Until(deadline); took < 250*time.Millisecond {
		t.Errorf("the stream ended after %s: the client's time limit cut it off", took)
	}
}
