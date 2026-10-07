// Package cli is taproot's shared core: the flag data every command reads,
// the controllers file, and the plumbing the commands ride to reach a
// controller. The root-level commands live here; the groups live in the
// subpackages, cli/scene and cli/serve, and cmd/taproot assembles the tree.
package cli

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/go-kt/cout"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

type FlagData struct {
	Out cout.Flags `mapstructure:",squash"`

	ConfigDir string        `mapstructure:"config-dir"`
	BackupDir string        `mapstructure:"backup-dir"`
	Timeout   time.Duration `mapstructure:"timeout"`
	DryRun    bool          `mapstructure:"dry-run"`

	Cmd FlagsCommands `mapstructure:",squash"`

	// shared is what the commands reading several controllers at once, and the
	// page serving several people at once, all reach through
	shared shared
}

type shared struct {
	// http is the one client every controller is reached through, so a page
	// served for days reuses its connections rather than opening more
	once sync.Once
	http *http.Client

	// would is what a dry run would have sent, kept for the --json document
	mu    sync.Mutex
	would []WouldSend
}

// FlagsCommands holds the flags that belong to a single command. They are
// registered on their command beside it (scoped help), and BindCommandFlags
// merges the executing command's flags into viper at run time, so the values
// arrive here through GetFlags like every other flag. Only the running
// command's fields carry values; the rest stay zero and unread.
type FlagsCommands struct {
	Wait      time.Duration `mapstructure:"wait"`       // find: how long to listen; connect: how long to wait for the button
	Scan      string        `mapstructure:"scan"`       // find: a subnet to knock on
	Name      string        `mapstructure:"name"`       // connect: what to call the controller
	TokenFile string        `mapstructure:"token-file"` // connect: a token already in hand
	Local     bool          `mapstructure:"local"`      // forget: leave the controller alone
	Force     bool          `mapstructure:"force"`      // replace what is already there
	By        int           `mapstructure:"by"`         // set: move a number by this much
	Fade      time.Duration `mapstructure:"fade"`       // set brightness: take this long over it
	Scene     FlagsScene    `mapstructure:",squash"`
	Serve     FlagsServe    `mapstructure:",squash"`
}

// FlagsScene configures the taproot scene commands.
type FlagsScene struct {
	From   string        `mapstructure:"from"`   // copy: the controller to read the scene from
	To     []string      `mapstructure:"to"`     // copy: the controllers to put it on
	ToAll  bool          `mapstructure:"to-all"` // copy: every other controller
	As     string        `mapstructure:"as"`     // copy, push: the name to store it under
	Select bool          `mapstructure:"select"` // copy, push: start it once it is there
	Out    string        `mapstructure:"out"`    // dump: the file to write
	Except []string      `mapstructure:"except"` // delete: every scene but these
	Save   string        `mapstructure:"save"`   // paint: keep it as a scene of this name
	Over   time.Duration `mapstructure:"over"`   // paint: how long the panels take to reach the colours
}

// FlagsServe configures taproot serve.
type FlagsServe struct {
	AllowHosts []string `mapstructure:"allow-host"` // also TAPROOT_ALLOW_HOSTS: the names the page may be reached by
}

func ConfigureFlags(root *cobra.Command) error {
	pflags := root.PersistentFlags()

	// General Flags (FlagData)
	pflags.String("config-dir", store.DefaultDir(), "where the controllers file and the backups live (or TAPROOT_CONFIG_DIR)")
	pflags.String("backup-dir", "", "where backups go (default: backups inside the config dir; or TAPROOT_BACKUP_DIR)")
	pflags.Duration("timeout", aurora.DefaultTimeout, "how long one request to a controller may take")
	pflags.Bool("dry-run", false, "change nothing: print each request that would have been sent to a controller, body and all")

	// Output Flags
	pflags.Bool("json", false, "print one JSON document instead of text, for scripts")
	pflags.Bool("quiet", false, "minimal output")
	pflags.Bool("silent", false, "suppress all output")
	pflags.BoolP("verbose", "v", false, "show extra detail")

	// binding map for viper/pflag -> env vars (first entry wins when multiple are set)
	m := map[string][]string{
		"config-dir": {"TAPROOT_CONFIG_DIR"},
		"backup-dir": {"TAPROOT_BACKUP_DIR"},
		"timeout":    {"TAPROOT_TIMEOUT"},
		"dry-run":    {},
		"json":       {},
		"quiet":      {"TAPROOT_OUTPUT_QUIET"},
		"silent":     {"TAPROOT_OUTPUT_SILENT"},
		"verbose":    {},
	}

	for name, envs := range m {
		if err := viper.BindPFlag(name, pflags.Lookup(name)); err != nil {
			return fmt.Errorf("error binding '%s' flag: %w", name, err)
		}

		if len(envs) > 0 {
			if err := viper.BindEnv(append([]string{name}, envs...)...); err != nil {
				return fmt.Errorf("error binding '%s' to env '%v' : %w", name, envs, err)
			}
		}
	}

	// --allow-host belongs to serve rather than the root, but the names a page is reached by are a
	// setting of wherever it runs, a container above all — so it alone gets an env binding
	if err := viper.BindEnv("allow-host", "TAPROOT_ALLOW_HOSTS"); err != nil {
		return fmt.Errorf("error binding 'allow-host' to env: %w", err)
	}

	viper.SetConfigName(".taproot")
	viper.SetConfigType("env")
	if home, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(home)
	}
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			clog.Log.Errorf("Error reading config file: %v", err)
		}
	}
	// the file is env-format and its keys are env var names (TAPROOT_CONFIG_DIR=...),
	// but the flags above are bound to those names as ENV vars, not as config
	// keys — so export what the file holds, the real environment winning
	for _, k := range viper.AllKeys() {
		name := strings.ToUpper(k)
		if !strings.HasPrefix(name, "TAPROOT_") {
			continue
		}
		if _, set := os.LookupEnv(name); !set {
			if err := os.Setenv(name, viper.GetString(k)); err != nil {
				return fmt.Errorf("exporting %s from the config file: %w", name, err)
			}
		}
	}

	return nil
}

// BindCommandFlags merges the executing command's flags into viper so
// GetFlags sees them alongside the root flags ConfigureFlags bound. Binding
// happens per run and only for the command actually executing, which is what
// lets same-named flags on different commands keep their own defaults.
func BindCommandFlags(cmd *cobra.Command) error {
	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		return fmt.Errorf("binding %s flags: %w", cmd.Name(), err)
	}
	return nil
}

// GetFlags returns the fully populated FlagData.
// We must unmarshal from Viper instead of using globally bound pflags variables
// because pflags only parses command-line arguments. Viper merges environment
// variables (and config files) on top of the CLI flags.
func GetFlags() *FlagData {
	var f FlagData
	if err := viper.Unmarshal(&f); err != nil {
		clog.Log.Fatalf("failed to unmarshal configuration: %v", err)
	}

	return &f
}

// OpenStore reads the controllers file.
func (f *FlagData) OpenStore() (*store.Store, error) {
	return store.Open(f.ConfigDir)
}

// BackupRoot is the directory backups are kept under.
func (f *FlagData) BackupRoot() string {
	if f.BackupDir != "" {
		return f.BackupDir
	}
	return filepath.Join(f.ConfigDir, "backups")
}

// Controller is the controller a person means by ref, and a client for it.
func (f *FlagData) Controller(ref string) (store.Controller, *aurora.Client, error) {
	s, err := f.OpenStore()
	if err != nil {
		return store.Controller{}, nil, err
	}
	ctl, err := s.Find(ref)
	if err != nil {
		return store.Controller{}, nil, err
	}
	c, err := f.Client(ctl)

	return ctl, c, err
}

// Client returns a client for a controller. With --dry-run it changes
// nothing: each request that would have changed the controller is printed
// instead of sent.
func (f *FlagData) Client(ctl store.Controller) (*aurora.Client, error) {
	f.shared.once.Do(func() { f.shared.http = NewHTTPClient(f.Timeout) })
	opts := []aurora.Option{aurora.WithHTTPClient(f.shared.http)}
	if f.DryRun {
		opts = append(opts, aurora.WithDryRun(func(r aurora.Request) { f.wouldSend(ctl.Name, r) }))
	}

	return aurora.New(ctl.Host, ctl.Token, opts...)
}
