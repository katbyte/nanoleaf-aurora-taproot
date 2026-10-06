package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/lib/cassette"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// The settings a run is told how to reach real controllers with.
const (
	envLive   = "TAPROOT_TEST_LIVE"   // the name of a controller to read
	envSpare  = "TAPROOT_TEST_SPARE"  // the name of one that may be written to
	envRecord = "TAPROOT_TEST_RECORD" // write down what they answer
	envVerify = "TAPROOT_TEST_VERIFY" // compare what they answer with what is written down
	envConfig = "TAPROOT_TEST_CONFIG" // where the controllers file naming them is, when not the usual place
)

// rig is one test's taproot: a configuration directory of its own, and the
// controllers in it, each behind a proxy.
type rig struct {
	t       *testing.T
	dir     string
	proxies map[string]*cassette.Proxy
}

// oneAtATime is held by the test that is talking to real controllers. Against
// recordings the tests run side by side; a real controller is a small thing
// that drops requests when several arrive at once, and the tests that write
// must not see each other's scenes.
var oneAtATime sync.Mutex

func newRig(t *testing.T) *rig {
	t.Helper()
	return &rig{t: t, dir: t.TempDir(), proxies: map[string]*cassette.Proxy{}}
}

// mode is how this run reaches controllers.
func mode() cassette.Mode {
	switch {
	case os.Getenv(envLive) == "":
		return cassette.Replay
	case os.Getenv(envRecord) != "":
		return cassette.Record
	case os.Getenv(envVerify) != "":
		return cassette.Verify
	default:
		return cassette.Live
	}
}

// reader connects the test's taproot to a controller it will only read,
// under the name the test knows it by. Its proxy refuses anything else, so a
// test that goes wrong cannot change it.
func (r *rig) reader(role string) {
	r.t.Helper()
	r.connect(role, os.Getenv(envLive), true)
}

// writer connects the test's taproot to a controller the test may write to.
// A run with no such controller, or with no recording of one, skips the test.
func (r *rig) writer(role string) {
	r.t.Helper()

	if mode() != cassette.Replay {
		spare := os.Getenv(envSpare)
		if spare == "" {
			r.t.Skipf("this test writes to a controller: set %s to the name of one that may be written to", envSpare)
		}
		if spare == os.Getenv(envLive) {
			r.t.Fatalf("%s and %s name the same controller: the one being read must never be written to", envSpare, envLive)
		}
	} else if !cassette.Recorded(r.file(role)) {
		r.t.Skipf("not recorded yet: this test writes to a controller, and is recorded with %s set (make record)", envSpare)
	}

	r.connect(role, os.Getenv(envSpare), false)
}

func (r *rig) file(role string) string {
	return filepath.Join("testdata", "cassettes", strings.ReplaceAll(r.t.Name(), "/", "_")+"."+role+".json")
}

func (r *rig) connect(role, named string, readOnly bool) {
	r.t.Helper()

	opts := cassette.Options{Mode: mode(), File: r.file(role), ReadOnly: readOnly}
	if opts.Mode != cassette.Replay {
		if len(r.proxies) == 0 {
			oneAtATime.Lock()
			r.t.Cleanup(oneAtATime.Unlock)
		}
		dir := os.Getenv(envConfig)
		if dir == "" {
			dir = store.DefaultDir()
		}
		mine, err := store.Open(dir)
		if err != nil {
			r.t.Fatal(err)
		}
		ctl, err := mine.Find(named)
		if err != nil {
			r.t.Fatalf("the controller to test against: %v", err)
		}
		opts.Target, opts.RealToken = ctl.Host, ctl.Token
		if !strings.Contains(opts.Target, ":") {
			opts.Target += ":16021"
		}
	} else if !cassette.Recorded(opts.File) {
		r.t.Fatalf("%s is missing: record it against a real controller (make record)", opts.File)
	}

	p, err := cassette.Start(opts)
	if err != nil {
		r.t.Fatal(err)
	}
	r.proxies[role] = p
	r.t.Cleanup(func() {
		// a problem at the proxy is a failure of the test whatever the commands made of it
		for _, problem := range p.Problems() {
			r.t.Errorf("%s: %s", role, problem)
		}
		if err := p.Close(); err != nil {
			r.t.Errorf("%s: saving the recording: %v", role, err)
		}
	})

	// the test's taproot knows the proxy as the controller, and holds only the token every test is given
	s, err := store.Open(r.dir)
	if err != nil {
		r.t.Fatal(err)
	}
	s.Put(store.Controller{Name: role, Device: role, Host: p.Host(), Serial: role, Token: cassette.Token})
	if err := s.Save(); err != nil {
		r.t.Fatal(err)
	}
}

// client is an SDK client for a controller of the rig, through its proxy.
func (r *rig) client(role string) *aurora.Client {
	r.t.Helper()

	c, err := aurora.New(r.proxies[role].Host(), cassette.Token)
	if err != nil {
		r.t.Fatal(err)
	}

	return c
}

// taproot runs the binary and returns what it printed and how it ended.
func (r *rig) taproot(args ...string) (string, error) {
	r.t.Helper()

	ctx, cancel := context.WithTimeout(r.t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, append([]string{"--config-dir", r.dir}, args...)...) //nolint:gosec // the binary this run built
	// nothing of the person's own setup, and no colour
	cmd.Env = []string{"HOME=" + r.dir, "PATH=" + os.Getenv("PATH"), "NO_COLOR=1", "TERM=dumb"}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()

	// the binary is never given a real token, and must not print the one it has
	if strings.Contains(out.String(), cassette.Token+`"`) || strings.Contains(out.String(), "/api/v1/"+cassette.Token) {
		r.t.Errorf("taproot %s printed its token:\n%s", strings.Join(args, " "), out.String())
	}

	return out.String(), err
}

func (r *rig) ok(args ...string) string {
	r.t.Helper()

	out, err := r.taproot(args...)
	if err != nil {
		r.t.Fatalf("taproot %s: %v\n%s", strings.Join(args, " "), err, out)
	}

	return out
}

func (r *rig) fails(args ...string) string {
	r.t.Helper()

	out, err := r.taproot(args...)
	if err == nil {
		r.t.Fatalf("taproot %s: want it to fail, got\n%s", strings.Join(args, " "), out)
	}

	return out
}

// writeScene writes a scene to a file, as taproot scene dump would.
func writeScene(t *testing.T, path string, e aurora.Effect) {
	t.Helper()

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// decode reads a command's --json document.
func decode(t *testing.T, out string, into any) {
	t.Helper()

	if err := json.Unmarshal([]byte(out), into); err != nil {
		t.Fatalf("not the JSON expected: %v\n%s", err, out)
	}
}

func want(t *testing.T, got string, parts ...string) {
	t.Helper()

	for _, part := range parts {
		if !strings.Contains(got, part) {
			t.Errorf("missing %q in:\n%s", part, got)
		}
	}
}
