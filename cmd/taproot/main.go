// Command taproot backs up, copies and controls the scenes of Nanoleaf Light
// Panels controllers over the local network.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	c "github.com/gookit/color"
	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/nanoleaf-aurora-taproot/cli"
	"github.com/katbyte/nanoleaf-aurora-taproot/cli/scene"
	"github.com/katbyte/nanoleaf-aurora-taproot/cli/serve"
)

func main() {
	os.Exit(run())
}

func run() int {
	// the log level comes from TAPROOT_LOG; read it once here, before anything logs
	clog.SetLevelFromEnv("TAPROOT_LOG")

	// the command groups are wired here rather than in a package: taproot builds
	// the root, and each group hangs off it
	cmd, err := cli.Make()
	if err != nil {
		clog.Log.Error(c.Sprintf("<red>taproot: building cmd</> %v", err))
		return 1
	}
	cmd.AddCommand(scene.Command(), serve.Command())

	// ctrl-c ends whatever is waiting: a connect asking for a token, a page being served
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.ExecuteContext(ctx); err != nil {
		clog.Log.Error(c.Sprintf("<red>taproot:</> %v", err))
		return 1
	}

	return 0
}
