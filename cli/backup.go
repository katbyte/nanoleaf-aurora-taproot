package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/katbyte/go-kt/cout"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/backup"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/push"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
)

// BackedUp is one controller's backup as the backup command reports it.
type BackedUp struct {
	Controller string `json:"controller"`
	Dir        string `json:"dir"`
	Scenes     int    `json:"scenes"`
	Running    string `json:"running"`
	Error      string `json:"error,omitempty"`
}

func (f *FlagData) Backup(ctx context.Context, args []string) error {
	s, err := f.OpenStore()
	if err != nil {
		return err
	}

	// one controller, into the directory named or a dated one; or every controller, each into its own
	var targets []store.Controller
	dir := ""
	if len(args) == 0 {
		if targets = s.Controllers; len(targets) == 0 {
			return errors.New("no controllers yet: taproot connect <address> gets a token for one")
		}
	} else {
		ctl, err := s.Find(args[0])
		if err != nil {
			return err
		}
		targets = []store.Controller{ctl}
		if len(args) == 2 {
			dir = args[1]
		}
	}

	now := time.Now()
	var out []BackedUp
	var failed []error
	for _, ctl := range targets {
		to := dir
		if to == "" {
			to = backup.Dir(f.BackupRoot(), ctl.Name, now)
		}
		b := BackedUp{Controller: ctl.Name, Dir: to}

		// a backup only reads the controller, so a dry run stops short of writing the files
		if f.DryRun {
			cout.Printf("<yellow>dry run:</> would back up %s to %s\n", Name(ctl.Name), Dim(to))
			out = append(out, b)
			continue
		}

		c, err := f.Client(ctl)
		if err == nil {
			var m *backup.Manifest
			if m, err = backup.Take(ctx, c, to); err == nil {
				b.Scenes, b.Running = len(m.Scenes), m.Running
				cout.Printf("<green>backed up</> %s: %s scenes in %s\n", Name(ctl.Name), Num(b.Scenes), Dim(to))
			}
		}
		if err != nil {
			b.Error = err.Error()
			failed = append(failed, fmt.Errorf("%s: %w", ctl.Name, err))
			cout.Errorf("<red>%s was not backed up:</> %s\n", Escape(ctl.Name), Escape(err.Error()))
		}
		out = append(out, b)
	}

	if _, err := f.Emit(out); err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d controllers were not backed up", len(failed), len(targets))
	}

	return nil
}

func (f *FlagData) Restore(ctx context.Context, ref, dir string) error {
	ctl, c, err := f.Controller(ref)
	if err != nil {
		return err
	}
	b, err := backup.Read(dir)
	if err != nil {
		return err
	}
	if b.Manifest != nil {
		cout.Printf("restoring %s scenes taken from %s on %s\n", Num(len(b.Scenes)), Device(b.Manifest.Controller.Name), Dim(b.Manifest.Taken))
	}

	report, err := push.Scenes(ctx, c, b.Scenes, push.Options{
		Controller: ctl.Name, BackupRoot: f.BackupRoot(), Force: f.Cmd.Force, DryRun: f.DryRun,
	})
	if err != nil {
		return err
	}
	if done, err := f.Emit(report); done {
		if err != nil {
			return err
		}
		return report.Err()
	}
	PrintReport(report)

	return report.Err()
}
