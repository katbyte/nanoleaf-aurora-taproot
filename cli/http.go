package cli

import (
	"net/http"
	"net/http/httputil"
	"regexp"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/katbyte/go-kt/chttp"
	"github.com/katbyte/go-kt/clog"
)

// NewHTTPClient returns the client taproot reaches controllers with: go-kt's
// timeouts and retries (a read that fails is tried again, a write never is),
// and TAPROOT_LOG=trace dumps of every exchange with the token taken out.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: chttp.NewRetryTransport("aurora", &traceTransport{next: chttp.NewBaseTransport()}, chttp.DefaultMaxRetry),
	}
}

// traceTransport is chttp's trace logging for an API that carries its token
// in the path. chttp's own dumps the request line as it is, which here would
// write the token into the log, and a token is a password that never expires.
type traceTransport struct {
	next http.RoundTripper
}

var (
	tokenInPath = regexp.MustCompile(`(/api/v1/)[A-Za-z0-9]{16,}`)
	tokenInBody = regexp.MustCompile(`("auth_token"\s*:\s*")[^"]*`)
)

// Redact takes the tokens out of a dump of a request or an answer: the one
// in the path, and the one a controller hands out in a body.
func Redact(dump []byte) []byte {
	return tokenInBody.ReplaceAll(tokenInPath.ReplaceAll(dump, []byte("${1}<token>")), []byte("${1}<token>"))
}

// RoundTrip implements http.RoundTripper.
func (t *traceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	trace := clog.Log.IsLevelEnabled(logrus.TraceLevel)
	if trace {
		if dump, err := httputil.DumpRequestOut(req, true); err == nil {
			clog.Log.Tracef("aurora request:\n%s", Redact(dump))
		}
	}

	resp, err := t.next.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// an event stream never ends, so its body cannot be dumped: the headers say enough
	if trace {
		stream := resp.Header.Get("Content-Type") == "text/event-stream"
		if dump, err := httputil.DumpResponse(resp, !stream); err == nil {
			clog.Log.Tracef("aurora response:\n%s", Redact(dump))
		}
	}

	return resp, nil
}
