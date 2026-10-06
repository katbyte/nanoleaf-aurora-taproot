// Package acceptance runs the taproot binary, as it ships, against a
// controller: a recording of one by default, a real one when asked.
//
// A controller cannot be started in a container, so what a test run has in
// its place is a recording made from a real one (lib/cassette). The same
// tests run three ways:
//
//	make test          against the recordings: no controller, which is what CI does
//	make testacc       against real controllers, to see taproot still works on them
//	make record        against real controllers, writing down what they answer
//	make record-check  against both, to find answers that firmware has changed
//
// The last three need TAPROOT_TEST_LIVE, the name of a controller taproot is
// connected to (taproot list). It is only ever read: its proxy refuses
// anything else. The tests that write need TAPROOT_TEST_SPARE as well, a
// second controller that may be written to. They add scenes under names
// starting "taproot test", start one, and take away what they added; they
// are skipped without it, and skipped in a normal run until they have been
// recorded.
package acceptance

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// bin is the taproot binary under test, built once for the run.
var bin string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "taproot-acceptance-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "acceptance:", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()

	bin = filepath.Join(dir, "taproot")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, "../cmd/taproot") //nolint:gosec // go, building this repository
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "acceptance: building taproot: %v\n%s", err, out)
		return 1
	}

	return m.Run()
}
