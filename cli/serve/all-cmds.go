// Package serve is taproot serve: a page over every controller taproot knows,
// with each one's panels drawn where they sit and moving the way its scene
// does, and with what the commands do behind buttons: connect, power,
// brightness, pick a scene, copy one to another controller, back up.
package serve

import (
	"github.com/spf13/cobra"

	"github.com/katbyte/nanoleaf-aurora-taproot/cli"
)

// DefaultAddr is where the page is served unless told otherwise: every
// interface, on the port that spells ROOT on a phone's keypad.
const DefaultAddr = "7668"

// Command returns taproot serve.
func Command() *cobra.Command {
	c := &cobra.Command{
		Use:   "serve [port|host:port]",
		Short: "serves a web page to see and control every controller: panels, power, brightness, scenes, copying, connecting",
		Long: `Serves a page, until interrupted, that does what the commands do. Each controller is drawn with its panels where they
really sit, lit the way its running scene moves; the controller reports where the panels are and which scene is running, but
not what colour each panel is at this instant, so the page works the motion out from the scene. From the page you can turn a
controller on and off, dim it, start a scene, copy a scene to other controllers, take a backup, and connect a new controller:
type its address or search for it, then hold its power button.

A bare port serves on every interface, so a phone on the same network can open it; host:port keeps it to one. The page has
no login: anyone who can reach the port can use it, so put one in front before letting it out of the house. It answers only
to names it expects (this machine's, localhost, an IP address, anything.local); --allow-host adds the name a proxy or a
container is reached by.

Tokens never reach the browser. --dry-run serves the page with every change printed here instead of sent.`,
		Aliases:       []string{"ui", "web"},
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			addr := DefaultAddr
			if len(args) == 1 {
				addr = args[0]
			}
			return run(cmd.Context(), cli.GetFlags(), addr)
		},
	}
	c.Flags().StringSlice("allow-host", nil, "a name the page may be reached by, besides this machine's own (repeat, or separate with commas; * allows any) — or set TAPROOT_ALLOW_HOSTS")

	return c
}
