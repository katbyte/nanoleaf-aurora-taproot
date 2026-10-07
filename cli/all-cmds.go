// The taproot root command and the root-level commands: find, connect, list,
// info, rename, get, set, forget, backup and restore. The command groups (scene, serve) are
// added by cmd/taproot; the flag-free shared helpers live in cli/

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/katbyte/go-kt/cout"
	"github.com/katbyte/go-kt/version"
)

// Make builds the taproot root command with its persistent flags and the
// root-level commands; cmd/taproot adds the scene and serve groups on top.
func Make() (*cobra.Command, error) {
	root := &cobra.Command{
		Use:   "taproot [command]",
		Short: "🌱 taproot — back up, copy and control Nanoleaf Light Panels scenes over the local network, with no app and no cloud",
		Long: `taproot talks straight to Nanoleaf Light Panels controllers (the original
Aurora, model NL22) on your own network. It finds them, gets a token from each
with one press of the power button, and then reads and writes their scenes:
back them up to files, copy one from the controller that still has it to the
ones that lost it, and start it. taproot serve puts the same on a web page.

Nothing is replaced without --force, every controller is backed up before
anything is written to it, and --dry-run prints exactly what would be sent
without sending it.`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := BindCommandFlags(cmd); err != nil {
				return err
			}
			GetFlags().Out.Apply()
			return nil
		},
		// with nothing asked of it, taproot says what it can be asked
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}

	root.AddCommand(&cobra.Command{
		Use:           "version",
		Short:         "displays the version",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			cout.Printf("🌱 taproot <yellow>%s</>\n", version.Version)
			return nil
		},
	})

	findCmd := &cobra.Command{
		Use:   "find",
		Short: "searches the network for controllers and says which ones taproot already holds a token for",
		Long: `Listens for controllers announcing themselves (mDNS) and lists the ones that answer: name, address, model, firmware, and
whether taproot is already connected to it. Announcements often do not cross from one network to another; --scan knocks on
every address of a subnet instead, which finds a controller wherever it can be reached but cannot learn its name.`,
		Aliases:       []string{"search", "discover"},
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			return GetFlags().Find(cmd.Context())
		},
	}
	findCmd.Flags().Duration("wait", 3*time.Second, "how long to listen for controllers")
	findCmd.Flags().String("scan", "", "also knock on every address of this subnet, e.g. 10.0.5.0/24, for networks announcements do not cross")
	root.AddCommand(findCmd)

	connectCmd := &cobra.Command{
		Use:   "connect [ip|host|name|all]",
		Short: "gets a token from a controller and saves it: asks until you have held its power button; all asks every one not connected yet",
		Long: `Connects to a controller by its address, or by part of the name it announces (taproot find lists them); with nothing
named it looks for one that is not connected yet. A controller only hands out a token for about 30 seconds after its power
button has been held for 5 to 7 seconds, until the light flashes, so connect keeps asking until it gets one or --wait runs
out. The token is saved in the controllers file, which only you can read, and is never printed.

taproot connect all asks every controller that is not connected yet, all at once, so the one whose button you hold is the
one that connects: the way to connect a particular controller when all you know is which one you are standing at. It says
which one answered and asks what to call it, then goes on asking the rest until they are all connected, you type q and
enter, or you press ctrl-c. It has no time limit unless --wait gives it one.

--token-file takes a token you already have, from a file holding it alone or as the controller sent it ({"auth_token": ...}),
instead of asking for a new one.`,
		Aliases:       []string{"pair"},
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			target := ""
			if len(args) == 1 {
				target = args[0]
			}
			if target == "all" {
				return GetFlags().ConnectAll(cmd.Context(), cmd.InOrStdin(), cmd.Flags().Changed("wait"))
			}
			return GetFlags().Connect(cmd.Context(), target)
		},
	}
	connectCmd.Flags().Duration("wait", 5*time.Minute, "how long to keep asking for a token (connect all keeps asking until stopped unless this is given)")
	connectCmd.Flags().String("scan", "", "when searching, also knock on every address of this subnet, e.g. 10.0.5.0/24")
	connectCmd.Flags().String("name", "", "what to call the controller (default: the name it gives itself, in lower case with dashes)")
	connectCmd.Flags().String("token-file", "", "a file holding a token you already have, to save instead of asking for a new one")
	root.AddCommand(connectCmd)

	root.AddCommand(&cobra.Command{
		Use:           "list",
		Short:         "lists the controllers taproot holds a token for, and what each is doing",
		Aliases:       []string{"ls"},
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			return GetFlags().List(cmd.Context())
		},
	})

	root.AddCommand(&cobra.Command{
		Use:           "info controller",
		Short:         "shows one controller: model, firmware, panels, what is running, and its scenes",
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return GetFlags().Info(cmd.Context(), args[0])
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "rename controller name",
		Short: "changes what taproot calls a controller",
		Long: `Gives a controller another name to go by on the command line and on the page: taproot rename 183 office. The name is
kept to lower case letters, digits and dashes, so "Living Room" becomes living-room. Only taproot's own name for it changes;
the name the controller gives itself stays as it is. Backups already taken stay in the folder named for the old name.`,
		Aliases:       []string{"name"},
		Args:          cobra.ExactArgs(2),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return GetFlags().Rename(args[0], args[1])
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "get controller|all [setting]",
		Short: "shows what a controller is set to: every setting, or the one named",
		Long: `Reads a controller's settings: every one with what it can be set to, or just the one named, printed alone for a script
to read. With all in place of a controller it reads every controller, one to a line.

The settings:

` + SettingsHelp(),
		Args:          cobra.RangeArgs(1, 2),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := ""
			if len(args) == 2 {
				name = args[1]
			}
			return GetFlags().Get(cmd.Context(), args[0], name)
		},
	})

	setCmd := &cobra.Command{
		Use:   "set controller|all setting [value]",
		Short: "changes one setting of a controller: power, brightness, scene, colour, and the rest",
		Long: `Sets one setting of a controller and says what the controller reports it as afterwards. With all in place of a
controller it sets every controller. --by moves a number by an amount in place of setting it: --by -10 dims by ten.

  taproot set office brightness 40
  taproot set office brightness --by -10
  taproot set all power off

The settings:

` + SettingsHelp(),
		Args:          cobra.RangeArgs(2, 3),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			value := ""
			if len(args) == 3 {
				value = args[2]
			}
			return GetFlags().Set(cmd.Context(), args[0], args[1], value, cmd.Flags().Changed("by"))
		},
	}
	setCmd.Flags().Int("by", 0, "move a number by this much, up or down, in place of setting it: --by -10")
	setCmd.Flags().Duration("fade", 0, "for brightness: take this long to get there, e.g. 3s")
	root.AddCommand(setCmd)

	forgetCmd := &cobra.Command{
		Use:   "forget controller",
		Short: "deletes a controller's token, from the controller and from the controllers file",
		Long: `Has the controller delete the token, then removes the controller from the controllers file. With --local the controller is
left alone and only the file changes, for one that is gone or unreachable: its token stays valid until the controller is reset.`,
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return GetFlags().Forget(cmd.Context(), args[0])
		},
	}
	forgetCmd.Flags().Bool("local", false, "only remove it from the controllers file; do not ask the controller to delete the token")
	root.AddCommand(forgetCmd)

	firmwareCmd := &cobra.Command{
		Use:   "firmware controller|all",
		Short: "what a controller says about a firmware update, and (trigger) has it install one",
		Long: `Asks a controller what it knows of a firmware update. Nanoleaf does not document this: it is what the app does. A
controller that has not heard from Nanoleaf's cloud answers with nothing, which is shown as it came.

taproot firmware trigger tells the controller to fetch and install the update itself, from Nanoleaf's cloud, as the
app's update button does. Nothing passes through taproot. It is sent whether or not the controller says an update is
waiting: what a controller does with it then is not yet known, and taproot firmware afterwards says what it reports.`,
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return GetFlags().Firmware(cmd.Context(), args[0])
		},
	}
	triggerCmd := &cobra.Command{
		Use:   "trigger controller|all",
		Short: "has the controller fetch and install its firmware from Nanoleaf's cloud, and watches it through",
		Long: `Backs the controller up, then tells it to fetch and install the update itself, from Nanoleaf's cloud; nothing passes
through taproot. It is sent whether or not the controller says an update is waiting. Then the controller is watched: it
goes quiet for a few minutes while it installs, and when it answers again what it holds is compared with what it held,
since an upgrade has been seen to drop a scene. What went is in the backup, and taproot restore puts it back.`,
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return GetFlags().TriggerFirmware(cmd.Context(), args[0])
		},
	}
	triggerCmd.Flags().Duration("wait", 10*time.Minute, "how long to watch for the controller to come back; 0 sends the trigger and stops")
	firmwareCmd.AddCommand(triggerCmd)
	root.AddCommand(firmwareCmd)

	root.AddCommand(&cobra.Command{
		Use:   "backup [controller] [dir]",
		Short: "saves every scene a controller holds to a directory; with no controller, backs up all of them",
		Long: `Reads everything from a controller and writes it to a directory: each scene as its own file exactly as the controller
sent it, all of them in one file, the plugins it has, and what it says about itself, with a manifest of checksums. The
directory defaults to a dated one under the backup dir. A backup is never written over.`,
		Args:          cobra.MaximumNArgs(2),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return GetFlags().Backup(cmd.Context(), args)
		},
	})

	restoreCmd := &cobra.Command{
		Use:   "restore controller dir",
		Short: "puts the scenes of a backup onto a controller, leaving alone what is already there",
		Long: `Reads a backup, checks it against its manifest, and adds each of its scenes to the controller. A scene the controller
already holds unchanged is left alone; one it holds under the same name but different is refused unless --force is given.
The controller is backed up first if anything is to be written. The backup can come from any controller.`,
		Args:          cobra.ExactArgs(2),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return GetFlags().Restore(cmd.Context(), args[0], args[1])
		},
	}
	restoreCmd.Flags().Bool("force", false, "replace scenes on the controller that differ from the backup's")
	root.AddCommand(restoreCmd)

	if err := ConfigureFlags(root); err != nil {
		return nil, fmt.Errorf("unable to configure flags: %w", err)
	}

	return root, nil
}
