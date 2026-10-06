// Package scene holds the scene commands: list what the controllers hold,
// dump a scene to a file, push one from a file, copy one between
// controllers, and start one.
package scene

import (
	"github.com/spf13/cobra"

	"github.com/katbyte/nanoleaf-aurora-taproot/cli"
)

// Flags wraps the shared flag data so the scene commands keep their method form.
type Flags struct{ *cli.FlagData }

// flags is every scene RunE's entry point to the fully populated Flags.
func flags() *Flags { return &Flags{cli.GetFlags()} }

// Command returns the scene command group.
func Command() *cobra.Command {
	c := &cobra.Command{
		Use:   "scene",
		Short: "the scenes: list, dump, push, copy and select them",
		Long: `A scene is what a controller calls an effect: a palette of colours and the plugin that moves them. taproot never builds one.
It reads a scene from a controller exactly as the controller holds it, and gives it to another exactly as it was read, which is
what makes a copy work whatever the firmware.

Writing is careful. A scene already on the controller and unchanged is left alone. One there under the same name but different
is refused unless --force is given. Before the first write the whole controller is backed up, and after each write the scene is
read back and compared with what was sent. --dry-run prints the exact request instead of sending it.`,
		Aliases: []string{"scenes", "s"},
		Args:    cobra.NoArgs,
		RunE:    func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}

	c.AddCommand(&cobra.Command{
		Use:   "list [controller]",
		Short: "lists a controller's scenes; with no controller, which controller holds which scene",
		Long: `With a controller: its scenes, the one running marked, with what kind each is and the plugin it runs. With none: every scene
on every controller side by side, which shows at a glance the one a controller has lost, and marks a scene that two
controllers hold under the same name but with different contents.`,
		Aliases:       []string{"ls"},
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if len(args) == 0 {
				return flags().Compare(cmd.Context())
			}
			return flags().List(cmd.Context(), args[0])
		},
	})

	dumpCmd := &cobra.Command{
		Use:   "dump controller [scene]",
		Short: "prints a scene as JSON, exactly as the controller holds it; every scene when none is named",
		Long: `Prints one scene as the controller sent it, or all of them as {"animations": [...]}, the controller's own shape for the
lot. Either is what taproot scene push takes back. --out writes a file instead, and will not write over one without --force.`,
		Args:          cobra.RangeArgs(1, 2),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := ""
			if len(args) == 2 {
				name = args[1]
			}
			return flags().Dump(cmd.Context(), args[0], name)
		},
	}
	dumpCmd.Flags().StringP("out", "o", "", "write to this file instead of printing")
	dumpCmd.Flags().Bool("force", false, "write over the file if it exists")
	c.AddCommand(dumpCmd)

	pushCmd := &cobra.Command{
		Use:   "push controller file",
		Short: "adds the scene, or scenes, in a JSON file to a controller (- reads from the pipe)",
		Long: `Adds what taproot scene dump wrote: one scene, or {"animations": [...]} holding several. The scene is sent as the file has
it. --as stores a single scene under another name, leaving any scene of the file's own name alone.`,
		Args:          cobra.ExactArgs(2),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return flags().Push(cmd.Context(), args[0], args[1])
		},
	}
	addWriteFlags(pushCmd)
	c.AddCommand(pushCmd)

	copyCmd := &cobra.Command{
		Use:   "copy scene --from controller (--to controller... | --to-all)",
		Short: "copies a scene from one controller to others",
		Long: `Reads the scene from the --from controller and adds it to each --to controller, or with --to-all to every other
controller taproot knows. A controller that already holds it unchanged is left alone.`,
		Aliases:       []string{"cp"},
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return flags().Copy(cmd.Context(), args[0])
		},
	}
	copyCmd.Flags().String("from", "", "the controller that has the scene")
	copyCmd.Flags().StringSlice("to", nil, "a controller to copy it to (repeat, or separate with commas)")
	copyCmd.Flags().Bool("to-all", false, "copy it to every other controller")
	addWriteFlags(copyCmd)
	c.AddCommand(copyCmd)

	c.AddCommand(&cobra.Command{
		Use:           "select controller scene",
		Short:         "starts a scene the controller holds",
		Aliases:       []string{"run", "start"},
		Args:          cobra.ExactArgs(2),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return flags().Select(cmd.Context(), args[0], args[1])
		},
	})

	return c
}

// Per-command flag registration, kept beside the commands that own them.

func addWriteFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("force", false, "replace a scene of the same name that is already there and different")
	cmd.Flags().String("as", "", "store the scene under this name instead of its own")
	cmd.Flags().Bool("select", false, "start the scene on each controller once it is there")
}
